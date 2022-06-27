package springboard

import (
	"bytes"
	"crypto/ed25519"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

const max_sig = (1 << 256) - 1

func RunServer(port uint, federates []string) (err error) {
	db := initDB()
	server := newSpring83Server(db, federates)

	http.HandleFunc("/", server.RootHandler)
	listenAddress := fmt.Sprintf(":%d", port)
	log.Printf("Listening on port %d", port)
	log.Fatal(http.ListenAndServe(listenAddress, nil))
	return
}

func initDB() *sql.DB {
	dbName := "./spring83.db"

	// if the db doesn't exist, create it
	if _, err := os.Stat(dbName); errors.Is(err, os.ErrNotExist) {
		log.Printf("initializing new database")
		db, err := sql.Open("sqlite", dbName)
		if err != nil {
			panic(err)
		}

		initSQL := `
		CREATE TABLE boards (
			key text NOT NULL PRIMARY KEY,
			board text,
			modified text,
			signature test
		);
		`

		_, err = db.Exec(initSQL)
		if err != nil {
			log.Fatalf("%q: %s\n", err, initSQL)
		}
		return db
	}

	db, err := sql.Open("sqlite", dbName)
	if err != nil {
		panic(err)
	}
	return db
}

func mustTemplate() *template.Template {
	f := page_template

	t, err := template.New("index").Parse(f)
	if err != nil {
		panic(err)
	}

	return t
}

type Spring83Server struct {
	db                 *sql.DB
	homeTemplate       *template.Template
	federates          []string
	propagationTracker *propagationTracker
}

func newSpring83Server(db *sql.DB, federates []string) *Spring83Server {
	return &Spring83Server{
		db:                 db,
		homeTemplate:       mustTemplate(),
		federates:          federates,
		propagationTracker: newPropagationTracker(),
	}
}

func (s *Spring83Server) getBoard(key string) (*Board, error) {
	query := `
		SELECT key, board, modified, signature
		FROM boards
		WHERE key=?
	`
	row := s.db.QueryRow(query, key)

	var dbkey, board, modified, signature string
	err := row.Scan(&dbkey, &board, &modified, &signature)
	if err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}
		return nil, nil
	}

	modifiedTime, err := time.Parse(time.RFC3339, modified)
	if err != nil {
		return nil, err
	}

	return &Board{
		Key:       key,
		Board:     board,
		Modified:  modifiedTime,
		Signature: signature,
	}, nil
}

func (s *Spring83Server) boardCount() (int, error) {
	query := `
		SELECT count(*)
		FROM boards
	`
	row := s.db.QueryRow(query)

	var count int
	err := row.Scan(&count)
	if err != nil {
		if err != sql.ErrNoRows {
			return 0, err
		}
		panic(err)
	}

	return count, nil
}

func (s *Spring83Server) getDifficulty() (float64, uint64, error) {
	count, err := s.boardCount()
	if err != nil {
		return 0, 0, err
	}

	difficultyFactor := math.Pow(float64(count)/10_000_000, 4)
	keyThreshold := uint64(max_sig * (1.0 - difficultyFactor))
	return difficultyFactor, keyThreshold, nil
}

func (s *Spring83Server) publishBoard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Spring-Version", "83")
	var err error

	key, err := hex.DecodeString(r.URL.Path[1:])
	if err != nil || len(key) != 32 {
		http.Error(w, "Invalid key", http.StatusBadRequest)
		return
	}
	keyStr := fmt.Sprintf("%x", key)
	log.Printf("Receiving board for %s", keyStr)
	log.Printf("%+v", r.Header)

	//do all checks we can do with the header first

	var ifUnmodifiedSince time.Time
	ifUnmodifiedSinceHeader := r.Header["If-Unmodified-Since"]
	if ifUnmodifiedSinceHeader != nil {
		// spec says "in HTTP format", but it's not entirely clear if this matches?
		if ifUnmodifiedSince, err = time.Parse(time.RFC1123, ifUnmodifiedSinceHeader[0]); err != nil {
			http.Error(w, "Invalid format for If-Unmodified-Since header", http.StatusBadRequest)
			return
		}
	}

	// curBoard is nil if there is no existing board for this key, and a Board object otherwise
	curBoard, err := s.getBoard(keyStr)
	if err != nil {
		log.Printf(err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if curBoard != nil && ifUnmodifiedSinceHeader != nil && !curBoard.Modified.Before(ifUnmodifiedSince) {
		http.Error(w, "Old content", http.StatusConflict)
		return
	}

	// if the server doesn't have any board stored for <key>, then it must
	// apply another check. The key, interpreted as a 256-bit number, must be
	// less than a threshold defined by the server's difficulty factor:
	if curBoard == nil {
		difficultyFactor, keyThreshold, err := s.getDifficulty()
		if err != nil {
			log.Printf(err.Error())
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Add("Spring-Difficulty", fmt.Sprintf("%f", difficultyFactor))

		// Using that difficulty factor, we can calculate the key threshold:
		//
		// MAX_KEY = (2**256 - 1)
		// key_threshold = MAX_KEY * (1.0 - 0.52) = <an inscrutable gigantic number>
		//
		// The server must reject PUT requests for new keys that are not less
		// than <an inscrutable gigantic number>
		if binary.BigEndian.Uint64(key) >= keyThreshold {
			if err != nil || len(key) != 32 {
				http.Error(w, "Key greater than threshold", http.StatusForbidden)
				return
			}
		}
	}

	var hexSignature []byte
	var strSignature string
	if signatureHeaders, ok := r.Header["Spring-Signature"]; !ok {
		http.Error(w, "missing Spring-Signature header", http.StatusBadRequest)
		return
	} else {
		strSignature = signatureHeaders[0]
		if len(strSignature) < 1 {
			http.Error(w, "Invalid Signature", http.StatusBadRequest)
			return
		}

		if len(strSignature) != 128 {
			http.Error(w, fmt.Sprintf("Expecting 64-bit signature %s %d", strSignature, len(strSignature)), http.StatusBadRequest)
			return
		}

		hexSignature, err = hex.DecodeString(strSignature)
		if err != nil {
			http.Error(w, "Unable to decode signature", http.StatusBadRequest)
			return
		}
	}

	// Spring '83 specifies a test keypair
	// Servers must not accept PUTs for this key, returning 401 Unauthorized.
	// The server may also use a denylist to block certain keys, rejecting all PUTs for those keys.
	denylist := []string{"fad415fbaa0339c4fd372d8287e50f67905321ccfd9c43fa4c20ac40afed1983"}
	for _, key := range denylist {
		if bytes.Compare(hexSignature, []byte(key)) == 0 {
			http.Error(w, "Denied", http.StatusUnauthorized)
		}
	}

	// Keys are of the form 83eMMYY
	// when PUTting, a key must
	// - be greater than today (more specifically the today must be before the first day of the next month following the expire, similar to credit cards)
	// - be less than two years from now
	// The server must reject other keys with 400 Bad Request.
	last4 := string(keyStr[60:64])
	today := time.Now()
	expiry, err := time.Parse("0206", last4)
	if keyStr[57:60] != "83e" || err != nil {
		http.Error(w, "Signature must end with 83eMMYY. You might be using an old key format. Delete your old key, update your client, and try again.", http.StatusBadRequest)
		return
	}
	if today.After(expiry.AddDate(0, 1, 0)) {
		http.Error(w, "Key has expired", http.StatusBadRequest)
		return
	}
	if expiry.After(today.AddDate(2, 0, 0)) {
		http.Error(w, "Key is set to expire more than two years in the future", http.StatusBadRequest)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Could not read body", http.StatusInternalServerError)
	}

	if len(body) > 2217 {
		http.Error(w, "Payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	dateTagRegExp := regexp.MustCompile(`(?i)<\s*time\s+datetime\s*=\s*"(\d\d\d\d-\d\d-\d\dT\d\d:\d\d:\d\dZ)"\s*\/?\s*>`)

	submatches := dateTagRegExp.FindSubmatch(body)
	if submatches == nil {
		http.Error(w, `Missing <time datetime="YYYY-MM-DDTHH:MM:SSZ"> tag`, http.StatusBadRequest)
		return
	}
	maybeDate := string(submatches[1][:])
	modifiedTime, err := time.Parse("2006-01-02T15:04:05Z", maybeDate)
	if err != nil {
		http.Error(w, fmt.Sprintf("Could not parse date %s", maybeDate), http.StatusBadRequest)
		return
	}
	if curBoard != nil && !curBoard.Modified.Before(modifiedTime) {
		http.Error(w, "Old content", http.StatusConflict)
		return
	}

	// at this point, we should have met all the preconditions prior to the
	// cryptographic check. By the spec, we should perform all
	// non-cryptographic checks first.
	if !ed25519.Verify(key, body, hexSignature) {
		http.Error(w, "Invalid signature", http.StatusBadRequest)
		return
	}

	newBoard := Board{
		Key:       keyStr,
		Board:     string(body[:]),
		Modified:  modifiedTime,
		Signature: strSignature,
	}
	_, err = s.db.Exec(`
		INSERT INTO boards (key, board, modified, signature)
		            values(?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			    board=?,
			    modified=?,
			    signature=?
		`, newBoard.Key, newBoard.Board, newBoard.ModifiedAtDBFormat(), newBoard.Signature,
		newBoard.Board, newBoard.ModifiedAtDBFormat(), newBoard.Signature)

	if err != nil {
		log.Printf("%s", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
	}

	s.propagateBoard(newBoard)
}

func (server *Spring83Server) propagateBoard(board Board) {
	rand.Seed(time.Now().UnixNano())
	for _, federate := range server.federates {
		server.propagationTracker.Schedule(board, federate)
	}
}

func (s *Spring83Server) loadBoards() ([]Board, error) {
	query := `
		SELECT key, board, modified
		FROM boards
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}

	boards := []Board{}
	for rows.Next() {
		var key, board, modified string

		err = rows.Scan(&key, &board, &modified)
		if err != nil {
			return nil, err
		}

		modifiedTime, err := time.Parse(time.RFC3339, modified)
		if err != nil {
			return nil, err
		}

		boards = append(boards, Board{
			Key:      key,
			Board:    board,
			Modified: modifiedTime,
		})
	}

	return boards, nil
}

// for now, on loads to /, I'm just going to show all boards no matter what
func (s *Spring83Server) showAllBoards(w http.ResponseWriter, r *http.Request) {
	boards, err := s.loadBoards()
	if err != nil {
		log.Printf(err.Error())
		http.Error(w, "Unable to load boards", http.StatusInternalServerError)
		return
	}

	difficultyFactor, _, err := s.getDifficulty()
	if err != nil {
		log.Printf(err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Add("Spring-Difficulty", fmt.Sprintf("%f", difficultyFactor))

	data := struct {
		Boards []Board
	}{
		Boards: boards,
	}

	s.homeTemplate.Execute(w, data)
}

func (s *Spring83Server) showBoard(w http.ResponseWriter, r *http.Request) {
	board, err := s.getBoard(r.URL.Path[1:])
	if err != nil {
		log.Printf(err.Error())
		http.Error(w, "Unable to load boards", http.StatusInternalServerError)
		return
	}
	if board == nil {
		http.Error(
			w,
			fmt.Sprintf("Could not find board %s", r.URL.Path[1:]),
			http.StatusNotFound)
		return
	}

	difficultyFactor, _, err := s.getDifficulty()
	if err != nil {
		log.Printf(err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Add("Spring-Difficulty", fmt.Sprintf("%f", difficultyFactor))
	w.Header().Add("Content-Type", "text/html;charset=utf-8")
	w.Header().Add("Spring-Signature", board.Signature)

	w.Header().Add("Content-Security-Policy", "default-src 'none'; style-src 'self' 'unsafe-inline'; font-src 'self'; script-src 'self'; form-action *; connect-src *;")

	w.Write([]byte(board.Board))
}

func (s *Spring83Server) showOptions(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Spring83Server) showFederation(w http.ResponseWriter, r *http.Request) {
	federationText := fmt.Sprintf("%s\n", strings.Join(s.federates, "\n"))
	w.Write([]byte(federationText))
}

func (s *Spring83Server) addCORSHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type, If-Modified-Since, Spring-Signature, Spring-Version")
	w.Header().Add("Access-Control-Expose-Headers", "Content-Type, Last-Modified, Spring-Difficulty, Spring-Signature, Spring-Version")
}

func (s *Spring83Server) RootHandler(w http.ResponseWriter, r *http.Request) {
	s.addCORSHeaders(w, r)
	if r.Method == "PUT" {
		s.publishBoard(w, r)
	} else if r.Method == "GET" {
		if len(r.URL.Path) == 1 {
			s.showAllBoards(w, r)
		} else {
			if r.URL.Path[1:] == "federation.txt" {
				s.showFederation(w, r)
			} else {
				s.showBoard(w, r)
			}
		}
	} else if r.Method == "OPTIONS" {
		s.showOptions(w, r)
	} else {
		http.Error(w, "Invalid method", http.StatusBadRequest)
	}
}

const page_template = `
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Spring83</title>
<link rel="icon" href="data:image/svg+xml,<svg xmlns=%22http://www.w3.org/2000/svg%22 viewBox=%220 0 100 100%22><text y=%22.9em%22 font-size=%2290%22>🌅</text></svg>">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
	body {
		background-color: lightyellow;
	}
	#containers {
		display: flex;
		flex-wrap: wrap;
	}
	.board {
		background-color: lightcyan;
		border: 1px dotted black;
		margin: 5px;
		padding: 10px;
		width: min-content;
		cursor: pointer;
	}
	.description {
		font-family: monospace;
		font-size: xx-small;
		display: flex;
		flex-wrap: wrap;
		justify-content: space-between;
	}
	.description {
		color: darkgray;
	}
	iframe {
		border: 0;
		height: 320px;
		width: 100% ;
		overflow: hidden;
		pointer-events: none;
	}
</style>
</head>
<body>
<h1>Spring 83</h1>
<div id="containers">
	{{ range .Boards }}
		<div id="b{{ .Key }}" class="board" onclick="window.open('/{{.Key}}', '_blank', 'height=800,width=564');">
			<iframe sandbox="allow-popups" src="/{{.Key}}"></iframe>
			<div class="description">
				<span class="modified">{{.Modified}}</span>
				<span class="full-page-link">Full Page</span>
				<span class="key">{{.Key}}</span>
			</div>
		</div>
	{{ end }}
</div>
</body>
</html>
`

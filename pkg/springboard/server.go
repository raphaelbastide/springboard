package springboard

import (
	"bytes"
	"crypto/ed25519"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"text/template"
	"time"

	_ "github.com/glebarez/go-sqlite"
	_ "github.com/lib/pq"
)

const max_sig = (1 << 256) - 1

func RunServer(port uint, federates []string, adminBoard string, fqdn string, propagateWait time.Duration, driver string, connectionString string) (err error) {
	repo := initDB(driver, connectionString)
	server := newSpring83Server(repo, federates, adminBoard, fqdn, propagateWait)
	go server.periodicallyPurgeOldBoards()
	http.HandleFunc("/", server.RootHandler)
	listenAddress := fmt.Sprintf(":%d", port)
	log.Printf("Listening on port %d", port)
	err = http.ListenAndServe(listenAddress, nil)
	if err != nil {
		return err
	}
	return
}

type BoardRepo interface {
	GetAllBoards() ([]Board, error)
	GetBoard(key string) (board *Board, err error)
	PublishBoard(Board) error
	DeleteBoardsBefore(string) error
	BoardCount() (int, error)
}

func initDB(driver, connectionString string) BoardRepo {
	if driver == "sqlite" {
		return newSqliteRepo(connectionString)
	} else {
		panic("Unsupported driver " + driver)
	}
}

func (s *Spring83Server) periodicallyPurgeOldBoards() {
	for true {
		expiry := time.Now().Add(-22 * 24 * time.Hour).Format(time.RFC3339)
		log.Printf("Deleting boards past their TTL (published before %s)", expiry)
		err := s.repo.DeleteBoardsBefore(expiry)
		if err != nil {
			log.Print(err)
		}
		time.Sleep(time.Minute)
	}
}

//go:embed assets/index.html
var indexTemplate string

func mustTemplate() *template.Template {

	t, err := template.New("index").Parse(indexTemplate)
	if err != nil {
		panic(err)
	}

	return t
}

type Spring83Server struct {
	repo               BoardRepo
	homeTemplate       *template.Template
	federates          []string
	adminBoard         string
	propagationTracker *propagationTracker
	fqdn               string
	propagateWait      time.Duration
}

func newSpring83Server(repo BoardRepo, federates []string, adminBoard string, fqdn string, propagateWait time.Duration) *Spring83Server {
	return &Spring83Server{
		repo:               repo,
		homeTemplate:       mustTemplate(),
		federates:          federates,
		adminBoard:         adminBoard,
		propagationTracker: newPropagationTracker(fqdn, propagateWait),
		fqdn:               fqdn,
		propagateWait:      propagateWait,
	}
}

func (s *Spring83Server) getBoard(key string) (*Board, error) {
	return s.repo.GetBoard(key)
}

func (s *Spring83Server) boardCount() (int, error) {
	return s.repo.BoardCount()
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

	var ifUnmodifiedSince time.Time
	ifUnmodifiedSinceHeader := r.Header["If-Unmodified-Since"]
	if ifUnmodifiedSinceHeader != nil {
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
	err = s.repo.PublishBoard(newBoard)
	if err != nil {
		log.Printf("%s", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
	}

	// Via headers are in the form "Via: Spring/83 servername.tld"
	var viaDomain string
	viaHeader := r.Header["Via"]
	if len(viaHeader) > 0 {
		tokens := strings.Split(viaHeader[0], " ")
		if len(tokens) == 2 {
			viaDomain = tokens[1]
		} else {
			log.Printf("Malformed Via header: %s", viaHeader)
		}
	}

	s.propagateBoard(newBoard, viaDomain)
}

func (server *Spring83Server) propagateBoard(board Board, viaDomain string) {
	rand.Seed(time.Now().UnixNano())
	for _, federate := range server.federates {
		normalizedFederate := strings.TrimPrefix(federate, "https://")
		normalizedFederate = strings.TrimPrefix(normalizedFederate, "http://")
		if normalizedFederate == viaDomain {
			continue
		}
		server.propagationTracker.Schedule(board, federate)
	}
}

func (s *Spring83Server) loadBoards() ([]Board, error) {
	return s.repo.GetAllBoards()
}

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
		AdminBoard Board
		Boards     []Board
	}{}

	for _, board := range boards {
		if board.Key == s.adminBoard {
			data.AdminBoard = board
		} else {
			data.Boards = append(data.Boards, board)
		}
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

func (s *Spring83Server) showIndexJson(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	type boardJson struct {
		Key    string    `json:"key"`
		Posted time.Time `json:"posted"`
	}
	type responseJson struct {
		AdminBoard boardJson   `json:"adminBoard"`
		Boards     []boardJson `json:"boards"`
	}

	var response responseJson

	boards, err := s.loadBoards()
	if err != nil {
		log.Printf("Error in showIndexJson: %s", err.Error())
		w.WriteHeader(500)
		w.Write([]byte(`{"error": "unexpected server error"}`))
		return
	}

	for _, board := range boards {
		jsonifiedBoard := boardJson{
			Key:    board.Key,
			Posted: board.Modified,
		}
		if board.Key == s.adminBoard {
			response.AdminBoard = jsonifiedBoard
		} else {
			response.Boards = append(response.Boards, jsonifiedBoard)
		}
	}

	encodedResponse, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error in showIndexJson: %s", err.Error())
		w.WriteHeader(500)
		w.Write([]byte(`{"error": "unexpected server error"}`))
		return
	}
	w.Write(encodedResponse)
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
			} else if r.URL.Path[1:] == "index.json" {
				s.showIndexJson(w, r)
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

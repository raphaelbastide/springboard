package springboard

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
)

func fileExists(name string) bool {
	if _, err := os.Stat(name); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

type Client struct {
	apiUrl string
}

func NewClient(apiUrl string) (client Client) {
	client.apiUrl = strings.TrimSuffix(apiUrl, "/")
	return
}

func (client Client) PostSignedBoard(board Board, viaFQDN string) (err error) {
	httpClient := &http.Client{}
	url := fmt.Sprintf("%s/%s", client.apiUrl, board.Key)
	fmt.Printf("URL: %s\n", url)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBufferString(board.Board))
	if err != nil {
		return
	}

	fmt.Printf("Spring-Signature: %s\n", board.Signature)
	req.Header.Set("Spring-Signature", board.Signature)

	dtHTTP := board.Modified.Format(time.RFC1123)
	req.Header.Set("If-Unmodified-Since", dtHTTP)
	req.Header.Set("Spring-Version", "83")
	req.Header.Set("Content-Type", "text/html;charset=utf-8")
	if viaFQDN != "" {
		req.Header.Set("Via", fmt.Sprintf("Spring/83 %s", viaFQDN))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	fmt.Printf("%s: %s\n", resp.Status, responseBody)
	return
}

func (client Client) SignAndPostBoard(boardText []byte, keyFolder string) (err error) {
	pubkey, privkey, err := GetKeys(keyFolder)
	if err != nil {
		return
	}

	gmt, err := time.LoadLocation("Etc/GMT")
	if err != nil {
		panic(err)
	}
	buffer, _ := time.ParseDuration("10m") // in case our computer is "fast" and the other computer is picky
	dt := time.Now().Add(-buffer).In(gmt)
	dtISO8601 := dt.Format("2006-01-02T15:04:05Z")
	boardText = append([]byte(fmt.Sprintf(`<time datetime="%s"></time>`, dtISO8601)), boardText...)

	if len(boardText) == 0 {
		err = fmt.Errorf("input required")
		return
	}
	if len(boardText) > 2217 {
		err = fmt.Errorf("input body too long")
		return
	}

	sig := ed25519.Sign(privkey, boardText)
	err = client.PostSignedBoard(Board{
		Key:       hex.EncodeToString(pubkey),
		Board:     string(boardText[:]),
		Modified:  dt,
		Signature: hex.EncodeToString(sig),
	}, "")
	if err != nil {
		err = errors.Wrap(err, "Could not post board")
		return
	}
	return
}

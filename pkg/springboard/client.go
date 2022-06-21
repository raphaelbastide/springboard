package springboard

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"
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

func (client Client) PostBoard(boardText []byte, keyFolder string) (err error) {
	serverUrl := client.apiUrl

	pubkey, privkey, err := GetKeys(keyFolder)
	if err != nil {
		return
	}

	httpClient := &http.Client{}

	gmt, err := time.LoadLocation("Etc/GMT")
	if err != nil {
		return
	}
	buffer, _ := time.ParseDuration("10m") // in case our computer is "fast" and the other computer is picky
	dt := time.Now().Add(-buffer).In(gmt)
	dtISO8601 := dt.Format("2006-01-02T15:04:05Z")
	boardText = append([]byte(fmt.Sprintf(`<time  datetime="%s">`, dtISO8601)), boardText...)

	if len(boardText) == 0 {
		err = fmt.Errorf("input required")
		return
	}
	if len(boardText) > 2217 {
		err = fmt.Errorf("input body too long")
		return
	}

	url := fmt.Sprintf("%s/%x", serverUrl, pubkey)
	fmt.Printf("URL: %s\n", url)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(boardText))
	if err != nil {
		return
	}

	sig := ed25519.Sign(privkey, boardText)
	fmt.Printf("Spring-Signature: %x\n", sig)
	req.Header.Set("Spring-Signature", fmt.Sprintf("%x", sig))

	dtHTTP := dt.Format(time.RFC1123)
	req.Header.Set("If-Unmodified-Since", dtHTTP)
	req.Header.Set("Spring-Version", "83")
	req.Header.Set("Content-Type", "text/html;charset=utf-8")

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

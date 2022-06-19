package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

func validKey() (foundPublicKey ed25519.PublicKey, foundPrivateKey ed25519.PrivateKey) {

	expiryYear := strconv.Itoa(time.Now().Year() + 1)
	expiryYearSuffix := expiryYear[len(expiryYear)-2:]
	expiryMonth := time.Now().Month()
	keyEnd := fmt.Sprintf("83e%02d%s", expiryMonth, expiryYearSuffix)
	nRoutines := runtime.NumCPU() - 1
	var waitGroup sync.WaitGroup
	var once sync.Once

	fmt.Println(" - looking for a key that ends in", keyEnd)
	fmt.Println(" - using", nRoutines, "cores")

	waitGroup.Add(nRoutines)
	for i := 0; i < nRoutines; i++ {
		go func(num int) {
			for foundPublicKey == nil {
				pub, priv, err := ed25519.GenerateKey(nil)
				if err != nil {
					panic(err)
				}

				pubStr := hex.EncodeToString(pub)
				pubSuffix := pubStr[len(pubStr)-len(keyEnd):]

				if pubSuffix == keyEnd {
					once.Do(func() {
						fmt.Printf("%s\n", fmt.Sprintf("%x", pub))
						foundPublicKey = pub
						foundPrivateKey = priv
					})
				}
			}
			waitGroup.Done()
		}(i)
	}

	waitGroup.Wait()

	return
}

func fileExists(name string) bool {
	if _, err := os.Stat(name); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func getKeys(keyFolder string) (ed25519.PublicKey, ed25519.PrivateKey) {
	configPath := ""
	var err error
	if keyFolder == "" {
		user, err := user.Current()
		if err != nil {
			panic(err)
		}

		configPath = os.Getenv("XDG_CONFIG_HOME")
		if configPath == "" {
			configPath = filepath.Join(user.HomeDir, ".config", "spring83")
		}
	} else {
		configPath = keyFolder
	}

	if err = os.MkdirAll(configPath, os.ModePerm); err != nil {
		panic(err)
	}

	pubfile := filepath.Join(configPath, "key.pub")
	privfile := filepath.Join(configPath, "key.priv")
	var pubkey ed25519.PublicKey
	var privkey ed25519.PrivateKey
	if fileExists(pubfile) && fileExists(privfile) {
		encodedPubKey, err := ioutil.ReadFile(pubfile)
		if err != nil {
			panic(err)
		}
		pubkey, err = hex.DecodeString(string(encodedPubKey[:]))
		if err != nil {
			panic(err)
		}
		encodedPrivKey, err := ioutil.ReadFile(privfile)
		if err != nil {
			panic(err)
		}
		privkey, err = hex.DecodeString(string(encodedPrivKey[:]))
		if err != nil {
			panic(err)
		}
	} else {
		fmt.Printf("I am fishing in the sea of all possible keys for a valid spring83 key. This may take a bit...\n")
		pubkey, privkey = validKey()
		os.WriteFile(pubfile, []byte(hex.EncodeToString(pubkey)), 0644)
		os.WriteFile(privfile, []byte(hex.EncodeToString(privkey)), 0600)
	}

	return pubkey, privkey
}

func main() {
	if len(os.Args) == 1 || len(os.Args) > 3 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		printHelp()
		os.Exit(0)
	}

	serverUrl := strings.TrimSuffix(os.Args[1], "/")
	keyFolder := ""
	if len(os.Args) == 3 {
		keyFolder = os.Args[2]
	}

	pubkey, privkey := getKeys(keyFolder)

	client := &http.Client{}

	gmt, _ := time.LoadLocation("GMT")
	buffer, _ := time.ParseDuration("10m") // in case our computer is "fast" and the other computer is picky
	body, err := ioutil.ReadAll(os.Stdin)
	dt := time.Now().Add(-buffer).In(gmt)
	dtISO8601 := dt.Format("2006-01-02T15:04:05Z")
	body = append([]byte(fmt.Sprintf(`<time  datetime="%s">`, dtISO8601)), body...)
	if err != nil {
		panic(err)
	}

	if len(body) == 0 {
		panic(fmt.Errorf("input required"))
	}
	if len(body) > 2217 {
		panic(fmt.Errorf("input body too long"))
	}

	url := fmt.Sprintf("%s/%x", serverUrl, pubkey)
	fmt.Printf("URL: %s\n", url)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	if err != nil {
		panic(err)
	}

	sig := ed25519.Sign(privkey, body)
	fmt.Printf("Spring-Signature: %x\n", sig)
	req.Header.Set("Spring-Signature", fmt.Sprintf("%x", sig))

	dtHTTP := dt.Format(time.RFC1123)
	req.Header.Set("If-Unmodified-Since", dtHTTP)
	req.Header.Set("Spring-Version", "83")
	req.Header.Set("Content-Type", "text/html;charset=utf-8")

	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s: %s\n", resp.Status, responseBody)
}

func printHelp() {
	fmt.Printf(`Usage:

%s SERVER_URL [KEY_PAIR_FOLDER_PATH]

Updates a board with the text from standard input. 
You can either pipe the input or enter it and press ctrl-d.

SERVER_URL:           the full URL for the spring83 server

KEY_PAIR_FOLDER_PATH: (optional) path of folder with valid public/private key path
                      if not provided, uses a standard path e.g. ~/.config/spring83
                      this folder will be create if it doesn't exist
                      creates/finds a new valid key pair if none exist at path

-h or --help:         displays this help	
`, os.Args[0])
}

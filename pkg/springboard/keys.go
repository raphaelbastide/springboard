package springboard

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
)

func ConfigPath() (configPath string) {
	user, err := user.Current()
	if err != nil {
		panic(err)
	}

	configPath = os.Getenv("XDG_CONFIG_HOME")
	if configPath == "" {
		configPath = filepath.Join(user.HomeDir, ".config", "spring83")
	}
	return
}

func getKeyPaths(keyPath string) (publicKeyPath string, privateKeyPath string) {
	if keyPath == "" {
		keyPath = ConfigPath()
	}
	publicKeyPath = filepath.Join(keyPath, "key.pub")
	privateKeyPath = filepath.Join(keyPath, "key.priv")
	return
}

func GetKeys(keyPath string) (pubkey ed25519.PublicKey, privkey ed25519.PrivateKey, err error) {
	pubfile, privfile := getKeyPaths(keyPath)
	var encodedPubKey []byte
	var encodedPrivKey []byte

	if fileExists(pubfile) && fileExists(privfile) {
		encodedPubKey, err = ioutil.ReadFile(pubfile)
		if err != nil {
			panic(err)
		}
		pubkey, err = hex.DecodeString(string(encodedPubKey[:]))
		if err != nil {
			panic(err)
		}
		encodedPrivKey, err = ioutil.ReadFile(privfile)
		if err != nil {
			panic(err)
		}
		privkey, err = hex.DecodeString(string(encodedPrivKey[:]))
		if err != nil {
			panic(err)
		}
	} else {
		actualKeyPath := filepath.Dir(pubfile)
		err = fmt.Errorf(`Could not load public and private keys at %s. You may need to run "springboard generate-key" first`, actualKeyPath)
		return
	}
	return
}

func GenerateValidKeys(keyPath string) (err error) {
	var foundPrivateKey ed25519.PrivateKey
	var foundPublicKey ed25519.PublicKey

	fmt.Printf("I am fishing in the sea of all possible keys for a valid spring83 key. This may take a bit...\n")

	pubfile, privfile := getKeyPaths(keyPath)
	actualKeyPath := filepath.Dir(privfile)

	if err = os.MkdirAll(actualKeyPath, os.ModePerm); err != nil {
		panic(err)
	}

	expiryYear := strconv.Itoa(time.Now().Year() + 1)
	expiryYearSuffix := expiryYear[len(expiryYear)-2:]
	expiryMonth := time.Now().Month()
	keyEnd := fmt.Sprintf("83e%02d%s", expiryMonth, expiryYearSuffix)
	nRoutines := runtime.NumCPU() - 1
	var waitGroup sync.WaitGroup
	var once sync.Once

	fmt.Println(" - looking for a key that ends in", keyEnd)
	fmt.Println(" - using", nRoutines, "cores")
	fmt.Println(" - writing keys to", actualKeyPath)

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

	os.WriteFile(pubfile, []byte(hex.EncodeToString(foundPublicKey)), 0644)
	os.WriteFile(privfile, []byte(hex.EncodeToString(foundPrivateKey)), 0600)
	return
}

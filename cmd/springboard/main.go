package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	"github.com/motevets/s83/pkg/springboard"
)

func main() {
	var err error
	if len(os.Args) == 1 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		printRootHelp()
		os.Exit(0)
	}

	switch os.Args[1] {
	case "post":
		err = post()
	case "serve":
		serve()
	case "generate-key":
		err = generateKey()
	case "help":
		help()
	default:
		err = fmt.Errorf("Unrecognized sub-command.")
		printRootHelp()
	}
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func help() {
	switch os.Args[2] {
	case "post":
		printPostHelp()
	case "serve":
		printServeHelp()
	case "generate-key":
		printGenerateKeyHelp()
	case "help":
		printRootHelp()
	default:
		fmt.Println("Unrecognized sub-command.")
		printRootHelp()
		os.Exit(1)
	}
}

func generateKey() (err error) {
	if len(os.Args) > 2 && (os.Args[2] == "-h" || os.Args[2] == "--help") {
		printGenerateKeyHelp()
		return
	}
	var keyPairDir string
	if len(os.Args) > 2 {
		keyPairDir = os.Args[2]
	}
	err = springboard.GenerateValidKeys(keyPairDir)
	return
}

func serve() {
	if len(os.Args) > 2 && (os.Args[2] == "-h" || os.Args[2] == "--help") {
		printServeHelp()
		return
	}

	port, err := strconv.ParseUint(os.Getenv("PORT"), 10, 16)
	if err != nil {
		port = 8000
	}

	springboard.RunServer(uint(port))
}

func post() (err error) {
	if len(os.Args) == 2 || (os.Args[2] == "-h" || os.Args[2] == "--help") {
		printPostHelp()
		return
	}
	var apiUrl string
	var keyPath string

	apiUrl = os.Args[2]
	if len(os.Args) > 3 {
		keyPath = os.Args[3]
	}

	client := springboard.NewClient(apiUrl)
	body, err := ioutil.ReadAll(os.Stdin)
	err = client.PostBoard(body, keyPath)

	return
}

func printServeHelp() {
	fmt.Println(`springboard serve

Usage:

  [PORT=...] springboard serve

Environment Variables:

  PORT: port on which to listen (default: 8000)`)
}

func printPostHelp() {
	fmt.Println(`springboard post

Usage:

  springboard post SERVER_URL [KEY_PAIR_FOLDER_PATH]

  Updates a board with the text from standard input. 
  You can either pipe the input or enter it and press ctrl-d.

Parameters:

  SERVER_URL:           the full URL for the spring83 server

  KEY_PAIR_FOLDER_PATH: (optional) path of folder with valid public/private key path
                        if not provided, uses a standard path e.g. ~/.config/spring83
                        this folder will be create if it doesn't exist
                        creates/finds a new valid key pair if none exist at path`)
}

func printGenerateKeyHelp() {
	fmt.Println(`springboard generate-key

Usage:

  springboard generate-key [KEY_LOCATION]

Parameters:

  KEY_LOCATION: (optional) path to a folder that contains a valid Spring '83 key pair (defaults to ~/.config/spring83)`)
}

func printRootHelp() {
	fmt.Println(`springboard

Usage:

  springboard SUBCOMMAND

Valid SUBCOMMANDS are:

  post (posts a board to a server)
  serve (starts a Spring '83 server)
  generate-keys (generates a new Spring '83 compliant key)
  help (shows the help for a sub-command)`)
}

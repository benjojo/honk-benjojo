package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var debugMode = false

func main() {
	server := ""
	username := ""
	password := ""

	flag.StringVar(&server, "server", server, "server to connnect")
	flag.StringVar(&username, "username", username, "username to use")
	flag.StringVar(&password, "password", password, "password to use")
	flag.BoolVar(&debugMode, "debug", debugMode, "debug mode")
	flag.Parse()

	if server == "" || username == "" || password == "" {
		flag.Usage()
		os.Exit(1)
	}
	if !(strings.HasPrefix(server, "https://") || strings.HasPrefix(server, "http://")) {
		server = "https://" + server
	}

	form := make(url.Values)
	form.Add("username", username)
	form.Add("password", password)
	form.Add("gettoken", "1")
	loginurl := fmt.Sprintf("%s/dologin", server)
	req, err := http.NewRequest("POST", loginurl, strings.NewReader(form.Encode()))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := http.DefaultClient
	if debugMode {
		client = debugClient
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	answer, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode != 200 {
		log.Fatalf("status: %d: %s", resp.StatusCode, answer)
	}
	fmt.Println(string(answer))
}

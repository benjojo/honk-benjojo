package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var debugMode = false

func honkahonk(server, token, wonk, wonkles string) {
	form := make(url.Values)
	form.Add("token", token)
	form.Add("action", "honk")
	form.Add("noise", wonk)
	form.Add("wonkles", wonkles)
	apiurl := fmt.Sprintf("https://%s/api", server)
	req, err := http.NewRequest("POST", apiurl, strings.NewReader(form.Encode()))
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
}

func main() {
	server := ""
	token := ""
	wonkles := ""
	flag.StringVar(&server, "server", server, "server to connnect")
	flag.StringVar(&token, "token", token, "auth token to use")
	flag.StringVar(&wonkles, "wonkles", wonkles, "wordlist to use")
	flag.BoolVar(&debugMode, "debug", debugMode, "debug mode")
	flag.Parse()

	if server == "" || token == "" || wonkles == "" {
		flag.Usage()
		os.Exit(1)
	}

	wordlist, err := fetchsome(wonkles)
	if err != nil {
		log.Printf("error fetching wonkles: %s", err)
	}
	var words []string
	for _, w := range strings.Split(string(wordlist), "\n") {
		words = append(words, w)
	}
	max := big.NewInt(int64(len(words)))
	i, _ := rand.Int(rand.Reader, max)
	wonk := words[i.Int64()]

	log.Printf("picking: %s", wonk)

	honkahonk(server, token, wonk, wonkles)
}

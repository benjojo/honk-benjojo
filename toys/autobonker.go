package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Honk struct {
	ID     int
	XID    string
	Honker string
}

type HonkSet struct {
	Honks []Honk
}

func gethonks(server, token string, wanted int) HonkSet {
	form := make(url.Values)
	form.Add("action", "gethonks")
	form.Add("page", "atme")
	form.Add("after", fmt.Sprintf("%d", wanted))
	form.Add("wait", "30")
	apiurl := fmt.Sprintf("https://%s/api?%s", server, form.Encode())
	req, err := http.NewRequest("GET", apiurl, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 502 {
			log.Printf("server error 502...")
			time.Sleep(5 * time.Minute)
			return HonkSet{}
		}
		answer, _ := ioutil.ReadAll(resp.Body)
		log.Fatalf("status: %d: %s", resp.StatusCode, answer)
	}
	var honks HonkSet
	d := json.NewDecoder(resp.Body)
	err = d.Decode(&honks)
	if err != nil {
		log.Fatal(err)
	}
	return honks
}

func bonk(server, token string, honk Honk) {
	log.Printf("bonking %s from %s", honk.XID, honk.Honker)
	form := make(url.Values)
	form.Add("action", "zonkit")
	form.Add("wherefore", "bonk")
	form.Add("what", honk.XID)
	apiurl := fmt.Sprintf("https://%s/api", server)
	req, err := http.NewRequest("POST", apiurl, strings.NewReader(form.Encode()))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		answer, _ := ioutil.ReadAll(resp.Body)
		log.Fatalf("status: %d: %s", resp.StatusCode, answer)
	}
}

func main() {
	server := ""
	token := ""
	flag.StringVar(&server, "server", server, "server to connnect")
	flag.StringVar(&token, "token", token, "auth token to use")
	flag.Parse()

	if server == "" || token == "" {
		flag.Usage()
		os.Exit(1)
	}

	wanted := 0

	for {
		honks := gethonks(server, token, wanted)
		for i, h := range honks.Honks {
			bonk(server, token, h)
			if i > 0 {
				time.Sleep(3 * time.Second)
			}
			if wanted < h.ID {
				wanted = h.ID
			}

		}
		time.Sleep(3 * time.Second)
	}
}

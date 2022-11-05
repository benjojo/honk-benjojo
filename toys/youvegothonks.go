package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"time"
)

type Honk struct {
	ID int
	Honker string
	Noise string
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
	req.Header.Add("Authorization", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
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
		for _, h := range honks.Honks {
			fmt.Printf("you've got a honk from %s\n%s\n", h.Honker, h.Noise)
			if wanted < h.ID {
				wanted = h.ID
			}
		}
		time.Sleep(1 * time.Second)
	}
}

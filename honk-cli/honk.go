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

var (
	honkSite = flag.String("honk.site", "benjojo.co.uk", "")
)

func getSiteToken() string {
	server := *honkSite

	username, password := fetchSiteCreds()
	if server == "" || username == "" || password == "" {
		os.Exit(1)
	}

	form := make(url.Values)
	form.Add("username", username)
	form.Add("password", password)
	form.Add("gettoken", "1")
	loginurl := fmt.Sprintf("https://%s/dologin", server)
	req, err := http.NewRequest("POST", loginurl, strings.NewReader(form.Encode()))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := http.DefaultClient

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
	return string(answer)
}

func fetchSiteCreds() (username, password string) {
	cfgD, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("Could not locate where to find honk creds")
	}

	unBytes, usernameerr := os.ReadFile(fmt.Sprintf("%s/honk/username", cfgD))
	if usernameerr != nil {
		log.Fatalf("Could not read the file for the username %v", usernameerr)
	}
	pwBytes, passworderr := os.ReadFile(fmt.Sprintf("%s/honk/password", cfgD))
	if passworderr != nil {
		log.Fatalf("Could not read the file for the password %v", passworderr)
	}

	return strings.Trim(string(unBytes), "\r\n\t "), strings.Trim(string(pwBytes), "\r\n\t ")
}

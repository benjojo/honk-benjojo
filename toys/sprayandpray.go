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

func sendmsg(server, token, msg, rcpt string) {
	form := make(url.Values)
	form.Add("token", token)
	form.Add("action", "sendactivity")
	form.Add("msg", msg)
	form.Add("rcpt", rcpt)
	apiurl := fmt.Sprintf("https://%s/api", server)
	req, err := http.NewRequest("POST", apiurl, strings.NewReader(form.Encode()))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
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
	var server, token, msgfile, rcpt string
	flag.StringVar(&server, "server", server, "server to connnect")
	flag.StringVar(&token, "token", token, "auth token to use")
	flag.StringVar(&msgfile, "msgfile", token, "file with message to send")
	flag.StringVar(&rcpt, "rcpt", rcpt, "rcpt to send it to")
	flag.Parse()

	if server == "" || token == "" || msgfile == "" || rcpt == "" {
		flag.Usage()
		os.Exit(1)
	}
	msg, err := ioutil.ReadFile(msgfile)
	if err != nil {
		log.Fatal(err)
	}

	sendmsg(server, token, string(msg), rcpt)
}

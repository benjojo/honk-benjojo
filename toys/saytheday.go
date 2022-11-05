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
	"time"
)

func lookandsay(n int) string {
	s := "1"

	numbers := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	var buf strings.Builder
	for i := 2; i <= n; i++ {
		count := 1
		prev := s[0]
		for j := 1; j < len(s); j++ {
			d := s[j]
			if d == prev {
				count++
			} else {
				buf.WriteString(numbers[count])
				buf.WriteByte(prev)
				count = 1
				prev = d
			}
		}
		buf.WriteString(numbers[count])
		buf.WriteByte(prev)
		s = buf.String()
		buf.Reset()
	}
	return s
}

func honkahonk(server, token, noise string) {
	form := make(url.Values)
	form.Add("token", token)
	form.Add("action", "honk")
	form.Add("noise", noise)
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
	server := ""
	token := ""
	flag.StringVar(&server, "server", server, "server to connnect")
	flag.StringVar(&token, "token", token, "auth token to use")
	flag.Parse()

	if server == "" || token == "" {
		flag.Usage()
		os.Exit(1)
	}

	day := time.Now().Day()
	say := lookandsay(day)

	honkahonk(server, token, say)
}

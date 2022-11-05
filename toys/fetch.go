package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

var debugClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	},
}

func fetchsome(url string) ([]byte, error) {
	client := http.DefaultClient
	if debugMode {
		client = debugClient
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("error fetching %s: %s", url, err)
		return nil, err
	}
	req.Header.Set("User-Agent", "honksnonk/4.0")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("error fetching %s: %s", url, err)
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 201:
	case 202:
	default:
		return nil, fmt.Errorf("http get not 200: %d %s", resp.StatusCode, url)
	}
	var buf bytes.Buffer
	limiter := io.LimitReader(resp.Body, 10*1024*1024)
	io.Copy(&buf, limiter)
	return buf.Bytes(), nil
}

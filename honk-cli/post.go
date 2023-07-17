package main

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

var (
	attachmentFile = flag.String("post.attachments", "", "provide a zip file of all the attachments you want to post")
)

func cliPost() {
	log.Printf("Fetching honk creds")
	tok := getSiteToken()

	f, err := os.CreateTemp("./", "honkpost*")
	defer os.Remove(f.Name())
	if err != nil {
		log.Fatalf("Cannot make temp file for posting: %v", err)
	}

	editor := exec.Command("editor", f.Name())
	editor.Stderr = os.Stderr
	editor.Stdout = os.Stdout
	editor.Stdin = os.Stdin

	editor.Run()
	editor.Wait()

	postContent, err := os.ReadFile(f.Name())
	if err != nil {
		log.Fatalf("could not read post: %v", err)
	}

	postContentString := strings.Trim(string(postContent), "\r\n\t ")

	var donkIDs []string
	if *attachmentFile != "" {
		donkIDs = uploadAttachments(tok)
	}

	buf := bytes.NewBuffer(nil)
	formWriter := multipart.NewWriter(buf)

	// multipart.Form.
	w, _ := formWriter.CreateFormField("token")
	w.Write([]byte(tok))
	w, _ = formWriter.CreateFormField("action")
	w.Write([]byte("honk"))
	w, _ = formWriter.CreateFormField("noise")
	w.Write([]byte((postContentString)))
	for _, donkID := range donkIDs {
		w, _ := formWriter.CreateFormField("donkxid")
		w.Write([]byte(donkID))
	}
	formWriter.Close()

	apiurl := fmt.Sprintf("https://%s/api", *honkSite)
	req, err := http.NewRequest("POST", apiurl, buf)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("Content-Type", formWriter.FormDataContentType())
	req.Header.Add("Authorization", tok)
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
	log.Printf("Output: %v", string(answer))
}

// Gives you all of the donk IDs
func uploadAttachments(tok string) []string {
	// donkxid
	zipfd, err := os.Open(*attachmentFile)
	if err != nil {
		log.Fatalf("Cannot open zip target %v", err)
	}

	stat, _ := zipfd.Stat()

	zipReader, zipError := zip.NewReader(zipfd, stat.Size())
	if err != nil {
		log.Fatalf("Cannot open ZIP: %v", zipError)
	}

	xids := make([]string, 0)

	for _, v := range zipReader.File {
		zipfile, err := v.Open()
		if err != nil {
			log.Printf("Cannot open zip file %v -> %v", v.Name, err)
			continue
		}

		actualFile, _ := ioutil.ReadAll(zipfile)

		buf := bytes.NewBuffer(nil)
		formWriter := multipart.NewWriter(buf)

		// multipart.Form.
		w, _ := formWriter.CreateFormField("token")
		w.Write([]byte(tok))
		w, _ = formWriter.CreateFormField("action")
		w.Write([]byte("donk"))
		w, _ = formWriter.CreateFormField("donkdesc")
		w.Write([]byte((v.Name)))
		w, _ = formWriter.CreateFormFile("donk", v.Name)
		io.Copy(w, bytes.NewReader(actualFile))
		formWriter.Close()

		apiurl := fmt.Sprintf("https://%s/api", *honkSite)
		req, err := http.NewRequest("POST", apiurl, buf)
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Add("Content-Type", formWriter.FormDataContentType())
		req.Header.Add("Authorization", tok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		defer resp.Body.Close()
		answer, _ := ioutil.ReadAll(resp.Body)
		xids = append(xids, string(answer))
		log.Printf("uploaded %v as %s", v.Name, answer)
	}

	return xids
}

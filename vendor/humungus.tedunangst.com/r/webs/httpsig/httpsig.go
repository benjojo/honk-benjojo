//
// Copyright (c) 2019 Ted Unangst <tedu@tedunangst.com>
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
// ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
// ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
// OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

// An implementation of HTTP Signatures
package httpsig

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"humungus.tedunangst.com/r/webs/junk"
)

func sb64(data []byte) string {
	var sb strings.Builder
	b64 := base64.NewEncoder(base64.StdEncoding, &sb)
	b64.Write(data)
	b64.Close()
	return sb.String()
}

func b64s(s string) []byte {
	var buf bytes.Buffer
	b64 := base64.NewDecoder(base64.StdEncoding, strings.NewReader(s))
	io.Copy(&buf, b64)
	return buf.Bytes()
}

func sb64sha256(content []byte) string {
	h := sha256.New()
	h.Write(content)
	return sb64(h.Sum(nil))
}

// Sign a request and add Signature header
func SignRequest(keyname string, key *rsa.PrivateKey, req *http.Request, content []byte) {
	headers := []string{"(request-target)", "date", "host", "content-type", "digest"}
	var stuff []string
	for _, h := range headers {
		var s string
		switch h {
		case "(request-target)":
			s = strings.ToLower(req.Method) + " " + req.URL.RequestURI()
		case "date":
			s = req.Header.Get(h)
			if s == "" {
				s = time.Now().UTC().Format(http.TimeFormat)
				req.Header.Set(h, s)
			}
		case "host":
			s = req.Header.Get(h)
			if s == "" {
				s = req.URL.Hostname()
				req.Header.Set(h, s)
			}
		case "content-type":
			s = req.Header.Get(h)
		case "digest":
			s = req.Header.Get(h)
			if s == "" {
				s = "SHA-256=" + sb64sha256(content)
				req.Header.Set(h, s)
			}
		}
		stuff = append(stuff, h+": "+s)
	}

	h := sha256.New()
	h.Write([]byte(strings.Join(stuff, "\n")))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h.Sum(nil))
	bsig := sb64(sig)

	sighdr := fmt.Sprintf(`keyId="%s",algorithm="%s",headers="%s",signature="%s"`,
		keyname, "rsa-sha256", strings.Join(headers, " "), bsig)
	req.Header.Set("Signature", sighdr)
}

var re_sighdrval = regexp.MustCompile(`(.*)="(.*)"`)

// Verify the Signature header for a request is valid.
// The request body should be provided separately.
// The lookupPubkey function takes a keyname and returns a public key.
// Returns keyname if known, and/or error.
func VerifyRequest(req *http.Request, content []byte, lookupPubkey func(string) *rsa.PublicKey) (string, error) {
	sighdr := req.Header.Get("Signature")
	if sighdr == "" {
		return "", fmt.Errorf("no signature header")
	}

	var keyname, algo, heads, bsig string
	for _, v := range strings.Split(sighdr, ",") {
		m := re_sighdrval.FindStringSubmatch(v)
		if len(m) != 3 {
			return "", fmt.Errorf("bad scan: %s from %s\n", v, sighdr)
		}
		switch m[1] {
		case "keyId":
			keyname = m[2]
		case "algorithm":
			algo = m[2]
		case "headers":
			heads = m[2]
		case "signature":
			bsig = m[2]
		default:
			return "", fmt.Errorf("bad sig val: %s", m[1])
		}
	}
	if keyname == "" || algo == "" || heads == "" || bsig == "" {
		return "", fmt.Errorf("missing a sig value")
	}

	key := lookupPubkey(keyname)
	if key == nil {
		return keyname, fmt.Errorf("no key for %s", keyname)
	}
	headers := strings.Split(heads, " ")
	var stuff []string
	for _, h := range headers {
		var s string
		switch h {
		case "(request-target)":
			s = strings.ToLower(req.Method) + " " + req.URL.RequestURI()
		case "host":
			s = req.Host
			if s == "" {
				log.Printf("warning: no host header value")
			}
		case "digest":
			s = req.Header.Get(h)
			expv := "SHA-256=" + sb64sha256(content)
			if s != expv {
				return "", fmt.Errorf("digest header '%s' did not match content", s)
			}
		default:
			s = req.Header.Get(h)
		}
		stuff = append(stuff, h+": "+s)
	}

	h := sha256.New()
	h.Write([]byte(strings.Join(stuff, "\n")))
	sig := b64s(bsig)
	err := rsa.VerifyPKCS1v15(key, crypto.SHA256, h.Sum(nil), sig)
	if err != nil {
		return keyname, err
	}
	return keyname, nil
}

// Unmarshall an ASCII string into (optional) private and public keys
func DecodeKey(s string) (pri *rsa.PrivateKey, pub *rsa.PublicKey, err error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		err = fmt.Errorf("no pem data")
		return
	}
	switch block.Type {
	case "PUBLIC KEY":
		var k interface{}
		k, err = x509.ParsePKIXPublicKey(block.Bytes)
		if k != nil {
			pub, _ = k.(*rsa.PublicKey)
		}
	case "RSA PUBLIC KEY":
		pub, err = x509.ParsePKCS1PublicKey(block.Bytes)
	case "RSA PRIVATE KEY":
		pri, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err == nil {
			pub = &pri.PublicKey
		}
	default:
		err = fmt.Errorf("unknown key type")
	}
	return
}

// Marshall an RSA key into an ASCII string
func EncodeKey(i interface{}) (string, error) {
	var b pem.Block
	var err error
	switch k := i.(type) {
	case *rsa.PrivateKey:
		b.Type = "RSA PRIVATE KEY"
		b.Bytes = x509.MarshalPKCS1PrivateKey(k)
	case *rsa.PublicKey:
		b.Type = "PUBLIC KEY"
		b.Bytes, err = x509.MarshalPKIXPublicKey(k)
	default:
		err = fmt.Errorf("unknown key type: %s", k)
	}
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&b)), nil
}

var cachedKeys = make(map[string]*rsa.PublicKey)
var cachedKeysLock sync.Mutex

// Get a key as typically used with ActivityPub
func ActivityPubKeyGetter(keyname string) (key *rsa.PublicKey) {
	cachedKeysLock.Lock()
	key = cachedKeys[keyname]
	cachedKeysLock.Unlock()
	if key != nil {
		return key
	}
	j, err := junk.Get(keyname, junk.GetArgs{Accept: "application/activity+json", Timeout: 5 * time.Second})
	if err != nil {
		log.Printf("error getting %s pubkey: %s", keyname, err)
		return
	}
	keyobj, ok := j.GetMap("publicKey")
	if ok {
		j = keyobj
	}
	data, ok := j.GetString("publicKeyPem")
	if !ok {
		log.Printf("error finding %s pubkey", keyname)
		return
	}
	_, ok = j.GetString("owner")
	if !ok {
		log.Printf("error finding %s pubkey owner", keyname)
		return
	}
	_, key, err = DecodeKey(data)
	if err != nil {
		log.Printf("error decoding %s pubkey: %s", keyname, err)
		return
	}
	cachedKeysLock.Lock()
	cachedKeys[keyname] = key
	cachedKeysLock.Unlock()
	return
}

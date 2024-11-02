//
// Copyright (c) 2024 Ted Unangst <tedu@tedunangst.com>
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

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/nacl/box"
	"humungus.tedunangst.com/r/webs/gencache"
)

type boxSecKey struct {
	key *[32]byte
}
type boxPubKey struct {
	key *[32]byte
}

func encryptString(plain string, seckey boxSecKey, pubkey boxPubKey) (string, error) {
	if seckey.key == nil {
		return "", fmt.Errorf("no secret key")
	}
	var nonce [24]byte
	rand.Read(nonce[:])
	out := box.Seal(nil, []byte(plain), &nonce, pubkey.key, seckey.key)

	var sb strings.Builder
	b64 := base64.NewEncoder(base64.StdEncoding, &sb)
	b64.Write(nonce[:])
	b64.Write(out)
	b64.Close()
	return sb.String(), nil
}

func decryptString(encmsg string, seckey boxSecKey, pubkey boxPubKey) (string, error) {
	if seckey.key == nil {
		return "", fmt.Errorf("no secret key")
	}
	var buf bytes.Buffer
	b64 := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encmsg))
	io.Copy(&buf, b64)
	data := buf.Bytes()
	if len(data) < 24 {
		return "", fmt.Errorf("not enough data")
	}
	var nonce [24]byte
	copy(nonce[:], data)
	data = data[24:]
	out, ok := box.Open(nil, data, &nonce, pubkey.key, seckey.key)
	if !ok {
		return "", fmt.Errorf("error decrypting chonk")
	}
	return string(out), nil
}

func b64tokey(s string) (*[32]byte, error) {
	var buf bytes.Buffer
	b64 := base64.NewDecoder(base64.StdEncoding, strings.NewReader(s))
	n, _ := io.Copy(&buf, b64)
	if n != 32 {
		return nil, fmt.Errorf("bad key size")
	}
	var key [32]byte
	copy(key[:], buf.Bytes())
	return &key, nil
}

func tob64(data []byte) string {
	var sb strings.Builder
	b64 := base64.NewEncoder(base64.StdEncoding, &sb)
	b64.Write(data)
	b64.Close()
	return sb.String()
}

func newChatKeys() (boxPubKey, boxSecKey) {
	pub, sec, _ := box.GenerateKey(rand.Reader)
	return boxPubKey{pub}, boxSecKey{sec}
}

var chatkeys = gencache.New(gencache.Options[string, boxPubKey]{Fill: func(xonker string) (boxPubKey, bool) {
	data := getxonker(xonker, chatKeyProp)
	if data == "" {
		dlog.Printf("hitting the webs for missing chatkey: %s", xonker)
		j, err := GetJunk(firstUserUID, xonker)
		if err != nil {
			ilog.Printf("error getting %s: %s", xonker, err)
			savexonker(xonker, "failed", chatKeyProp)
			return boxPubKey{}, true
		}
		allinjest(originate(xonker), j)
		data = getxonker(xonker, chatKeyProp)
		if data == "" {
			ilog.Printf("key not found after ingesting")
			savexonker(xonker, "failed", chatKeyProp)
			return boxPubKey{}, true
		}
	}
	if data == "failed" {
		ilog.Printf("lookup previously failed chatkey %s", xonker)
		return boxPubKey{}, true
	}
	var pubkey boxPubKey
	var err error
	pubkey.key, err = b64tokey(data)
	if err != nil {
		ilog.Printf("error decoding %s pubkey: %s", xonker, err)
	}
	return pubkey, true
}, Limit: 512})

func getchatkey(xonker string) (boxPubKey, bool) {
	pubkey, _ := chatkeys.Get(xonker)
	return pubkey, pubkey.key != nil
}

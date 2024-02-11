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

// A wrapper to make dealing with untyped json a little easier.
package junk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Some json. See methods.
type Junk map[string]interface{}

// Create some fresh junk.
func New() Junk {
	return make(map[string]interface{})
}

// Write json. Pretty printed.
func (j Junk) Write(w io.Writer) error {
	e := json.NewEncoder(w)
	e.SetEscapeHTML(false)
	e.SetIndent("", "  ")
	err := e.Encode(j)
	return err
}

// Read and decode json into a junk object.
func Read(r io.Reader) (Junk, error) {
	decoder := json.NewDecoder(r)
	var j Junk
	err := decoder.Decode(&j)
	if err != nil {
		return nil, err
	}
	return j, nil
}

// Return as bytes
func (j Junk) ToBytes() []byte {
	var buf bytes.Buffer
	j.Write(&buf)
	return buf.Bytes()
}

// Return as string
func (j Junk) ToString() string {
	var buf bytes.Buffer
	j.Write(&buf)
	return buf.String()
}

// Read from bytes
func FromBytes(b []byte) (Junk, error) {
	return Read(bytes.NewReader(b))
}

// Read from string
func FromString(s string) (Junk, error) {
	return Read(strings.NewReader(s))
}

// Additional arguments for the Get function
type GetArgs struct {
	Accept  string // Accept: header
	Agent   string // User-Agent: header
	Timeout time.Duration
	Client  *http.Client
	Fixup   func(*http.Request) error
	Limit   int64
}

// Fetch json from url via http and return some junk.
func Get(url string, args GetArgs) (Junk, error) {
	client := http.DefaultClient
	if args.Client != nil {
		client = args.Client
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if args.Accept != "" {
		req.Header.Set("Accept", args.Accept)
	}
	if args.Agent != "" {
		req.Header.Set("User-Agent", args.Agent)
	}
	if args.Fixup != nil {
		err = args.Fixup(req)
		if err != nil {
			return nil, err
		}
	}
	if args.Timeout != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), args.Timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
	case 201:
	case 202:
	default:
		return nil, fmt.Errorf("http get status: %d", resp.StatusCode)
	}
	var r io.Reader = resp.Body
	if args.Limit > 0 {
		r = io.LimitReader(r, args.Limit)
	}
	return Read(r)
}

func jsonfindinterface(ii interface{}, keys []interface{}) interface{} {
	for _, key := range keys {
		idx, ok := key.(int)
		if ok {
			m, ok := ii.([]interface{})
			if !ok || idx >= len(m) {
				return nil
			}
			ii = m[idx]
		} else {
			m, ok := ii.(map[string]interface{})
			if !ok {
				m, ok = ii.(Junk)
			}
			if !ok {
				return nil
			}
			ii = m[key.(string)]
			if ii == nil {
				return nil
			}
		}
	}
	return ii
}

// Find and return a string value (and true for success).
// keys may be strings or integers used to walk through the object.
func (j Junk) GetString(keys ...interface{}) (string, bool) {
	s, ok := jsonfindinterface(j, keys).(string)
	return s, ok
}

// Find and return a float value (and true for success).
// keys may be strings or integers used to walk through the object.
func (j Junk) GetNumber(keys ...interface{}) (float64, bool) {
	s, ok := jsonfindinterface(j, keys).(float64)
	return s, ok
}

// Find and return an array (and true for success).
// keys may be strings or integers used to walk through the object.
func (j Junk) GetArray(keys ...interface{}) ([]interface{}, bool) {
	a, ok := jsonfindinterface(j, keys).([]interface{})
	if ok {
		for i, ii := range a {
			j, ok := ii.(map[string]interface{})
			if ok {
				a[i] = Junk(j)
			}
		}
	}
	return a, ok
}

// Find and return some more junk (and true for success).
// keys may be strings or integers used to walk through the object.
func (j Junk) GetMap(keys ...interface{}) (Junk, bool) {
	ii := jsonfindinterface(j, keys)
	m, ok := ii.(map[string]interface{})
	if !ok {
		m, ok = ii.(Junk)
	}
	return m, ok
}

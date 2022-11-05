package junk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type Junk map[string]interface{}

func New() Junk {
	return make(map[string]interface{})
}

func (j Junk) Write(w io.Writer) error {
	e := json.NewEncoder(w)
	e.SetEscapeHTML(false)
	e.SetIndent("", "  ")
	err := e.Encode(j)
	return err
}

func Read(r io.Reader) (Junk, error) {
	decoder := json.NewDecoder(r)
	var j Junk
	err := decoder.Decode(&j)
	if err != nil {
		return nil, err
	}
	return j, nil
}

type GetArgs struct {
	Accept  string
	Agent   string
	Timeout time.Duration
}

func Get(url string, args GetArgs) (Junk, error) {
	client := http.DefaultClient
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

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http get status: %d", resp.StatusCode)
	}
	return Read(resp.Body)
}

func jsonfindinterface(ii interface{}, keys []string) interface{} {
	for _, key := range keys {
		idx, err := strconv.Atoi(key)
		if err == nil {
			m := ii.([]interface{})
			if idx >= len(m) {
				return nil
			}
			ii = m[idx]
		} else {
			m, ok := ii.(map[string]interface{})
			if !ok {
				m = ii.(Junk)
			}
			ii = m[key]
			if ii == nil {
				return nil
			}
		}
	}
	return ii
}
func (j Junk) FindString(keys []string) (string, bool) {
	s, ok := jsonfindinterface(j, keys).(string)
	return s, ok
}
func (j Junk) FindArray(keys []string) ([]interface{}, bool) {
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
func (j Junk) FindMap(keys []string) (Junk, bool) {
	ii := jsonfindinterface(j, keys)
	m, ok := ii.(map[string]interface{})
	if !ok {
		m, ok = ii.(Junk)
	}
	return m, ok
}
func (j Junk) GetString(key string) (string, bool) {
	return j.FindString([]string{key})
}
func (j Junk) GetArray(key string) ([]interface{}, bool) {
	return j.FindArray([]string{key})
}
func (j Junk) GetMap(key string) (Junk, bool) {
	return j.FindMap([]string{key})
}

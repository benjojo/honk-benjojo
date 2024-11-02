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

package main

import (
	"bytes"
	"errors"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"humungus.tedunangst.com/r/webs/gate"
	"humungus.tedunangst.com/r/webs/image"
)

type Shrinker struct {
}

type ShrinkerArgs struct {
	Buf    []byte
	Params image.Params
}

type ShrinkerResult struct {
	Image *image.Image
}

var shrinkgate = gate.NewLimiter(4)

func (s *Shrinker) Shrink(args *ShrinkerArgs, res *ShrinkerResult) error {
	shrinkgate.Start()
	defer shrinkgate.Finish()
	img, err := image.Vacuum(bytes.NewReader(args.Buf), args.Params)
	if err != nil {
		return err
	}
	res.Image = img
	return nil
}

func backendSockname() string {
	return dataDir + "/backend.sock"
}

var bomFuck = []byte{0xef, 0xbb, 0xbf}

func isSVG(data []byte) bool {
	if bytes.HasPrefix(data, bomFuck) {
		data = data[3:]
	}
	ct := http.DetectContentType(data)
	if strings.HasPrefix(ct, "text/xml") || strings.HasPrefix(ct, "text/plain") {
		// this seems suboptimal
		prefixes := []string{
			`<svg `,
			`<!DOCTYPE svg PUBLIC`,
			`<?xml version="1.0" encoding="UTF-8"?> <svg `,
		}
		for _, pre := range prefixes {
			if bytes.HasPrefix(data, []byte(pre)) {
				return true
			}
		}
	}
	return ct == "image/svg+xml"
}

func imageFromSVG(data []byte) (*image.Image, error) {
	if bytes.HasPrefix(data, bomFuck) {
		data = data[3:]
	}
	if len(data) > 100000 {
		return nil, errors.New("my svg is too big")
	}
	svg := &image.Image{
		Data:   data,
		Format: "svg+xml",
	}
	return svg, nil
}

func callshrink(data []byte, params image.Params) (*image.Image, error) {
	if isSVG(data) {
		return imageFromSVG(data)
	}
	cl, err := rpc.Dial("unix", backendSockname())
	if err != nil {
		return nil, err
	}
	defer cl.Close()
	var res ShrinkerResult
	err = cl.Call("Shrinker.Shrink", &ShrinkerArgs{
		Buf:    data,
		Params: params,
	}, &res)
	if err != nil {
		return nil, err
	}
	return res.Image, nil
}

func lilshrink(data []byte) (*image.Image, error) {
	params := image.Params{
		LimitSize: 14200 * 4200,
		MaxWidth:  256,
		MaxHeight: 256,
		MaxSize:   16 * 1024,
	}
	return callshrink(data, params)
}
func bigshrink(data []byte) (*image.Image, error) {
	params := image.Params{
		LimitSize: 14200 * 4200,
		MaxWidth:  2600,
		MaxHeight: 2048,
		MaxSize:   768 * 1024,
	}
	return callshrink(data, params)
}

func shrinkit(data []byte) (*image.Image, error) {
	params := image.Params{
		LimitSize: 4200 * 4200,
		MaxWidth:  2048,
		MaxHeight: 2048,
	}
	return callshrink(data, params)
}

func orphancheck() {
	var b [1]byte
	os.Stdin.Read(b[:])
	dlog.Printf("backend shutting down")
	os.Exit(0)
}

func backendServer() {
	dlog.Printf("backend server running")
	closedatabases()
	go orphancheck()
	signal.Ignore(syscall.SIGINT)
	shrinker := new(Shrinker)
	srv := rpc.NewServer()
	err := srv.Register(shrinker)
	if err != nil {
		elog.Panicf("unable to register shrinker: %s", err)
	}

	sockname := backendSockname()
	err = os.Remove(sockname)
	if err != nil && !os.IsNotExist(err) {
		elog.Panicf("unable to unlink socket: %s", err)
	}

	lis, err := net.Listen("unix", sockname)
	if err != nil {
		elog.Panicf("unable to register shrinker: %s", err)
	}
	err = setLimits()
	if err != nil {
		elog.Printf("error setting backend limits: %s", err)
	}
	securitizebackend()
	srv.Accept(lis)
}

func runBackendServer() {
	r, w, err := os.Pipe()
	if err != nil {
		elog.Panicf("can't pipe: %s", err)
	}
	proc := exec.Command(os.Args[0], reexecArgs("backend")...)
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	proc.Stdin = r
	err = proc.Start()
	if err != nil {
		elog.Panicf("can't exec backend: %s", err)
	}
	workinprogress++
	var mtx sync.Mutex
	go func() {
		<-endoftheworld
		mtx.Lock()
		defer mtx.Unlock()
		w.Close()
		w = nil
		readyalready <- true
	}()
	go func() {
		err := proc.Wait()
		mtx.Lock()
		defer mtx.Unlock()
		if w != nil {
			elog.Printf("lost the backend: %s", err)
			w.Close()
		}
	}()
}

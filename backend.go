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
	"log"
	"net"
	"net/rpc"
	"os"
	"os/exec"

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

func (s *Shrinker) Shrink(args *ShrinkerArgs, res *ShrinkerResult) error {
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

func shrinkit(data []byte) (*image.Image, error) {
	cl, err := rpc.Dial("unix", backendSockname())
	if err != nil {
		return nil, err
	}
	defer cl.Close()
	var res ShrinkerResult
	err = cl.Call("Shrinker.Shrink", &ShrinkerArgs{
		Buf:    data,
		Params: image.Params{LimitSize: 4200 * 4200, MaxWidth: 2048, MaxHeight: 2048},
	}, &res)
	if err != nil {
		return nil, err
	}
	return res.Image, nil
}

func backendServer() {
	log.Printf("backend server running")
	shrinker := new(Shrinker)
	srv := rpc.NewServer()
	err := srv.Register(shrinker)
	if err != nil {
		log.Panicf("unable to register shrinker: %s", err)
	}

	sockname := backendSockname()
	err = os.Remove(sockname)
	if err != nil && !os.IsNotExist(err) {
		log.Panicf("unable to unlink socket: %s", err)
	}

	lis, err := net.Listen("unix", sockname)
	if err != nil {
		log.Panicf("unable to register shrinker: %s", err)
	}
	for _, h := range preservehooks {
		h()
	}
	srv.Accept(lis)
}

func startBackendServer() {
	proc := exec.Command(os.Args[0], "-datadir", dataDir, "backend")
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	err := proc.Start()
	if err != nil {
		log.Panicf("can't exec backend: %s", err)
	}
}

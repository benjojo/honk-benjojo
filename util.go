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

/*
#include <termios.h>

void
termecho(int on)
{
	struct termios t;
	tcgetattr(1, &t);
	if (on)
		t.c_lflag |= ECHO;
	else
		t.c_lflag &= ~ECHO;
	tcsetattr(1, TCSADRAIN, &t);
}
*/
import "C"

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"

	"golang.org/x/crypto/bcrypt"
	_ "humungus.tedunangst.com/r/go-sqlite3"
	"humungus.tedunangst.com/r/webs/httpsig"
)

var savedstyleparams = make(map[string]string)

func getstyleparam(file string) string {
	if p, ok := savedstyleparams[file]; ok {
		return p
	}
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return ""
	}
	hasher := sha512.New()
	hasher.Write(data)

	return fmt.Sprintf("?v=%.8x", hasher.Sum(nil))
}

var dbtimeformat = "2006-01-02 15:04:05"

var alreadyopendb *sql.DB
var dbname = "honk.db"
var stmtConfig *sql.Stmt
var myVersion = 13

func initdb() {
	schema, err := ioutil.ReadFile("schema.sql")
	if err != nil {
		log.Fatal(err)
	}
	_, err = os.Stat(dbname)
	if err == nil {
		log.Fatalf("%s already exists", dbname)
	}
	db, err := sql.Open("sqlite3", dbname)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		os.Remove(dbname)
		os.Exit(1)
	}()
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		C.termecho(1)
		fmt.Printf("\n")
		os.Remove(dbname)
		os.Exit(1)
	}()

	for _, line := range strings.Split(string(schema), ";") {
		_, err = db.Exec(line)
		if err != nil {
			log.Print(err)
			return
		}
	}
	defer db.Close()
	r := bufio.NewReader(os.Stdin)

	err = createuser(db, r)
	if err != nil {
		log.Print(err)
		return
	}

	fmt.Printf("listen address: ")
	addr, err := r.ReadString('\n')
	if err != nil {
		log.Print(err)
		return
	}
	addr = addr[:len(addr)-1]
	if len(addr) < 1 {
		log.Print("that's way too short")
		return
	}
	_, err = db.Exec("insert into config (key, value) values (?, ?)", "listenaddr", addr)
	if err != nil {
		log.Print(err)
		return
	}
	fmt.Printf("server name: ")
	addr, err = r.ReadString('\n')
	if err != nil {
		log.Print(err)
		return
	}
	addr = addr[:len(addr)-1]
	if len(addr) < 1 {
		log.Print("that's way too short")
		return
	}
	_, err = db.Exec("insert into config (key, value) values (?, ?)", "servername", addr)
	if err != nil {
		log.Print(err)
		return
	}
	var randbytes [16]byte
	rand.Read(randbytes[:])
	key := fmt.Sprintf("%x", randbytes)
	_, err = db.Exec("insert into config (key, value) values (?, ?)", "csrfkey", key)
	if err != nil {
		log.Print(err)
		return
	}
	_, err = db.Exec("insert into config (key, value) values (?, ?)", "dbversion", myVersion)
	if err != nil {
		log.Print(err)
		return
	}
	prepareStatements(db)
	db.Close()
	fmt.Printf("done.\n")
	os.Exit(0)
}

func adduser() {
	db := opendatabase()
	defer func() {
		os.Exit(1)
	}()
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		C.termecho(1)
		fmt.Printf("\n")
		os.Exit(1)
	}()

	r := bufio.NewReader(os.Stdin)

	err := createuser(db, r)
	if err != nil {
		log.Print(err)
		return
	}

	db.Close()
	os.Exit(0)
}

func createuser(db *sql.DB, r *bufio.Reader) error {
	fmt.Printf("username: ")
	name, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	name = name[:len(name)-1]
	if len(name) < 1 {
		return fmt.Errorf("that's way too short")
	}
	C.termecho(0)
	fmt.Printf("password: ")
	pass, err := r.ReadString('\n')
	C.termecho(1)
	fmt.Printf("\n")
	if err != nil {
		return err
	}
	pass = pass[:len(pass)-1]
	if len(pass) < 6 {
		return fmt.Errorf("that's way too short")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), 12)
	if err != nil {
		return err
	}
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	pubkey, err := httpsig.EncodeKey(&k.PublicKey)
	if err != nil {
		return err
	}
	seckey, err := httpsig.EncodeKey(k)
	if err != nil {
		return err
	}
	_, err = db.Exec("insert into users (username, displayname, about, hash, pubkey, seckey, options) values (?, ?, ?, ?, ?, ?, ?)", name, name, "what about me?", hash, pubkey, seckey, "")
	if err != nil {
		return err
	}
	return nil
}

func opendatabase() *sql.DB {
	if alreadyopendb != nil {
		return alreadyopendb
	}
	var err error
	_, err = os.Stat(dbname)
	if err != nil {
		log.Fatalf("unable to open database: %s", err)
	}
	db, err := sql.Open("sqlite3", dbname)
	if err != nil {
		log.Fatalf("unable to open database: %s", err)
	}
	stmtConfig, err = db.Prepare("select value from config where key = ?")
	if err != nil {
		log.Fatal(err)
	}
	alreadyopendb = db
	return db
}

func getconfig(key string, value interface{}) error {
	m, ok := value.(*map[string]bool)
	if ok {
		rows, err := stmtConfig.Query(key)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			err = rows.Scan(&s)
			if err != nil {
				return err
			}
			(*m)[s] = true
		}
		return nil
	}
	row := stmtConfig.QueryRow(key)
	err := row.Scan(value)
	if err == sql.ErrNoRows {
		err = nil
	}
	return err
}

func openListener() (net.Listener, error) {
	var listenAddr string
	err := getconfig("listenaddr", &listenAddr)
	if err != nil {
		return nil, err
	}
	if listenAddr == "" {
		return nil, fmt.Errorf("must have listenaddr")
	}
	proto := "tcp"
	if listenAddr[0] == '/' {
		proto = "unix"
		err := os.Remove(listenAddr)
		if err != nil && !os.IsNotExist(err) {
			log.Printf("unable to unlink socket: %s", err)
		}
	}
	listener, err := net.Listen(proto, listenAddr)
	if err != nil {
		return nil, err
	}
	if proto == "unix" {
		os.Chmod(listenAddr, 0777)
	}
	return listener, nil
}

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
	"humungus.tedunangst.com/r/webs/login"
)

var savedassetparams = make(map[string]string)

func getassetparam(file string) string {
	if p, ok := savedassetparams[file]; ok {
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
var stmtConfig *sql.Stmt

func initdb() {
	dbname := dataDir + "/honk.db"
	_, err := os.Stat(dbname)
	if err == nil {
		log.Fatalf("%s already exists", dbname)
	}
	db, err := sql.Open("sqlite3", dbname)
	if err != nil {
		log.Fatal(err)
	}
	alreadyopendb = db
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

	for _, line := range strings.Split(sqlSchema, ";") {
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
	err = createserveruser(db)
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
	setconfig("listenaddr", addr)
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
	setconfig("servername", addr)
	var randbytes [16]byte
	rand.Read(randbytes[:])
	key := fmt.Sprintf("%x", randbytes)
	setconfig("csrfkey", key)
	setconfig("dbversion", myVersion)

	setconfig("servermsg", "<h2>Things happen.</h2>")
	setconfig("aboutmsg", "<h3>What is honk?</h3>\n<p>Honk is amazing!")
	setconfig("loginmsg", "<h2>login</h2>")
	setconfig("debug", 0)

	initblobdb()

	prepareStatements(db)
	db.Close()
	fmt.Printf("done.\n")
	os.Exit(0)
}

func initblobdb() {
	blobdbname := dataDir + "/blob.db"
	_, err := os.Stat(blobdbname)
	if err == nil {
		log.Fatalf("%s already exists", blobdbname)
	}
	blobdb, err := sql.Open("sqlite3", blobdbname)
	if err != nil {
		log.Print(err)
		return
	}
	_, err = blobdb.Exec("create table filedata (xid text, media text, content blob)")
	if err != nil {
		log.Print(err)
		return
	}
	_, err = blobdb.Exec("create index idx_filexid on filedata(xid)")
	if err != nil {
		log.Print(err)
		return
	}
	blobdb.Close()
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

func chpass() {
	if len(os.Args) < 3 {
		fmt.Printf("need a username\n")
		os.Exit(1)
	}
	user, err := butwhatabout(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
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

	db := opendatabase()
	login.Init(db)

	r := bufio.NewReader(os.Stdin)

	pass, err := askpassword(r)
	if err != nil {
		log.Print(err)
		return
	}
	err = login.SetPassword(user.ID, pass)
	if err != nil {
		log.Print(err)
		return
	}
	fmt.Printf("done\n")
	os.Exit(0)
}

func askpassword(r *bufio.Reader) (string, error) {
	C.termecho(0)
	fmt.Printf("password: ")
	pass, err := r.ReadString('\n')
	C.termecho(1)
	fmt.Printf("\n")
	if err != nil {
		return "", err
	}
	pass = pass[:len(pass)-1]
	if len(pass) < 6 {
		return "", fmt.Errorf("that's way too short")
	}
	return pass, nil
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
	pass, err := askpassword(r)
	if err != nil {
		return err
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
	about := "what about me?"
	_, err = db.Exec("insert into users (username, displayname, about, hash, pubkey, seckey, options) values (?, ?, ?, ?, ?, ?, ?)", name, name, about, hash, pubkey, seckey, "{}")
	if err != nil {
		return err
	}
	return nil
}

func createserveruser(db *sql.DB) error {
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
	name := "server"
	about := "server"
	hash := "*"
	_, err = db.Exec("insert into users (userid, username, displayname, about, hash, pubkey, seckey, options) values (?, ?, ?, ?, ?, ?, ?, ?)", serverUID, name, name, about, hash, pubkey, seckey, "")
	if err != nil {
		return err
	}
	return nil
}

func opendatabase() *sql.DB {
	if alreadyopendb != nil {
		return alreadyopendb
	}
	dbname := dataDir + "/honk.db"
	_, err := os.Stat(dbname)
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

func openblobdb() *sql.DB {
	blobdbname := dataDir + "/blob.db"
	_, err := os.Stat(blobdbname)
	if err != nil {
		log.Fatalf("unable to open database: %s", err)
	}
	db, err := sql.Open("sqlite3", blobdbname)
	if err != nil {
		log.Fatalf("unable to open database: %s", err)
	}
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

func setconfig(key string, val interface{}) error {
	db := opendatabase()
	_, err := db.Exec("insert into config (key, value) values (?, ?)", key, val)
	return err
}

func updateconfig(key string, val interface{}) error {
	db := opendatabase()
	_, err := db.Exec("update config set value = ? where key = ?", val, key)
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

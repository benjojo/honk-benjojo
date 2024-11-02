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
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"humungus.tedunangst.com/r/go-sqlite3"
	"humungus.tedunangst.com/r/termvc"
	"humungus.tedunangst.com/r/webs/httpsig"
	"humungus.tedunangst.com/r/webs/login"
)

var re_plainname = regexp.MustCompile("^[[:alnum:]_-]+$")

var dbtimeformat = "2006-01-02 15:04:05"

var alreadyopendb *sql.DB
var stmtConfig *sql.Stmt

func init() {
	vers, num, _ := sqlite3.Version()
	if num < 3034000 {
		fmt.Fprintf(os.Stderr, "libsqlite is too old. required: %s found: %s\n",
			"3.34.0", vers)
		os.Exit(1)
	}
}

func initdb() {
	blobdbname := dataDir + "/blob.db"
	dbname := dataDir + "/honk.db"
	_, err := os.Stat(dbname)
	if err == nil {
		elog.Fatalf("%s already exists", dbname)
	}
	db, err := sql.Open("sqlite3", dbname)
	if err != nil {
		elog.Fatal(err)
	}
	alreadyopendb = db

	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		elog.Print(err)
		return
	}
	for _, line := range strings.Split(sqlSchema, ";") {
		_, err = db.Exec(line)
		if err != nil {
			elog.Print(err)
			return
		}
	}
	initblobdb(blobdbname)

	prepareStatements(db)

	setcsrfkey()
	setconfig("dbversion", myVersion)

	setconfig("servermsg", "<h2>Things happen.</h2>")
	setconfig("aboutmsg", "<h3>What is honk?</h3><p>Honk is amazing!")
	setconfig("loginmsg", "<h2>login</h2>")
	setconfig("devel", 0)

	cleanup := func() {
		os.Remove(dbname)
		os.Remove(blobdbname)
		os.Exit(1)
	}
	defer cleanup()

	if !termvc.IsTerm() {
		simplesetup(db)
		return
	}

	termvc.Start()
	defer termvc.Restore()
	go termvc.Catch(cleanup)

	app := termvc.NewApp()
	t1 := termvc.NewTextArea()
	t1.Value = "\n\n\tHello.\n\t\tWelcome to honk setup."
	var inputs []termvc.Element
	var offset int
	listenfield := termvc.NewTextInput("listen address", &offset)
	inputs = append(inputs, listenfield)
	serverfield := termvc.NewTextInput("server name", &offset)
	inputs = append(inputs, serverfield)
	namefield := termvc.NewTextInput("username", &offset)
	inputs = append(inputs, namefield)
	passfield := termvc.NewPasswordInput("password", &offset)
	inputs = append(inputs, passfield)
	okay := false
	btn := termvc.NewButton("let's go!")
	left := 25
	inputs = append(inputs, termvc.NewHPad(&left, btn, nil))
	form := termvc.NewForm(inputs...)
	t2 := termvc.NewTextArea()
	group := termvc.NewVStack(t1, form, t2)
	group.SetFocus(1)
	app.Element = group
	app.Screen = termvc.NewScreen()
	btn.Submit = func() {
		t2.Value = ""
		addr := listenfield.Value
		if len(addr) < 1 {
			t2.Value += "listen address is way too short.\n"
			elog.Print("that's way too short")
			return
		}
		setconfig("listenaddr", addr)

		addr = serverfield.Value
		if len(addr) < 1 {
			t2.Value += "server name is way too short.\n"
			return
		}
		setconfig("servername", addr)
		err = createuser(db, namefield.Value, passfield.Value)
		if err != nil {
			t2.Value += fmt.Sprintf("error: %s\n", err)
			return
		}
		// must came later or user above will have negative id
		err = createserveruser(db)
		if err != nil {
			elog.Print(err)
			return
		}
		okay = true
		app.Quit()
	}

	app.Loop()
	if !okay {
		return
	}

	db.Close()
	termvc.Restore()
	fmt.Printf("done.\n")
	os.Exit(0)
}

func simplesetup(db *sql.DB) {
	r := bufio.NewReader(os.Stdin)
	fmt.Printf("username: ")
	name, err := r.ReadString('\n')
	name = name[:len(name)-1]
	fmt.Printf("password: ")
	pass, err := r.ReadString('\n')
	pass = pass[:len(pass)-1]
	err = createuser(db, name, pass)
	if err != nil {
		elog.Print(err)
		return
	}
	// must came later or user above will have negative id
	err = createserveruser(db)
	if err != nil {
		elog.Print(err)
		return
	}

	fmt.Printf("listen address: ")
	addr, err := r.ReadString('\n')
	if err != nil {
		elog.Print(err)
		return
	}
	addr = addr[:len(addr)-1]
	if len(addr) < 1 {
		elog.Print("that's way too short")
		return
	}
	setconfig("listenaddr", addr)
	fmt.Printf("server name: ")
	addr, err = r.ReadString('\n')
	if err != nil {
		elog.Print(err)
		return
	}
	addr = addr[:len(addr)-1]
	if len(addr) < 1 {
		elog.Print("that's way too short")
		return
	}
	setconfig("servername", addr)

	db.Close()
	fmt.Printf("done.\n")
	os.Exit(0)
}

func setcsrfkey() {
	var randbytes [16]byte
	rand.Read(randbytes[:])
	key := fmt.Sprintf("%x", randbytes)
	setconfig("csrfkey", key)
}

func initblobdb(blobdbname string) {
	_, err := os.Stat(blobdbname)
	if err == nil {
		elog.Fatalf("%s already exists", blobdbname)
	}
	blobdb, err := sql.Open("sqlite3", blobdbname)
	if err != nil {
		elog.Print(err)
		return
	}
	_, err = blobdb.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		elog.Print(err)
		return
	}
	_, err = blobdb.Exec("create table filedata (xid text, content blob)")
	if err != nil {
		elog.Print(err)
		return
	}
	_, err = blobdb.Exec("create index idx_filexid on filedata(xid)")
	if err != nil {
		elog.Print(err)
		return
	}
	blobdb.Close()
}

func adduser() {
	termvc.Start()
	defer termvc.Restore()
	go termvc.Catch(nil)

	db := opendatabase()
	app := termvc.NewApp()
	t1 := termvc.NewTextArea()
	t1.Value = "\n\n\tHello.\n\t\tLet's invite a friend!"
	var inputs []termvc.Element
	var offset int
	namefield := termvc.NewTextInput("username", &offset)
	inputs = append(inputs, namefield)
	passfield := termvc.NewPasswordInput("password", &offset)
	inputs = append(inputs, passfield)
	btn := termvc.NewButton("let's go!")
	left := 25
	inputs = append(inputs, termvc.NewHPad(&left, btn, nil))
	form := termvc.NewForm(inputs...)
	t2 := termvc.NewTextArea()
	group := termvc.NewVStack(t1, form, t2)
	group.SetFocus(1)
	app.Element = group
	app.Screen = termvc.NewScreen()
	btn.Submit = func() {
		t2.Value = ""
		err := createuser(db, namefield.Value, passfield.Value)
		if err != nil {
			t2.Value += fmt.Sprintf("error: %s\n", err)
			return
		}
		app.Quit()
	}

	app.Loop()
}

func deluser(username string) {
	user, _ := getUserBio(username)
	if user == nil {
		elog.Printf("no userfound")
		return
	}
	userid := user.ID
	db := opendatabase()

	where := " where honkid in (select honkid from honks where userid = ?)"
	sqlMustQuery(db, "delete from donks"+where, userid)
	sqlMustQuery(db, "delete from onts"+where, userid)
	sqlMustQuery(db, "delete from honkmeta"+where, userid)
	where = " where chonkid in (select chonkid from chonks where userid = ?)"
	sqlMustQuery(db, "delete from donks"+where, userid)

	sqlMustQuery(db, "delete from honks where userid = ?", userid)
	sqlMustQuery(db, "delete from chonks where userid = ?", userid)
	sqlMustQuery(db, "delete from honkers where userid = ?", userid)
	sqlMustQuery(db, "delete from zonkers where userid = ?", userid)
	sqlMustQuery(db, "delete from doovers where userid = ?", userid)
	sqlMustQuery(db, "delete from hfcs where userid = ?", userid)
	sqlMustQuery(db, "delete from auth where userid = ?", userid)
	sqlMustQuery(db, "delete from users where userid = ?", userid)
}

func chpass(username string) {
	panic("todo")
	user, err := getUserBio(username)
	if err != nil {
		elog.Fatal(err)
	}
	db := opendatabase()
	login.Init(login.InitArgs{Db: db, Logger: ilog})

	pass := "password"
	err = login.SetPassword(int64(user.ID), pass)
	if err != nil {
		elog.Print(err)
		return
	}
}

func createuser(db *sql.DB, name, pass string) error {
	if len(name) < 1 {
		return fmt.Errorf("username is way too short")
	}
	if !re_plainname.MatchString(name) {
		return fmt.Errorf("alphanumeric only please")
	}
	if _, err := getUserBio(name); err == nil {
		return fmt.Errorf("user already exists")
	}
	if len(pass) < 6 {
		return fmt.Errorf("password is way too short")
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
	chatpubkey, chatseckey := newChatKeys()
	var opts UserOptions
	opts.ChatPubKey = tob64(chatpubkey.key[:])
	opts.ChatSecKey = tob64(chatseckey.key[:])
	jopt, _ := encodeJson(opts)
	about := "what about me?"
	_, err = db.Exec("insert into users (username, displayname, about, hash, pubkey, seckey, options) values (?, ?, ?, ?, ?, ?, ?)", name, name, about, hash, pubkey, seckey, jopt)
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
		elog.Fatalf("unable to open database: %s", err)
	}
	db, err := sql.Open("sqlite3", dbname)
	if err != nil {
		elog.Fatalf("unable to open database: %s", err)
	}
	stmtConfig, err = db.Prepare("select value from config where key = ?")
	if err != nil {
		elog.Fatal(err)
	}
	alreadyopendb = db
	return db
}

func openblobdb() *sql.DB {
	blobdbname := dataDir + "/blob.db"
	_, err := os.Stat(blobdbname)
	if err != nil {
		return nil
		//elog.Fatalf("unable to open database: %s", err)
	}
	db, err := sql.Open("sqlite3", blobdbname)
	if err != nil {
		elog.Fatalf("unable to open database: %s", err)
	}
	return db
}

func getConfigValue(key string, value interface{}) error {
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
	db.Exec("delete from config where key = ?", key)
	_, err := db.Exec("insert into config (key, value) values (?, ?)", key, val)
	return err
}

func getenv(key, def string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return def
}

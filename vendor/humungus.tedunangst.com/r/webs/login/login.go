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

// Simple cookie and password based logins.
// See Init for required schema.
package login

import (
	"context"
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"hash"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"humungus.tedunangst.com/r/webs/cache"
)

// represents a logged in user
type UserInfo struct {
	UserID   int64
	Username string
}

type keytype struct{}

var thekey keytype

var dbtimeformat = "2006-01-02 15:04:05"

// Check for auth cookie. Allows failure.
func Checker(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userinfo, ok := checkauthcookie(r)
		if ok {
			ctx := context.WithValue(r.Context(), thekey, userinfo)
			r = r.WithContext(ctx)
		}
		handler.ServeHTTP(w, r)
	})
}

// Check for auth cookie. On failure redirects to /login.
// Must already be wrapped in Checker.
func Required(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := GetUserInfo(r) != nil
		if !ok {
			loginredirect(w, r)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

// Check that the form value "token" is valid auth token
func TokenRequired(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userinfo, ok := checkformtoken(r)
		if ok {
			ctx := context.WithValue(r.Context(), thekey, userinfo)
			r = r.WithContext(ctx)
			handler.ServeHTTP(w, r)
		} else {
			http.Error(w, "valid token required", http.StatusForbidden)
		}
	})
}

// Get UserInfo for this request, if any.
func GetUserInfo(r *http.Request) *UserInfo {
	userinfo, ok := r.Context().Value(thekey).(*UserInfo)
	if !ok {
		return nil
	}
	return userinfo
}

func calculateCSRF(salt, action, auth string) string {
	hasher := sha512.New512_256()
	zero := []byte{0}
	hasher.Write(zero)
	hasher.Write([]byte(auth))
	hasher.Write(zero)
	hasher.Write([]byte(csrfkey))
	hasher.Write(zero)
	hasher.Write([]byte(salt))
	hasher.Write(zero)
	hasher.Write([]byte(action))
	hasher.Write(zero)
	hash := hexsum(hasher)

	return salt + hash
}

// Get a CSRF token for given action.
func GetCSRF(action string, r *http.Request) string {
	_, ok := checkauthcookie(r)
	if !ok {
		return ""
	}
	auth := getauthcookie(r)
	if auth == "" {
		return ""
	}
	hasher := sha512.New512_256()
	io.CopyN(hasher, rand.Reader, 32)
	salt := hexsum(hasher)

	return calculateCSRF(salt, action, auth)
}

// Checks that CSRF value is correct.
func CheckCSRF(action string, r *http.Request) bool {
	auth := getauthcookie(r)
	if auth == "" {
		return false
	}
	csrf := r.FormValue("CSRF")
	if len(csrf) != authlen*2 {
		return false
	}
	salt := csrf[0:authlen]
	rv := calculateCSRF(salt, action, auth)
	ok := subtle.ConstantTimeCompare([]byte(rv), []byte(csrf)) == 1
	return ok
}

// Wrap a handler with CSRF checking.
func CSRFWrap(action string, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := CheckCSRF(action, r)
		if !ok {
			http.Error(w, "invalid csrf", 403)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func CSRFWrapFunc(action string, fn http.HandlerFunc) http.Handler {
	return CSRFWrap(action, fn)
}

func loginredirect(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    "",
		MaxAge:   -1,
		Secure:   securecookies,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

var authregex = regexp.MustCompile("^[[:alnum:]]+$")
var authlen = 32

var stmtUserName, stmtUserAuth, stmtUpdateUser, stmtSaveAuth, stmtDeleteAuth *sql.Stmt
var stmtDeleteOneAuth *sql.Stmt
var csrfkey string
var securecookies bool

func getconfig(db *sql.DB, key string, value interface{}) error {
	row := db.QueryRow("select value from config where key = ?", key)
	err := row.Scan(value)
	if err == sql.ErrNoRows {
		err = nil
	}
	return err
}

// Init. Must be called with the database.
// Requires a users table with (userid, username, hash) columns and a
// auth table with (userid, hash, expiry) columns.
// Requires a config table with (key, value) ('csrfkey', some secret).
func Init(db *sql.DB) {
	var err error
	stmtUserName, err = db.Prepare("select userid, hash from users where username = ? and userid > 0")
	if err != nil {
		log.Panic(err)
	}
	stmtUserAuth, err = db.Prepare("select userid, username from users where userid = (select userid from auth where hash = ? and expiry > ?)")
	if err != nil {
		log.Panic(err)
	}
	stmtUpdateUser, err = db.Prepare("update users set hash = ? where userid = ?")
	if err != nil {
		log.Panic(err)
	}
	stmtSaveAuth, err = db.Prepare("insert into auth (userid, hash, expiry) values (?, ?, ?)")
	if err != nil {
		log.Panic(err)
	}
	stmtDeleteAuth, err = db.Prepare("delete from auth where userid = ?")
	if err != nil {
		log.Panic(err)
	}
	stmtDeleteOneAuth, err = db.Prepare("delete from auth where hash = ?")
	if err != nil {
		log.Panic(err)
	}
	debug := false
	getconfig(db, "debug", &debug)
	securecookies = !debug
	getconfig(db, "csrfkey", &csrfkey)
}

var authinprogress = make(map[string]bool)
var authprogressmtx sync.Mutex

func rateandwait(username string) bool {
	authprogressmtx.Lock()
	defer authprogressmtx.Unlock()
	if authinprogress[username] {
		return false
	}
	authinprogress[username] = true
	go func(name string) {
		time.Sleep(1 * time.Second / 2)
		authprogressmtx.Lock()
		authinprogress[name] = false
		authprogressmtx.Unlock()
	}(username)
	return true
}

func getauthcookie(r *http.Request) string {
	cookie, err := r.Cookie("auth")
	if err != nil {
		return ""
	}
	auth := cookie.Value
	if !(len(auth) == authlen && authregex.MatchString(auth)) {
		log.Printf("login: bad auth: %s", auth)
		return ""
	}
	return auth
}

func getformtoken(r *http.Request) string {
	token := r.FormValue("token")
	if token == "" {
		token = r.Header.Get("Authorization")
	}
	if strings.HasPrefix(token, "Bearer ") {
		token = token[7:]
	}
	if token == "" {
		return ""
	}
	if !(len(token) == authlen && authregex.MatchString(token)) {
		log.Printf("login: bad token: %s", token)
		return ""
	}
	return token
}

var validcookies = cache.New(cache.Options{Filler: func(cookie string) (*UserInfo, bool) {
	hasher := sha512.New512_256()
	hasher.Write([]byte(cookie))
	authhash := hexsum(hasher)
	now := time.Now().UTC().Format(dbtimeformat)
	row := stmtUserAuth.QueryRow(authhash, now)
	var userinfo UserInfo
	err := row.Scan(&userinfo.UserID, &userinfo.Username)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("login: no auth found")
		} else {
			log.Printf("login: error scanning auth row: %s", err)
		}
		return nil, false
	}
	return &userinfo, true
}, Duration: 5 * time.Minute})

func checkauthcookie(r *http.Request) (*UserInfo, bool) {
	cookie := getauthcookie(r)
	if cookie == "" {
		return nil, false
	}
	var userinfo *UserInfo
	ok := validcookies.Get(cookie, &userinfo)
	return userinfo, ok
}

func checkformtoken(r *http.Request) (*UserInfo, bool) {
	token := getformtoken(r)
	if token == "" {
		return nil, false
	}
	var userinfo *UserInfo
	ok := validcookies.Get(token, &userinfo)
	return userinfo, ok
}

func loaduser(username string) (int64, []byte, bool) {
	row := stmtUserName.QueryRow(username)
	var userid int64
	var hash []byte
	err := row.Scan(&userid, &hash)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("login: no username found")
		} else {
			log.Printf("login: error loading username: %s", err)
		}
		return -1, nil, false
	}
	return userid, hash, true
}

var userregex = regexp.MustCompile("^[[:alnum:]]+$")
var userlen = 32
var passlen = 128

func hexsum(h hash.Hash) string {
	return fmt.Sprintf("%x", h.Sum(nil))[0:authlen]
}

// Default handler for /dologin
// Requires username and password form values.
// Redirects to / on success and /login on failure.
func LoginFunc(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")
	gettoken := r.FormValue("gettoken") == "1"

	if len(username) == 0 || len(username) > userlen ||
		!userregex.MatchString(username) || len(password) == 0 ||
		len(password) > passlen {
		log.Printf("login: invalid password attempt")
		if gettoken {
			http.Error(w, "incorrect", http.StatusForbidden)
		} else {
			loginredirect(w, r)
		}
		return
	}
	userid, hash, ok := loaduser(username)
	if !ok {
		loginredirect(w, r)
		return
	}

	if !rateandwait(username) {
		if gettoken {
			http.Error(w, "incorrect", http.StatusForbidden)
		} else {
			loginredirect(w, r)
		}
		return
	}

	err := bcrypt.CompareHashAndPassword(hash, []byte(password))
	if err != nil {
		log.Printf("login: incorrect password")
		if gettoken {
			http.Error(w, "incorrect", http.StatusForbidden)
		} else {
			loginredirect(w, r)
		}
		return
	}
	hasher := sha512.New512_256()
	io.CopyN(hasher, rand.Reader, 32)
	auth := hexsum(hasher)

	maxage := 3600 * 24 * 30

	if gettoken {
		maxage = 3600 * 24 * 30 * 12
	} else {

		http.SetCookie(w, &http.Cookie{
			Name:     "auth",
			Value:    auth,
			MaxAge:   maxage,
			Secure:   securecookies,
			HttpOnly: true,
		})
	}

	hasher.Reset()
	hasher.Write([]byte(auth))
	authhash := hexsum(hasher)

	expiry := time.Now().UTC().Add(time.Duration(maxage) * time.Second).Format(dbtimeformat)
	_, err = stmtSaveAuth.Exec(userid, authhash, expiry)
	if err != nil {
		log.Printf("error saving auth: %s", err)
	}

	log.Printf("login: successful login")
	if gettoken {
		w.Write([]byte(auth))
	} else {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func deleteauth(userid int64) error {
	defer validcookies.Flush()
	_, err := stmtDeleteAuth.Exec(userid)
	return err
}

func deleteoneauth(auth string) error {
	defer validcookies.Flush()
	hasher := sha512.New512_256()
	hasher.Write([]byte(auth))
	authhash := hexsum(hasher)
	_, err := stmtDeleteOneAuth.Exec(authhash)
	return err
}

// Handler for /dologout route.
func LogoutFunc(w http.ResponseWriter, r *http.Request) {
	userinfo, ok := checkauthcookie(r)
	if ok && CheckCSRF("logout", r) {
		err := deleteauth(userinfo.UserID)
		if err != nil {
			log.Printf("login: error deleting old auth: %s", err)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "auth",
			Value:    "",
			MaxAge:   -1,
			Secure:   securecookies,
			HttpOnly: true,
		})
	}
	_, ok = checkformtoken(r)
	if ok {
		auth := getformtoken(r)
		deleteoneauth(auth)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Change password helper.
// Requires oldpass and newpass form values.
// Requires logout csrf token.
func ChangePassword(w http.ResponseWriter, r *http.Request) error {
	userinfo, ok := checkauthcookie(r)
	if !ok || !CheckCSRF("logout", r) {
		return fmt.Errorf("unauthorized")
	}

	oldpass := r.FormValue("oldpass")
	newpass := r.FormValue("newpass")

	if len(oldpass) == 0 || len(oldpass) > passlen ||
		len(newpass) == 0 || len(newpass) > passlen {
		log.Printf("login: invalid password attempt")
		return fmt.Errorf("bad password")
	}
	if len(newpass) < 6 {
		return fmt.Errorf("newpassword is too short")
	}
	userid, hash, ok := loaduser(userinfo.Username)
	if !ok {
		return fmt.Errorf("error")
	}

	err := bcrypt.CompareHashAndPassword(hash, []byte(oldpass))
	if err != nil {
		log.Printf("login: incorrect password")
		return fmt.Errorf("bad password")
	}
	hash, err = bcrypt.GenerateFromPassword([]byte(newpass), 12)
	if err != nil {
		log.Printf("error generating hash: %s", err)
		return fmt.Errorf("error")
	}
	_, err = stmtUpdateUser.Exec(hash, userinfo.UserID)
	if err != nil {
		log.Printf("login: error updating user: %s", err)
		return fmt.Errorf("error")
	}

	err = deleteauth(userid)
	if err != nil {
		log.Printf("login: error deleting old auth: %s", err)
		return fmt.Errorf("error")
	}

	hasher := sha512.New512_256()
	io.CopyN(hasher, rand.Reader, 32)
	auth := hexsum(hasher)

	maxage := 3600 * 24 * 30

	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    auth,
		MaxAge:   maxage,
		Secure:   securecookies,
		HttpOnly: true,
	})

	hasher.Reset()
	hasher.Write([]byte(auth))
	authhash := hexsum(hasher)

	expiry := time.Now().UTC().Add(time.Duration(maxage) * time.Second).Format(dbtimeformat)
	_, err = stmtSaveAuth.Exec(userid, authhash, expiry)
	if err != nil {
		log.Printf("error saving auth: %s", err)
	}

	return nil
}

// Set password for a user.
func SetPassword(userid int64, newpass string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newpass), 12)
	if err != nil {
		log.Printf("error generating hash: %s", err)
		return fmt.Errorf("error")
	}
	_, err = stmtUpdateUser.Exec(hash, userid)
	if err != nil {
		log.Printf("login: error updating user: %s", err)
		return fmt.Errorf("error")
	}

	err = deleteauth(userid)
	if err != nil {
		log.Printf("login: error deleting old auth: %s", err)
		return fmt.Errorf("error")
	}
	return nil
}

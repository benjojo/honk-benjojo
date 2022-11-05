// Copyright (C) 2014 Yasuhiro Matsumoto <mattn.jp@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package sqlite3

/*
#include <sqlite3.h>
#include <stdio.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#ifdef __CYGWIN__
# include <errno.h>
#endif

static int
_sqlite3_open_v2(const char *filename, sqlite3 **ppDb, int flags, const char *zVfs) {
#ifdef SQLITE_OPEN_URI
  return sqlite3_open_v2(filename, ppDb, flags | SQLITE_OPEN_URI, zVfs);
#else
  return sqlite3_open_v2(filename, ppDb, flags, zVfs);
#endif
}

static int
_sqlite3_bind_text(sqlite3_stmt *stmt, int n, _GoString_ s) {
  const char *p = _GoStringPtr(s);
  if (p == NULL)
    p = "";
  return sqlite3_bind_text(stmt, n, p, _GoStringLen(s), SQLITE_TRANSIENT);
}

static int
_sqlite3_bind_blob(sqlite3_stmt *stmt, int n, const void *p, int np) {
  if (p == NULL)
    p = "";
  return sqlite3_bind_blob(stmt, n, p, np, SQLITE_TRANSIENT);
}

static int
_sqlite3_exec(sqlite3* db, const char* pcmd, long long* rowid, long long* changes)
{
  int rv = sqlite3_exec(db, pcmd, 0, 0, 0);
  *rowid = (long long) sqlite3_last_insert_rowid(db);
  *changes = (long long) sqlite3_changes(db);
  return rv;
}

static int
_sqlite3_step(sqlite3_stmt* stmt, long long* rowid, long long* changes)
{
  int rv = sqlite3_step(stmt);
  sqlite3* db = sqlite3_db_handle(stmt);
  *rowid = (long long) sqlite3_last_insert_rowid(db);
  *changes = (long long) sqlite3_changes(db);
  return rv;
}

void _sqlite3_result_text(sqlite3_context* ctx, const char* s) {
  sqlite3_result_text(ctx, s, -1, &free);
}

void _sqlite3_result_blob(sqlite3_context* ctx, const void* b, int l) {
  sqlite3_result_blob(ctx, b, l, SQLITE_TRANSIENT);
}


int _sqlite3_create_function(
  sqlite3 *db,
  const char *zFunctionName,
  int nArg,
  int eTextRep,
  uintptr_t pApp,
  void (*xFunc)(sqlite3_context*,int,sqlite3_value**),
  void (*xStep)(sqlite3_context*,int,sqlite3_value**),
  void (*xFinal)(sqlite3_context*)
) {
  return sqlite3_create_function(db, zFunctionName, nArg, eTextRep, (void*) pApp, xFunc, xStep, xFinal);
}

*/
import "C"
import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// SQLiteTimestampFormats is timestamp formats understood by both this module
// and SQLite.  The first format in the slice will be used when saving time
// values into the database. When parsing a string from a timestamp or datetime
// column, the formats are tried in order.
var SQLiteTimestampFormats = []string{
	// By default, store timestamps with whatever timezone they come with.
	// When parsed, they will be returned with the same timezone.
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
}

func init() {
	sql.Register("sqlite3", &SQLiteDriver{})
}

// Version returns SQLite library version information.
func Version() (libVersion string, libVersionNumber int, sourceID string) {
	libVersion = C.GoString(C.sqlite3_libversion())
	libVersionNumber = int(C.sqlite3_libversion_number())
	sourceID = C.GoString(C.sqlite3_sourceid())
	return libVersion, libVersionNumber, sourceID
}

// SQLiteDriver implement sql.Driver.
type SQLiteDriver struct {
	Extensions  []string
	ConnectHook func(*SQLiteConn) error
}

// SQLiteConn implement sql.Conn.
type SQLiteConn struct {
	db          *C.sqlite3
	loc         *time.Location
	txlock      string
}

// SQLiteTx implemen sql.Tx.
type SQLiteTx struct {
	c *SQLiteConn
}

// SQLiteStmt implement sql.Stmt.
type SQLiteStmt struct {
	c      *SQLiteConn
	s      *C.sqlite3_stmt
	t      string
	closed bool
	cls    bool
}

// SQLiteResult implement sql.Result.
type SQLiteResult struct {
	id      int64
	changes int64
}

// SQLiteRows implement sql.Rows.
type SQLiteRows struct {
	s        *SQLiteStmt
	nc       int
	cols     []string
	decltype []string
	cls      bool
	done     chan chan struct{}
}


// Commit transaction.
func (tx *SQLiteTx) Commit() error {
	_, err := tx.c.exec(context.Background(), "COMMIT", nil)
	if err != nil && err.(Error).Code == C.SQLITE_BUSY {
		// sqlite3 will leave the transaction open in this scenario.
		// However, database/sql considers the transaction complete once we
		// return from Commit() - we must clean up to honour its semantics.
		tx.c.exec(context.Background(), "ROLLBACK", nil)
	}
	return err
}

// Rollback transaction.
func (tx *SQLiteTx) Rollback() error {
	_, err := tx.c.exec(context.Background(), "ROLLBACK", nil)
	return err
}

// AutoCommit return which currently auto commit or not.
func (c *SQLiteConn) AutoCommit() bool {
	return int(C.sqlite3_get_autocommit(c.db)) != 0
}

func (c *SQLiteConn) lastError() error {
	rv := C.sqlite3_errcode(c.db)
	if rv == C.SQLITE_OK {
		return nil
	}
	return Error{
		Code:         ErrNo(rv),
		ExtendedCode: ErrNoExtended(C.sqlite3_extended_errcode(c.db)),
		err:          C.GoString(C.sqlite3_errmsg(c.db)),
	}
}

// Exec implements Execer.
func (c *SQLiteConn) Exec(query string, args []driver.Value) (driver.Result, error) {
	list := make([]namedValue, len(args))
	for i, v := range args {
		list[i] = namedValue{
			Ordinal: i + 1,
			Value:   v,
		}
	}
	return c.exec(context.Background(), query, list)
}

func (c *SQLiteConn) exec(ctx context.Context, query string, args []namedValue) (driver.Result, error) {
	start := 0
	for {
		s, err := c.prepare(ctx, query)
		if err != nil {
			return nil, err
		}
		var res driver.Result
		if s.(*SQLiteStmt).s != nil {
			na := s.NumInput()
			if len(args) < na {
				return nil, fmt.Errorf("Not enough args to execute query. Expected %d, got %d.", na, len(args))
			}
			for i := 0; i < na; i++ {
				args[i].Ordinal -= start
			}
			res, err = s.(*SQLiteStmt).exec(ctx, args[:na])
			if err != nil && err != driver.ErrSkip {
				s.Close()
				return nil, err
			}
			args = args[na:]
			start += na
		}
		tail := s.(*SQLiteStmt).t
		s.Close()
		if tail == "" {
			return res, nil
		}
		query = tail
	}
}

type namedValue struct {
	Name    string
	Ordinal int
	Value   driver.Value
}

// Query implements Queryer.
func (c *SQLiteConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	list := make([]namedValue, len(args))
	for i, v := range args {
		list[i] = namedValue{
			Ordinal: i + 1,
			Value:   v,
		}
	}
	return c.query(context.Background(), query, list)
}

func (c *SQLiteConn) query(ctx context.Context, query string, args []namedValue) (driver.Rows, error) {
	start := 0
	for {
		s, err := c.prepare(ctx, query)
		if err != nil {
			return nil, err
		}
		s.(*SQLiteStmt).cls = true
		na := s.NumInput()
		if len(args) < na {
			return nil, fmt.Errorf("Not enough args to execute query. Expected %d, got %d.", na, len(args))
		}
		for i := 0; i < na; i++ {
			args[i].Ordinal -= start
		}
		rows, err := s.(*SQLiteStmt).query(ctx, args[:na])
		if err != nil && err != driver.ErrSkip {
			s.Close()
			return rows, err
		}
		args = args[na:]
		start += na
		tail := s.(*SQLiteStmt).t
		if tail == "" {
			return rows, nil
		}
		rows.Close()
		s.Close()
		query = tail
	}
}

// Begin transaction.
func (c *SQLiteConn) Begin() (driver.Tx, error) {
	return c.begin(context.Background())
}

func (c *SQLiteConn) begin(ctx context.Context) (driver.Tx, error) {
	if _, err := c.exec(ctx, c.txlock, nil); err != nil {
		return nil, err
	}
	return &SQLiteTx{c}, nil
}

func errorString(err Error) string {
	return C.GoString(C.sqlite3_errstr(C.int(err.Code)))
}

// Open database and return a new connection.
// You can specify a DSN string using a URI as the filename.
//   test.db
//   file:test.db?cache=shared&mode=memory
//   :memory:
//   file::memory:
// go-sqlite3 adds the following query parameters to those used by SQLite:
//   _loc=XXX
//     Specify location of time format. It's possible to specify "auto".
//   _busy_timeout=XXX
//     Specify value for sqlite3_busy_timeout.
//   _txlock=XXX
//     Specify locking behavior for transactions.  XXX can be "immediate",
//     "deferred", "exclusive".
func (d *SQLiteDriver) Open(dsn string) (driver.Conn, error) {
	if C.sqlite3_threadsafe() == 0 {
		return nil, errors.New("sqlite library was not compiled for thread-safe operation")
	}

	var loc *time.Location
	txlock := "BEGIN"
	busyTimeout := 5000
	pos := strings.IndexRune(dsn, '?')
	if pos >= 1 {
		params, err := url.ParseQuery(dsn[pos+1:])
		if err != nil {
			return nil, err
		}

		// _loc
		if val := params.Get("_loc"); val != "" {
			if val == "auto" {
				loc = time.Local
			} else {
				loc, err = time.LoadLocation(val)
				if err != nil {
					return nil, fmt.Errorf("Invalid _loc: %v: %v", val, err)
				}
			}
		}

		// _busy_timeout
		if val := params.Get("_busy_timeout"); val != "" {
			iv, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("Invalid _busy_timeout: %v: %v", val, err)
			}
			busyTimeout = int(iv)
		}

		// _txlock
		if val := params.Get("_txlock"); val != "" {
			switch val {
			case "immediate":
				txlock = "BEGIN IMMEDIATE"
			case "exclusive":
				txlock = "BEGIN EXCLUSIVE"
			case "deferred":
				txlock = "BEGIN"
			default:
				return nil, fmt.Errorf("Invalid _txlock: %v", val)
			}
		}

		if !strings.HasPrefix(dsn, "file:") {
			dsn = dsn[:pos]
		}
	}

	var db *C.sqlite3
	name := C.CString(dsn)
	defer C.free(unsafe.Pointer(name))
	rv := C._sqlite3_open_v2(name, &db,
		C.SQLITE_OPEN_FULLMUTEX|
			C.SQLITE_OPEN_READWRITE|
			C.SQLITE_OPEN_CREATE,
		nil)
	if rv != 0 {
		return nil, Error{Code: ErrNo(rv)}
	}
	if db == nil {
		return nil, errors.New("sqlite succeeded without returning a database")
	}

	rv = C.sqlite3_busy_timeout(db, C.int(busyTimeout))
	if rv != C.SQLITE_OK {
		return nil, Error{Code: ErrNo(rv)}
	}

	conn := &SQLiteConn{db: db, loc: loc, txlock: txlock}

	if d.ConnectHook != nil {
		if err := d.ConnectHook(conn); err != nil {
			return nil, err
		}
	}
	runtime.SetFinalizer(conn, (*SQLiteConn).Close)
	return conn, nil
}

// Close the connection.
func (c *SQLiteConn) Close() error {
	rv := C.sqlite3_close_v2(c.db)
	if rv != C.SQLITE_OK {
		return c.lastError()
	}
	c.db = nil
	runtime.SetFinalizer(c, nil)
	return nil
}

// Prepare the query string. Return a new statement.
func (c *SQLiteConn) Prepare(query string) (driver.Stmt, error) {
	return c.prepare(context.Background(), query)
}

func (c *SQLiteConn) prepare(ctx context.Context, query string) (driver.Stmt, error) {
	pquery := C.CString(query)
	defer C.free(unsafe.Pointer(pquery))
	var s *C.sqlite3_stmt
	var tail *C.char
	rv := C.sqlite3_prepare_v2(c.db, pquery, -1, &s, &tail)
	if rv != C.SQLITE_OK {
		return nil, c.lastError()
	}
	var t string
	if tail != nil && *tail != '\000' {
		t = strings.TrimSpace(C.GoString(tail))
	}
	ss := &SQLiteStmt{c: c, s: s, t: t}
	runtime.SetFinalizer(ss, (*SQLiteStmt).Close)
	return ss, nil
}

// Close the statement.
func (s *SQLiteStmt) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if s.c == nil || s.c.db == nil {
		return errors.New("sqlite statement with already closed database connection")
	}
	rv := C.sqlite3_finalize(s.s)
	if rv != C.SQLITE_OK {
		return s.c.lastError()
	}
	runtime.SetFinalizer(s, nil)
	return nil
}

// NumInput return a number of parameters.
func (s *SQLiteStmt) NumInput() int {
	return int(C.sqlite3_bind_parameter_count(s.s))
}

type bindArg struct {
	n int
	v driver.Value
}

func (s *SQLiteStmt) bind(args []namedValue) error {
	rv := C.sqlite3_reset(s.s)
	if rv != C.SQLITE_ROW && rv != C.SQLITE_OK && rv != C.SQLITE_DONE {
		return s.c.lastError()
	}

	for i, v := range args {
		if v.Name != "" {
			cname := C.CString(":" + v.Name)
			args[i].Ordinal = int(C.sqlite3_bind_parameter_index(s.s, cname))
			C.free(unsafe.Pointer(cname))
		}
	}

	for _, arg := range args {
		n := C.int(arg.Ordinal)
		switch v := arg.Value.(type) {
		case nil:
			rv = C.sqlite3_bind_null(s.s, n)
		case string:
			rv = C._sqlite3_bind_text(s.s, n, v)
		case int64:
			rv = C.sqlite3_bind_int64(s.s, n, C.sqlite3_int64(v))
		case bool:
			if bool(v) {
				rv = C.sqlite3_bind_int(s.s, n, 1)
			} else {
				rv = C.sqlite3_bind_int(s.s, n, 0)
			}
		case float64:
			rv = C.sqlite3_bind_double(s.s, n, C.double(v))
		case []byte:
			if v == nil {
				rv = C.sqlite3_bind_null(s.s, n)
			} else {
				ln := len(v)
				var p unsafe.Pointer
				if ln > 0 {
					p = unsafe.Pointer(&v[0])
				}
				rv = C._sqlite3_bind_blob(s.s, n, p, C.int(ln))
			}
		case time.Time:
			ts := v.Format(SQLiteTimestampFormats[0])
			rv = C._sqlite3_bind_text(s.s, n, ts)
		}
		if rv != C.SQLITE_OK {
			return s.c.lastError()
		}
	}
	return nil
}

// Query the statement with arguments. Return records.
func (s *SQLiteStmt) Query(args []driver.Value) (driver.Rows, error) {
	list := make([]namedValue, len(args))
	for i, v := range args {
		list[i] = namedValue{
			Ordinal: i + 1,
			Value:   v,
		}
	}
	return s.query(context.Background(), list)
}

func interrupt(ctx context.Context, db *C.sqlite3, done chan chan struct{}) {
	var back chan struct{}
	select {
	case <-ctx.Done():
		C.sqlite3_interrupt(db)
		back = <-done
	case back = <-done:
	}
	close(back)
}

func (s *SQLiteStmt) query(ctx context.Context, args []namedValue) (driver.Rows, error) {
	if err := s.bind(args); err != nil {
		return nil, err
	}

	rows := &SQLiteRows{
		s:        s,
		nc:       int(C.sqlite3_column_count(s.s)),
		cols:     nil,
		decltype: nil,
		cls:      s.cls,
	}

	if ctx.Done() != nil {
		rows.done = make(chan chan struct{})
		go interrupt(ctx, rows.s.c.db, rows.done)
	}

	return rows, nil
}

// LastInsertId teturn last inserted ID.
func (r *SQLiteResult) LastInsertId() (int64, error) {
	return r.id, nil
}

// RowsAffected return how many rows affected.
func (r *SQLiteResult) RowsAffected() (int64, error) {
	return r.changes, nil
}

// Exec execute the statement with arguments. Return result object.
func (s *SQLiteStmt) Exec(args []driver.Value) (driver.Result, error) {
	list := make([]namedValue, len(args))
	for i, v := range args {
		list[i] = namedValue{
			Ordinal: i + 1,
			Value:   v,
		}
	}
	return s.exec(context.Background(), list)
}

func (s *SQLiteStmt) exec(ctx context.Context, args []namedValue) (driver.Result, error) {
	if err := s.bind(args); err != nil {
		C.sqlite3_reset(s.s)
		C.sqlite3_clear_bindings(s.s)
		return nil, err
	}

	if ctx.Done() != nil {
		done := make(chan chan struct{})
		back := make(chan struct{})
		defer func() {
			done<-back
			close(done)
			select {
			case <-back:
			}
		}()
		go interrupt(ctx, s.c.db, done)
	}

	var rowid, changes C.longlong
	rv := C._sqlite3_step(s.s, &rowid, &changes)
	if rv != C.SQLITE_ROW && rv != C.SQLITE_OK && rv != C.SQLITE_DONE {
		err := s.c.lastError()
		C.sqlite3_reset(s.s)
		C.sqlite3_clear_bindings(s.s)
		return nil, err
	}

	return &SQLiteResult{id: int64(rowid), changes: int64(changes)}, nil
}

// Close the rows.
func (rc *SQLiteRows) Close() error {
	if rc.s.closed {
		return nil
	}
	if rc.done != nil {
		back := make(chan struct{})
		rc.done<-back
		close(rc.done)
		select {
		case <-back:
		}
		rc.done = nil
	}
	if rc.cls {
		return rc.s.Close()
	}
	rv := C.sqlite3_reset(rc.s.s)
	if rv != C.SQLITE_OK {
		return rc.s.c.lastError()
	}
	return nil
}

// Columns return column names.
func (rc *SQLiteRows) Columns() []string {
	if rc.nc != len(rc.cols) {
		rc.cols = make([]string, rc.nc)
		for i := 0; i < rc.nc; i++ {
			rc.cols[i] = C.GoString(C.sqlite3_column_name(rc.s.s, C.int(i)))
		}
	}
	return rc.cols
}

// DeclTypes return column types.
func (rc *SQLiteRows) DeclTypes() []string {
	if rc.decltype == nil {
		rc.decltype = make([]string, rc.nc)
		for i := 0; i < rc.nc; i++ {
			rc.decltype[i] = strings.ToLower(C.GoString(C.sqlite3_column_decltype(rc.s.s, C.int(i))))
		}
	}
	return rc.decltype
}

// Next move cursor to next.
func (rc *SQLiteRows) Next(dest []driver.Value) error {
	rv := C.sqlite3_step(rc.s.s)
	if rv == C.SQLITE_DONE {
		return io.EOF
	}
	if rv != C.SQLITE_ROW {
		rv = C.sqlite3_reset(rc.s.s)
		if rv != C.SQLITE_OK {
			return rc.s.c.lastError()
		}
		return nil
	}

	rc.DeclTypes()

	for i := range dest {
		switch C.sqlite3_column_type(rc.s.s, C.int(i)) {
		case C.SQLITE_INTEGER:
			val := int64(C.sqlite3_column_int64(rc.s.s, C.int(i)))
			switch rc.decltype[i] {
			case "timestamp", "datetime", "date":
				var t time.Time
				// Assume a millisecond unix timestamp if it's 13 digits -- too
				// large to be a reasonable timestamp in seconds.
				if val > 1e12 || val < -1e12 {
					val *= int64(time.Millisecond) // convert ms to nsec
				} else {
					val *= int64(time.Second) // convert sec to nsec
				}
				t = time.Unix(0, val).UTC()
				if rc.s.c.loc != nil {
					t = t.In(rc.s.c.loc)
				}
				dest[i] = t
			case "boolean":
				dest[i] = val > 0
			default:
				dest[i] = val
			}
		case C.SQLITE_FLOAT:
			dest[i] = float64(C.sqlite3_column_double(rc.s.s, C.int(i)))
		case C.SQLITE_BLOB:
			p := C.sqlite3_column_blob(rc.s.s, C.int(i))
			if p == nil {
				dest[i] = []byte{}
				continue
			}
			n := C.sqlite3_column_bytes(rc.s.s, C.int(i))
			dest[i] = C.GoBytes(p, n)
		case C.SQLITE_NULL:
			dest[i] = nil
		case C.SQLITE_TEXT:
			var err error
			var timeVal time.Time

			n := int(C.sqlite3_column_bytes(rc.s.s, C.int(i)))
			s := C.GoStringN((*C.char)(unsafe.Pointer(C.sqlite3_column_text(rc.s.s, C.int(i)))), C.int(n))

			switch rc.decltype[i] {
			case "timestamp", "datetime", "date":
				var t time.Time
				s = strings.TrimSuffix(s, "Z")
				for _, format := range SQLiteTimestampFormats {
					if timeVal, err = time.ParseInLocation(format, s, time.UTC); err == nil {
						t = timeVal
						break
					}
				}
				if err != nil {
					// The column is a time value, so return the zero time on parse failure.
					t = time.Time{}
				}
				if rc.s.c.loc != nil {
					t = t.In(rc.s.c.loc)
				}
				dest[i] = t
			default:
				dest[i] = []byte(s)
			}

		}
	}
	return nil
}

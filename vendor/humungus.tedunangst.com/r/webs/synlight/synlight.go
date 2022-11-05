//
// Copyright (c) 2018,2019 Ted Unangst <tedu@tedunangst.com>
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

// something like pygments in go
// written before I discovered https://github.com/alecthomas/chroma
package synlight

import (
	"bufio"
	"bytes"
	"html/template"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type token struct {
	name      string
	re        *regexp.Regexp
	state     int
	nextstate int
}

// A syntax highlighter
type Lighter struct {
	markers map[string]*marker
	aliases map[string]string
	lexers  map[string][]*token
	write   func(io.Writer, []byte)
}

type marker struct {
	before []byte
	after  []byte
}

// Options for creating a new highlighter.
// HTML or TTY output are supported.
type Options struct {
	Format OutputFormat
}

type OutputFormat int

const (
	None OutputFormat = iota
	HTML
	TTY
)

var htmlmarkers = make(map[string]*marker)
var ttymarkers = make(map[string]*marker)

func init() {
	htmlmarkers["keyword"] = newmarker("<span class=kw>", "</span>")
	htmlmarkers["builtin"] = newmarker("<span class=bi>", "</span>")
	htmlmarkers["string"] = newmarker("<span class=st>", "</span>")
	htmlmarkers["number"] = newmarker("<span class=nm>", "</span>")
	htmlmarkers["type"] = newmarker("<span class=tp>", "</span>")
	htmlmarkers["operator"] = newmarker("<span class=op>", "</span>")
	htmlmarkers["comment"] = newmarker("<span class=cm>", "</span>")
	htmlmarkers["addline"] = newmarker("<span class=al>", "</span>")
	htmlmarkers["delline"] = newmarker("<span class=dl>", "</span>")

	ttymarkers["keyword"] = newmarker("\x1b[33m", "\x1b[0m")
	ttymarkers["builtin"] = newmarker("\x1b[32m", "\x1b[0m")
	ttymarkers["string"] = newmarker("\x1b[31m", "\x1b[0m")
	ttymarkers["number"] = newmarker("\x1b[31m", "\x1b[0m")
	ttymarkers["type"] = newmarker("\x1b[32m", "\x1b[0m")
	ttymarkers["comment"] = newmarker("\x1b[34m", "\x1b[0m")
	ttymarkers["addline"] = newmarker("\x1b[32m", "\x1b[0m")
	ttymarkers["delline"] = newmarker("\x1b[31m", "\x1b[0m")
}

func newmarker(before, after string) *marker {
	return &marker{before: []byte(before), after: []byte(after)}
}

func plainwrite(w io.Writer, data []byte) {
	w.Write(data)
}

func newtoken(name string, regex string) *token {
	m := strings.Split(name, ":")
	state := 0
	nextstate := 0
	if len(m) == 3 {
		name = m[0]
		state, _ = strconv.Atoi(m[1])
		nextstate, _ = strconv.Atoi(m[2])
	}
	return &token{
		name,
		regexp.MustCompile("^" + regex),
		state,
		nextstate,
	}
}

// Add a new lexer to this highlighter.
func (hl *Lighter) AddLexer(lang string, r io.Reader) {
	var tokens []*token
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		m := strings.SplitN(line, " ", 2)
		tokens = append(tokens, newtoken(m[0], m[1]))
	}
	tokens = append(tokens, newtoken("unknown", "(?s:.)"))
	hl.lexers[lang] = tokens
}

// Create a new highlighter.
// It should be reused if possible.
func New(options Options) *Lighter {
	hl := new(Lighter)
	switch options.Format {
	case HTML:
		hl.markers = htmlmarkers
		hl.write = template.HTMLEscape
	case TTY:
		hl.markers = ttymarkers
		hl.write = plainwrite
	default:
		panic("invalid output format")
	}

	hl.lexers = make(map[string][]*token)
	hl.AddLexer("c", strings.NewReader(lexer_c))
	hl.AddLexer("diff", strings.NewReader(lexer_diff))
	hl.AddLexer("go", strings.NewReader(lexer_go))
	hl.AddLexer("html", strings.NewReader(lexer_html))
	hl.AddLexer("js", strings.NewReader(lexer_js))
	hl.AddLexer("lua", strings.NewReader(lexer_lua))
	hl.AddLexer("py", strings.NewReader(lexer_py))
	hl.AddLexer("rs", strings.NewReader(lexer_rs))
	hl.AddLexer("sql", strings.NewReader(lexer_sql))

	hl.aliases = make(map[string]string)
	hl.aliases["h"] = "c"
	hl.aliases["patch"] = "diff"
	hl.aliases["python"] = "py"
	hl.aliases["rust"] = "rs"
	hl.aliases["xml"] = "html"

	return hl
}

var pairnames = []string{"string", "keyword", "comment", "builtin"}

// Highlight code, writing it to w.
func (hl *Lighter) Highlight(data []byte, filename string, w io.Writer) {
	dot := strings.LastIndex(filename, ".")
	ext := filename[dot+1:]

	alt, ok := hl.aliases[ext]
	if ok {
		ext = alt
	}

	markers := hl.markers

	tokens := hl.lexers[ext]
	if tokens == nil {
		hl.write(w, data)
		return
	}

	dataloc := 0
	state := 0
	pairidx := uint(0)
	for dataloc < len(data) {
	restart:
		for _, tok := range tokens {
			if tok.state != state {
				continue
			}
			m := tok.re.Find(data[dataloc:])
			if m != nil {
				state = tok.nextstate
				name := tok.name
				if name == "pair" {
					name = pairnames[pairidx%uint(len(pairnames))]
					pairidx += 1
				} else if name == "unpair" {
					pairidx -= 1
					name = pairnames[pairidx%uint(len(pairnames))]
				}
				mk := markers[name]
				if mk != nil {
					w.Write(mk.before)
				}
				hl.write(w, m)
				if mk != nil {
					w.Write(mk.after)
				}
				dataloc += len(m)
				goto restart
			}
		}
		state = 0
	}
}

// Highlight code, returning a string
func (hl *Lighter) HighlightString(data string, filename string) string {
	var buf bytes.Buffer
	hl.Highlight([]byte(data), filename, &buf)
	return buf.String()
}

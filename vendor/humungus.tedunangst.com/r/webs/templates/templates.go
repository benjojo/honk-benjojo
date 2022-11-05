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

// Utilities to make html/templates a little easier to work with
package templates

import (
	"errors"
	"html/template"
	"io"
)

// Wrapper around html/template supporting hot reloads.
// Includes a map function passing multiple arguments to nested templates.
type Template struct {
	names     []string
	templates *template.Template
	reload    bool
}

func mapmaker(values ...interface{}) (map[string]interface{}, error) {
	if len(values)%2 != 0 {
		return nil, errors.New("need arguments in pairs")
	}
	dict := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, errors.New("key must be string")
		}
		dict[key] = values[i+1]
	}
	return dict, nil
}

func loadtemplates(filenames ...string) (*template.Template, error) {
	templates := template.New("")
	templates.Funcs(template.FuncMap{
		"map": mapmaker,
	})
	templates, err := templates.ParseFiles(filenames...)
	if err != nil {
		return nil, err
	}
	return templates, nil
}

// Execute the template
func (t *Template) Execute(w io.Writer, name string, data interface{}) error {
	if t.reload {
		templates, err := loadtemplates(t.names...)
		if err != nil {
			return err
		}
		return templates.ExecuteTemplate(w, name, data)
	}
	return t.templates.ExecuteTemplate(w, name, data)
}

// Load templates.
// If reload is true, they are reloaded every execution.
func Load(reload bool, filenames ...string) *Template {
	t := new(Template)
	t.names = filenames
	t.reload = reload
	templates, err := loadtemplates(filenames...)
	if err != nil {
		panic(err)
	}
	if !reload {
		t.templates = templates
	}
	return t
}

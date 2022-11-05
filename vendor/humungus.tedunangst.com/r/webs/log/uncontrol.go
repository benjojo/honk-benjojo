package log

import (
	"io"
)

type unctrlwriter struct {
	w io.Writer
}

func filteredwrite(w io.Writer, p []byte) (int, error) {
	r := make([]byte, len(p))
	for i, c := range p {
		if c < 32 && i != len(p)-1 {
			if c == '\t' || c == '\n' {
				c = ' '
			} else {
				c = '.'
			}
		}
		r[i] = c
	}
	return w.Write(r)
}

func (u unctrlwriter) Write(p []byte) (int, error) {
	for i, c := range p {
		if c < 32 && i != len(p)-1 {
			return filteredwrite(u.w, p)
		}
	}
	return u.w.Write(p)
}

func unctrl(w io.Writer) io.Writer {
	return unctrlwriter{w: w}
}

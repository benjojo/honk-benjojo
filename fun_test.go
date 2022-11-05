package main

import (
	"testing"
)

func TestObfusbreak(t *testing.T) {
	input := `link to https://example.com/ with **bold** text`
	output := `link to <a href="https://example.com/">https://example.com/</a> with <b>bold</b> text`

	tmp := obfusbreak(input)
	if tmp != output {
		t.Errorf("%s is not %s", tmp, output)
	}
}

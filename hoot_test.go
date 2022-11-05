package main

import (
	"fmt"
	"os"
	"testing"
)

func TestHooterize(t *testing.T) {
	fd, err := os.Open("lasthoot.html")
	if err != nil {
		return
	}
	seen := make(map[string]bool)
	hoots := hootextractor(fd, "lasthoot.html", seen)
	fmt.Printf("hoots: %s\n", hoots)
}

package main

import "flag"

func main() {
	flag.Parse()

	if flag.Arg(0) == "post" {
		cliPost()
		return
	}

	// if flag.Arg(0) == "follow" {
	// 	if cliAccounts() {
	// 		return
	// 	}
	// }
}

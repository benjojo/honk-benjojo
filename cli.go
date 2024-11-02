package main

import (
	"fmt"
	"os"
	"strconv"

	"humungus.tedunangst.com/r/webs/totp"
)

type cmd struct {
	help     string
	help2    string
	callback func(args []string)
	nargs    int
}

var commands = map[string]cmd{
	"init": {
		help: "initialize honk",
		callback: func(args []string) {
			initdb()
		},
	},
	"upgrade": {
		help: "upgrade honk",
		callback: func(args []string) {
			upgradedb()
		},
	},
	"version": {
		help: "print version",
		callback: func(args []string) {
			fmt.Println(softwareVersion)
			os.Exit(0)
		},
	},
	"admin": {
		help: "admin interface",
		callback: func(args []string) {
			adminscreen()
		},
	},
	"import": {
		help:  "import data into honk",
		help2: "import username honk|mastodon|twitter srcdir",
		callback: func(args []string) {
			importMain(args[1], args[2], args[3])
		},
		nargs: 4,
	},
	"export": {
		help:  "export data from honk",
		help2: "export username destdir",
		callback: func(args []string) {
			export(args[1], args[2])
		},
		nargs: 3,
	},
	"dumpthread": {
		help:  "export a thread for debugging",
		help2: "dumpthread user convoy",
		callback: func(args []string) {
			dumpthread(args[1], args[2])
		},
		nargs: 3,
	},
	"rawimport": {
		help:  "import activity objects for debugging",
		help2: "rawimport username filename",
		callback: func(args []string) {
			rawimport(args[1], args[2])
		},
		nargs: 3,
	},
	"devel": {
		help:  "turn devel on/off",
		help2: "devel (on|off)",
		callback: func(args []string) {
			switch args[1] {
			case "on":
				setconfig("devel", 1)
			case "off":
				setconfig("devel", 0)
			default:
				errx("argument must be on or off")
			}
		},
		nargs: 2,
	},
	"setconfig": {
		help:  "set honk config",
		help2: "setconfig key val",
		callback: func(args []string) {
			var val interface{}
			var err error
			if val, err = strconv.Atoi(args[2]); err != nil {
				val = args[2]
			}
			setconfig(args[1], val)
		},
		nargs: 3,
	},
	"adduser": {
		help: "add a user to honk",
		callback: func(args []string) {
			adduser()
		},
	},
	"deluser": {
		help:  "delete a user from honk",
		help2: "deluser username",
		callback: func(args []string) {
			deluser(args[1])
		},
		nargs: 2,
	},
	"chpass": {
		help:  "change password of an account",
		help2: "chpass username",
		callback: func(args []string) {
			chpass(args[1])
		},
		nargs: 2,
	},
	"follow": {
		help:  "follow an account",
		help2: "follow username url",
		callback: func(args []string) {
			user, err := getUserBio(args[1])
			if err != nil {
				errx("user %s not found", args[1])
			}
			var meta HonkerMeta
			mj, _ := encodeJson(&meta)
			honkerid, flavor, err := savehonker(user, args[2], "", "presub", "", mj)
			if err != nil {
				errx("had some trouble with that: %s", err)
			}
			if flavor == "presub" {
				followyou(user, honkerid, true)
			}
		},
		nargs: 3,
	},
	"unfollow": {
		help:  "unfollow an account",
		help2: "unfollow username url",
		callback: func(args []string) {
			user, err := getUserBio(args[1])
			if err != nil {
				errx("user not found")
			}

			honkerid, err := gethonker(user.ID, args[2])
			if err != nil {
				errx("sorry couldn't find them")
			}
			unfollowyou(user, honkerid, true)
		},
		nargs: 3,
	},
	"sendmsg": {
		help:  "send a raw activity",
		help2: "sendmsg username filename rcpt",
		callback: func(args []string) {
			user, err := getUserBio(args[1])
			if err != nil {
				errx("user %s not found", args[1])
			}
			data, err := os.ReadFile(args[2])
			if err != nil {
				errx("can't read file: %s", err)
			}
			deliverate(user.ID, args[3], data)
		},
		nargs: 4,
	},
	"cleanup": {
		help: "clean up stale data from database",
		callback: func(args []string) {
			arg := "30"
			if len(args) > 1 {
				arg = args[1]
			}
			cleanupdb(arg)
		},
	},
	"storefiles": {
		help: "store attachments as files",
		callback: func(args []string) {
			setconfig("usefilestore", 1)
		},
	},
	"storeblobs": {
		help: "store attachments as blobs",
		callback: func(args []string) {
			setconfig("usefilestore", 0)
		},
	},
	"extractblobs": {
		help: "extract blobs to file store",
		callback: func(args []string) {
			extractblobs()
		},
	},
	"unplug": {
		help:  "disconnect from a dead server",
		help2: "unplug servername",
		callback: func(args []string) {
			name := args[1]
			unplugserver(name)
		},
		nargs: 2,
	},
	"backup": {
		help: "backup honk",
		callback: func(args []string) {
			if len(args) < 2 {
				errx("usage: honk backup dirname")
			}
			name := args[1]
			backupDatabase(name)
		},
	},
	"ping": {
		help: "ping from user to user/url",
		callback: func(args []string) {
			if len(args) < 3 {
				errx("usage: honk ping (from username) (to username or url)")
			}
			name := args[1]
			targ := args[2]
			user, err := getUserBio(name)
			if err != nil {
				errx("unknown user %s", name)
			}
			ping(user, targ)
		},
	},
	"extractchatkey": {
		help: "extract secret chat key from user",
		callback: func(args []string) {
			if len(args) < 3 || args[2] != "yesimsure" {
				errx("usage: honk extractchatkey [username] yesimsure")
			}
			user, _ := getUserBio(args[1])
			if user == nil {
				errx("user not found")
			}
			fmt.Printf("%s\n", user.Options.ChatSecKey)
			user.Options.ChatSecKey = ""
			j, err := encodeJson(user.Options)
			if err == nil {
				db := opendatabase()
				_, err = db.Exec("update users set options = ? where username = ?", j, user.Name)
			}
			if err != nil {
				elog.Printf("error bouting what: %s", err)
			}
		},
	},
	"run": {
		help: "run honk",
		callback: func(args []string) {
			serve()
		},
	},
	"backend": {
		help: "run backend",
		callback: func(args []string) {
			backendServer()
		},
	},
	"totp": {
		help: "generate totp code",
		callback: func(args []string) {
			if len(args) != 2 {
				errx("usage: honk totp secret")
			}
			fmt.Printf("code: %d\n", totp.GenerateCode(args[1]))
		},
	},
	"test": {
		help: "run test",
		callback: func(args []string) {
			ElaborateUnitTests()
		},
	},
}

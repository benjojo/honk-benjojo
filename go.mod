module humungus.tedunangst.com/r/honk

go 1.16

require (
	github.com/andybalholm/cascadia v1.3.1
	github.com/gorilla/mux v1.8.0
	github.com/mattn/go-runewidth v0.0.13
	golang.org/x/crypto v0.0.0-20220411220226-7b82a4e95df4
	golang.org/x/image v0.0.0-20220413100746-70e8d0d3baa9
	golang.org/x/net v0.0.0-20220425223048-2871e0cb64e4
	humungus.tedunangst.com/r/go-sqlite3 v1.1.3
	humungus.tedunangst.com/r/webs v0.6.56
)

replace humungus.tedunangst.com/r/webs/image => ./image

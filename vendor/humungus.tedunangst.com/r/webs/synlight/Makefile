
all: test

test: lexers.go
	go test

lexers.go: genlexer.go lexers/*.txt
	go run genlexer.go lexers/*.txt
	go fmt

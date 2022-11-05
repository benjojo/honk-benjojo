
all: test

test: lexers.go
	go test

lexers.go: genlexer.py lexers/*.txt
	python2.7 genlexer.py lexers/*.txt
	go fmt

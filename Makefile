
all: honk

honk: *.go go.mod
	go build -mod=`ls -d vendor 2> /dev/null` -o honk

clean:
	rm -f honk

test:
	go test

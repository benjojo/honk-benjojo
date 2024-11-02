
all: honk

honk: .preflightcheck schema.sql *.go go.mod
	env CGO_ENABLED=1 go build -mod=`ls -d vendor 2> /dev/null` -o honk

.preflightcheck: *.go preflight.sh
	@sh ./preflight.sh

help:
	for m in docs/*.[13578] ; do \
	mandoc -T html -O style=mandoc.css,man=%N.%S.html $$m | sed -E 's/<a class="Lk" href="([[:alnum:]._-]*)">/<img src="\1"><br>/g' > $$m.html ; \
	done

clean:
	rm -f honk

test:
	go test

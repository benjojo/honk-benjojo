set -e

go version > /dev/null 2>&1 || (echo go 1.18+ is required && false)

v=`go version | egrep -o "go1\.[^.]+"` || echo failed to identify go version
if [ "$v" \< "go1.18" ] ; then
	echo go version is too old: $v
	echo go 1.18+ is required
	false
fi

if [ \! \( -e /usr/include/sqlite3.h -o -e /usr/local/include/sqlite3.h -o `uname` = "Darwin" \) ] ; then
	echo unable to find sqlite3.h header
	echo please install libsqlite3 dev package
	false
fi

touch .preflightcheck


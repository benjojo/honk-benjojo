set -e

go version > /dev/null 2>&1 || (echo go 1.20+ is required && false)

v=`go version | egrep -o "go1\.[^.]+"` || echo failed to identify go version
if [ "$v" \< "go1.20" ] ; then
	echo go version is too old: $v
	echo go 1.20+ is required
	false
fi

sqlhdr=
if [ `uname` = "Darwin" ] ; then
	: # okay
else
	if [ -e /usr/include/sqlite3.h ] ; then
		sqlhdr=/usr/include/sqlite3.h
	elif [ -e /usr/local/include/sqlite3.h ] ; then
		sqlhdr=/usr/local/include/sqlite3.h
	else
		echo unable to find sqlite3.h header
		echo please install libsqlite3 dev package
		false
	fi
	sqlvers=`grep "#define SQLITE_VERSION_NUMBER" $sqlhdr | cut -f3 -d' '`
	if [ $sqlvers -lt 3034000 ] ; then
		echo sqlite3.h header is too old: $sqlvers
		echo version 3.34.0+ is required
		false
	fi
fi

touch .preflightcheck

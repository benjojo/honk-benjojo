set -e

go version > /dev/null 2>&1 || (echo go 1.13+ is required && false)

v=`go version | egrep -o 'go1[^ ]+'`
case $v in
	go1.10*|go1.11*|go1.12*)
		echo go version is too old: $v
		echo go 1.13+ is required
		false
		;;
	go1.1*)
		# just pretend nobody is still using go 1.1 or 1.2
		;;
	go1.2*)
		;;
	*)
		echo unknown go version: $v
		false
		;;
esac

if [ \! \( -e /usr/include/sqlite3.h -o -e /usr/local/include/sqlite3.h \) ] ; then
	echo unable to find sqlite3.h header
	echo please install libsqlite3 dev package
	false
fi

touch .preflightcheck


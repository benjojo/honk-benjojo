import sys
import re

lfd = open("lexers.go", "w")

def appendlexer(f):
	fd = open(f)
	name = f[7:-4]
	lfd.write("var lexer_%s = \"\" + \n" % name)
	for line in fd:
		lfd.write("\"")
		line = line.strip()
		line = line.replace("\\", "\\\\")
		line = line.replace("\"", "\\\"")
		lfd.write(line)
		lfd.write("\\n\" + \n")
	lfd.write("\"\"\n")

lfd.write("package synlight\n\n")
for f in sys.argv[1:]:
	appendlexer(f)

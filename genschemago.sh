echo "package main" > schema.go
echo "var sqlSchema = \`" >> schema.go
cat schema.sql >> schema.go
echo "\`" >> schema.go
go fmt schema.go



PROGS=autobonker gettoken saytheday sprayandpray wonkawonk youvegothonks

all: $(PROGS)

clean:
	rm -f $(PROGS)

autobonker: autobonker.go
	go build autobonker.go

gettoken: gettoken.go
	go build gettoken.go fetch.go

saytheday: saytheday.go
	go build saytheday.go

sprayandpray: sprayandpray.go
	go build sprayandpray.go

wonkawonk: wonkawonk.go fetch.go
	go build wonkawonk.go fetch.go

youvegothonks: youvegothonks.go
	go build youvegothonks.go

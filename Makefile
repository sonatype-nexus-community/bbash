.PHONY: all test build go-build air
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test

all: test

air:
	$(GOBUILD) -o ./tmp/bbash ./server.go

run-air:
	docker run --name bbash_postgres -p 5432:5432 -e POSTGRES_PASSWORD=bbash -e POSTGRES_DB=db -d postgres
	air -c .air.toml && docker stop bbash_postgres && docker rm bbash_postgres

build: go-build

go-build:
	$(GOBUILD) -o bbash ./server.go

test: build
	$(GOTEST) -v ./... 2>&1

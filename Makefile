.PHONY: all test build go-build air docker run-air go-alpine-build
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test

all: test

air:
	$(GOBUILD) -o ./tmp/bbash ./server.go

docker:
	docker build -t bug-bash .
	docker image prune --force --filter label=stage=builder 

run-air:
	docker run --name bug_bash_postgres -p 5432:5432 -e POSTGRES_PASSWORD=bug_bash -e POSTGRES_DB=db -d postgres
	air -c .air.toml && docker stop bug_bash_postgres && docker rm bug_bash_postgres

build: go-build

go-build:
	$(GOBUILD) -o bbash ./server.go

go-alpine-build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o bbash ./server.go

test: build
	$(GOTEST) -v ./... 2>&1

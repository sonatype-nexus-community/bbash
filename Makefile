.PHONY: all test build go-build
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test

all: test

build: go-build

go-build:
	$(GOBUILD) -o bbash ./server.go

test: build
	$(GOTEST) -v ./... 2>&1

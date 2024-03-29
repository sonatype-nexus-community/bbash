.PHONY: all test build yarn air docker go-build go-alpine-build run-air run-air-alone
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test

AIRCMD=~/go/bin/air

TAG_COMMIT := $(shell git rev-list --abbrev-commit --tags --max-count=1)
TAG := $(shell git describe --abbrev=0 --tags ${TAG_COMMIT} 2>/dev/null || true)
COMMIT := $(shell git rev-parse --short HEAD)
DATE := $(shell git log -1 --format=%cd --date=iso)
VERSION := $(TAG:v%=%)
ifneq ($(COMMIT), $(TAG_COMMIT))
	VERSION := 0.0.0-dev
endif
ifeq ($(VERSION),)
	VERSION := $(COMMIT)-$(DATE)
endif
ifneq ($(shell git status --porcelain),)
	VERSION := $(VERSION)-dirty
endif
GOBUILD_FLAGS=-ldflags="-X 'github.com/sonatype-nexus-community/bbash/buildversion.BuildVersion=$(VERSION)' \
	   -X 'github.com/sonatype-nexus-community/bbash/buildversion.BuildTime=$(DATE)' \
	   -X 'github.com/sonatype-nexus-community/bbash/buildversion.BuildCommit=$(COMMIT)'"

all: test

docker:
	yarn version --patch
	docker build -t bug-bash .
	docker image prune --force --filter label=stage=builder 

build: yarn go-build

yarn:
	yarn && yarn build

go-build:
	echo "VERSION: $(VERSION)"
	echo "DATE: $(DATE)"
	echo "COMMIT: $(COMMIT)"
	$(GOBUILD) -o bbash $(GOBUILD_FLAGS) ./server.go

go-alpine-build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o bbash $(GOBUILD_FLAGS) ./server.go

air: yarn
	$(GOBUILD) -o ./tmp/bbash $(GOBUILD_FLAGS) ./server.go

run-air: air
	docker run --name bug_bash_postgres -p 5432:5432 -e POSTGRES_PASSWORD=bug_bash -e POSTGRES_DB=db -d postgres
	air -c .air.toml && docker stop bug_bash_postgres && docker rm bug_bash_postgres

run-air-alone: yarn
	$(AIRCMD) -c .air.toml

test: build
	$(GOTEST) -v ./... 2>&1

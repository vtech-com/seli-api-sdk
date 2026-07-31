MODULE := github.com/vtech-com/seli-api-sdk
VERSION_PKG := $(MODULE)/internal/version

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -ldflags "\
  -X $(VERSION_PKG).Version=$(VERSION) \
  -X $(VERSION_PKG).Commit=$(COMMIT) \
  -X $(VERSION_PKG).Date=$(DATE)"

DIST_DIR := dist
BINARY   := $(DIST_DIR)/seli

.PHONY: build test lint check fmt release-snapshot install clean

build:
	@mkdir -p $(DIST_DIR)
	go build $(LDFLAGS) -o $(BINARY) .

test:
	go test ./... -count=1 -race -coverprofile=coverage.out

lint:
	golangci-lint run ./...

check: lint test

fmt:
	gofmt -l -s -w .

release-snapshot:
	goreleaser release --snapshot --clean

install:
	go install $(LDFLAGS) .

clean:
	rm -rf $(DIST_DIR) coverage.out

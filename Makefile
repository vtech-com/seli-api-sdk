MODULE := github.com/vtech-com/seli-api-sdk
VERSION_PKG := $(MODULE)/internal/version

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# The API host is configuration, never code: the binary's fallback host is
# injected here from the environment and is empty when SELI_DEFAULT_API_URL is
# unset (the CLI then needs --api-url or SELI_API_URL at runtime).
SELI_DEFAULT_API_URL ?=

# Where `make update-spec` fetches the OpenAPI document from. Defaults to the
# default host's /api/openapi; empty when neither variable is set.
SELI_SPEC_URL ?= $(if $(SELI_DEFAULT_API_URL),$(SELI_DEFAULT_API_URL)/api/openapi)

LDFLAGS := -ldflags "\
  -X $(VERSION_PKG).Version=$(VERSION) \
  -X $(VERSION_PKG).Commit=$(COMMIT) \
  -X $(VERSION_PKG).Date=$(DATE) \
  -X $(VERSION_PKG).DefaultAPIURL=$(SELI_DEFAULT_API_URL)"

DIST_DIR := dist
BINARY   := $(DIST_DIR)/seli

.PHONY: build test lint check fmt release-snapshot install clean update-spec

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

## update-spec: Fetch the latest OpenAPI spec from the Seli platform
## The platform serves minified JSON on one line; pretty-print it to 2-space
## indent so the committed spec stays reviewable and successive runs produce
## stable diffs. The URL comes from SELI_SPEC_URL, else SELI_DEFAULT_API_URL.
update-spec:
	@test -n "$(SELI_SPEC_URL)" || { echo "update-spec: set SELI_SPEC_URL (or SELI_DEFAULT_API_URL; the spec is served at <host>/api/openapi)" >&2; exit 1; }
	curl -fsSL "$(SELI_SPEC_URL)" | \
		python3 -c "import json,sys; json.dump(json.load(sys.stdin), open('api/openapi.json','w'), indent=2, ensure_ascii=False); open('api/openapi.json','a').write(chr(10))"
	@echo "Updated api/openapi.json from $(SELI_SPEC_URL) (pretty-printed)"

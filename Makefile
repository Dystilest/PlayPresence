VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
PKG     := github.com/Dystilest/PlayPresence/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT)

.PHONY: build test race lint vuln

build:
	go build -ldflags "$(LDFLAGS)" -o playpresence ./cmd/playpresence

# Fast run, no C toolchain needed.
test:
	go test ./...

# Race detector; requires CGO and a C compiler (CI always runs this).
race:
	go test -race ./...

lint:
	golangci-lint run

vuln:
	govulncheck ./...

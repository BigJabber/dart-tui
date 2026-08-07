# Build targets for the darts scoreboard.
#
# CGO is disabled everywhere. The SQLite driver is modernc.org/sqlite, a pure-Go
# implementation, so a Windows build needs no MinGW, no Visual Studio build tools
# and no SQLite DLL — just the Go toolchain.

BINARY  := darts
PKG     := ./cmd/darts
DIST    := dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

export CGO_ENABLED = 0

.PHONY: all
all: test build

## build: build for the current platform
.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

## run: build and run
.PHONY: run
run:
	go run $(PKG)

## test: run every test
.PHONY: test
test:
	go test ./...

## test-race: run the tests with the race detector
.PHONY: test-race
test-race:
	CGO_ENABLED=1 go test -race ./...

## cover: report test coverage per package
.PHONY: cover
cover:
	go test -cover ./...

## vet: run go vet
.PHONY: vet
vet:
	go vet ./...

## fmt: format the source
.PHONY: fmt
fmt:
	gofmt -w cmd internal

## check: everything CI runs
.PHONY: check
check: fmt vet test

## windows: build the Windows executables
.PHONY: windows
windows: windows-amd64 windows-arm64

.PHONY: windows-amd64
windows-amd64:
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-windows-amd64.exe $(PKG)

.PHONY: windows-arm64
windows-arm64:
	GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-windows-arm64.exe $(PKG)

.PHONY: linux-amd64
linux-amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-linux-amd64 $(PKG)

## release: build every distributed binary
.PHONY: release
release: clean windows linux-amd64
	@echo
	@ls -lh $(DIST)

## clean: remove build output
.PHONY: clean
clean:
	rm -rf $(DIST) $(BINARY) $(BINARY).exe

## help: list the targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'

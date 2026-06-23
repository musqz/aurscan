BIN     := aurscan
PREFIX  ?= /usr/local
PKGPATH := github.com/manticore-projects/aurscan/internal/version

# Version info from git (works in a checkout; release tarballs override via
# VERSION=... on the make command line, as the PKGBUILD does).
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(PKGPATH).Version=$(VERSION) \
	-X $(PKGPATH).Commit=$(COMMIT) \
	-X $(PKGPATH).Date=$(DATE)
GOFLAGS := -trimpath -buildmode=pie -ldflags="$(LDFLAGS)"

# Release artifacts are hardened to match Arch's Go package guidelines: PIE +
# full RELRO, while staying static and portable. Full RELRO (DT_BIND_NOW) needs
# the external linker, so CGO is enabled purely as the link driver; netgo and
# osusergo force Go's pure-Go resolver/user lookup so the binary stays static
# with no glibc NSS at runtime. arm64 cross-links with aarch64-linux-gnu-gcc
# (override CC_arm64 if your toolchain differs). This matches the release CI so
# `make release` reproduces the published binaries (#30).
CC_arm64    ?= aarch64-linux-gnu-gcc
REL_LDFLAGS := $(LDFLAGS) -linkmode=external -extldflags '-static-pie -Wl,-z,relro -Wl,-z,now'
REL_FLAGS   := -trimpath -buildmode=pie -tags 'netgo osusergo' -ldflags="$(REL_LDFLAGS)"

build:
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(BIN) ./cmd/aurscan

version: build
	./$(BIN) --version

test:
	go vet ./...
	go test ./...

release:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build $(REL_FLAGS) -o aurscan-linux-amd64 ./cmd/aurscan
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=$(CC_arm64) go build $(REL_FLAGS) -o aurscan-linux-arm64 ./cmd/aurscan

install: build
	install -Dm755 $(BIN) $(DESTDIR)$(PREFIX)/bin/$(BIN)
	ln -sf $(BIN) $(DESTDIR)$(PREFIX)/bin/syay
	ln -sf $(BIN) $(DESTDIR)$(PREFIX)/bin/aurscan-edit

clean:
	rm -f $(BIN) aurscan-linux-amd64 aurscan-linux-arm64

.PHONY: build version test release install clean

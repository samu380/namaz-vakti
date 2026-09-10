# Baut das namaz-CLI. Ohne Argumente: Binary für die eigene Plattform.

BINARY  := namaz
PKG     := ./cmd/namaz
DIST    := dist

# Version aus dem Git-Tag, sonst "dev". Wird per -ldflags eingebettet und
# von `namaz --surum` ausgegeben.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/samu380/namaz-vakti/internal/cli.Version=$(VERSION)

# CGO aus: erzeugt eine statisch gelinkte Binary ohne libc-Abhängigkeit,
# die sich zwischen Rechnern und Distributionen kopieren lässt.
export CGO_ENABLED = 0

.PHONY: all build test vet fmt install clean dist

all: build

build:
	go build -ldflags '$(LDFLAGS)' -o $(BINARY) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

# Installiert nach $(go env GOPATH)/bin
install:
	go install -ldflags '$(LDFLAGS)' $(PKG)

clean:
	rm -rf $(BINARY) $(DIST)

# Baut die Binaries für alle unterstützten Plattformen nach dist/.
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64

dist: clean
	@mkdir -p $(DIST)
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		out=$(DIST)/$(BINARY)-$$os-$$arch; \
		echo "  $$out"; \
		GOOS=$$os GOARCH=$$arch go build -ldflags '$(LDFLAGS)' -o $$out $(PKG) || exit 1; \
	done
	@echo "Fertig. Version: $(VERSION)"

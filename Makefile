# RGO Makefile

GOCACHE ?= /tmp/rgo-go-build-cache
GOMODCACHE ?= /tmp/rgo-go-mod-cache
GOENV = GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE)

.PHONY: help test clean build fmt lint check
.PHONY: full-spec full-ruby-spec full-rails-spec

help:
	@echo "RGo - Ruby VM in Go"
	@echo ""
	@echo "Commands:"
	@echo "  make build   Build the rgo binary"
	@echo "  make test    Run all tests"
	@echo "  make fmt     Format code"
	@echo "  make lint    Run go vet"
	@echo "  make check   Format + lint + test"
	@echo "  make clean   Remove build artifacts"

build:
	$(GOENV) go build -o rgo ./cmd/rgo

test:
	scripts/safe_go_test.sh ./...

fmt:
	$(GOENV) go fmt ./...

lint:
	$(GOENV) go vet ./...

check: fmt lint test

clean:
	rm -f rgo

full-spec:
	scripts/full_spec_gate.sh

full-ruby-spec:
	scripts/full_spec_gate.sh --ruby-only

full-rails-spec:
	scripts/full_spec_gate.sh --rails-only

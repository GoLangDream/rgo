# RGO Makefile

.PHONY: help test clean build fmt lint check

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
	go build -o rgo ./cmd/rgo

test:
	go test ./...

fmt:
	go fmt ./...

lint:
	go vet ./...

check: fmt lint test

clean:
	rm -f rgo

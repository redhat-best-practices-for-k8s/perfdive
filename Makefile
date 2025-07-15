# Makefile for perfdive

# Variables
BINARY_NAME=perfdive
VERSION?=dev
BUILD_TIME=$(shell date +%Y-%m-%d_%H:%M:%S)
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"

# Default target
.DEFAULT_GOAL := build

# Build the application
.PHONY: build
build:
	go build -o ${BINARY_NAME} ${LDFLAGS} .

# Clean build artifacts
.PHONY: clean
clean:
	go clean
	rm -f ${BINARY_NAME}

# Run tests
.PHONY: test
test:
	go test -v ./...

# Run go mod tidy
.PHONY: tidy
tidy:
	go mod tidy

# Format code
.PHONY: fmt
fmt:
	go fmt ./...

# Run go vet
.PHONY: vet
vet:
	go vet ./...

# Run all checks (fmt, vet, test)
.PHONY: check
check: fmt vet test

# Install dependencies
.PHONY: deps
deps:
	go mod download

# Build for multiple platforms
.PHONY: build-all
build-all:
	GOOS=linux GOARCH=amd64 go build -o ${BINARY_NAME}-linux-amd64 ${LDFLAGS} .
	GOOS=darwin GOARCH=amd64 go build -o ${BINARY_NAME}-darwin-amd64 ${LDFLAGS} .
	GOOS=darwin GOARCH=arm64 go build -o ${BINARY_NAME}-darwin-arm64 ${LDFLAGS} .
	GOOS=windows GOARCH=amd64 go build -o ${BINARY_NAME}-windows-amd64.exe ${LDFLAGS} .

# Run the application with example args (requires configuration)
.PHONY: run
run: build
	./${BINARY_NAME} --help

# Show help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build      - Build the perfdive binary"
	@echo "  clean      - Remove build artifacts"
	@echo "  test       - Run tests"
	@echo "  tidy       - Run go mod tidy"
	@echo "  fmt        - Format Go code"
	@echo "  vet        - Run go vet"
	@echo "  check      - Run fmt, vet, and test"
	@echo "  deps       - Download dependencies"
	@echo "  build-all  - Build for multiple platforms"
	@echo "  run        - Build and show help"
	@echo "  help       - Show this help message" 
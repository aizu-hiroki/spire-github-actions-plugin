# Makefile for spire-github-actions-plugin

GOOS   ?= linux
GOARCH ?= amd64
OUT    ?= bin

.PHONY: all build test lint clean

all: build

build: $(OUT)/spire-plugin-github-actions-agent \
       $(OUT)/spire-plugin-github-actions-server

$(OUT)/spire-plugin-github-actions-agent:
	@mkdir -p $(OUT)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $@ ./cmd/nodeattestor-agent

$(OUT)/spire-plugin-github-actions-server:
	@mkdir -p $(OUT)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $@ ./cmd/nodeattestor-server

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf $(OUT)

# Makefile for spire-github-actions-plugin
# Builds three binaries: node attestor (agent), node attestor (server), workload attestor.

GOOS   ?= linux
GOARCH ?= amd64
OUT    ?= bin

.PHONY: all build test lint clean

all: build

build: $(OUT)/spire-plugin-github-actions-agent \
       $(OUT)/spire-plugin-github-actions-server \
       $(OUT)/spire-plugin-github-actions-workload

$(OUT)/spire-plugin-github-actions-agent:
	@mkdir -p $(OUT)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $@ ./cmd/nodeattestor-agent

$(OUT)/spire-plugin-github-actions-server:
	@mkdir -p $(OUT)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $@ ./cmd/nodeattestor-server

$(OUT)/spire-plugin-github-actions-workload:
	@mkdir -p $(OUT)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $@ ./cmd/workloadattestor

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf $(OUT)

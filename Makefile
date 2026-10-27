GO ?= go
BINARY ?= infinity-iac

.PHONY: all build test test-race vet fmt-check clean

all: test vet build

build:
	mkdir -p bin
	$(GO) build -trimpath -o bin/$(BINARY) ./cmd/infinity-iac

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt-check:
	test -z "$$($(GO)fmt -l .)"

clean:
	rm -rf bin dist

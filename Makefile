GO ?= go
BINARY ?= infinity-iac

.PHONY: all build test test-race test-oci-integration vet fmt-check validate-recipes verify-release clean

all: test vet build

build:
	mkdir -p bin
	$(GO) build -trimpath -o bin/$(BINARY) ./cmd/infinity-iac

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-oci-integration:
	INFINITY_IAC_OCI_INTEGRATION=1 $(GO) test ./internal/provider -run '^TestOCIProviderLiveReadOnlyAccess$$' -count=1 -v

vet:
	$(GO) vet ./...

fmt-check:
	test -z "$$($(GO)fmt -l ./cmd ./internal ./recipes)"

validate-recipes:
	$(GO) run ./cmd/infinity-iac list recipes --output json

verify-release:
	./scripts/verify-release.sh dist

clean:
	rm -rf bin dist

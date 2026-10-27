# Infinity-IaC

Infinity-IaC is a Go CLI for infrastructure lifecycle workflows. The current
release provides a safe local vertical slice with access profiles, typed server
specifications, deterministic instance lifecycle operations, JSON/YAML plans,
plan-hash-bound apply, and structured JSON output.

## Quick start

`sh
go build -o bin/infinity-iac ./cmd/infinity-iac
INFINITY_IAC_CONFIG="$PWD/.infinity-iac/config.json" \
  ./bin/infinity-iac setup oracle --name default --profile DEFAULT --region local
INFINITY_IAC_CONFIG="$PWD/.infinity-iac/config.json" \
  ./bin/infinity-iac create server --name demo --provider oracle --yes
INFINITY_IAC_CONFIG="$PWD/.infinity-iac/config.json" \
  ./bin/infinity-iac list instances --output json
`

The current provider backend is a deterministic file provider for development
and tests. It does not provision a real cloud instance yet; real Oracle, AWS,
and Google Cloud adapters are the next provider layer.

## Development

`sh
make test
make test-race
make vet
make fmt-check
make build
`

GitHub Actions runs formatting, vet, race-enabled tests, and cross-platform
builds on pushes and pull requests. Pushing a tag matching `v*.*.*` builds
Linux, macOS, and Windows release binaries and publishes checksums to a GitHub
Release.

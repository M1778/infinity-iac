# Infinity-IaC 1.0.1

Infinity-IaC is a Go CLI for production Oracle Cloud Infrastructure lifecycle
workflows. Version 1.0.1 provisions and manages compute instances through the
OCI Go SDK, with typed server specifications, JSON/YAML plans, plan-hash-bound
apply, and structured JSON output.

## Quick start

```sh
go build -o bin/infinity-iac ./cmd/infinity-iac
INFINITY_IAC_CONFIG="$PWD/.infinity-iac/config.json" \
  ./bin/infinity-iac setup oracle --name default --profile DEFAULT \
    --region us-phoenix-1 --compartment OCID --subnet OCID \
    --availability-domain PHX-AD-1 --image OCID --shape VM.Standard.E4.Flex
INFINITY_IAC_CONFIG="$PWD/.infinity-iac/config.json" \
  ./bin/infinity-iac create server --name demo --provider oracle --yes
INFINITY_IAC_CONFIG="$PWD/.infinity-iac/config.json" \
  ./bin/infinity-iac list instances --output json
```

The OCI adapter uses the standard OCI CLI configuration and never copies private
keys into Infinity-IaC configuration.
Recipes are versioned YAML/JSON documents validated before they are listed or
selected; the built-in catalog is embedded in release binaries.

## Development

```sh
make test
make test-race
make vet
make fmt-check
make validate-recipes
make build
```

See [the command reference](docs/command-reference.md) for manifests,
completion setup, lifecycle commands, and reserved product areas.

GitHub Actions runs formatting, vet, race-enabled tests, and cross-platform
builds on pushes and pull requests. Pushing a tag matching `v*.*.*` builds
Linux, macOS, and Windows release binaries and publishes checksums to a GitHub
Release together with Unix and Windows installers. Install Unix releases with:

```sh
curl -fsSL https://raw.githubusercontent.com/M1778/infinity-iac/master/install.sh | sh
```

Windows releases include `infinity-iac-setup.exe`, which installs the CLI for
the current user and can add it to the user PATH.

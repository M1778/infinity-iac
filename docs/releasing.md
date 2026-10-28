# Releasing Infinity-IaC

This guide is for repository maintainers. Releases are tag-driven and are
published by `.github/workflows/release.yml`.

## Before tagging

Run the same checks used by CI and verify the version constant in
`internal/cli/cli.go`:

```sh
make fmt-check
make vet
make test-race
make validate-recipes
make build
sh -n install.sh scripts/verify-release.sh
git diff --check
```

For a local production credential check, set the read-only integration variables
from a shell or secret manager and run:

```sh
export INFINITY_IAC_OCI_INTEGRATION=1
export INFINITY_IAC_OCI_PROFILE=DEFAULT
export INFINITY_IAC_OCI_CONFIG="$HOME/.oci/config" # optional
export INFINITY_IAC_OCI_REGION=us-phoenix-1
export INFINITY_IAC_OCI_COMPARTMENT=ocid1.compartment.oc1..example
export INFINITY_IAC_OCI_SUBNET=ocid1.subnet.oc1..example
export INFINITY_IAC_OCI_AVAILABILITY_DOMAIN=PHX-AD-1
export INFINITY_IAC_OCI_IMAGE=ocid1.image.oc1..example
export INFINITY_IAC_OCI_SHAPE=VM.Standard.E4.Flex
make test-oci-integration
```

This test reads the OCI private key referenced by the local config and performs
only compartment, subnet, image, availability-domain, and instance-list reads.
It never creates or deletes an instance. Do not put the OCI config or private
key in the repository, CI logs, or issue comments.

Update user-facing release notes and `TODO.md` as needed. Do not put OCI
private keys or other credentials in the repository or GitHub Actions
variables.

## Publish a release

Create a semver tag and push it to `master`:

```sh
git tag v1.0.1
git push origin v1.0.1
```

The workflow runs on tags matching `v*.*.*` and performs these stages:

1. Cross-compiles Linux amd64/arm64, macOS amd64/arm64, and Windows amd64
   binaries with `CGO_ENABLED=0`.
2. Verifies the Windows binary before passing it to Inno Setup.
3. Builds `infinity-iac-setup.exe` for per-user Windows installation.
4. Adds the Unix `install.sh` script and creates a deterministic `SHA256SUMS`.
5. Runs `scripts/verify-release.sh` against every required asset.
6. Creates the GitHub Release with generated notes and all release assets.

Monitor the run and release with GitHub CLI:

```sh
gh run list --workflow release.yml --limit 5
gh run watch RUN_ID
gh release view v1.0.1
```

The release must contain these assets:

```text
infinity-iac_linux_amd64
infinity-iac_linux_arm64
infinity-iac_darwin_amd64
infinity-iac_darwin_arm64
infinity-iac_windows_amd64.exe
infinity-iac-setup.exe
install.sh
SHA256SUMS
```

## Verify installation URLs

After publication, test the exact assets referenced by the installer:

```sh
curl -fsSI https://github.com/M1778/infinity-iac/releases/download/v1.0.1/infinity-iac_linux_amd64
curl -fsSI https://github.com/M1778/infinity-iac/releases/latest/download/SHA256SUMS
```

Use a clean temporary prefix for an end-to-end Unix installer check:

```sh
tmp_prefix="$(mktemp -d)"
curl -fsSL https://raw.githubusercontent.com/M1778/infinity-iac/master/install.sh \
  | sh -s -- --version v1.0.1 --prefix "$tmp_prefix"
"$tmp_prefix/infinity-iac" version
rm -rf "$tmp_prefix"
```

Windows installation should be checked on a clean user profile. Confirm that
the binary launches, the optional user `PATH` entry works in a new shell, and
uninstall removes only the path entry managed by the installer.

## Go package publication

The repository is a Go module, but the current public surface is the CLI. The
provider, domain, recipe, and config packages live under `internal/` and cannot
be imported by another module. Publishing an `infinity-iac` library therefore
requires a deliberate public package boundary and its compatibility policy.
The CI workflow for publishing that package is tracked in `TODO.md`; do not
describe the current CLI release as an SDK release until that work is complete.

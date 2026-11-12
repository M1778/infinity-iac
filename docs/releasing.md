# Releasing Infinity-IaC

Releases use `.github/workflows/release.yml` and `v*.*.*` tags. A source merge
alone does not publish installer assets. Never move a published tag or bypass
protected checks.

## Before tagging

Run the complete local checks from a clean reviewed checkout:

```sh
make fmt-check vet test-race validate-recipes build
go run ./cmd/docs-data --check
npm ci --ignore-scripts
npx --no-install playwright install --with-deps chromium
npm run test:docs
sh -n install.sh scripts/verify-release.sh
git diff --check
./bin/infinity-iac --version
```

The source/default `internal/cli.Version` must match the tag without its `v`.
Release jobs check source version, test/vet, inject the tag version into every
binary, and verify native Linux and Windows reports. Formatting includes the
public root package, not just internal directories.

Review the final diff for secrets. Do not commit OCI config, private keys,
application input secrets, raw live logs, cloud journals, local harness settings,
node_modules, or temporary tests. Runtime counts and blocked tests must be
reported honestly in catalog docs/notes.

## Live checks are opt-in

Read-only credential tests use `INFINITY_IAC_OCI_INTEGRATION=1` or
`INFINITY_IAC_OCI_AUTH_TEST=1` and the documented local OCI profile/resource
variables. They never create cloud resources.

`scripts/live-test.py` is an explicitly billable, opt-in test using a fresh private
workspace, standard local SDK/config, a separate managed test access name, and
exact-resource cleanup. It requires `INFINITY_IAC_LIVE_TEST=1` and `--yes`,
explicit image/domain/SSH source, bounded deadline, and a independently tested
cleanup path before allocation. Prepare/check the journal and arm an approved
cleanup fallback. Never enable it in ordinary CI or target personal resources.
The Python OCI SDK and SSH utilities must already be installed for this test.

If allocation fails with quota/auth/invalid configuration, stop. Do not switch
to personal resources, retry a service-limit fault indefinitely, or report guest
lifecycle as tested. Verify cleanup and preserve private evidence.

## PR and tag publication

1. Commit reviewed project files only and push the feature branch.
2. Open a PR and pass Linux/macOS/Windows and documentation checks.
3. Merge through protection and wait for master CI and Pages validation.
4. Create a new semver tag on the exact verified merge commit, then push it.

For the 1.1.0 source, after the merge is verified:

```sh
git tag v1.1.0 VERIFIED_MERGE_COMMIT
git push origin v1.1.0
gh run list --workflow release.yml --limit 5
gh run watch RUN_ID --exit-status
gh release view v1.1.0
```

Build/package jobs have read-only repository access; publication alone receives
contents write. Pages write/ID-token permissions belong only to deployment,
not browser tests. Do not store OCI secrets in GitHub workflows.

## Assets and fresh installation

The workflow produces:

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

Checksums cover every required binary/installer. Before publishing, the verifier
checks native version as well as content. After publication, download the exact
tag assets, run checksum verification, and test a clean temporary install prefix:

```sh
tmp_prefix="$(mktemp -d)"
sh install.sh --version v1.1.0 --prefix "$tmp_prefix"
"$tmp_prefix/infinity-iac" --version
"$tmp_prefix/infinity-iac" list recipes --output json
"$tmp_prefix/infinity-iac" recipe render --name oc-hunter
```

Expected version is `1.1.0`, with the documented embedded inventory and commands.
Do not replace a working system-wide executable until fresh installation passes.
Windows installation/uninstallation additionally needs a clean user-profile
check; successful cross-compilation or Inno packaging is not that check.

Open the actual Pages URL after deployment and check mobile/desktop, themes,
keyboard/menu/filter/copy behavior and source links, not merely the upload job.

## Go module publication and compatibility

The root `infinityiac` package is importable without exposing raw unchecked OCI
clients. It aliases selected domain/request/recipe contracts and delegates to
guarded provider implementations. Offline entry points include `BuiltinCatalog`,
`RenderCloudInit(Recipe, RenderOptions)`, `RenderLocalScript`, typed input
resolution, and `NewPlan`. Cloud constructors use real local OCI config; callers
must enforce confirmation and context deadlines.

The same semver Git tag publishes the Go module; no separate credential-backed
registry workflow is necessary. External-package tests verify the boundary.
Older tags do not contain this public surface. Avoid breaking exported contracts
without a deliberate version change and compatibility tests.

## Known limits

Keep unverified app/platform/backup and live-compute coverage explicit. Current
first-party networking was exercised live with confirmed cleanup; A1 compute
allocation was blocked by service limits, not passed. Do not describe every
experimental catalog definition as runtime certified.

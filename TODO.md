# Infinity-IaC development TODO

## Distribution and installation

- [x] Add a repository `install.sh` installer that can be fetched directly from GitHub and installs the latest Infinity-IaC release binary.
  - Detect Linux and macOS operating systems and `amd64`/`arm64` architectures.
  - Download the matching GitHub Release artifact and verify its published checksum before installation.
  - Install to a user-writable directory by default, with an explicit override for system-wide installation.
  - Print the installed version and the PATH action the user may need to take.
  - Keep the script safe for noninteractive use and fail clearly when the platform or artifact is unsupported.
- [x] Add a Windows `setup.exe` installer for Infinity-IaC.
  - Install and uninstall the selected release cleanly.
  - Add the installation directory to the user PATH.
  - Preserve cross-platform shell completion setup guidance.
  - Verify the downloaded binary against the published checksum before packaging or installing it.
- [x] Extend GitHub Actions release automation to build, package, checksum, and publish `install.sh` and `setup.exe` alongside the existing platform binaries.
  - Test the shell installer on supported Linux and macOS runners.
  - Build the Windows installer from the tagged release source and attach it to the same GitHub Release.
  - Keep artifact names and release metadata deterministic so the installers can resolve the correct version.

## GitHub releases and Go packages

- [x] Keep tag-driven GitHub Release CI publishing the platform binaries,
  `install.sh`, Windows `setup.exe`, and `SHA256SUMS`.
  - Build from `v*.*.*` tags only.
  - Verify every required asset before `gh release create`.
  - Document the release and rollback checks in `docs/releasing.md`.
- [ ] Publish an importable `infinity-iac` Go library/module package.
  - Move or expose only the stable public API from the current `internal/`
    implementation; keep provider credentials and lifecycle invariants behind
    that API.
  - Add package API, compatibility, documentation, and focused tests before
    publishing.
  - Add a protected GitHub Actions publish job for the selected registry or
    Go module tag flow, with release provenance and no stored OCI secrets.

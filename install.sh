#!/usr/bin/env sh
set -eu

repository=${INFINITY_IAC_REPOSITORY:-M1778/infinity-iac}
version=${INFINITY_IAC_VERSION:-latest}
prefix=${INFINITY_IAC_PREFIX:-$HOME/.local/bin}

usage() {
  cat <<'EOF'
Install Infinity-IaC from GitHub Releases.

Usage: install.sh [--prefix DIR] [--version VERSION] [--repo OWNER/REPOSITORY]

The default installation directory is ~/.local/bin.
EOF
}

die() {
  printf 'infinity-iac installer: %s\n' "$*" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case $1 in
    --prefix) [ "$#" -ge 2 ] || die "--prefix requires a directory"; prefix=$2; shift 2 ;;
    --version) [ "$#" -ge 2 ] || die "--version requires a release tag"; version=$2; shift 2 ;;
    --repo) [ "$#" -ge 2 ] || die "--repo requires OWNER/REPOSITORY"; repository=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v uname >/dev/null 2>&1 || die "uname is required"

case $repository in
  ""|*/*/*|*/|/*) die "repository must be OWNER/REPOSITORY" ;;
  */*) ;;
  *) die "repository must be OWNER/REPOSITORY" ;;
esac

os=$(uname -s)
case $os in
  Linux) target_os=linux ;;
  Darwin) target_os=darwin ;;
  *) die "unsupported operating system: $os" ;;
esac

machine=$(uname -m)
case $machine in
  x86_64|amd64) target_arch=amd64 ;;
  arm64|aarch64) target_arch=arm64 ;;
  *) die "unsupported architecture: $machine" ;;
esac

case $version in
  latest) release_base=https://github.com/$repository/releases/latest/download ;;
  v*) release_base=https://github.com/$repository/releases/download/$version ;;
  *) version=v$version; release_base=https://github.com/$repository/releases/download/$version ;;
esac

asset=infinity-iac_${target_os}_${target_arch}
tmpdir=$(mktemp -d 2>/dev/null || mktemp -d -t infinity-iac)
trap 'rm -rf "$tmpdir"' EXIT HUP INT TERM

binary=$tmpdir/$asset
checksums=$tmpdir/SHA256SUMS
curl --fail --location --silent --show-error "$release_base/$asset" --output "$binary" || die "could not download $asset"
curl --fail --location --silent --show-error "$release_base/SHA256SUMS" --output "$checksums" || die "could not download SHA256SUMS"

expected=$(awk -v file="$asset" '$2 == file { print $1; exit }' "$checksums")
[ -n "$expected" ] || die "checksum entry for $asset is missing"
case $expected in
  *[!0123456789abcdefABCDEF]*) die "invalid checksum for $asset" ;;
esac
[ "${#expected}" -eq 64 ] || die "invalid checksum for $asset"

if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$binary" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$binary" | awk '{print $1}')
else
  die "sha256sum or shasum is required to verify the release"
fi
[ "$actual" = "$expected" ] || die "checksum verification failed for $asset"

mkdir -p "$prefix" || die "could not create $prefix"
target=$prefix/infinity-iac
if command -v install >/dev/null 2>&1; then
  install -m 0755 "$binary" "$target" || die "could not install to $target"
else
  cp "$binary" "$target" && chmod 0755 "$target" || die "could not install to $target"
fi

printf 'Installed Infinity-IaC release %s at %s\n' "$version" "$target"
case :$PATH: in
  *:$prefix:*) ;;
  *) printf 'Add %s to PATH to run infinity-iac.\n' "$prefix" ;;
esac

#!/usr/bin/env sh
set -eu

dist=${1:-dist}
checksums=$dist/SHA256SUMS

die() {
  printf 'release verification: %s\n' "$*" >&2
  exit 1
}

[ -d "$dist" ] || die "artifact directory does not exist: $dist"
[ -f "$checksums" ] || die "missing $checksums"

required='infinity-iac_linux_amd64
infinity-iac_linux_arm64
infinity-iac_darwin_amd64
infinity-iac_darwin_arm64
infinity-iac_windows_amd64.exe
infinity-iac-setup.exe
install.sh'

for asset in $required; do
  [ -f "$dist/$asset" ] || die "missing release asset: $asset"
  expected=$(awk -v file="$asset" '$2 == file { print $1; exit }' "$checksums")
  [ -n "$expected" ] || die "missing checksum entry: $asset"
  case $expected in
    *[!0123456789abcdefABCDEF]*) die "invalid checksum entry: $asset" ;;
  esac
  [ "${#expected}" -eq 64 ] || die "invalid checksum entry: $asset"
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$dist/$asset" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$dist/$asset" | awk '{print $1}')
  else
    die "sha256sum or shasum is required"
  fi
  [ "$actual" = "$expected" ] || die "checksum mismatch: $asset"
done

printf 'release artifacts verified in %s\n' "$dist"

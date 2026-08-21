#!/usr/bin/env bash
set -euo pipefail

# Builds every DevParity release asset and writes the basename checksum
# manifest, using exactly the flags and asset mapping the release workflow
# ships. This script is the single source of truth shared by release.yml, the
# CI smoke step, and the Go smoke test so that a dry run exercises the same
# code path as production.
#
# Environment:
#   VERSION  (required) release version, e.g. v0.1.0-beta.1
#   DEST     (optional) output directory, defaults to ./dist

: "${VERSION:?VERSION is required}"
DEST="${DEST:-dist}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

mkdir -p "$DEST"
assets=()
for target in \
  linux-amd64 linux-arm64 \
  darwin-amd64 darwin-arm64 \
  windows-amd64.exe; do
  case "$target" in
    linux-amd64) goos=linux; goarch=amd64; asset=devparity-linux-amd64 ;;
    linux-arm64) goos=linux; goarch=arm64; asset=devparity-linux-arm64 ;;
    darwin-amd64) goos=darwin; goarch=amd64; asset=devparity-darwin-amd64 ;;
    darwin-arm64) goos=darwin; goarch=arm64; asset=devparity-darwin-arm64 ;;
    windows-amd64.exe) goos=windows; goarch=amd64; asset=devparity-windows-amd64.exe ;;
  esac
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "-s -w -X github.com/wenn-id/devparity/internal/cli.Version=${VERSION}" \
    -o "${DEST}/${asset}" ./cmd/devparity
  assets+=("$asset")
done

# Every asset must carry the expected embedded version.
for asset in "${assets[@]}"; do
  if ! grep -a -F -- "$VERSION" "${DEST}/${asset}" >/dev/null; then
    echo "embedded version mismatch in $asset" >&2
    exit 1
  fi
done

# The native asset must report the version from its own subcommand.
native_os="$(go env GOOS)"
native_arch="$(go env GOARCH)"
native_asset="devparity-${native_os}-${native_arch}"
case "$native_os" in
  darwin) native_asset="devparity-darwin-${native_arch}" ;;
  windows) native_asset="devparity-windows-${native_arch}.exe" ;;
esac
test "$("${DEST}/${native_asset}" version)" = "$VERSION"

# Generate a basename checksum manifest that the composite action verifies.
# Emit "<hash>  <basename>" regardless of how sha256sum prefixes binary files
# ("*name" on some coreutils), so both entrypoints match consistently.
: > "$DEST/checksums.txt"
for asset in "${assets[@]}"; do
  printf '%s  %s\n' "$(sha256sum "$DEST/$asset" | awk '{print $1}')" "$asset" >> "$DEST/checksums.txt"
done

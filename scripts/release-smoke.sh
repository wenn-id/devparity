#!/usr/bin/env bash
set -euo pipefail

# End-to-end release + composite-action smoke test. It builds the real release
# assets, verifies each asset's embedded version, then runs the composite
# action's download/verify/run logic against the local release directory over
# file://. No release is published and no network access is required beyond the
# local filesystem. Linux only (Bash + GNU coreutils).
#
# Environment:
#   DEVPARITY_SMOKE_VERSION (optional) release version, defaults to v0.1.0-beta.1

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if ! command -v go >/dev/null 2>&1; then
  echo "release smoke requires go on PATH" >&2
  exit 2
fi
if ! command -v curl >/dev/null 2>&1; then
  echo "release smoke requires curl on PATH" >&2
  exit 2
fi

VERSION="${DEVPARITY_SMOKE_VERSION:-v0.1.0-beta.1}"

DIST="$(mktemp -d)"
WORKSPACE="$(mktemp -d)"
RUNNER_TEMP="$(mktemp -d)"
SUMMARY="$(mktemp)"
trap 'rm -rf "$DIST" "$WORKSPACE" "$RUNNER_TEMP"; rm -f "$SUMMARY"' EXIT

# 1) Build the same assets and manifest as the release workflow.
VERSION="$VERSION" DEST="$DIST" bash "$ROOT/scripts/release-build.sh"

# 2) Every asset must carry the expected embedded version.
for asset in \
  devparity-linux-amd64 \
  devparity-linux-arm64 \
  devparity-darwin-amd64 \
  devparity-darwin-arm64 \
  devparity-windows-amd64.exe; do
  grep -a -F -- "$VERSION" "$DIST/$asset" >/dev/null || {
    echo "embedded version mismatch in $asset" >&2
    exit 1
  }
done

# 3) The native asset must report the version from its own subcommand.
test "$("$DIST/devparity-linux-amd64" version)" = "$VERSION"

# 4) Exercise the composite-action download/verify/run logic against the local
#    release directory. A clean fixture must pass doctor and write a summary.
cp -R "$ROOT/testdata/repos/clean-node/." "$WORKSPACE/"
RUNNER_OS=Linux \
RUNNER_ARCH=X64 \
RUNNER_TEMP="$RUNNER_TEMP" \
GITHUB_WORKSPACE="$WORKSPACE" \
GITHUB_STEP_SUMMARY="$SUMMARY" \
DEVPARITY_VERSION="$VERSION" \
DEVPARITY_STRICT=false \
  bash "$ROOT/scripts/action-entrypoint.sh" "file://$DIST"

# 5) The doctor summary must have been written by the downloaded binary.
test -s "$SUMMARY"

echo "release smoke passed for $VERSION"

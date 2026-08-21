#!/usr/bin/env bash
set -euo pipefail

# Composite-action entrypoint for non-Windows runners. Extracted from
# action.yml so the release smoke test can exercise the exact download,
# checksum-verify, and run logic against a local file:// release directory
# before any tag is published. The shipped action passes no arguments, so its
# base URL is always the public release download location. Tests may pass a
# local release base as the first positional argument.
#
# Environment:
#   RUNNER_OS             (required) e.g. Linux / macOS
#   RUNNER_ARCH           (required) e.g. X64 / ARM64
#   RUNNER_TEMP           (required) runner temp directory
#   DEVPARITY_VERSION     (required) release tag, e.g. v0.1.0-beta.1
#   DEVPARITY_STRICT      (required) "true" to fail when drift is found
#   GITHUB_WORKSPACE      (required) target repository to run doctor against

case "${RUNNER_OS}-${RUNNER_ARCH}" in
  Linux-X64) asset=devparity-linux-amd64 ;;
  Linux-ARM64) asset=devparity-linux-arm64 ;;
  macOS-X64) asset=devparity-darwin-amd64 ;;
  macOS-ARM64) asset=devparity-darwin-arm64 ;;
  *) echo "unsupported runner" >&2; exit 2 ;;
esac
production_release=true
if [[ $# -gt 0 ]]; then production_release=false; fi
base="${1:-https://github.com/wenn-id/devparity/releases/download/${DEVPARITY_VERSION}}"
workdir="$(mktemp -d "${RUNNER_TEMP%/}/devparity.XXXXXX")"
trap 'rm -rf "$workdir"' EXIT
curl --fail --location --silent --show-error --output "$workdir/$asset" "${base}/${asset}"
curl --fail --location --silent --show-error --output "$workdir/checksums.txt" "${base}/checksums.txt"
cd "$workdir"
checksum_line="$(grep -E "^[0-9a-fA-F]{64}  ${asset}$" checksums.txt || true)"
if [[ -z "$checksum_line" ]]; then
  echo "checksum entry missing for $asset" >&2
  exit 1
fi
if [[ "${RUNNER_OS}" == "macOS" ]]; then
  # macOS ships shasum, not GNU sha256sum.
  printf '%s\n' "$checksum_line" | shasum -a 256 -c -
else
  printf '%s\n' "$checksum_line" | sha256sum -c -
fi
if [[ "$production_release" == true ]]; then
  command -v gh >/dev/null 2>&1 || {
    echo "GitHub CLI is required to verify release provenance" >&2
    exit 2
  }
  gh attestation verify "$workdir/$asset" --repo wenn-id/devparity >/dev/null
fi
chmod +x "$asset"
args=(doctor --format github)
if [[ "${DEVPARITY_STRICT}" == "true" ]]; then args+=(--strict); fi
cd "${GITHUB_WORKSPACE:-$OLDPWD}"
"$workdir/$asset" "${args[@]}"

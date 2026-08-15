#!/usr/bin/env bash
set -euo pipefail

# Composite-action entrypoint for non-Windows runners. Extracted from
# action.yml so the release smoke test can exercise the exact download,
# checksum-verify, and run logic against a local file:// release directory
# before any tag is published. Production behavior is unchanged: the base URL
# defaults to the public release download location.
#
# Environment:
#   RUNNER_OS             (required) e.g. Linux / macOS
#   RUNNER_ARCH           (required) e.g. X64 / ARM64
#   RUNNER_TEMP           (required) runner temp directory
#   DEVPARITY_VERSION     (required) release tag, e.g. v0.1.0-beta.1
#   DEVPARITY_STRICT      (required) "true" to fail when drift is found
#   GITHUB_WORKSPACE      (required) target repository to run doctor against
#   DEVPARITY_RELEASE_BASE (optional) download base override (file:// in smoke)

case "${RUNNER_OS}-${RUNNER_ARCH}" in
  Linux-X64) asset=devparity-linux-amd64 ;;
  Linux-ARM64) asset=devparity-linux-arm64 ;;
  macOS-X64) asset=devparity-darwin-amd64 ;;
  macOS-ARM64) asset=devparity-darwin-arm64 ;;
  *) echo "unsupported runner" >&2; exit 2 ;;
esac
base="${DEVPARITY_RELEASE_BASE:-https://github.com/wenn-id/devparity/releases/download/${DEVPARITY_VERSION}}"
workdir="$(mktemp -d "${RUNNER_TEMP%/}/devparity.XXXXXX")"
trap 'rm -rf "$workdir"' EXIT
curl --fail --location --silent --show-error --output "$workdir/$asset" "${base}/${asset}"
curl --fail --location --silent --show-error --output "$workdir/checksums.txt" "${base}/checksums.txt"
cd "$workdir"
if [[ "${RUNNER_OS}" == "macOS" ]]; then
  # macOS ships shasum, not GNU sha256sum.
  grep "  ${asset}$" checksums.txt | shasum -a 256 -c -
else
  grep "  ${asset}$" checksums.txt | sha256sum -c -
fi
chmod +x "$asset"
args=(doctor --format github)
if [[ "${DEVPARITY_STRICT}" == "true" ]]; then args+=(--strict); fi
cd "${GITHUB_WORKSPACE:-$OLDPWD}"
"$workdir/$asset" "${args[@]}"

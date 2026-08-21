# Composite-action entrypoint for Windows runners.
#
# Mirrors scripts/action-entrypoint.sh: downloads the pinned release asset,
# verifies its checksums.txt manifest, and runs `doctor --format github`.
# Extracted from action.yml so the release smoke test can exercise the exact
# download-and-verify logic against a local file:// release directory before
# any tag is published. The shipped action passes no arguments, so its base
# URL is always the public release download location. Tests may pass a local
# release base as the first positional argument.
#
# Environment:
#   RUNNER_OS        (required) "Windows"
#   RUNNER_ARCH      (required) e.g. X64
#   RUNNER_TEMP      (required) runner temp directory
#   DEVPARITY_VERSION (required) release tag, e.g. v0.1.0-beta.1
#   DEVPARITY_STRICT (required) "true" to fail when drift is found
#   GITHUB_STEP_SUMMARY (required) doctor step summary output path
#   GITHUB_WORKSPACE  (required) target repository to run doctor against

param(
  [string]$ReleaseBaseUrl = "https://github.com/wenn-id/devparity/releases/download/$env:DEVPARITY_VERSION"
)

$ErrorActionPreference = 'Stop'

switch ("$env:RUNNER_OS-$env:RUNNER_ARCH") {
  'Windows-X64' { $asset = 'devparity-windows-amd64.exe'; break }
  default { throw "unsupported runner $env:RUNNER_OS-$env:RUNNER_ARCH" }
}

if ([string]::IsNullOrWhiteSpace($env:RUNNER_TEMP)) { throw 'RUNNER_TEMP is not set' }
$workDir = Join-Path $env:RUNNER_TEMP ("devparity-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $workDir -Force | Out-Null

try {
  $binary = Join-Path $workDir $asset
  $checksums = Join-Path $workDir 'checksums.txt'
  Invoke-WebRequest -UseBasicParsing -Uri "$ReleaseBaseUrl/$asset" -OutFile $binary
  Invoke-WebRequest -UseBasicParsing -Uri "$ReleaseBaseUrl/checksums.txt" -OutFile $checksums

  # Shared checksum contract with the bash entrypoint: exactly two spaces
  # before the asset basename, then end-of-line.
  $checksumLine = Get-Content -LiteralPath $checksums |
    Where-Object { $_ -match ('^([0-9a-fA-F]{64})  ' + [regex]::Escape($asset) + '$') } |
    Select-Object -First 1
  if (-not $checksumLine) { throw "checksum entry missing for $asset" }
  $expected = ($checksumLine -split '\s+', 2)[0].ToUpperInvariant()
  $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $binary).Hash.ToUpperInvariant()
  if ($actual -ne $expected) { throw "checksum mismatch for $asset" }

  $arguments = @('doctor', '--format', 'github')
  if ($env:DEVPARITY_STRICT -eq 'true') { $arguments += '--strict' }
  Push-Location $env:GITHUB_WORKSPACE
  try {
    & $binary @arguments
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  } finally {
    Pop-Location
  }
} finally {
  Remove-Item -LiteralPath $workDir -Recurse -Force -ErrorAction SilentlyContinue
}
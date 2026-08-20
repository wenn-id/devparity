# DevParity

DevParity is a small, offline-first Go binary for finding onboarding drift in Node.js repositories. It compares the Node.js and package-manager requirements that are already written in repository files, then reports source locations and deterministic suggestions.

## Quick start

```sh
devparity doctor .
devparity doctor . --strict
devparity doctor . --format json
devparity docs verify .
```

## Installation

DevParity is distributed as a single binary for Linux, macOS, and Windows. A tagged beta release publishes these assets:

- `devparity-linux-amd64` and `devparity-linux-arm64`
- `devparity-darwin-amd64` and `devparity-darwin-arm64`
- `devparity-windows-amd64.exe`
- `checksums.txt`

Download the binary for your runner from the matching [GitHub release](https://github.com/wenn-id/devparity/releases), download `checksums.txt` into the same directory, and verify it before running the program. On Linux:

```sh
grep '  devparity-linux-amd64$' checksums.txt | sha256sum -c -
chmod +x devparity-linux-amd64
./devparity-linux-amd64 doctor .
```

On macOS, use the matching Darwin asset and verify it with:

```sh
grep '  devparity-darwin-arm64$' checksums.txt | shasum -a 256 -c -
chmod +x devparity-darwin-arm64
./devparity-darwin-arm64 doctor .
```

On Windows PowerShell, compare the downloaded executable with its entry in `checksums.txt`:

```powershell
$expected = (Get-Content .\checksums.txt | Where-Object { $_ -match '  devparity-windows-amd64\.exe$' } | Select-Object -First 1) -split '\s+', 2 | Select-Object -First 1
$actual = (Get-FileHash .\devparity-windows-amd64.exe -Algorithm SHA256).Hash.ToLower()
if ($actual -ne $expected) { throw 'checksum mismatch' }
.\devparity-windows-amd64.exe doctor .
```

The release workflow and composite action use the same basename checksum manifest. If no tagged release is available yet, build from source with Go 1.26.5:

```sh
go build -trimpath -o devparity ./cmd/devparity
./devparity doctor .
```

Static commands read only the supported root artifacts: `package.json`, root lock/version files, the root `Dockerfile`, root `README.md`/`CONTRIBUTING.md`, and `.github/workflows/*.yml`/`*.yaml`. They do not start processes, access the network, or write to the target repository.

## What it checks

- Node constraints from `package.json`, `.nvmrc`, `.node-version`, `.tool-versions`, Dockerfile `FROM node:<tag>`, supported workflow setup steps, and simple README prose.
- npm, pnpm, and Yarn declarations, lockfiles, and direct commands.
- Missing package scripts and command differences between marked documentation and supported GitHub Actions commands.
- Marked documentation blocks using an exact adjacent marker:

  ```markdown
  <!-- devparity:run -->
  ```sh
  npm ci
  npm test
  ```
  ```

  Supported fence languages are `sh`, `shell`, `bash`, `powershell`, and `pwsh`. One direct npm, pnpm, or Yarn command is accepted per line; shell operators and substitutions are reported as inconclusive.

Bun is explicitly unsupported in the beta. Dynamic workflow expressions, reusable workflows, composite actions, variable Docker tags, prerelease Node constraints, and ambiguous prose are reported as inconclusive rather than guessed.

## Execution modes

`docs verify` is static by default. Host execution is intentionally unsandboxed and requires both flags every time:

```sh
devparity docs verify . --execute --trust-repository
```

The command prints a warning before execution. Use repeated `--env NAME` to forward named environment variables and `--timeout 10m` to set a timeout. Requested variables are snapshotted before execution: a missing variable is an operational error (exit 2), while an explicitly empty variable is forwarded as `NAME=`. Environment inheritance is otherwise allowlisted and output is capped at 1 MiB per stream. Token-shaped values are redacted before reporting; exact forwarded-value redaction is applied only to values at least six Unicode characters long that contain both letters and non-letters, so ordinary output is not corrupted by values such as `CI=1`, `NODE_VERSION=18.20.1`, or `RUNNER_OS=ubuntu`.

Container execution copies the repository to a temporary workspace, excludes `.git`, `node_modules`, and `.devparity`, rejects symlinks and special files, uses a non-root user, drops capabilities, defaults to no network, and limits CPU, memory, and processes:

```sh
devparity docs verify . --container --node-version 22
devparity docs verify . --container --node-version 22 --allow-network
```

Docker is preferred and Podman is the fallback. PowerShell container fences are skipped. A missing runtime is a skipped result; copy, launch, cleanup, or runtime errors are operational failures.

## Exit codes and output

- `0`: completed successfully; non-strict findings are allowed.
- `1`: strict drift or a documentation command failed.
- `2`: usage, parsing, permission, runtime, or other operational failure.

JSON output uses `schema_version: 1`. Findings sort by source path, line, and rule ID. `doctor --format github` appends Markdown to `GITHUB_STEP_SUMMARY` and requires that environment variable.

## GitHub Action

Use the composite action to run `doctor` and publish its GitHub job summary. The action supports Linux, macOS, and Windows hosted runners and downloads the pinned release asset for the runner architecture, verifies `checksums.txt`, and removes its temporary download directory when it finishes:

```yaml
steps:
  - uses: actions/checkout@v4
  - uses: wenn-id/devparity@v0.1.0-beta.1
    with:
      version: v0.1.0-beta.1
      strict: "true"
```

Inputs:

- `version` — required release tag; defaults to `v0.1.0-beta.1`.
- `strict` — required string boolean; defaults to `false`. Set it to `true` to fail when drift is found.

The action currently maps Linux x64/ARM64, macOS x64/ARM64, and Windows x64. Unsupported runner architectures fail explicitly rather than selecting an unverified asset.

## Security and disclosure

Repository contents are untrusted by default. Prefer static mode or the no-network container mode for repositories you have not reviewed. Host execution is intentionally unsandboxed and requires both `--execute` and `--trust-repository` on every invocation; trust is never persisted. See [SECURITY.md](SECURITY.md) for the private reporting path and supported versions.

## Repository governance

The default `main` branch is protected. Pull requests must pass these required checks on the current head before merging:

- `verify / test (ubuntu-latest)`
- `verify / test (windows-latest)`
- `verify / test (macos-14)`
- `verify / container`

Protection also enforces strict status checks, administrator enforcement, stale-review dismissal, and conversation resolution. Changes are delivered through pull requests; direct force-pushes and branch deletion are disabled.

## Beta status

This is a public-beta design and implementation. The module path is `github.com/wenn-id/devparity`. Public tags and release assets will be published only after their matching CI and verification gates pass.

DevParity has no config file, database, daemon, plugins, telemetry, auto-fix, SARIF output, API, or UI in this beta.

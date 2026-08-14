# DevParity

DevParity is a small, offline-first Go binary for finding onboarding drift in Node.js repositories. It compares the Node.js and package-manager requirements that are already written in repository files, then reports source locations and deterministic suggestions.

## Quick start

```sh
devparity doctor .
devparity doctor . --strict
devparity doctor . --format json
devparity docs verify .
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

The command prints a warning before execution. Use repeated `--env NAME` to forward named environment variables and `--timeout 10m` to set a timeout. Requested variables are snapshotted before execution: a missing variable is an operational error (exit 2), while an explicitly empty variable is forwarded as `NAME=`. Environment inheritance is otherwise allowlisted and output is capped at 1 MiB per stream and redacted before reporting.

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

## Security and disclosure

Do not use host execution on an untrusted repository. Prefer the default static mode or the no-network container mode. If you find a security issue, avoid publishing secrets or an exploit and report the smallest reproducible details to the project maintainers through the repository's private security channel once one is configured.

## Beta status

This is a public-beta design and implementation. The module path is `github.com/wenn-id/devparity`. Public tags and release assets will be published only after their matching CI and verification gates pass.

DevParity has no config file, database, daemon, plugins, telemetry, auto-fix, SARIF output, API, or UI in this beta.

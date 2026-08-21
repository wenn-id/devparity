# Contributing to DevParity

Thanks for helping improve DevParity. The project is a Go CLI that analyzes repository artifacts and can optionally execute marked documentation commands. Contributions should preserve its static-by-default and fail-closed security boundaries.

## Before opening a pull request

1. Read `README.md` and `SECURITY.md`.
2. For a bug, reproduce it on `main` and add a regression test that fails before the fix.
3. Keep changes scoped to the issue. Do not commit generated release assets, credentials, or repository fixtures containing secrets.
4. Run the same checks used by CI:

```sh
gofmt -w .
go mod tidy -diff
go mod verify
go vet ./...
go test ./...
go build -trimpath ./cmd/devparity
```

On Linux, also run `staticcheck ./...` and the release smoke test when changing release or composite-action code:

```sh
DEVPARITY_RELEASE_SMOKE=1 bash scripts/release-smoke.sh
```

## Pull requests

- Use a focused branch and a conventional commit subject where practical (`fix:`, `feat:`, `test:`, `docs:`, or `chore:`).
- Describe the user-visible behavior, security impact, tests run, and any known limitations.
- Link the issue with `Fixes #N` when the pull request fully resolves it.
- Do not bypass required checks, hooks, or review requirements.
- Changes that execute repository content, alter release provenance, or modify security boundaries need explicit tests and security notes.

## Design boundaries

Static analysis must not execute commands, access the network, or write to the target repository. Host execution remains opt-in and requires both `--execute` and `--trust-repository`; container execution should remain least-privileged, no-network by default, and bounded.

## Reporting security issues

Do not open a public issue for an unpatched vulnerability. Follow the private process in [SECURITY.md](SECURITY.md).

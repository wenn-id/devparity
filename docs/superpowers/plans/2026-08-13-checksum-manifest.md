# Release Checksum Manifest Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate basename-compatible release checksums and prove the release/action contract in CI.

**Architecture:** Keep the existing `assets` Bash array as the single source of asset names. Generate `checksums.txt` from inside `dist` so `sha256sum` records basenames, then run a Linux-only CI smoke test that reproduces the release command and the composite action's verification command for every asset.

**Tech Stack:** Bash, GitHub Actions, GNU `sha256sum`, Go contract tests in `internal/cli/action_test.go`.

## Prerequisites

- Go 1.26.5 (or the repository-compatible Go toolchain) is available as `go` on `PATH`.
- `actionlint` is installed and available as `actionlint` on `PATH`.
- A POSIX shell with GNU `sha256sum` is available for the checksum smoke test.

## Global Constraints

- Preserve the five release asset names: `devparity-linux-amd64`, `devparity-linux-arm64`, `devparity-darwin-amd64`, `devparity-darwin-arm64`, and `devparity-windows-amd64.exe`.
- Keep the composite action's existing basename lookup: `grep "  ${asset}$" checksums.txt | sha256sum -c -`.
- Generate the manifest with `(cd dist && sha256sum "${assets[@]}" > checksums.txt)`.
- Keep the smoke test Linux-only because it requires Bash and GNU `sha256sum`.
- Do not add runtime dependencies or change the action's platform mapping.

---

### Task 1: Add the failing checksum contract test

**Files:**
- Modify: `internal/cli/action_test.go`

**Interfaces:**
- Consumes: `.github/workflows/release.yml`, `action.yml`, and `.github/workflows/ci.yml` as text fixtures.
- Produces: A regression test that fails until the release command and CI smoke-test contract are present.

- [ ] **Step 1: Write the failing test**

Extend the action contract test to read all three files, normalize CRLF to LF, and assert:

```go
const manifestCommand = `(cd dist && sha256sum "${assets[@]}" > checksums.txt)`
if !strings.Contains(releaseText, manifestCommand) {
	t.Fatalf("release workflow does not generate basename checksums with %q", manifestCommand)
}
if strings.Contains(releaseText, "sha256sum dist/* > dist/checksums.txt") {
	t.Fatal("release workflow still writes dist-prefixed checksum paths")
}
if !strings.Contains(actionText, `grep "  ${asset}$" checksums.txt | sha256sum -c -`) {
	t.Fatal("action checksum verification no longer expects basename entries")
}
if !strings.Contains(ciText, "Verify release checksum manifest") {
	t.Fatal("CI is missing the release checksum smoke test")
}
```

Also assert that the smoke test contains the same manifest command, the same
`grep`/`sha256sum -c` verification pipeline, and all five asset basenames.

- [ ] **Step 2: Run the focused test and verify it fails**

Run:

```powershell
go test ./internal/cli -run TestReleaseChecksumsUseBasenames -count=1
```

Expected: `FAIL` because the current release workflow still contains
`sha256sum dist/* > dist/checksums.txt` and CI has no smoke-test step.

### Task 2: Generate basename checksums in the release workflow

**Files:**
- Modify: `.github/workflows/release.yml`

**Interfaces:**
- Consumes: the existing `assets` Bash array populated by the build loop.
- Produces: `dist/checksums.txt` with one basename entry per release asset.

- [ ] **Step 1: Replace the incompatible command**

Replace:

```bash
sha256sum dist/* > dist/checksums.txt
```

with:

```bash
(cd dist && sha256sum "${assets[@]}" > checksums.txt)
```

Keep this command after the version checks and before artifact upload/publishing.

- [ ] **Step 2: Run the focused contract test**

Run the command from Task 1. Expected: it still fails until the CI smoke test is added, while the release checksum assertion passes.

### Task 3: Add the exact-manifest CI smoke test

**Files:**
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: the five release asset names and the release/action checksum commands.
- Produces: a Linux CI step that fails on path-format or verification regressions.

- [ ] **Step 1: Add a Linux-only smoke step**

Add this step to the Linux matrix execution after the Go build/test checks:

```yaml
      - if: runner.os == 'Linux'
        name: Verify release checksum manifest
        shell: bash
        run: |
          set -euo pipefail
          dist="$(mktemp -d)"
          trap 'rm -rf "$dist"' EXIT
          assets=(
            devparity-linux-amd64
            devparity-linux-arm64
            devparity-darwin-amd64
            devparity-darwin-arm64
            devparity-windows-amd64.exe
          )
          for asset in "${assets[@]}"; do
            printf '%s\n' "$asset" > "$dist/$asset"
          done
          (cd "$dist" && sha256sum "${assets[@]}" > checksums.txt)
          for asset in "${assets[@]}"; do
            (cd "$dist" && grep "  ${asset}$" checksums.txt | sha256sum -c -)
          done
```

- [ ] **Step 2: Run the focused contract test and shell equivalent**

Run the Go test from Task 1 and execute the smoke block in Git Bash with a
temporary directory. Expected: both pass and every asset reports `OK`.

### Task 4: Full verification and publish

**Files:**
- Verify: `.github/workflows/release.yml`, `.github/workflows/ci.yml`, `action.yml`, and `internal/cli/action_test.go`

**Interfaces:**
- Consumes: all implementation changes from Tasks 1–3.
- Produces: a tested branch and pull request for issue #2.

- [ ] **Step 1: Run formatting and static checks**

```powershell
gofmt -w internal/cli/action_test.go
git diff --check
go vet ./...
go mod verify
```

- [ ] **Step 2: Run the complete Go test suite**

```powershell
go test ./...
```

Expected: all packages pass.

- [ ] **Step 3: Lint the workflows**

```powershell
actionlint
```

Expected: no output and exit code 0.

- [ ] **Step 4: Commit and push**

```powershell
git add .github/workflows/release.yml .github/workflows/ci.yml internal/cli/action_test.go docs/superpowers/plans/2026-08-13-checksum-manifest.md
git commit -m "fix: make release checksums action-compatible"
git push -u origin agent/issue-2-checksum-manifest
```

- [ ] **Step 5: Open the pull request**

Create a draft PR targeting `main` with a body that links issue #2, explains
the `dist/` prefix mismatch, describes the basename manifest fix and CI smoke
test, and lists the verification commands from Step 1–3.

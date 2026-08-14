# Release Gate Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended; execute inline here because the user approved the design). Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make release publishing wait for every CI gate on the exact tag commit while keeping write credentials only in the publish job.

**Architecture:** Move the existing CI matrix and container checks into reusable `.github/workflows/verify.yml`, called by both push/PR CI and tag releases. Release tags run `verify`, then a read-only `package` job builds/uploads assets, then a write-enabled `publish` job downloads those assets and creates the release. Contract tests inspect all workflow files.

**Tech Stack:** GitHub Actions reusable workflows, Go contract tests, Bash, artifact upload/download, Go 1.26.5.

## Global Constraints

- `verify.yml` is triggered only by `workflow_call`.
- Verification jobs use `contents: read`; no verification job may request `contents: write`.
- Release `publish` must declare `needs: [verify, package]` and be the only job with `contents: write`.
- The same reusable verification workflow serves normal CI and tag release verification.
- Preserve `gofmt`/diff, vet, tests, Linux race, matrix builds, checksum smoke, and container integration gates.
- Verification checks out `${{ github.sha }}` so the called workflow tests the caller's exact commit.

---

### Task 1: Add a failing workflow dependency and permission contract test

**Files:**
- Modify: `internal/cli/action_test.go`

**Interfaces:** Reads `.github/workflows/verify.yml`, `.github/workflows/ci.yml`, and `.github/workflows/release.yml`; produces a regression test for reusable workflow, dependency, and permission contracts.

- [ ] **Step 1: Write the failing test**

Add `TestReleaseWaitsForAllVerificationGates` that normalizes each workflow's
line endings and asserts:

```go
if !strings.Contains(verifyText, "workflow_call:") { t.Fatal("verify workflow is not reusable") }
for _, gate := range []string{"gofmt -w .", "go vet ./...", "go test ./...", "go test -race ./...", "go build -trimpath ./cmd/devparity", "Verify release checksum manifest", "DEVPARITY_CONTAINER_TEST: \"1\""} {
	if !strings.Contains(verifyText, gate) { t.Fatalf("verification workflow missing %q", gate) }
}
if !strings.Contains(ciText, "uses: ./.github/workflows/verify.yml") { t.Fatal("CI does not call reusable verification workflow") }
if !strings.Contains(releaseText, "verify:\n    uses: ./.github/workflows/verify.yml") { t.Fatal("release does not call reusable verification workflow") }
if !strings.Contains(releaseText, "publish:\n    needs: [verify, package]") { t.Fatal("publish does not wait for verification and packaging") }
if !strings.Contains(releaseText, "permissions:\n      contents: write\n    steps:") { t.Fatal("publish job lacks isolated write permission") }
if strings.Contains(verifyText, "contents: write") { t.Fatal("verification workflow has write permission") }
```

Also assert `package` has `needs: verify`, `contents: read`, uploads
`release-assets`, and `publish` downloads `release-assets` before running
`gh release create`.

- [ ] **Step 2: Run the focused test and verify it fails**

```powershell
go test ./internal/cli -run TestReleaseWaitsForAllVerificationGates -count=1
```

Expected: `FAIL` because `verify.yml` does not exist and the current release
job has no reusable dependency or isolated package/publish jobs.

### Task 2: Create reusable verification and thin CI caller

**Files:**
- Create: `.github/workflows/verify.yml`
- Modify: `.github/workflows/ci.yml`

**Interfaces:** The reusable workflow consumes caller context and `${{ github.sha }}` and produces all existing CI gates.

- [ ] **Step 1: Create `verify.yml`**

Create a `workflow_call` workflow with `permissions: contents: read`, the
existing OS matrix, and these steps in each matrix job:

```yaml
- uses: actions/checkout@v6
  with:
    ref: ${{ github.sha }}
- uses: actions/setup-go@v7
  with:
    go-version: 1.26.5
- run: gofmt -w .
- run: git diff --exit-code
- run: go vet ./...
- run: go test ./...
- if: runner.os == 'Linux'
  run: go test -race ./...
- run: go build -trimpath ./cmd/devparity
```

Preserve the current Linux checksum smoke step and container job, including
`DEVPARITY_CONTAINER_TEST: "1"`; both must also checkout `${{ github.sha }}`.

- [ ] **Step 2: Replace `ci.yml` jobs with a reusable caller**

Keep the `push`, `pull_request`, read-only permissions, and concurrency:

```yaml
jobs:
  verify:
    uses: ./.github/workflows/verify.yml
    permissions:
      contents: read
```

- [ ] **Step 3: Run the focused contract test**

Run the Task 1 test. Expected: it still fails only on release package/publish
assertions.

### Task 3: Gate and isolate release packaging/publishing

**Files:**
- Modify: `.github/workflows/release.yml`

**Interfaces:** The release workflow consumes the reusable verification workflow and tag `${{ github.sha }}`, then produces a read-only package artifact and isolated write-only publish job.

- [ ] **Step 1: Add the reusable verification job**

Under the tag trigger and top-level `contents: read`, add:

```yaml
  verify:
    uses: ./.github/workflows/verify.yml
    permissions:
      contents: read
```

- [ ] **Step 2: Move asset building into read-only `package`**

Create `package` with `needs: verify`, Ubuntu, `permissions: contents: read`,
checkout `ref: ${{ github.sha }}`, and the existing build/version/checksum
script. Upload `dist` as:

```yaml
- uses: actions/upload-artifact@v7
  with:
    name: release-assets
    path: dist
```

- [ ] **Step 3: Create isolated `publish` job**

Create `publish` with `needs: [verify, package]`, Ubuntu, and
`permissions: contents: write`. It must have no checkout, Go setup, tests, or
build commands. Download and publish only:

```yaml
- uses: actions/download-artifact@v7
  with:
    name: release-assets
    path: dist
- env:
    GH_TOKEN: ${{ github.token }}
  run: gh release create "$GITHUB_REF_NAME" dist/* --generate-notes
```

- [ ] **Step 4: Run the focused contract test**

Run the Task 1 test. Expected: PASS.

### Task 4: Full verification and publish

**Files:** Verify `.github/workflows/verify.yml`, `.github/workflows/ci.yml`, `.github/workflows/release.yml`, and `internal/cli/action_test.go`; include the approved spec and this plan.

- [ ] **Step 1: Run formatting and static checks**

```powershell
gofmt -w internal/cli/action_test.go
git diff --check
go vet ./...
go mod verify
actionlint
```

- [ ] **Step 2: Run the complete Go test suite**

```powershell
go test ./...
```

Expected: all packages pass.

- [ ] **Step 3: Commit and push**

```powershell
git add .github/workflows/verify.yml .github/workflows/ci.yml .github/workflows/release.yml internal/cli/action_test.go docs/superpowers/specs/2026-08-13-release-gates-design.md docs/superpowers/plans/2026-08-13-release-gates.md
git commit -m "ci: gate releases on reusable verification"
git push -u origin agent/issue-3-release-gates
```

- [ ] **Step 4: Open the pull request**

Create a draft PR targeting `main`, link issue #3, explain the independent
workflow race and permission split, and list the verification commands.

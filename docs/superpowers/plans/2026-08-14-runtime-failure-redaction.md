# Runtime Failure Redaction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop repository-controlled container `stderr` from changing ordinary command failures into runtime errors, while redacting token-shaped output from every outward runtime error.

**Architecture:** Keep the existing `CommandFunc` boundary and `ExecutionResult` flow. Classify runtime failures only from a non-nil command error or Docker/Podman exit code `125`; treat all other exits as command findings. Reuse the existing `Redactor` instance for command output and runtime error text.

**Tech Stack:** Go standard library, existing `internal/execute` package, Go `testing` package.

## Global Constraints

- Do not add dependencies or change public APIs.
- Do not classify runtime failures from command-controlled `stdout` or `stderr` text.
- Preserve command output phrases such as `permission denied` when the command exits nonzero normally.
- Redact token-shaped output before returning it in `ExecutionResult` or an error.
- Keep the known Windows limitation: the full suite contains host tests that require POSIX `sh`.

---

### Task 1: Add regression tests for runtime classification and redaction

**Files:**
- Modify: `internal/execute/container_test.go`
- Test target: `internal/execute/container_test.go`

**Interfaces:**
- Consumes: existing `RunContainer`, `commandFunc`, `lookPath`, `NewContainerGrant`, and `model.ExecutionResult`.
- Produces: failing tests that define issue #7 behavior before production code changes.

- [ ] **Step 1: Write the failing spoofed-stderr test**

Add:

```go
func TestRunContainerDoesNotClassifySpoofedRuntimeStderr(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "/fake/docker", nil }
	commandFunc = func(_ context.Context, _ string, _ []string) ([]byte, []byte, int, error) {
		return []byte("permission denied ghp_abcdefghijklmnopqrstuvwxyz123456"), []byte("cannot connect ghp_abcdefghijklmnopqrstuvwxyz123456"), 1, nil
	}

	result, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{Shell: "sh", Script: "false"}, Options{Root: t.TempDir(), NodeVersion: "22"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if result.Status != model.StatusFinding || result.ExitCode != 1 {
		t.Fatalf("result=%#v", result)
	}
	if !strings.Contains(result.Stderr, "cannot connect") {
		t.Fatalf("stderr=%q lost command output", result.Stderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, "ghp_abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("token leaked in result=%#v", result)
	}
}
```

- [ ] **Step 2: Write the failing exit-125 redaction test**

Add a test that returns exit code `125` with token-shaped `stderr`, then asserts the returned runtime error is non-nil and does not contain the token:

```go
func TestRunContainerRedactsExit125RuntimeFailure(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "/fake/docker", nil }
	commandFunc = func(_ context.Context, _ string, _ []string) ([]byte, []byte, int, error) {
		return nil, []byte("runtime unavailable ghp_abcdefghijklmnopqrstuvwxyz123456"), 125, nil
	}

	_, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{Shell: "sh", Script: "true"}, Options{Root: t.TempDir(), NodeVersion: "22"})
	if err == nil {
		t.Fatal("expected runtime error")
	}
	if strings.Contains(err.Error(), "ghp_abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("token leaked in error=%q", err)
	}
}
```

- [ ] **Step 3: Write the failing command-error redaction test**

Add a test that returns a non-nil command error containing a token-shaped string:

```go
func TestRunContainerRedactsCommandError(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "/fake/docker", nil }
	commandFunc = func(_ context.Context, _ string, _ []string) ([]byte, []byte, int, error) {
		return nil, nil, -1, errors.New("cannot start runtime ghp_abcdefghijklmnopqrstuvwxyz123456")
	}

	_, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{Shell: "sh", Script: "true"}, Options{Root: t.TempDir(), NodeVersion: "22"})
	if err == nil {
		t.Fatal("expected runtime error")
	}
	if strings.Contains(err.Error(), "ghp_abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("token leaked in error=%q", err)
	}
}
```

- [ ] **Step 4: Run only the new tests and verify RED**

Run:

```text
go test ./internal/execute -run "TestRunContainer(DoesNotClassifySpoofedRuntimeStderr|RedactsExit125RuntimeFailure|RedactsCommandError)" -count=1
```

Expected: FAIL because the current code classifies the spoofed phrase, treats exit `125` as a normal result, and returns `runErr` without redaction.

- [ ] **Step 5: Commit the regression tests**

```text
git add internal/execute/container_test.go
git commit -m "test: cover container runtime failure spoofing"
```

### Task 2: Implement state-based runtime classification with redaction

**Files:**
- Modify: `internal/execute/container.go`
- Test: `internal/execute/container_test.go`

**Interfaces:**
- Consumes: the failing tests from Task 1 and the existing `CommandFunc` return values.
- Produces: `RunContainer` behavior where only `runErr` and exit code `125` are runtime failures.

- [ ] **Step 1: Create one container redactor and remove stderr marker matching**

Before invoking `commandFunc`, create `redactor := NewRedactor(nil)`. Delete `runtimeFailure(stderr []byte)` and its substring marker list. Do not add a replacement that reads command output text.

- [ ] **Step 2: Redact non-nil command errors before returning them**

Use a string-formatted error so the raw error is not retained in the outward error chain:

```go
if runErr != nil {
	return model.ExecutionResult{}, fmt.Errorf("container runtime failed: %s", redactor.Redact(runErr.Error()))
}
```

- [ ] **Step 3: Classify exit code 125 and redact its stderr**

After the `runErr` check, classify only the Docker/Podman runtime exit code:

```go
if exit == 125 {
	return model.ExecutionResult{}, fmt.Errorf("container runtime failed: %s", redactor.Redact(strings.TrimSpace(string(stderr))))
}
```

- [ ] **Step 4: Reuse the redactor for normal output and timeout text**

Build the result with `redactor.Redact(string(stdout))` and `redactor.Redact(string(stderr))`. When appending `commandContext.Err()`, pass its error string through the same redactor.

- [ ] **Step 5: Run the new tests and verify GREEN**

```text
go test ./internal/execute -run "TestRunContainer(DoesNotClassifySpoofedRuntimeStderr|RedactsExit125RuntimeFailure|RedactsCommandError)" -count=1
```

Expected: PASS with zero failures.

- [ ] **Step 6: Run the complete execute package tests**

```text
go test ./internal/execute -count=1
```

Expected: PASS, except for the existing Windows `sh` limitation if POSIX `sh` is unavailable; capture the exact result.

- [ ] **Step 7: Commit the implementation**

```text
git add internal/execute/container.go
git commit -m "fix: classify container runtime failures by state"
```

### Task 3: Verify repository-wide behavior and inspect the final diff

**Files:**
- Verify: `internal/execute/container.go`
- Verify: `internal/execute/container_test.go`
- Verify: `docs/superpowers/specs/2026-08-14-runtime-failure-redaction-design.md`

**Interfaces:**
- Consumes: committed implementation and regression tests.
- Produces: fresh verification evidence and a reviewable diff ready for push.

- [ ] **Step 1: Run the repository-wide test suite**

```text
go test ./... -count=1
```

Record whether the only failures are the known host tests requiring POSIX `sh` on Windows. Any new failure must be fixed before delivery.

- [ ] **Step 2: Check formatting and diff hygiene**

Run each command separately:

```text
gofmt -w internal/execute/container.go internal/execute/container_test.go
```

```text
git diff --check
```

```text
git status --short
```

Expected: formatted Go files, no whitespace errors, and only intended changes.

- [ ] **Step 3: Re-run focused tests after formatting**

```text
go test ./internal/execute -run "TestRunContainer(DoesNotClassifySpoofedRuntimeStderr|RedactsExit125RuntimeFailure|RedactsCommandError)" -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit any formatting-only update if needed**

```text
git add internal/execute/container.go internal/execute/container_test.go
git commit -m "style: format container execution changes"
```

Skip this commit when the worktree is clean after the implementation commit.

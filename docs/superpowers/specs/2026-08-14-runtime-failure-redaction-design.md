# Runtime Failure Redaction Design

## Goal

Prevent repository-controlled container command `stderr` from reclassifying ordinary command failures as Docker/Podman runtime failures, while ensuring token-shaped output is redacted from every outward runtime error.

## Scope

This change is limited to container execution in `internal/execute/container.go` and its regression tests in `internal/execute/container_test.go`. It does not change host execution, report schemas, CLI flags, or dependencies.

## Behavior

`RunContainer` determines runtime failure from execution state, not arbitrary command output:

- A non-nil `runErr` from `CommandFunc` is a runtime failure.
- Exit code `125` is a runtime failure because Docker and Podman reserve it for a runtime-level `run` error.
- Any other exit code, including nonzero values, produces a normal `ExecutionResult`; nonzero results are findings.
- `stderr` text never changes failure classification.

## Redaction

Create one redactor for the container execution and apply it to `stdout`, `stderr`, the text of `runErr`, and `stderr` included in an exit-125 runtime error. Runtime errors must not expose token-shaped strings even when those strings originate in command-controlled output or a mocked command error.

## Tests

Regression coverage will verify:

1. Exit `1` with spoofed runtime phrases and a GitHub-token-shaped string returns a finding, preserves the phrase as command output, and redacts the token.
2. Exit `125` with token-shaped `stderr` returns a runtime error without exposing the token.
3. A non-nil command error containing a token returns a runtime error without exposing the token.

The existing container argument and network tests remain unchanged. The full suite is expected to retain the known Windows limitation: host tests that invoke POSIX `sh` cannot run when `sh` is absent from PATH.

## Alternatives considered

- Classifying only `runErr` is smaller but fails to distinguish Docker/Podman exit `125` from a command failure.
- Introducing a new typed command-result abstraction is unnecessary for this localized fix and would expand the public surface.

The selected design uses the existing command boundary, exit `125`, and the existing `Redactor`.

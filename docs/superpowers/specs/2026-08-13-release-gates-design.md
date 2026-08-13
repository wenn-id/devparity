# Release Gate Design

## Context

The release workflow is triggered by `v*` tag pushes and currently runs only
`go test ./...` before building and publishing assets. The main CI workflow
runs formatting, vet, tests, race tests, cross-platform builds, checksum smoke
tests, and container integration independently. A tag can therefore publish
before the matching commit's complete verification succeeds.

## Goal

Make release publishing depend on every required verification gate for the
exact tag commit while keeping publish credentials unavailable to verification
jobs.

## Design

Create `.github/workflows/verify.yml` as a reusable workflow triggered only by
`workflow_call`. It contains the existing matrix checks for Linux, Windows, and
macOS, the Linux checksum smoke test, and the Linux container integration job.
The caller supplies no write credentials; the reusable workflow's jobs use the
caller-provided read-only permission.

Change `.github/workflows/ci.yml` into a thin push/pull-request trigger that
calls `verify.yml`. This preserves the existing CI entry point while ensuring
normal CI and release verification share the same implementation.

Change `.github/workflows/release.yml` to have two jobs:

1. `verify` calls `verify.yml` on the tag commit.
2. `publish` runs on Ubuntu with `needs: verify`, has the only
   `contents: write` permission, builds/verifies assets, and publishes the
   release.

The release workflow itself keeps `contents: read` at the top level. The
verification caller job explicitly grants only `contents: read`; the publish
job explicitly grants `contents: write`. No verification step runs in a job
that can publish.

## Exact-commit behavior

For a tag push, the reusable workflow checks out the tag SHA from the caller's
event context. The publish job uses the same tag ref and cannot start until all
matrix and container jobs in `verify` complete successfully.

## Regression coverage

Extend the workflow contract tests to assert:

- `verify.yml` is reusable and contains format, vet, test, race, build,
  checksum, and container gates;
- `ci.yml` calls `verify.yml` for push and pull-request events;
- `release.yml` calls `verify.yml`, has `publish.needs: verify`, and keeps
  write permission only on `publish`;
- no verification job or reusable workflow permission requests `contents: write`.

## Scope

This change reorganizes workflow execution and permissions only. It does not
change Go application behavior, release asset names, checksum format, or the
existing CI gate commands.

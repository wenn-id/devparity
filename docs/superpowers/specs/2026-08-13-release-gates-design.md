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

Change `.github/workflows/release.yml` to have three jobs:

1. `verify` calls `verify.yml` on the tag commit.
2. `package` runs on Ubuntu with `needs: verify`, keeps
   `contents: read`, checks out the tag commit, builds and verifies all
   release assets, and uploads them as the `release-assets` artifact.
3. `publish` runs on Ubuntu with `needs: [verify, package]`, has the only
   `contents: write` permission, downloads the `release-assets` artifact,
   and publishes the release without checking out the repository or
   rebuilding assets.

The release workflow itself keeps `contents: read` at the top level. The
verification caller and package jobs explicitly grant only `contents: read`;
the publish job explicitly grants `contents: write`. No verification or
packaging step runs in a job that can publish.

## Exact-commit behavior

For a tag push, the reusable workflow and the `package` job check out the tag
SHA from the caller's event context. The `publish` job consumes the artifact
created from that same SHA and cannot start until all matrix and container
jobs in `verify`, plus packaging, complete successfully.

## Regression coverage

Extend the workflow contract tests to assert:

- `verify.yml` is reusable and contains format, vet, test, race, build,
  checksum, and container gates;
- `ci.yml` calls `verify.yml` for push and pull-request events;
- `release.yml` calls `verify.yml`, keeps packaging behind `verify`, has
  `publish.needs: [verify, package]`, and keeps write permission only on
  `publish`;
- `package` uploads `release-assets` and `publish` downloads that artifact
  without checkout or build steps;
- no verification job or reusable workflow permission requests `contents: write`.

## Scope

This change reorganizes workflow execution and permissions only. It does not
change Go application behavior, release asset names, checksum format, or the
existing CI gate commands.

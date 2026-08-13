# Release Checksum Manifest Design

## Context

The release workflow currently runs `sha256sum dist/* > dist/checksums.txt`.
GNU `sha256sum` records paths with the `dist/` prefix, while the composite
action downloads each binary into its current directory and verifies a basename
such as `devparity-linux-amd64`. The generated manifest therefore cannot be
consumed by the action.

## Goal

Generate a checksum manifest whose entries use asset basenames and prove in CI
that every published asset can be verified using the exact command in
`action.yml`.

## Design

The release workflow already collects the five built asset names in the
`assets` Bash array. It will generate the manifest from inside `dist`:

```bash
(cd dist && sha256sum "${assets[@]}" > checksums.txt)
```

This keeps `checksums.txt` in `dist` for artifact upload and release publishing,
while making each record compatible with the action's basename lookup. The
existing action checksum command remains unchanged because it already expects
basename records.

## CI smoke test

The Linux CI job will create temporary files named after all five release
assets, generate a manifest with the exact release command, and verify every
record with the exact `grep ... | sha256sum -c -` command used by the action.
The test fails if the workflow regresses to `dist/`-prefixed records or if any
asset cannot be selected and verified from the manifest.

## Regression coverage

The existing workflow contract test will assert that the release workflow uses
the basename-producing checksum command, that the action keeps its basename
lookup, and that the CI smoke test contains the matching generation and
verification commands.

## Scope

This change modifies only release checksum generation and its CI coverage. It
does not change release asset names, platform mapping, download URLs, or the
action's runtime behavior.

# DevParity Public Beta Design

**Status:** Approved for specification  
**Date:** 2026-08-12  
**Target:** Public beta in eight weeks  
**Working name:** DevParity

## Summary

DevParity is a local-first CLI for open-source maintainers. Its first product promise is:

> Run one command in a Node.js repository and see where the documented onboarding contract disagrees with the repository and GitHub Actions.

The public beta follows a static-first model. `devparity doctor` is useful without Docker or Podman and never executes repository code. Executable documentation is a separate capability that requires an explicit trust grant for host execution or an optional container runtime.

The beta deliberately does not attempt full local/CI parity. It establishes the smaller wedge required to validate that broader product direction: evidence-backed detection of repository onboarding drift.

## Product Decisions

| Decision | Choice |
|---|---|
| Initial user | Maintainers of public open-source repositories |
| Initial ecosystem | Node.js |
| Supported package managers | npm, pnpm, and Yarn |
| Unsupported package manager | Bun; reported as unsupported rather than guessed |
| Delivery horizon | Public beta in eight weeks |
| Default operating mode | Static analysis only |
| Host execution | Explicit opt-in on every invocation |
| Container runtime | Optional; Docker and Podman supported |
| CI provider | GitHub Actions, using a documented subset |
| Implementation language | Go |
| Distribution | Cross-platform single binaries with checksums |
| License | Apache-2.0 |

`DevParity` remains a working name until package namespaces and trademark conflicts are checked before public release.

## Goals

The public beta must:

1. Install as one binary without a language runtime.
2. Produce useful findings without configuration or a container runtime.
3. Detect provable Node version and package-manager contradictions.
4. Identify documented or CI package scripts that do not exist.
5. Show possible drift between documented and CI install, test, and build commands.
6. Validate explicitly marked documentation commands statically.
7. Execute marked documentation only after an explicit trust decision.
8. Produce deterministic terminal, JSON, and GitHub job-summary output.
9. Work on Windows, macOS, and Linux.
10. Be tested against 10–20 real public Node.js repositories before beta release.

## Non-goals

The public beta does not include:

- Python, Go, Bun, or non-Node repositories;
- GitLab CI or other CI providers;
- full GitHub Actions emulation or local CI replay;
- a generic normalized task graph;
- automatic file modification or suggested patches;
- SARIF output;
- a database, history, telemetry, daemon, API server, or web UI;
- plugins or a plugin SDK;
- remote execution;
- accounts, authentication, or organization analytics;
- a hosted service;
- automatic selection of a canonical version when sources disagree.

These capabilities require evidence from beta usage before entering scope.

## CLI Contract

### Static repository diagnosis

```text
devparity doctor [path] [--format text|json] [--strict]
```

`path` defaults to the current directory. The command performs no process execution, network access, or repository writes.

Default output is human-readable text. `--format json` emits JSON schema version `1`. Findings do not cause a non-zero exit by default; `--strict` turns findings into a failing result suitable for CI.

### Documentation verification

```text
devparity docs verify [path] [--format text|json]
devparity docs verify [path] --execute --trust-repository [--timeout <duration>] [--env <name>]...
devparity docs verify [path] --container [--allow-network] [--timeout <duration>]
devparity docs verify [path] --container --node-version <version> [--timeout <duration>]
```

Without an execution flag, the command only validates marked blocks and referenced package scripts.

`--execute` and `--container` are mutually exclusive. Host execution is rejected unless `--trust-repository` is present in the same invocation. Trust is never persisted.

Execution timeout defaults to ten minutes. Each `--env <name>` snapshots the current host value before execution. An absent named variable is an operational error reported before execution; an explicitly empty value is valid and is forwarded as `NAME=`.

Container execution derives a concrete Node version from an unambiguous source such as `.nvmrc` or `.node-version`. If only a range or conflicting values exist, execution is `inconclusive` until the user supplies `--node-version`.

### Documentation marker

Only a shell fence immediately preceded by this exact marker is eligible:

````markdown
<!-- devparity:run -->
```sh
npm ci
npm test
```
````

The beta accepts `sh`, `shell`, `bash`, `powershell`, and `pwsh` fences. The entire fence is one script, preserving state between lines. Its working directory is the repository root.

If the requested shell is not available in host mode, the block is `skipped`. Container mode supports the POSIX shell fence types. PowerShell fences require host execution during the beta.

## Supported Repository Evidence

Discovery reads only these paths and patterns:

- `package.json`;
- `package-lock.json`, `npm-shrinkwrap.json`, `pnpm-lock.yaml`, and `yarn.lock`;
- `.nvmrc`, `.node-version`, and `.tool-versions`;
- root `Dockerfile`;
- `README.md` and `CONTRIBUTING.md`;
- `.github/workflows/*.yml` and `.github/workflows/*.yaml`.

The beta does not recursively scan arbitrary Markdown or Dockerfiles.

### Node version evidence

Supported sources are:

- `package.json` field `engines.node`;
- a single version in `.nvmrc` or `.node-version`;
- the `nodejs` entry in `.tool-versions`;
- a literal Node image tag in the root Dockerfile;
- explicit Node requirements in README or CONTRIBUTING lines;
- `actions/setup-node` field `with.node-version`;
- a simple GitHub Actions matrix reference backed by a literal version array.

Dynamic expressions, reusable workflows, generated Docker arguments, aliases without a concrete meaning, and unrecognized prose are reported as unsupported or inconclusive.

### Package-manager evidence

Supported sources are:

- `package.json` field `packageManager`;
- the present lockfile;
- documented npm, pnpm, or Yarn commands;
- npm, pnpm, or Yarn commands in supported GitHub Actions `run` steps.

### Package-script evidence

The beta recognizes direct invocations of package scripts, including:

- `npm run <script>` and the npm aliases `npm test` and `npm start`;
- `pnpm run <script>` and `pnpm <script>`;
- `yarn run <script>` and `yarn <script>`.

Shell indirection, generated commands, nested scripts, and arbitrary JavaScript evaluation are not interpreted.

## Finding Rules

### `node-version-conflict`

Emitted when two or more understood Node constraints have no compatible intersection. The finding lists every conflicting source. DevParity does not choose which source is authoritative.

### `package-manager-conflict`

Emitted when the `packageManager` field, lockfile, or an unambiguous install command names different package managers.

### `missing-package-script`

Emitted when marked documentation or a supported workflow step invokes a package script absent from `package.json`.

### `workflow-command-drift`

Emitted as a warning when understood install, test, or build commands in documentation and GitHub Actions differ. This rule reports evidence without claiming the difference is necessarily wrong.

### Operational findings

Malformed supported files produce a scoped parse finding. Unsupported syntax produces `inconclusive` evidence rather than a guessed result. A failure in one extractor does not stop unrelated extractors or rules.

## Architecture

DevParity uses a small pipeline:

```text
repository files
-> extracted facts
-> deterministic rules
-> findings
-> terminal / JSON / GitHub summary

marked documentation
-> execution plan
-> explicit permission gate
-> host or container executor
-> execution findings
```

### CLI

The CLI parses subcommands and flags, selects the reporter, and creates execution permission only when the required flags are present. Go's standard `flag` package is sufficient for the beta; no CLI framework is required.

### Discovery

Discovery resolves the repository root and returns the supported artifact paths. It follows no symlink that escapes the repository and applies file-size limits before parsing.

### Extractors

Each extractor reads one artifact type and emits facts with source locations. Extractors do not compare facts or execute commands.

### Rules

Rules are pure functions from facts to findings. They do not access the filesystem, environment, clock, or network.

### Docs verifier

The verifier finds exact opt-in markers, parses the adjacent fence, validates recognized package-script references, and creates an execution plan. It never executes the plan itself.

### Executor

The executor accepts an execution plan only with a permission value created by the CLI. Host and container execution share result capture, timeouts, output limits, and redaction but have separate launch paths.

### Reporter

The reporter renders the same ordered findings as terminal text, JSON, or a GitHub job summary. Sorting is stable by path, line, and rule ID.

## Core Data Model

The beta needs only these core records:

```text
SourceRef
  path
  line
  field

Fact
  kind
  subject
  value
  source

Finding
  rule_id
  severity
  status
  message
  evidence[]
  suggestion

DocBlock
  id
  shell
  script
  source

ExecutionResult
  block_id
  mode
  exit_code
  duration
  stdout
  stderr
  status
```

No persistence model is needed.

## Dependencies

The implementation prefers the Go standard library. Two problem-specific dependencies are justified:

- a maintained YAML parser for GitHub Actions;
- a maintained Node-compatible semantic-version parser.

Markdown marker extraction uses a small line scanner rather than a general Markdown renderer. New dependencies require a concrete beta requirement.

## Error Model

Every check ends as `pass`, `finding`, `skipped`, or `inconclusive`.

- `pass`: the supported check completed without drift;
- `finding`: supported evidence proves or suggests drift;
- `skipped`: an optional operation could not run, such as a missing host shell;
- `inconclusive`: the evidence uses unsupported or ambiguous syntax.

### Exit codes

| Code | Meaning |
|---:|---|
| `0` | The scan completed. Non-strict findings may be present. |
| `1` | `--strict` found drift, or an explicitly executed documentation block failed. |
| `2` | CLI usage, repository validation, or an operational failure prevented a valid result. |

JSON output always remains syntactically valid when reporting a handled repository or parse error. A process-level failure before reporting may emit only a diagnostic to stderr and exit `2`.

## Security Model

Repository contents are untrusted by default.

### Static mode

`devparity doctor` and static `devparity docs verify`:

- do not spawn processes;
- do not open network connections;
- do not write to the repository;
- enforce supported-path and file-size limits;
- reject symlink traversal outside the repository.

### Host execution

Host execution:

- requires both `--execute` and `--trust-repository` every time;
- prints that it is not sandboxed before running;
- runs only explicitly marked blocks;
- defaults to a ten-minute timeout, adjustable with `--timeout`;
- caps captured stdout and stderr at 1 MiB per stream;
- inherits only `PATH`, platform temporary-directory variables, `HOME` or `USERPROFILE`, and `SYSTEMROOT` on Windows;
- forwards each additional variable only through a repeated `--env <name>` flag;
- redacts exact forwarded values when they are at least six Unicode characters long, contain letters, and are not common low-entropy states; GitHub and npm token formats, bearer tokens, and PEM private-key bodies are redacted unconditionally before display;
- cannot promise filesystem or network isolation.

The user must trust the repository before selecting this mode.

### Container execution

Container execution:

- uses Docker or Podman already installed by the user;
- copies the repository into a temporary workspace, excluding `.git`, `node_modules`, and DevParity temporary data;
- never mounts the original source as writable;
- runs as a non-root user;
- requests dropped Linux capabilities and no-new-privileges; an unsupported runtime option produces an operational error rather than silently weakening isolation;
- does not mount a container socket;
- disables network by default;
- requires `--allow-network` to enable ordinary container networking;
- defaults to a ten-minute timeout, two CPUs, 2 GiB memory, 256 processes, and 1 MiB output per stream; unsupported CPU, memory, or process limits produce an explicit warning;
- removes the temporary workspace after the run.

The beta does not implement a domain-level network allowlist. Explicit network enablement is the documented security boundary.

### Privacy

DevParity stores no execution history and sends no telemetry. Source code, facts, findings, and command output remain local. Reports contain paths, commands, and evidence and must therefore be reviewed before public sharing.

## Output Contract

Every finding includes:

- rule ID;
- severity;
- status;
- concise message;
- all supporting source references;
- the conflicting raw values;
- a manual remediation suggestion.

JSON has top-level `schema_version: 1`, tool version, repository path, summary counts, and ordered findings. Additive fields are allowed within schema version `1`; removals or semantic changes require a new schema version.

The GitHub Action runs static `doctor`, writes a job summary, and defaults to non-blocking behavior. Maintainers opt into blocking behavior by enabling strict mode.

## Testing Strategy

Testing uses Go's built-in test tooling.

### Extractor tests

Table-driven fixtures cover valid, malformed, missing, ambiguous, and unsupported forms for every artifact type.

### Rule tests

Pure rule tests provide facts as input and assert exact findings. Version-range boundary cases and differing but compatible ranges receive dedicated cases.

### Golden CLI tests

Small fixture repositories produce deterministic terminal and JSON snapshots. Ordering is tested explicitly.

### Security regression tests

Tests must prove that:

- static commands never launch a child process;
- static commands leave repository contents unchanged;
- host execution fails unless both trust flags are present;
- timeouts terminate child processes;
- output limits truncate safely;
- sufficiently specific forwarded secret values are redacted; short, numeric-looking, and common low-entropy values are forwarded without becoming exact substring redaction patterns so ordinary output is not corrupted;
- source symlinks cannot escape the repository;
- container execution has no network by default;
- the original source tree remains unchanged;
- temporary workspaces are removed after success and failure.

### Compatibility matrix

Pull requests run unit and static CLI tests on Windows, macOS, and Linux. Container tests run on Linux for Docker and on a scheduled or release workflow for Podman when the runner supports it.

The package-manager fixture set covers npm, pnpm, and Yarn. Bun fixtures assert an explicit unsupported result.

### Repository corpus

Before beta, DevParity is run manually against 10–20 public Node.js repositories. Minimal sanitized snapshots of relevant files become offline regression fixtures when their licenses permit redistribution. Otherwise, the test records only a synthetic reproduction of the discovered pattern.

## Delivery Plan

### Weeks 1–2: static foundation

- CLI and exit-code contract;
- source, fact, and finding models;
- repository discovery;
- package.json, version-file, lockfile, and Dockerfile extractors;
- Node-version and package-manager rules;
- unit fixtures.

### Week 3: documentation and GitHub Actions

- README and CONTRIBUTING evidence extraction;
- supported GitHub Actions subset;
- package-script extraction;
- unsupported and inconclusive reporting.

### Week 4: findings and reports

- missing-script and workflow-drift rules;
- deterministic terminal reporter;
- JSON schema version `1`;
- golden CLI tests.

### Week 5: executable documentation

- exact marker and fence extraction;
- static docs verification;
- host execution permission gate;
- timeout, environment, output cap, and redaction.

### Week 6: container execution

- Docker and Podman command adapters;
- temporary workspace;
- non-root and no-network defaults;
- container security regression tests.

### Week 7: distribution

- GitHub Action in warning mode;
- Windows, macOS, and Linux release binaries;
- release checksums;
- installation and security documentation;
- demo fixture repository.

### Week 8: public validation

- run against 10–20 public Node.js repositories;
- classify and reduce false positives;
- close security and portability regressions;
- publish the public beta.

## Release Gates

The public beta ships only when:

1. The Windows, macOS, and Linux test matrix passes.
2. Every finding has source evidence and a location.
3. Static scans neither execute processes nor change repository contents.
4. No host command can run without both explicit trust flags.
5. Container security fixtures pass.
6. Terminal and JSON outputs are deterministic.
7. Ten or more public repositories have completed a static scan.
8. DevParity's own README passes static DevParity verification.
9. Release assets include checksums.

## Success Metrics

The beta is validated when:

- a maintainer can install and run `devparity doctor` without configuration;
- static analysis completes in under two seconds on a GitHub-hosted Ubuntu runner using the benchmark fixture of 10,000 files and at most 1 MiB of supported artifacts;
- real contradictions are found in the public-repository corpus;
- false positives are reproducible and can be fixed through rule changes rather than repository-specific exceptions;
- at least three external maintainers can understand and act on a finding without reading DevParity internals.

Finding count is not a success metric. The product is successful when findings reduce onboarding ambiguity.

## Deferred Work

After beta, evidence may justify:

1. Python and Go extractors;
2. broader GitHub Actions semantics or local CI replay;
3. suggested patches;
4. SARIF;
5. baselines and history;
6. signed releases, artifact attestations, and SBOM publication;
7. a plugin protocol;
8. GitLab CI;
9. an optional local UI.

None of these receive beta scaffolding.

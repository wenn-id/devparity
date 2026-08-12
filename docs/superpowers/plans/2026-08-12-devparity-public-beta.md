# DevParity Public Beta Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an eight-week public beta that diagnoses onboarding drift in Node.js repositories and optionally verifies marked documentation through explicitly trusted host or container execution.

**Architecture:** A Go single-binary pipeline discovers an allowlisted set of artifacts, extracts source-located facts, evaluates deterministic rules, and renders stable reports. Executable documentation is separate: marked blocks become plans, an explicit permission gate authorizes execution, and host/container runners return redacted results.

**Tech Stack:** Go 1.26.5 with `go 1.26.0`, Go standard library, `go.yaml.in/yaml/v3 v3.0.5`, `github.com/Masterminds/semver/v3 v3.5.0`, Docker/Podman CLIs, GitHub Actions.

## Global Constraints

- Module path: `github.com/devparity/devparity`; public tags are blocked until ownership of that namespace is confirmed.
- `doctor` works without Go, Node.js, Docker, or Podman installed.
- Beta scope is Node.js with npm, pnpm, and Yarn; Bun returns an explicit unsupported result.
- Static mode reads only `package.json`, supported root lock/version files, the root `Dockerfile`, root `README.md`/`CONTRIBUTING.md`, and `.github/workflows/*.{yml,yaml}`.
- Static commands do not spawn processes, access the network, or write to the target repository.
- Host execution requires `--execute --trust-repository` every time; trust is never persisted.
- Container execution uses a temporary copy, non-root user, no socket, and no network unless explicitly enabled.
- Defaults: ten-minute timeout, 1 MiB per stream, two CPUs, 2 GiB memory, 256 processes.
- Findings sort by path, line, and rule ID. JSON uses `schema_version: 1`.
- Exit codes: `0` completed/non-strict, `1` strict drift or command failure, `2` usage/operational failure.
- No config file, database, daemon, plugins, telemetry, auto-fix, SARIF, API, or UI.
- Use table-driven standard-library tests. Each behavior starts red, turns green, then ends with `go test ./...`.

---

## Planned File Structure

```text
cmd/devparity/main.go
internal/
  analyze/{doctor.go,doctor_test.go,static_safety_test.go}
  cli/{run.go,run_test.go,args.go,args_test.go,doctor.go,doctor_test.go,docs.go,docs_test.go}
  docs/{blocks.go,blocks_test.go,verify.go,verify_test.go}
  execute/{permission.go,redactor.go,redactor_test.go,host.go,host_test.go,container.go,container_test.go,workspace.go,workspace_test.go}
  extract/{jsonpos.go,jsonpos_test.go,packagejson.go,packagejson_test.go,versions.go,versions_test.go,markdown.go,markdown_test.go,workflow.go,workflow_test.go}
  model/{types.go,sort.go,sort_test.go}
  nodecmd/{parse.go,parse_test.go}
  report/{text.go,text_test.go,json.go,json_test.go,github.go,github_test.go}
  repository/{discover.go,discover_test.go,read.go,read_test.go}
  rules/{evaluate.go,versions.go,packages.go,commands.go,evaluate_test.go}
  semverx/{intersect.go,intersect_test.go}
testdata/repos/{clean-node,drifted-node,malicious-docs}/
.github/workflows/{ci.yml,release.yml}
action.yml
go.mod
go.sum
LICENSE
README.md
```

There is no generic extractor registry, plugin interface, persistence layer, or dependency-injection framework.

### Task 1: Bootstrap the module and CLI entry point

**Files:**
- Create: `go.mod`
- Create: `cmd/devparity/main.go`
- Create: `internal/cli/run.go`
- Test: `internal/cli/run_test.go`

**Interfaces:**
- Produces: `cli.Run(args []string, stdout, stderr io.Writer) int`
- Produces: `cli.Version string`, default `"dev"`, replaceable through linker flags
- Consumes: nothing

- [ ] **Step 1: Write the module file and failing test**

```go
// go.mod
module github.com/devparity/devparity

go 1.26.0
```

```go
package cli

import (
    "bytes"
    "strings"
    "testing"
)

func TestRunVersion(t *testing.T) {
    var out, errOut bytes.Buffer
    code := Run([]string{"version"}, &out, &errOut)
    if code != 0 || strings.TrimSpace(out.String()) != "dev" || errOut.Len() != 0 {
        t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
    }
}

func TestRunWithoutCommandIsUsageError(t *testing.T) {
    var out, errOut bytes.Buffer
    if code := Run(nil, &out, &errOut); code != 2 {
        t.Fatalf("code=%d, want 2", code)
    }
}
```

- [ ] **Step 2: Verify red**

Run: `go test ./internal/cli -v`  
Expected: FAIL because `Run` is undefined.

- [ ] **Step 3: Implement the minimal dispatcher and entry point**

```go
// internal/cli/run.go
package cli

import (
    "fmt"
    "io"
)

var Version = "dev"

func Run(args []string, stdout, stderr io.Writer) int {
    if len(args) == 1 && args[0] == "version" {
        fmt.Fprintln(stdout, Version)
        return 0
    }
    fmt.Fprintln(stderr, "usage: devparity <doctor|docs|version>")
    return 2
}
```

```go
// cmd/devparity/main.go
package main

import (
    "os"
    "github.com/devparity/devparity/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr)) }
```

- [ ] **Step 4: Verify green and commit**

Run: `go test ./internal/cli -v && go run ./cmd/devparity version`  
Expected: PASS and stdout `dev`.

```bash
git add go.mod cmd/devparity internal/cli
git commit -m "feat: bootstrap devparity cli"
```

### Task 2: Define stable core records and ordering

**Files:**
- Create: `internal/model/types.go`
- Create: `internal/model/sort.go`
- Test: `internal/model/sort_test.go`

**Interfaces:**
- Produces: `SourceRef`, `Fact`, `Finding`, `DocBlock`, `ExecutionResult`, `Summary`, `Report`
- Produces: `SortFindings([]Finding)` and `Summarize([]Finding) Summary`

- [ ] **Step 1: Write a failing ordering test**

```go
func TestSortFindings(t *testing.T) {
    got := []Finding{
        {RuleID: "z", Evidence: []Fact{{Source: SourceRef{Path: "b.md", Line: 1}}}},
        {RuleID: "b", Evidence: []Fact{{Source: SourceRef{Path: "a.md", Line: 4}}}},
        {RuleID: "a", Evidence: []Fact{{Source: SourceRef{Path: "a.md", Line: 4}}}},
    }
    SortFindings(got)
    if got[0].RuleID != "a" || got[1].RuleID != "b" || got[2].RuleID != "z" {
        t.Fatalf("unexpected order: %#v", got)
    }
}
```

- [ ] **Step 2: Verify red**

Run: `go test ./internal/model -v`  
Expected: FAIL because the records do not exist.

- [ ] **Step 3: Implement exact records**

```go
type Status string
type Severity string
type FactKind string

const (
    StatusPass Status = "pass"
    StatusFinding Status = "finding"
    StatusSkipped Status = "skipped"
    StatusInconclusive Status = "inconclusive"
    SeverityInfo Severity = "info"
    SeverityWarning Severity = "warning"
    SeverityError Severity = "error"
)

type SourceRef struct { Path string `json:"path"`; Line int `json:"line"`; Field string `json:"field"` }
type Fact struct { Kind FactKind `json:"kind"`; Subject string `json:"subject"`; Value string `json:"value"`; Source SourceRef `json:"source"` }
type Finding struct { RuleID string `json:"rule_id"`; Severity Severity `json:"severity"`; Status Status `json:"status"`; Message string `json:"message"`; Evidence []Fact `json:"evidence"`; Suggestion string `json:"suggestion"` }
type DocBlock struct { ID string `json:"id"`; Shell string `json:"shell"`; Script string `json:"script"`; Source SourceRef `json:"source"` }
type ExecutionResult struct { BlockID string `json:"block_id"`; Mode string `json:"mode"`; ExitCode int `json:"exit_code"`; Duration int64 `json:"duration_ms"`; Stdout string `json:"stdout"`; Stderr string `json:"stderr"`; Status Status `json:"status"` }
type Summary struct { Pass int `json:"pass"`; Finding int `json:"finding"`; Skipped int `json:"skipped"`; Inconclusive int `json:"inconclusive"` }
type Report struct { SchemaVersion int `json:"schema_version"`; ToolVersion string `json:"tool_version"`; Repository string `json:"repository"`; Summary Summary `json:"summary"`; Results []Finding `json:"results"` }
```

Use `sort.SliceStable` with first evidence path/line and rule ID. Evidence-less findings sort by empty path and line zero. `Summarize` is one switch over status.

- [ ] **Step 4: Verify and commit**

Run: `go test ./internal/model -v`  
Expected: PASS.

```bash
git add internal/model
git commit -m "feat: define report model"
```

### Task 3: Discover and safely read supported artifacts

**Files:**
- Create: `internal/repository/discover.go`
- Create: `internal/repository/read.go`
- Test: `internal/repository/discover_test.go`
- Test: `internal/repository/read_test.go`

**Interfaces:**
- Produces: `Artifacts` and `Discover(path string) (Artifacts, error)`
- Produces: `Read(root, path string, maxBytes int64) ([]byte, error)`

- [ ] **Step 1: Write failing allowlist and traversal tests**

```go
func TestDiscoverAllowlistedArtifacts(t *testing.T) {
    root := t.TempDir()
    writeFixture(t, root, "package.json", "{}")
    writeFixture(t, root, "docs/ignored.md", "# ignored")
    writeFixture(t, root, ".github/workflows/ci.yml", "jobs: {}")
    got, err := Discover(root)
    if err != nil { t.Fatal(err) }
    if got.PackageJSON != "package.json" || len(got.Workflows) != 1 || len(got.Markdown) != 0 {
        t.Fatalf("unexpected artifacts: %#v", got)
    }
}

func TestReadRejectsEscapingPath(t *testing.T) {
    if _, err := Read(t.TempDir(), "../outside", 1024); err == nil {
        t.Fatal("expected escaping path error")
    }
}
```

The test-local `writeFixture` uses `os.MkdirAll` and `os.WriteFile`.

- [ ] **Step 2: Verify red**

Run: `go test ./internal/repository -v`  
Expected: FAIL because discovery and safe reading are undefined.

- [ ] **Step 3: Implement the allowlist and safe reader**

```go
type Artifacts struct {
    Root string
    PackageJSON string
    Lockfiles []string
    VersionFiles []string
    Dockerfile string
    Markdown []string
    Workflows []string
}
```

`Discover` resolves the root, requires a directory, tests exact root filenames, and globs only `.github/workflows/*.yml` and `*.yaml`. Return slash-normalized, sorted relative paths.

`Read` cleans/joins the path, rejects absolute and escaping paths, resolves symlinks, verifies the result remains inside root, requires a regular file, enforces `maxBytes`, then reads it.

- [ ] **Step 4: Verify and commit**

Run: `go test ./internal/repository -v`  
Expected: PASS; symlink cases may skip only if the OS denies test symlink creation.

```bash
git add internal/repository
git commit -m "feat: discover supported repository artifacts"
```

### Task 4: Parse Node commands, package.json, and lockfiles

**Files:**
- Create: `internal/nodecmd/parse.go`
- Test: `internal/nodecmd/parse_test.go`
- Create: `internal/extract/packagejson.go`
- Test: `internal/extract/packagejson_test.go`
- Create: `internal/extract/jsonpos.go`
- Test: `internal/extract/jsonpos_test.go`

**Interfaces:**
- Produces: `nodecmd.Command` and `nodecmd.Parse(line string) (Command, bool)`
- Produces: `extract.PackageJSON(root, path string) ([]model.Fact, []model.Finding)`
- Produces: `extract.Lockfiles(paths []string) []model.Fact`

- [ ] **Step 1: Write command-parser tests**

```go
type Command struct { Manager, Operation, Script, Raw string }

func TestParse(t *testing.T) {
    tests := []struct{ line, manager, operation, script string }{
        {"npm ci", "npm", "install", ""},
        {"npm run integration", "npm", "script", "integration"},
        {"npm test", "npm", "test", "test"},
        {"pnpm build", "pnpm", "build", "build"},
        {"yarn run lint", "yarn", "script", "lint"},
    }
    for _, tt := range tests {
        got, ok := Parse(tt.line)
        if !ok || got.Manager != tt.manager || got.Operation != tt.operation || got.Script != tt.script {
            t.Fatalf("%q => %#v, %v", tt.line, got, ok)
        }
    }
}
```

Use `strings.Fields`. Shell operators, pipes, assignments, and substitutions return false.

- [ ] **Step 2: Verify red, implement the switch parser, and verify green**

Run before: `go test ./internal/nodecmd -v`  
Expected: FAIL. Implement exact approved forms, rerun, expect PASS.

- [ ] **Step 3: Write failing package extraction tests**

Fixture:

```json
{"engines":{"node":">=20 <23"},"packageManager":"pnpm@10.0.0","scripts":{"test":"node --test","build":"tsc"}}
```

Assert facts `node.constraint`, `package.manager.declared`, and two `package.script` values with fields such as `engines.node` and `scripts.test`.

- [ ] **Step 4: Add exact JSON source positions**

Write a failing `jsonpos_test.go` for nested fields and script names, then implement:

```go
func jsonFieldLines(data []byte) (map[string]int, error)
```

Walk `json.Decoder.Token()` values while maintaining object/array path stacks; call `InputOffset()` after each key and convert the key's byte offset to a 1-based line. Return paths such as `engines.node`, `packageManager`, and `scripts.test`. Duplicate keys at the same path are a parse error rather than silently selecting one.

Run: `go test ./internal/extract -run TestJSONFieldLines -v`  
Expected: PASS after implementation.

- [ ] **Step 5: Implement extraction**

Decode into a typed struct and use `jsonFieldLines` for each fact's line. Malformed or duplicate-key JSON emits `parse-error`/`inconclusive`. Split `packageManager` at its final `@`; npm/pnpm/Yarn are facts, Bun emits `package-manager-unsupported`. Map lockfiles with:

```go
var lockfileManager = map[string]string{
    "package-lock.json": "npm", "npm-shrinkwrap.json": "npm",
    "pnpm-lock.yaml": "pnpm", "yarn.lock": "yarn",
}
```

- [ ] **Step 6: Verify and commit**

Run: `go test ./internal/nodecmd ./internal/extract -v`  
Expected: PASS.

```bash
git add internal/nodecmd internal/extract
git commit -m "feat: extract node package evidence"
```

### Task 5: Extract version files, Dockerfile, and prose requirements

**Files:**
- Create: `internal/extract/versions.go`
- Test: `internal/extract/versions_test.go`
- Create: `internal/extract/markdown.go`
- Test: `internal/extract/markdown_test.go`

**Interfaces:**
- Produces: `VersionFiles(root string, paths []string) ([]model.Fact, []model.Finding)`
- Produces: `Dockerfile(root, path string) ([]model.Fact, []model.Finding)`
- Produces: `MarkdownVersions(root string, paths []string) ([]model.Fact, []model.Finding)`
- Consumes: `repository.Read`

- [ ] **Step 1: Write failing table tests**

```text
.nvmrc:        v22.4.1                   -> 22.4.1
.node-version: 20                        -> 20
.tool-versions nodejs 22.3.0             -> 22.3.0
Dockerfile:    FROM node:22-slim         -> 22
Dockerfile:    FROM node:${NODE_VERSION} -> inconclusive
README.md:     Requires Node.js >=20 <23 -> >=20 <23
```

Every understood value becomes `node.constraint` with the exact line.

- [ ] **Step 2: Verify red**

Run: `go test ./internal/extract -run 'TestVersion|TestDockerfile|TestMarkdownVersion' -v`  
Expected: FAIL because the functions are undefined.

- [ ] **Step 3: Implement bounded parsers**

Version files accept one non-empty logical line. `.tool-versions` accepts the first `nodejs` pair. Docker accepts case-insensitive literal `FROM node:<tag>` and strips image suffixes; variables become inconclusive.

Markdown scans lines with:

```go
regexp.MustCompile(`(?i)\bnode(?:\.js)?\s+(?:version\s+)?(v?[0-9xX*][0-9A-Za-z.*xX^~<>=| -]*)`)
```

Trim punctuation. More than one match on a line is inconclusive.

- [ ] **Step 4: Verify and commit**

Run: `go test ./internal/extract -v`  
Expected: PASS.

```bash
git add internal/extract/versions* internal/extract/markdown*
git commit -m "feat: extract node version evidence"
```

### Task 6: Extract marked documentation and validate scripts

**Files:**
- Create: `internal/docs/blocks.go`
- Test: `internal/docs/blocks_test.go`
- Create: `internal/docs/verify.go`
- Test: `internal/docs/verify_test.go`

**Interfaces:**
- Produces: `docs.Extract(root string, paths []string) ([]model.DocBlock, []model.Finding)`
- Produces: `docs.Validate(blocks []model.DocBlock, facts []model.Fact) []model.Finding`
- Consumes: `nodecmd.Parse` and package-script facts

- [ ] **Step 1: Write failing marker tests**

```go
func TestExtractRequiresExactAdjacentMarker(t *testing.T) {
    input := "<!-- devparity:run -->\n```sh\nnpm ci\nnpm test\n```\n" +
        "```sh\nnpm run ignored\n```\n"
    blocks, findings := extractFixture(t, input)
    if len(findings) != 0 || len(blocks) != 1 { t.Fatalf("blocks=%#v findings=%#v", blocks, findings) }
    if blocks[0].Shell != "sh" || blocks[0].Script != "npm ci\nnpm test" { t.Fatalf("%#v", blocks[0]) }
}
```

Add non-adjacent marker, unterminated fence, unsupported language, and PowerShell acceptance cases.

- [ ] **Step 2: Verify red**

Run: `go test ./internal/docs -run TestExtract -v`  
Expected: FAIL.

- [ ] **Step 3: Implement a three-state line scanner**

Use `bufio.Scanner` with a 1 MiB token cap. States are normal, marker-seen, and in-fence. IDs are `<slash-path>:<opening-line>`. Languages: `sh`, `shell`, `bash`, `powershell`, `pwsh`.

- [ ] **Step 4: Add failing static validation tests**

`npm run missing` without a matching `package.script` yields `missing-package-script`. Unrecognized shell syntax yields an inconclusive result, not a guessed missing script.

- [ ] **Step 5: Implement, verify, and commit**

Split blocks into lines, parse exact commands, and compare recognized scripts. Emit one finding per missing script and pass when every recognized script exists.

Run: `go test ./internal/docs -v`  
Expected: PASS.

```bash
git add internal/docs
git commit -m "feat: validate marked documentation"
```

### Task 7: Parse the supported GitHub Actions subset

**Files:**
- Create: `internal/extract/workflow.go`
- Test: `internal/extract/workflow_test.go`
- Modify: `go.mod`
- Create/Modify: `go.sum`

**Interfaces:**
- Produces: `extract.Workflows(root string, paths []string) ([]model.Fact, []model.Finding)`
- Consumes: YAML v3 nodes and `nodecmd.Parse`

- [ ] **Step 1: Pin YAML and write failing tests**

Run: `go get go.yaml.in/yaml/v3@v3.0.5`.

```yaml
jobs:
  test:
    strategy:
      matrix:
        node: [20, 22]
    steps:
      - uses: actions/setup-node@v6
        with:
          node-version: ${{ matrix.node }}
      - run: npm ci
      - run: npm test
```

Assert two `node.constraint` and two `workflow.command` facts with YAML node lines.

- [ ] **Step 2: Verify red**

Run: `go test ./internal/extract -run TestWorkflow -v`  
Expected: FAIL.

- [ ] **Step 3: Implement YAML-node traversal**

Decode to `yaml.Node`. Traverse only literal job matrices, `jobs.*.steps[]`, `actions/setup-node`, its `with.node-version`, and scalar `run`. Resolve only exact `${{ matrix.<name> }}` backed by a literal local sequence. Reusable workflows, composite actions, dynamic expressions, aliases, and non-scalar runs emit `workflow-unsupported`/`inconclusive`.

- [ ] **Step 4: Test malformed and unsupported workflows**

One broken workflow must not suppress another workflow's facts.

Run: `go test ./internal/extract -run TestWorkflow -v`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/extract/workflow*
git commit -m "feat: extract github actions evidence"
```

### Task 8: Implement stable Node constraint intersection

**Files:**
- Create: `internal/semverx/intersect.go`
- Test: `internal/semverx/intersect_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Produces: `Normalize(raw string) (string, error)`
- Produces: `IntersectsAll(raw []string) (bool, error)`
- Consumes: Masterminds semver

- [ ] **Step 1: Pin semver and write boundary tests**

Run: `go get github.com/Masterminds/semver/v3@v3.5.0`.

```go
func TestIntersectsAll(t *testing.T) {
    tests := []struct{name string; in []string; want bool}{
        {"overlap", []string{">=20 <23", "22.x"}, true},
        {"disjoint", []string{">=20 <22", "22.x"}, false},
        {"exact", []string{"22.4.1", "^22.0.0"}, true},
        {"gap", []string{">1.2.3 <1.2.4", "1.x"}, false},
        {"or", []string{"18.x || 22.x", ">=21 <23"}, true},
    }
    for _, tt := range tests {
        got, err := IntersectsAll(tt.in)
        if err != nil || got != tt.want { t.Fatalf("%s: got=%v err=%v", tt.name, got, err) }
    }
}
```

Empty and prerelease constraints are errors/inconclusive for beta.

- [ ] **Step 2: Verify red**

Run: `go test ./internal/semverx -v`  
Expected: FAIL.

- [ ] **Step 3: Implement boundary-candidate evaluation**

Normalize whitespace/leading `v`; expand bare integers to `<major>.x`; parse every constraint; reject prerelease syntax. Extract numeric boundaries and generate each boundary, previous/next patch, next minor, next major, plus `0.0.0`. Deduplicate and return true if any candidate satisfies every constraint.

Keep generation private as `candidates(raw []string) ([]*semver.Version, error)`.

- [ ] **Step 4: Add table coverage and commit**

Cover caret, tilde, wildcard, comparator, OR, and hyphen ranges around majors 0, 18, 20, 22, and 23.

Run: `go test ./internal/semverx -v`  
Expected: PASS.

```bash
git add go.mod go.sum internal/semverx
git commit -m "feat: compare node version constraints"
```

### Task 9: Evaluate rules and assemble the static analyzer

**Files:**
- Create: `internal/rules/evaluate.go`
- Create: `internal/rules/versions.go`
- Create: `internal/rules/packages.go`
- Create: `internal/rules/commands.go`
- Test: `internal/rules/evaluate_test.go`
- Create: `internal/analyze/doctor.go`
- Test: `internal/analyze/doctor_test.go`
- Test: `internal/analyze/static_safety_test.go`

**Interfaces:**
- Produces: `rules.Evaluate(facts []model.Fact) []model.Finding`
- Produces: `analyze.Doctor(root, toolVersion string) (model.Report, error)`
- Consumes: all extractors, semver intersection, discovery

- [ ] **Step 1: Write one failing test per rule**

Direct-fact cases:

- incompatible/compatible Node constraints -> `node-version-conflict`;
- declared pnpm plus npm lockfile -> `package-manager-conflict`;
- docs/workflow script absent from package scripts -> `missing-package-script`;
- docs `npm test` versus workflow `npm run test:ci` -> `workflow-command-drift`;
- unsupported version syntax.

Assert exact rule IDs, statuses, severity, complete evidence, and non-authoritative suggestions.

- [ ] **Step 2: Verify red**

Run: `go test ./internal/rules -v`  
Expected: FAIL.

- [ ] **Step 3: Implement four concrete rules**

```go
func Evaluate(facts []model.Fact) []model.Finding {
    out := []model.Finding{evaluateVersions(facts), evaluatePackageManager(facts)}
    out = append(out, evaluateMissingScripts(facts)...)
    out = append(out, evaluateWorkflowDrift(facts)...)
    model.SortFindings(out)
    return out
}
```

Do not add a registry or interface.

- [ ] **Step 4: Write and implement analyzer fixture test**

Create `testdata/repos/drifted-node` with all conflict sources. `Doctor` discovers, calls extractors explicitly, appends operational findings, evaluates/sorts, and summarizes. No goroutines.

- [ ] **Step 5: Add the static safety test**

Parse Go imports below `internal/analyze`, `extract`, `repository`, and `rules`; fail on `os/exec` or imports beginning `net`.

Run: `go test ./internal/rules ./internal/analyze -v`  
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/rules internal/analyze testdata/repos/drifted-node
git commit -m "feat: diagnose static repository drift"
```

### Task 10: Render reports and expose `doctor`

**Files:**
- Create: `internal/report/text.go`
- Test: `internal/report/text_test.go`
- Create: `internal/report/json.go`
- Test: `internal/report/json_test.go`
- Create: `internal/report/github.go`
- Test: `internal/report/github_test.go`
- Create: `internal/cli/doctor.go`
- Test: `internal/cli/doctor_test.go`
- Create: `internal/cli/args.go`
- Test: `internal/cli/args_test.go`
- Modify: `internal/cli/run.go`

**Interfaces:**
- Produces: `report.Text(w io.Writer, report model.Report) error`
- Produces: `report.JSON(w io.Writer, report model.Report) error`
- Produces: `report.GitHub(w io.Writer, report model.Report) error`
- Produces: `cli.runDoctor(args []string, stdout, stderr io.Writer) int`
- Consumes: `analyze.Doctor`

- [ ] **Step 1: Write failing golden reporter tests**

Use one fixed report. Text must show rule ID, status, every `path:line`, raw values, and suggestion. JSON ends with newline and decodes to schema version 1. GitHub output is Markdown.

- [ ] **Step 2: Verify red**

Run: `go test ./internal/report -v`  
Expected: FAIL.

- [ ] **Step 3: Implement reporters**

Use `fmt.Fprintf` for text/Markdown and `json.Encoder` with two-space indentation. Reporters do not mutate or resort.

- [ ] **Step 4: Write CLI tests**

```text
doctor <clean>                -> 0
doctor <drifted>              -> 0 with findings
doctor <drifted> --strict     -> 1
doctor <clean> --format json  -> valid schema 1
doctor <clean> --format bad   -> 2
```

- [ ] **Step 5: Support flags before or after the positional path**

Write table tests for `doctor --strict repo`, `doctor repo --strict`, value flags in both positions, two positional paths, and a missing flag value. Implement:

```go
func normalizePathArgs(args []string, valueFlags map[string]bool) (flags []string, path string, err error)
```

The helper recognizes the command's exact bool/value flag names, moves at most one positional path after all flags, and rejects unknown flags, two paths, or missing values. It is not a general CLI framework.

- [ ] **Step 6: Implement with `flag.FlagSet`**

Normalize args, then accept one optional path, `--format text|json`, and `--strict`. Operational errors go to stderr/2. Strict returns 1 if any result is `finding`. Dispatch from `Run`.

- [ ] **Step 7: Verify and commit**

Run: `go test ./internal/report ./internal/cli ./internal/analyze -v`  
Expected: PASS.

Run: `go run ./cmd/devparity doctor testdata/repos/drifted-node`  
Expected: exit 0 with evidence-backed findings.

```bash
git add internal/report internal/cli
git commit -m "feat: expose drift doctor reports"
```

### Task 11: Expose static `docs verify`

**Files:**
- Create: `internal/cli/docs.go`
- Test: `internal/cli/docs_test.go`
- Modify: `internal/cli/run.go`
- Create: `testdata/repos/clean-node/README.md`
- Create: `testdata/repos/clean-node/package.json`

**Interfaces:**
- Produces: static `devparity docs verify`
- Consumes: docs extractor/validator, package facts, reporters

- [ ] **Step 1: Write failing CLI tests**

Test a clean block, missing script, unsupported shell, and malformed fence. Hash the fixture before/after to prove no writes.

- [ ] **Step 2: Verify red**

Run: `go test ./internal/cli -run TestDocs -v`  
Expected: FAIL.

- [ ] **Step 3: Implement static orchestration**

Use `normalizePathArgs` so flags may precede or follow the path, then parse `docs verify [path] --format text|json`. Discover root Markdown, extract blocks/package facts, validate, build schema-1 report, render. Execution flags remain usage errors until Tasks 12–13.

- [ ] **Step 4: Verify and commit**

Run: `go test ./internal/cli ./internal/docs -v`  
Expected: PASS.

Run: `go run ./cmd/devparity docs verify testdata/repos/clean-node`  
Expected: exit 0.

```bash
git add internal/cli testdata/repos/clean-node
git commit -m "feat: expose static docs verification"
```

### Task 12: Add redaction, permission gating, and host execution

**Files:**
- Create: `internal/execute/permission.go`
- Create: `internal/execute/redactor.go`
- Test: `internal/execute/redactor_test.go`
- Create: `internal/execute/host.go`
- Test: `internal/execute/host_test.go`
- Modify: `internal/docs/verify.go`
- Modify: `internal/docs/verify_test.go`
- Modify: `internal/cli/docs.go`
- Modify: `internal/cli/docs_test.go`
- Create: `testdata/repos/malicious-docs/{README.md,package.json}`

**Interfaces:**
- Produces: `NewHostGrant(trusted bool) (Grant, error)`
- Produces: `Options`
- Produces: `RunHost(ctx context.Context, grant Grant, block model.DocBlock, opts Options) (model.ExecutionResult, error)`
- Produces: `docs.ExecutionFindings(blocks []model.DocBlock, results []model.ExecutionResult) ([]model.Finding, error)`

- [ ] **Step 1: Write failing permission/redaction tests**

```go
func TestNewHostGrantRequiresTrust(t *testing.T) {
    if _, err := NewHostGrant(false); err == nil { t.Fatal("expected trust error") }
}

func TestRedactorRemovesForwardedAndKnownTokens(t *testing.T) {
    r := NewRedactor([]string{"exact-secret"})
    got := r.Redact("exact-secret ghp_abcdefghijklmnopqrstuvwxyz123456 npm_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
    if strings.Contains(got, "exact-secret") || strings.Contains(got, "ghp_") || strings.Contains(got, "npm_") {
        t.Fatalf("secret remained: %q", got)
    }
}
```

Add bearer-token and PEM body cases.

- [ ] **Step 2: Verify red, implement grant/redactor, verify green**

Run before: `go test ./internal/execute -v`  
Expected: FAIL. `Grant` fields remain unexported; zero grant invalid. Redactor combines exact values and compiled patterns.

- [ ] **Step 3: Write host-runner tests**

Cover zero grant, success, nonzero exit, 100 ms timeout, 1 MiB truncation per stream, minimal environment, named environment forwarding, redaction, and absent requested variable. Use `sh` on Unix and PowerShell on Windows.

- [ ] **Step 4: Implement host execution**

```go
type Options struct {
    Root string
    Timeout time.Duration
    MaxOutput int64
    EnvNames []string
    AllowNetwork bool
    NodeVersion string
}
```

Use `exec.CommandContext`. Build environment explicitly from PATH, temp variables, HOME/USERPROFILE, SYSTEMROOT, and requested names—not `os.Environ()`. Capture in capped writers and redact before returning.

- [ ] **Step 5: Map execution results into the report contract**

Write failing tests, then implement `ExecutionFindings`. Match result to block ID and emit:

- `docs-command-passed` / info / pass for exit 0;
- `docs-command-failed` / error / finding for nonzero exit;
- `docs-command-skipped` / warning / skipped when the shell is unavailable.

Evidence uses the block source and includes redacted stdout/stderr as facts only when non-empty. A missing or duplicate block ID returns an operational error rather than a guessed finding.

- [ ] **Step 6: Wire flags and exit semantics**

Accept `--execute --trust-repository`, repeated `--env`, and `--timeout`. Reject partial combinations. Print `host execution is not sandboxed` before execution. Append execution findings to the static report. Any `docs-command-failed` returns exit 1; permission, launch, or mapping failures return exit 2.

Run: `go test ./internal/execute ./internal/cli -v`  
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/execute internal/docs internal/cli testdata/repos/malicious-docs
git commit -m "feat: add trusted host docs execution"
```

### Task 13: Add temporary workspaces and Docker/Podman execution

**Files:**
- Create: `internal/execute/workspace.go`
- Test: `internal/execute/workspace_test.go`
- Create: `internal/execute/container.go`
- Test: `internal/execute/container_test.go`
- Modify: `internal/cli/docs.go`
- Modify: `internal/cli/docs_test.go`

**Interfaces:**
- Produces: `CopyWorkspace(root string) (path string, cleanup func() error, err error)`
- Produces: `NewContainerGrant() Grant`
- Produces: `RunContainer(ctx context.Context, grant Grant, block model.DocBlock, opts Options) (model.ExecutionResult, error)`
- Produces: test seam `CommandFunc func(context.Context, string, []string) ([]byte, []byte, int, error)`

- [ ] **Step 1: Write workspace security tests**

Fixture includes files, `.git`, `node_modules`, escaping symlink, and a sentinel. Assert excluded directories, symlink rejection, unchanged source hash, and cleanup after success/failure.

- [ ] **Step 2: Verify red and implement bounded copy**

Run: `go test ./internal/execute -run TestCopyWorkspace -v`  
Expected: FAIL, then PASS after `filepath.WalkDir` implementation. Skip `.git`, `node_modules`, `.devparity`; reject devices, sockets, pipes, and symlinks.

- [ ] **Step 3: Write container argument tests with fake command function**

Assert args contain:

```text
run --rm --user 10001:10001 --cap-drop ALL
--security-opt no-new-privileges --network none
--cpus 2 --memory 2g --pids-limit 256
-v <temporary-copy>:/workspace -w /workspace node:<version>
```

`--allow-network` removes only `--network none`; original root never appears in the mount.

- [ ] **Step 4: Implement runtime/version selection**

Find Docker then Podman. Explicit `--node-version` wins; otherwise accept one concrete, unambiguous `.nvmrc`/`.node-version` fact. Range-only/conflicting evidence is inconclusive before launch. POSIX fences run via `sh -eu -c`; PowerShell container fences are skipped.

- [ ] **Step 5: Add optional live tests and CLI wiring**

`DEVPARITY_CONTAINER_TEST=1` enables a harmless live run that verifies no network, unchanged source, and cleanup. Make `--container` mutually exclusive with host flags. Allow `--allow-network`/`--node-version` only with container mode. Feed results through `docs.ExecutionFindings`; script failures exit 1, while runtime, copy, or cleanup failures exit 2.

Run: `go test ./internal/execute ./internal/cli -v`  
Expected: PASS.

With Docker: `DEVPARITY_CONTAINER_TEST=1 go test ./internal/execute -run TestLiveContainer -v`  
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/execute internal/cli
git commit -m "feat: add isolated container docs execution"
```

### Task 14: Add GitHub summary integration and composite action

**Files:**
- Create: `action.yml`
- Create: `internal/cli/action_test.go`
- Modify: `internal/cli/doctor.go`
- Modify: `internal/report/github.go`

**Interfaces:**
- Produces: `doctor --format github`
- Produces: composite-action inputs `version` and `strict`
- Consumes: release assets and `GITHUB_STEP_SUMMARY`

- [ ] **Step 1: Write failing GitHub-format test**

Set `GITHUB_STEP_SUMMARY` to a temp file, run `doctor --format github`, assert Markdown in that file and empty stdout. Missing summary path is exit 2.

- [ ] **Step 2: Verify red and implement summary output**

Run before: `go test ./internal/cli -run TestDoctorGitHub -v`  
Expected: FAIL. Accept `github`, append/create/write the file, call `report.GitHub`, and propagate write/close errors. Rerun and expect PASS.

- [ ] **Step 3: Create the composite action**

```yaml
name: DevParity Doctor
description: Detect onboarding drift in a Node.js repository
inputs:
  version:
    description: DevParity release tag
    required: true
    default: v0.1.0-beta.1
  strict:
    description: Fail when drift is found
    required: true
    default: "false"
runs:
  using: composite
  steps:
    - shell: bash
      env:
        DEVPARITY_VERSION: ${{ inputs.version }}
        DEVPARITY_STRICT: ${{ inputs.strict }}
      run: |
        set -euo pipefail
        case "${RUNNER_OS}-${RUNNER_ARCH}" in
          Linux-X64) asset=devparity-linux-amd64 ;;
          macOS-X64) asset=devparity-darwin-amd64 ;;
          macOS-ARM64) asset=devparity-darwin-arm64 ;;
          Windows-X64) asset=devparity-windows-amd64.exe ;;
          *) echo "unsupported runner" >&2; exit 2 ;;
        esac
        base="https://github.com/devparity/devparity/releases/download/${DEVPARITY_VERSION}"
        curl -fsSLO "${base}/${asset}"
        curl -fsSLO "${base}/checksums.txt"
        grep "  ${asset}$" checksums.txt | sha256sum -c -
        chmod +x "${asset}"
        args=(doctor --format github)
        if [[ "${DEVPARITY_STRICT}" == "true" ]]; then args+=(--strict); fi
        "./${asset}" "${args[@]}"
```

The action test parses YAML v3 and asserts both inputs, composite mode, and absence of write permissions or `pull_request_target`.

- [ ] **Step 4: Verify and commit**

Run: `go test ./internal/cli -v`  
Expected: PASS.

```bash
git add action.yml internal/cli internal/report
git commit -m "feat: add github action summary"
```

### Task 15: Add CI, release assets, docs, and beta gates

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `.github/workflows/release.yml`
- Create: `LICENSE`
- Create: `README.md`
- Complete: `testdata/repos/clean-node/**`
- Complete: `testdata/repos/drifted-node/**`
- Complete: `testdata/repos/malicious-docs/**`
- Create: `docs/beta-corpus.md`

**Interfaces:**
- Produces: cross-platform binaries, checksums, user/security docs, and offline regression fixtures
- Consumes: all prior tasks

- [ ] **Step 1: Add cross-platform CI**

Use `actions/checkout@v6`, `actions/setup-go@v7`, `go-version: 1.26.5`, top-level `contents: read`, and matrix `ubuntu-latest`, `windows-latest`, `macos-13`, `macos-14`.

```yaml
- run: gofmt -w .
- run: git diff --exit-code
- run: go vet ./...
- run: go test ./...
- if: runner.os == 'Linux'
  run: go test -race ./...
- run: go build -trimpath ./cmd/devparity
```

Add a Linux Docker job with `DEVPARITY_CONTAINER_TEST=1`.

- [ ] **Step 2: Add the release workflow**

For `v*` tags, test then cross-compile with `CGO_ENABLED=0` for Linux amd64/arm64, Darwin amd64/arm64, Windows amd64.

```bash
go build -trimpath -ldflags "-s -w -X github.com/devparity/devparity/internal/cli.Version=${GITHUB_REF_NAME}" -o "dist/${asset}" ./cmd/devparity
```

Generate `checksums.txt`, upload via `actions/upload-artifact@v7`, and publish with `gh release create "$GITHUB_REF_NAME" dist/*`. Only the release job gets `contents: write`.

- [ ] **Step 3: Add license and README**

Use canonical Apache-2.0 text. README covers promise, installation, all commands/markers, unsandboxed host warning, container no-network default, supported/unsupported syntax, exit codes, schema compatibility, disclosure instructions, and namespace caveat.

- [ ] **Step 4: Complete fixtures and run the full suite**

Clean produces passes; drifted triggers all four rules; malicious attempts environment printing, long output/sleep, source writes, symlink escape, and network.

Run:

```bash
go test ./...
go test -race ./...
go vet ./...
gofmt -w .
git diff --exit-code
```

Expected: all commands succeed with no diff.

- [ ] **Step 5: Verify behavior gates**

```bash
go run ./cmd/devparity doctor testdata/repos/clean-node --strict
go run ./cmd/devparity doctor testdata/repos/drifted-node
go run ./cmd/devparity docs verify testdata/repos/clean-node
```

Expected: exit 0. Then:

```bash
go run ./cmd/devparity doctor testdata/repos/drifted-node --strict
```

Expected: exit 1. Fixture hashes before/after static commands must match.

- [ ] **Step 6: Validate 10–20 public repositories**

Record URL, commit SHA, package manager, runtime, true findings, false positives, and unsupported constructs in `docs/beta-corpus.md`. Add redistributable minimal snapshots or synthetic reproductions with source links. Every reproducible false positive gets a regression test. Release requires at least three external maintainers to interpret a finding without internal guidance.

- [ ] **Step 7: Self-dogfood and commit**

Mark DevParity README blocks, then run:

```bash
go run ./cmd/devparity doctor . --strict
go run ./cmd/devparity docs verify .
```

Expected: exit 0.

```bash
git add .github action.yml LICENSE README.md testdata docs/beta-corpus.md
git commit -m "chore: prepare devparity public beta"
```

## Final Verification

Run from a clean checkout:

```bash
go version
go mod verify
gofmt -w .
git diff --exit-code
go vet ./...
go test ./...
go test -race ./...
go build -trimpath ./cmd/devparity
go run ./cmd/devparity doctor . --strict
go run ./cmd/devparity docs verify .
git status --short
```

Expected:

- Go reports 1.26.5;
- module verification succeeds;
- formatting produces no diff;
- vet, tests, race tests, and build succeed;
- self-diagnosis and static docs verification exit 0;
- Git working tree is clean.

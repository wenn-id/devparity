package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wenn-id/devparity/internal/model"
)

func TestWorkflowExtractsMatrixNodeAndCommands(t *testing.T) {
	root := t.TempDir()
	writeWorkflowFixture(t, root, "ci.yml", `name: CI
on: push
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
`)

	facts, findings := Workflows(root, []string{"ci.yml"})
	if len(findings) != 0 {
		t.Fatalf("unexpected findings: %#v", findings)
	}
	if len(facts) != 4 {
		t.Fatalf("facts=%#v, want two node constraints and two commands", facts)
	}
	assertWorkflowFact(t, facts, "node.constraint", "20", "ci.yml", 7)
	assertWorkflowFact(t, facts, "node.constraint", "22", "ci.yml", 7)
	assertWorkflowFact(t, facts, "workflow.command", "npm ci", "ci.yml", 12)
	assertWorkflowFact(t, facts, "workflow.command", "npm test", "ci.yml", 13)
}

func TestWorkflowExtractsLiteralSetupNodeVersion(t *testing.T) {
	root := t.TempDir()
	writeWorkflowFixture(t, root, "literal.yaml", `jobs:
  test:
    steps:
      - uses: actions/setup-node@v6
        with:
          node-version: 22
`)

	facts, findings := Workflows(root, []string{"literal.yaml"})
	if len(findings) != 0 {
		t.Fatalf("unexpected findings: %#v", findings)
	}
	if len(facts) != 1 {
		t.Fatalf("facts=%#v, want one node constraint", facts)
	}
	assertWorkflowFact(t, facts, "node.constraint", "22", "literal.yaml", 6)
}

func TestWorkflowBrokenFileDoesNotSuppressAnotherWorkflow(t *testing.T) {
	root := t.TempDir()
	writeWorkflowFixture(t, root, "broken.yml", "jobs:\n  broken: [\n")
	writeWorkflowFixture(t, root, "good.yml", `jobs:
  test:
    steps:
      - run: pnpm test
`)

	facts, findings := Workflows(root, []string{"broken.yml", "good.yml"})
	if len(facts) != 1 {
		t.Fatalf("facts=%#v, want good workflow fact", facts)
	}
	assertWorkflowFact(t, facts, "workflow.command", "pnpm test", "good.yml", 4)
	if len(findings) != 1 || findings[0].RuleID != "parse-error" || findings[0].Status != model.StatusInconclusive {
		t.Fatalf("findings=%#v, want one inconclusive parse finding", findings)
	}
	if findings[0].Evidence[0].Source.Path != "broken.yml" {
		t.Fatalf("finding evidence=%#v, want broken.yml", findings[0].Evidence)
	}
}

func TestWorkflowUnsupportedFormsAreInconclusive(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "reusable workflow",
			body: `jobs:
  call:
    uses: ./.github/workflows/reusable.yml
`,
		},
		{
			name: "composite action",
			body: `jobs:
  test:
    steps:
      - uses: ./.github/actions/test
`,
		},
		{
			name: "dynamic matrix",
			body: `jobs:
  test:
    strategy:
      matrix:
        node: ${{ fromJSON(vars.NODE_VERSIONS) }}
    steps:
      - uses: actions/setup-node@v6
        with:
          node-version: ${{ matrix.node }}
`,
		},
		{
			name: "dynamic reference",
			body: `jobs:
  test:
    strategy:
      matrix:
        node: [20, 22]
    steps:
      - uses: actions/setup-node@v6
        with:
          node-version: ${{ matrix["node"] }}
`,
		},
		{
			name: "non scalar run",
			body: `jobs:
  test:
    steps:
      - run:
          command: npm test
`,
		},
		{
			name: "alias",
			body: `jobs:
  test:
    strategy:
      matrix:
        node: &versions [20, 22]
    steps:
      - uses: actions/setup-node@v6
        with:
          node-version: ${{ matrix.node }}
`,
		},
		{
			name: "multiline run",
			body: `jobs:
  test:
    steps:
      - run: |
          npm
          ci
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeWorkflowFixture(t, root, "unsupported.yml", test.body)
			facts, findings := Workflows(root, []string{"unsupported.yml"})
			if len(facts) != 0 {
				t.Fatalf("facts=%#v, want no facts", facts)
			}
			if len(findings) == 0 {
				t.Fatal("want inconclusive unsupported finding")
			}
			for _, finding := range findings {
				if finding.Status != model.StatusInconclusive || finding.RuleID != "workflow-unsupported" {
					t.Fatalf("finding=%#v, want workflow-unsupported inconclusive", finding)
				}
			}
		})
	}
}

func TestWorkflowOnlyResolvesExactLocalMatrixReference(t *testing.T) {
	root := t.TempDir()
	writeWorkflowFixture(t, root, "exact.yml", `jobs:
  test:
    strategy:
      matrix:
        node: [20, 22]
        os: [ubuntu-latest]
    steps:
      - uses: actions/setup-node@v6
        with:
          node-version: '${{ matrix.node }}'
      - run: npm test
`)

	facts, findings := Workflows(root, []string{"exact.yml"})
	if len(findings) != 0 {
		t.Fatalf("unexpected findings: %#v", findings)
	}
	if len(facts) != 3 {
		t.Fatalf("facts=%#v, want two node constraints and one command", facts)
	}
	assertWorkflowFact(t, facts, "node.constraint", "20", "exact.yml", 5)
	assertWorkflowFact(t, facts, "node.constraint", "22", "exact.yml", 5)
	assertWorkflowFact(t, facts, "workflow.command", "npm test", "exact.yml", 11)
}

func TestWorkflowIgnoresNonNodeRunSteps(t *testing.T) {
	root := t.TempDir()
	writeWorkflowFixture(t, root, "shell.yml", `jobs:
  verify:
    steps:
      - run: go test ./...
      - run: git diff --exit-code
      - run: docker build .
      - run: make test
      - run: echo done
      - run: echo npm install
      - run: |
          # npm install
          echo still not a Node command
      - run: |
          cat <<'NODE_COMMANDS'
          npm install
          NODE_COMMANDS
          echo done
      - run: |
          cat <<-NODE_COMMANDS
          npx eslint .
          NODE_COMMANDS
          echo done
`)

	facts, findings := Workflows(root, []string{"shell.yml"})
	if len(facts) != 0 || len(findings) != 0 {
		t.Fatalf("facts=%#v findings=%#v, want both empty", facts, findings)
	}
}

func TestWorkflowReportsOnlyMalformedNodeLikeRunSteps(t *testing.T) {
	root := t.TempDir()
	writeWorkflowFixture(t, root, "node-like.yml", `jobs:
  verify:
    steps:
      - run: npm install
      - run: npx eslint .
      - run: bunx prettier .
      - run: |
          echo preparing
          npm install
      - run: |
          cat <<'NODE_COMMANDS'
          npm test
          NODE_COMMANDS
          npm install
`)

	facts, findings := Workflows(root, []string{"node-like.yml"})
	if len(facts) != 1 {
		t.Fatalf("facts=%#v, want npm install fact", facts)
	}
	assertWorkflowFact(t, facts, "workflow.command", "npm install", "node-like.yml", 4)
	if facts[0].Subject != "install" {
		t.Fatalf("fact=%#v, want install subject", facts[0])
	}
	if len(findings) != 3 {
		t.Fatalf("findings=%#v, want three Node-like inconclusive findings", findings)
	}
	for _, finding := range findings {
		if finding.RuleID != "workflow-unsupported" || finding.Status != model.StatusInconclusive {
			t.Fatalf("finding=%#v, want workflow-unsupported inconclusive", finding)
		}
	}
}

func TestWorkflowDoesNotTreatPackageManagerBuiltinsAsScripts(t *testing.T) {
	root := t.TempDir()
	writeWorkflowFixture(t, root, "builtins.yml", `jobs:
  verify:
    steps:
      - run: pnpm update
      - run: pnpm list
      - run: yarn remove
      - run: pnpm run build
`)

	facts, findings := Workflows(root, []string{"builtins.yml"})
	if len(facts) != 4 {
		t.Fatalf("facts=%#v, want three builtin facts and one shorthand script fact", facts)
	}
	assertWorkflowFact(t, facts, "workflow.command", "pnpm run build", "builtins.yml", 7)
	if len(findings) != 0 {
		t.Fatalf("findings=%#v, want no findings for recognized builtins", findings)
	}
}

func TestWorkflowNodeLikeDetectionTreatsHeredocScalarsAsOpaque(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "quoted heredoc payload", value: "cat <<'EOF'\nnpm install\nEOF\necho done", want: false},
		{name: "tab stripped payload", value: "cat <<-EOF\n	npx eslint .\n	EOF\necho done", want: false},
		{name: "multiple payloads", value: "cat <<A <<'B'\nnpm test\nA\nyarn lint\nB\necho done", want: false},
		{name: "node command is literal first token", value: "npm install <<EOF\ndata\nEOF", want: true},
		{name: "ordinary multiline node command", value: "echo preparing\nnpm install", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := workflowCommandLooksNodeLike(test.value); got != test.want {
				t.Fatalf("workflowCommandLooksNodeLike(%q)=%v, want %v", test.value, got, test.want)
			}
		})
	}
}

func assertWorkflowFact(t *testing.T, facts []model.Fact, kind, value, path string, line int) {
	t.Helper()
	for _, fact := range facts {
		if fact.Kind == model.FactKind(kind) && fact.Value == value {
			if fact.Source.Path != path || fact.Source.Line != line {
				t.Fatalf("fact=%#v, want path=%q line=%d", fact, path, line)
			}
			return
		}
	}
	t.Fatalf("missing fact kind=%q value=%q path=%q line=%d in %#v", kind, value, path, line, facts)
}

func writeWorkflowFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

package analyze

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wenn-id/devparity/internal/model"
)

func TestDoctorFindsDriftedNodeRepository(t *testing.T) {
	root := t.TempDir()
	writeAnalyzeFixture(t, root, "package.json", `{
  "engines": {"node": ">=20 <22"},
  "packageManager": "pnpm@10.0.0",
  "scripts": {"test": "node --test"}
}`)
	writeAnalyzeFixture(t, root, "package-lock.json", "{}")
	writeAnalyzeFixture(t, root, ".nvmrc", "22\n")
	writeAnalyzeFixture(t, root, "README.md", "Requires Node.js >=20 <22.\n\n<!-- devparity:run -->\n```sh\nnpm test\nnpm run missing\n```\n")
	writeAnalyzeFixture(t, root, ".github/workflows/ci.yml", "jobs:\n  test:\n    steps:\n      - run: npm run test:ci\n")

	report, err := Doctor(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 1 || report.ToolVersion != "test" {
		t.Fatalf("report=%#v", report)
	}
	for _, rule := range []string{"node-version-conflict", "package-manager-conflict", "missing-package-script", "workflow-command-drift"} {
		if !hasRule(report.Results, rule) {
			t.Fatalf("missing rule %q in %#v", rule, report.Results)
		}
	}
	if report.Summary.Finding == 0 {
		t.Fatalf("summary=%#v", report.Summary)
	}
}

func TestDoctorCleanRepositoryHasNoFindingStatus(t *testing.T) {
	root := t.TempDir()
	writeAnalyzeFixture(t, root, "package.json", `{"engines":{"node":">=20 <23"},"scripts":{"test":"node --test"}}`)
	report, err := Doctor(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Finding != 0 {
		t.Fatalf("summary=%#v results=%#v", report.Summary, report.Results)
	}
}

func TestDocCommandFactsUseScriptLineNumbers(t *testing.T) {
	facts := docCommandFacts([]model.DocBlock{{
		Source: model.SourceRef{Path: "README.md", Line: 4},
		Script: "npm test\nnpm run build",
	}})
	var commandLines []int
	for _, fact := range facts {
		if fact.Kind == model.FactKind("doc.command") {
			commandLines = append(commandLines, fact.Source.Line)
		}
	}
	if len(commandLines) != 2 || commandLines[0] != 5 || commandLines[1] != 6 {
		t.Fatalf("facts=%#v, want command lines 5 and 6", facts)
	}
}

func hasRule(findings []model.Finding, rule string) bool {
	for _, finding := range findings {
		if finding.RuleID == rule && finding.Status == model.StatusFinding {
			return true
		}
	}
	return false
}

func writeAnalyzeFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

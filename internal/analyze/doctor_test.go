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

func TestDoctorReportsEachMissingDocumentationScriptOnce(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "repos", "drifted-node")
	report, err := Doctor(root, "test")
	if err != nil {
		t.Fatal(err)
	}

	var missing []model.Finding
	for _, finding := range report.Results {
		if finding.RuleID == "missing-package-script" && finding.Status == model.StatusFinding {
			for _, evidence := range finding.Evidence {
				if evidence.Subject == "missing" || evidence.Value == "missing" {
					missing = append(missing, finding)
					break
				}
			}
		}
	}
	if len(missing) != 1 {
		t.Fatalf("missing-package-script findings for missing=%d, want 1: %#v", len(missing), missing)
	}
	if len(missing[0].Evidence) != 1 || missing[0].Evidence[0].Kind != model.FactKind("doc.command") || missing[0].Evidence[0].Source.Path != "README.md" || missing[0].Evidence[0].Source.Line != 8 {
		t.Fatalf("finding=%#v, want precise README.md:8 doc.command evidence", missing[0])
	}
	if report.Summary.Finding != 5 {
		t.Fatalf("summary=%#v, want 5 logical findings", report.Summary)
	}
}

func TestDoctorDetectsNodeDriftFromFlaggedDockerImage(t *testing.T) {
	root := t.TempDir()
	writeAnalyzeFixture(t, root, "package.json", `{"engines":{"node":">=22"}}`)
	writeAnalyzeFixture(t, root, "Dockerfile", "FROM --platform=linux/amd64 docker.io/library/node:18\n")

	report, err := Doctor(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !hasRule(report.Results, "node-version-conflict") {
		t.Fatalf("missing Node version conflict in %#v", report.Results)
	}
}

func TestDoctorBunPackageRetainsScriptFacts(t *testing.T) {
	root := t.TempDir()
	writeAnalyzeFixture(t, root, "package.json", `{
  "packageManager": "bun@1.1.0",
  "scripts": {"test": "bun test", "build": "tsc"}
}`)
	writeAnalyzeFixture(t, root, "README.md", "<!-- devparity:run -->\n```sh\nnpm run build\n```\n")

	report, err := Doctor(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !hasRuleWithStatus(report.Results, "package-manager-unsupported", model.StatusInconclusive) {
		t.Fatalf("missing unsupported-manager finding in %#v", report.Results)
	}
	if hasRule(report.Results, "missing-package-script") {
		t.Fatalf("Bun package scripts were reported missing: %#v", report.Results)
	}
}

func TestDoctorInstallAliasesDoNotInventMissingScripts(t *testing.T) {
	root := t.TempDir()
	writeAnalyzeFixture(t, root, "package.json", `{"packageManager":"pnpm@10.0.0","scripts":{"build":"tsc"}}`)
	writeAnalyzeFixture(t, root, "README.md", "<!-- devparity:run -->\n```sh\n# install dependencies\nnpm install\npnpm i\nyarn\npnpm run build\n```\n")
	writeAnalyzeFixture(t, root, ".github/workflows/ci.yml", "jobs:\n  test:\n    steps:\n      - run: pnpm i\n      - run: pnpm run build\n")

	report, err := Doctor(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(report.Results, "missing-package-script") {
		t.Fatalf("install aliases invented missing scripts: %#v", report.Results)
	}
	if hasRuleWithStatus(report.Results, "doc-command-unsupported", model.StatusInconclusive) {
		t.Fatalf("comments or install aliases made docs inconclusive: %#v", report.Results)
	}
}

func TestDoctorBuiltinsPreservePackageManagerEvidenceWithoutMissingScripts(t *testing.T) {
	for _, test := range []struct {
		name  string
		path  string
		value string
	}{
		{name: "documentation", path: "README.md", value: "<!-- devparity:run -->\n```sh\nyarn publish\n```\n"},
		{name: "workflow", path: ".github/workflows/ci.yml", value: "jobs:\n  test:\n    steps:\n      - run: yarn version\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeAnalyzeFixture(t, root, "package.json", `{"packageManager":"pnpm@10.0.0"}`)
			writeAnalyzeFixture(t, root, test.path, test.value)

			report, err := Doctor(root, "test")
			if err != nil {
				t.Fatal(err)
			}
			if !hasRule(report.Results, "package-manager-conflict") {
				t.Fatalf("builtin command lost package-manager evidence: %#v", report.Results)
			}
			if hasRule(report.Results, "missing-package-script") {
				t.Fatalf("builtin command invented missing scripts: %#v", report.Results)
			}
		})
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
	return hasRuleWithStatus(findings, rule, model.StatusFinding)
}

func hasRuleWithStatus(findings []model.Finding, rule string, status model.Status) bool {
	for _, finding := range findings {
		if finding.RuleID == rule && finding.Status == status {
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

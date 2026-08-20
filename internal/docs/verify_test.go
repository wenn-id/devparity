package docs

import (
	"testing"

	"github.com/wenn-id/devparity/internal/model"
)

func TestValidateReportsOneFindingPerMissingScript(t *testing.T) {
	blocks := []model.DocBlock{{
		ID:     "README.md:2",
		Shell:  "sh",
		Script: "npm run missing\nnpm run missing\nnpm test",
		Source: model.SourceRef{Path: "README.md", Line: 2},
	}}
	findings := Validate(blocks, []model.Fact{{
		Kind:    model.FactKind("package.script"),
		Subject: "test",
		Value:   "node --test",
		Source:  model.SourceRef{Path: "package.json", Line: 4, Field: "scripts.test"},
	}})
	if len(findings) != 1 {
		t.Fatalf("findings=%#v", findings)
	}
	finding := findings[0]
	if finding.RuleID != "missing-package-script" || finding.Status != model.StatusFinding {
		t.Fatalf("finding=%#v", finding)
	}
	if len(finding.Evidence) == 0 || finding.Evidence[0].Subject != "missing" {
		t.Fatalf("evidence=%#v", finding.Evidence)
	}
}

func TestValidatePassesWhenRecognizedScriptsExist(t *testing.T) {
	blocks := []model.DocBlock{{
		ID:     "README.md:2",
		Shell:  "bash",
		Script: "npm ci\nnpm test\npnpm run build\nyarn lint",
		Source: model.SourceRef{Path: "README.md", Line: 2},
	}}
	facts := []model.Fact{
		{Kind: model.FactKind("package.script"), Subject: "test", Value: "node --test"},
		{Kind: model.FactKind("package.script"), Subject: "build", Value: "tsc"},
		{Kind: model.FactKind("package.script"), Subject: "lint", Value: "eslint ."},
	}
	findings := Validate(blocks, facts)
	if len(findings) != 1 || findings[0].RuleID != "doc-script-validation" || findings[0].Status != model.StatusPass {
		t.Fatalf("findings=%#v", findings)
	}
}

func TestValidateSkipsCommentsAndRecognizesInstallCommands(t *testing.T) {
	block := model.DocBlock{
		ID:     "README.md:2",
		Shell:  "sh",
		Script: "# install dependencies\n  # package-manager aliases\nnpm install\npnpm i\nyarn\nnpm run build",
		Source: model.SourceRef{Path: "README.md", Line: 2},
	}
	findings := Validate([]model.DocBlock{block}, []model.Fact{{Kind: model.FactKind("package.script"), Subject: "build"}})
	if len(findings) != 1 || findings[0].RuleID != "doc-script-validation" || findings[0].Status != model.StatusPass {
		t.Fatalf("findings=%#v", findings)
	}
	if !CanExecute(block) {
		t.Fatal("commented install block should use the validation command grammar")
	}
}

func TestValidateDoesNotGuessMissingScriptForUnrecognizedSyntax(t *testing.T) {
	blocks := []model.DocBlock{{
		ID:     "README.md:2",
		Shell:  "sh",
		Script: "npm run missing && npm test",
		Source: model.SourceRef{Path: "README.md", Line: 2},
	}}
	findings := Validate(blocks, nil)
	if len(findings) != 1 || findings[0].RuleID != "doc-command-unsupported" || findings[0].Status != model.StatusInconclusive {
		t.Fatalf("findings=%#v", findings)
	}
}

func TestCanExecuteUsesValidationCommandGrammar(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		want   bool
	}{
		{name: "direct commands", script: "npm ci\npnpm test\nyarn run lint", want: true},
		{name: "comments and install aliases", script: "# install\nnpm install\npnpm i\nyarn", want: true},
		{name: "package-manager builtins are not executable", script: "pnpm update\nyarn publish", want: false},
		{name: "comments only", script: "# nothing to execute\n  # still nothing", want: false},
		{name: "empty block", script: "\n  \n", want: false},
		{name: "mixed unsupported line", script: "npm test\necho pwned > marker", want: false},
		{name: "substitution", script: "npm test $(uname)", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanExecute(model.DocBlock{Script: tc.script}); got != tc.want {
				t.Fatalf("CanExecute(%q)=%v, want %v", tc.script, got, tc.want)
			}
		})
	}
}

func TestValidateRecognizesPackageScriptAliases(t *testing.T) {
	blocks := []model.DocBlock{{
		ID:     "README.md:2",
		Shell:  "pwsh",
		Script: "npm start\npnpm test\nyarn run lint",
		Source: model.SourceRef{Path: "README.md", Line: 2},
	}}
	facts := []model.Fact{
		{Kind: model.FactKind("package.script"), Subject: "start"},
		{Kind: model.FactKind("package.script"), Subject: "test"},
		{Kind: model.FactKind("package.script"), Subject: "lint"},
	}
	findings := Validate(blocks, facts)
	if len(findings) != 1 || findings[0].Status != model.StatusPass {
		t.Fatalf("findings=%#v", findings)
	}
}

func TestExecutionFindingsMapResultsToBlocks(t *testing.T) {
	blocks := []model.DocBlock{
		{ID: "README.md:2", Shell: "sh", Script: "npm test", Source: model.SourceRef{Path: "README.md", Line: 2}},
		{ID: "README.md:8", Shell: "sh", Script: "npm build", Source: model.SourceRef{Path: "README.md", Line: 8}},
		{ID: "README.md:14", Shell: "sh", Script: "npm lint", Source: model.SourceRef{Path: "README.md", Line: 14}},
	}
	findings, err := ExecutionFindings(blocks, []model.ExecutionResult{
		{BlockID: "README.md:2", ExitCode: 0, Status: model.StatusPass, Stdout: "ok"},
		{BlockID: "README.md:8", ExitCode: 3, Status: model.StatusFinding, Stderr: "failed"},
		{BlockID: "README.md:14", Status: model.StatusSkipped},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 3 {
		t.Fatalf("findings=%#v", findings)
	}
	if findings[0].RuleID != "docs-command-passed" || findings[1].RuleID != "docs-command-failed" || findings[2].RuleID != "docs-command-skipped" {
		t.Fatalf("findings=%#v", findings)
	}
	if len(findings[0].Evidence) != 2 || len(findings[1].Evidence) != 2 {
		t.Fatalf("findings=%#v", findings)
	}
}

func TestExecutionFindingsRejectMissingOrDuplicateBlockIDs(t *testing.T) {
	blocks := []model.DocBlock{{ID: "README.md:2", Source: model.SourceRef{Path: "README.md", Line: 2}}}
	if _, err := ExecutionFindings(blocks, []model.ExecutionResult{{BlockID: "missing"}}); err == nil {
		t.Fatal("expected missing block error")
	}
	if _, err := ExecutionFindings(append(blocks, blocks[0]), []model.ExecutionResult{{BlockID: "README.md:2"}}); err == nil {
		t.Fatal("expected duplicate block error")
	}
}

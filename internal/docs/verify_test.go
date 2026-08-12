package docs

import (
	"testing"

	"github.com/devparity/devparity/internal/model"
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

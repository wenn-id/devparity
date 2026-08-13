package rules

import (
	"strings"
	"testing"

	"github.com/wenn-id/devparity/internal/model"
)

func TestEvaluateNodeVersionConflict(t *testing.T) {
	findings := Evaluate([]model.Fact{
		fact("node.constraint", "node", ">=20 <22", "README.md", 2),
		fact("node.constraint", "node", "22.x", ".nvmrc", 1),
	})
	finding := mustFinding(t, findings, "node-version-conflict")
	if finding.Status != model.StatusFinding || finding.Severity != model.SeverityError {
		t.Fatalf("finding=%#v", finding)
	}
	if len(finding.Evidence) != 2 || finding.Suggestion == "" || !strings.Contains(strings.ToLower(finding.Suggestion), "author") {
		t.Fatalf("finding=%#v", finding)
	}
}

func TestEvaluateCompatibleNodeConstraintsPass(t *testing.T) {
	finding := mustFinding(t, Evaluate([]model.Fact{
		fact("node.constraint", "node", ">=20 <23", "README.md", 2),
		fact("node.constraint", "node", "22.x", ".nvmrc", 1),
	}), "node-version-conflict")
	if finding.Status != model.StatusPass {
		t.Fatalf("finding=%#v", finding)
	}
}

func TestEvaluatePackageManagerConflict(t *testing.T) {
	finding := mustFinding(t, Evaluate([]model.Fact{
		fact("package.manager.declared", "package-manager", "pnpm", "package.json", 3),
		fact("package.manager.lockfile", "package-manager", "npm", "package-lock.json", 1),
	}), "package-manager-conflict")
	if finding.Status != model.StatusFinding || len(finding.Evidence) != 2 {
		t.Fatalf("finding=%#v", finding)
	}
}

func TestEvaluateMissingPackageScript(t *testing.T) {
	finding := mustFinding(t, Evaluate([]model.Fact{
		fact("package.script", "test", "node --test", "package.json", 4),
		fact("doc.command", "missing", "npm run missing", "README.md", 8),
	}), "missing-package-script")
	if finding.Status != model.StatusFinding || finding.Severity != model.SeverityError {
		t.Fatalf("finding=%#v", finding)
	}
}

func TestEvaluateWorkflowCommandDrift(t *testing.T) {
	finding := mustFinding(t, Evaluate([]model.Fact{
		fact("doc.command", "test", "npm test", "README.md", 8),
		fact("workflow.command", "test:ci", "npm run test:ci", ".github/workflows/ci.yml", 12),
	}), "workflow-command-drift")
	if finding.Status != model.StatusFinding || finding.Severity != model.SeverityWarning || len(finding.Evidence) != 2 {
		t.Fatalf("finding=%#v", finding)
	}
}

func TestEvaluateUnsupportedNodeConstraintIsInconclusive(t *testing.T) {
	finding := mustFinding(t, Evaluate([]model.Fact{
		fact("node.constraint", "node", "22.0.0-beta.1", "README.md", 2),
	}), "node-version-unsupported")
	if finding.Status != model.StatusInconclusive {
		t.Fatalf("finding=%#v", finding)
	}
}

func mustFinding(t *testing.T, findings []model.Finding, ruleID string) model.Finding {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			return finding
		}
	}
	t.Fatalf("missing %q in %#v", ruleID, findings)
	return model.Finding{}
}

func fact(kind model.FactKind, subject, value, path string, line int) model.Fact {
	return model.Fact{Kind: kind, Subject: subject, Value: value, Source: model.SourceRef{Path: path, Line: line}}
}

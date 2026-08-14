package rules

import (
	"fmt"
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

func TestEvaluateWorkflowCommandDriftDeduplicatesEvidence(t *testing.T) {
	facts := make([]model.Fact, 0, 200)
	for index := 0; index < 100; index++ {
		facts = append(facts,
			fact("doc.command", "test", "npm test", "README.md", 8),
			fact("workflow.command", "test:ci", "npm run test:ci", ".github/workflows/ci.yml", 12),
		)
	}

	finding := mustFinding(t, Evaluate(facts), "workflow-command-drift")
	if finding.Status != model.StatusFinding {
		t.Fatalf("finding=%#v", finding)
	}
	if len(finding.Evidence) != 2 {
		t.Fatalf("evidence=%d, want 2 deduplicated facts: %#v", len(finding.Evidence), finding.Evidence)
	}
	seen := make(map[model.Fact]struct{}, len(finding.Evidence))
	for _, evidence := range finding.Evidence {
		if _, exists := seen[evidence]; exists {
			t.Fatalf("duplicate evidence=%#v", evidence)
		}
		seen[evidence] = struct{}{}
	}
}

func TestEvaluateWorkflowCommandDriftBoundsAdversarialEvidence(t *testing.T) {
	const commandCount = 500
	facts := make([]model.Fact, 0, commandCount*2)
	for index := 0; index < commandCount; index++ {
		facts = append(facts,
			fact("doc.command", "test", fmt.Sprintf("npm run doc-%d", index), "README.md", index+1),
			fact("workflow.command", "test:ci", fmt.Sprintf("npm run workflow-%d", index), ".github/workflows/ci.yml", index+1),
		)
	}

	finding := mustFinding(t, Evaluate(facts), "workflow-command-drift")
	if finding.Status != model.StatusFinding {
		t.Fatalf("finding=%#v", finding)
	}
	if len(finding.Evidence) > 256 {
		t.Fatalf("evidence=%d, want <= 256 facts", len(finding.Evidence))
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

func TestEvaluateCompatibleHyphenRangePass(t *testing.T) {
	finding := mustFinding(t, Evaluate([]model.Fact{
		fact("node.constraint", "node", "20.1.0 - 22.9.9", "package.json", 3),
		fact("node.constraint", "node", "22", ".nvmrc", 1),
	}), "node-version-conflict")
	if finding.Status != model.StatusPass {
		t.Fatalf("finding=%#v", finding)
	}
}

func TestEvaluateIncompatibleHyphenRangeConflict(t *testing.T) {
	finding := mustFinding(t, Evaluate([]model.Fact{
		fact("node.constraint", "node", "20.1.0 - 22.9.9", "package.json", 3),
		fact("node.constraint", "node", "23", ".nvmrc", 1),
	}), "node-version-conflict")
	if finding.Status != model.StatusFinding {
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

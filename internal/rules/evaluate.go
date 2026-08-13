package rules

import (
	"github.com/wenn-id/devparity/internal/model"
)

func Evaluate(facts []model.Fact) []model.Finding {
	out := []model.Finding{
		evaluateVersions(facts),
		evaluatePackageManager(facts),
	}
	out = append(out, evaluateMissingScripts(facts)...)
	out = append(out, evaluateWorkflowDrift(facts))
	model.SortFindings(out)
	return out
}

func finding(ruleID string, severity model.Severity, status model.Status, message string, evidence []model.Fact, suggestion string) model.Finding {
	return model.Finding{RuleID: ruleID, Severity: severity, Status: status, Message: message, Evidence: evidence, Suggestion: suggestion}
}

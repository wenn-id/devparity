package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wenn-id/devparity/internal/model"
	"github.com/wenn-id/devparity/internal/nodecmd"
)

const maxWorkflowDriftEvidence = 256

func evaluateWorkflowDrift(facts []model.Fact) model.Finding {
	docs := make(map[string][]model.Fact)
	workflows := make(map[string][]model.Fact)
	seenDocs := make(map[model.Fact]struct{}, len(facts))
	seenWorkflows := make(map[model.Fact]struct{}, len(facts))
	for _, fact := range facts {
		switch fact.Kind {
		case model.FactKind("doc.command"):
			if _, exists := seenDocs[fact]; exists {
				continue
			}
			seenDocs[fact] = struct{}{}
			class := commandClass(fact)
			if class == "builtin" {
				continue
			}
			docs[class] = append(docs[class], fact)
		case model.FactKind("workflow.command"):
			if _, exists := seenWorkflows[fact]; exists {
				continue
			}
			seenWorkflows[fact] = struct{}{}
			class := commandClass(fact)
			if class == "builtin" {
				continue
			}
			workflows[class] = append(workflows[class], fact)
		}
	}

	classes := make([]string, 0, len(docs))
	for class := range docs {
		if len(workflows[class]) > 0 {
			classes = append(classes, class)
		}
	}
	sort.Strings(classes)

	evidence := make([]model.Fact, 0, maxWorkflowDriftEvidence)
	seenEvidence := make(map[model.Fact]struct{}, maxWorkflowDriftEvidence)
	findingClasses := make([]string, 0, len(classes))
	for _, class := range classes {
		docFacts := docs[class]
		workflowFacts := workflows[class]
		workflowRepresentatives := uniqueValues(workflowFacts)
		classFinding := false
		for _, fact := range docFacts {
			workflow, differs := differingWorkflowFact(workflowRepresentatives, fact.Value)
			if !differs {
				continue
			}
			classFinding = true
			appendEvidence(&evidence, seenEvidence, fact)
			appendEvidence(&evidence, seenEvidence, workflow)
		}
		if classFinding {
			findingClasses = append(findingClasses, class)
		}
	}

	if len(findingClasses) == 0 {
		if len(docs) == 0 && len(workflows) == 0 {
			return finding("workflow-command-drift", model.SeverityInfo, model.StatusNoEvidence, "No documentation or workflow commands found to compare", nil, "No command authority is selected.")
		}
		// Commands exist, but a comparison only happens for a class present on
		// both sides. One-sided or non-overlapping evidence was never checked,
		// so it must not be reported as agreement.
		if len(classes) == 0 {
			return finding("workflow-command-drift", model.SeverityInfo, model.StatusNoEvidence, "No command class appears in both documentation and workflows; nothing to compare", nil, "Document and automate the same command class to enable a comparison.")
		}
		return finding("workflow-command-drift", model.SeverityWarning, model.StatusPass, "Documentation and workflow commands agree", evidence, "No command authority is selected.")
	}
	return finding("workflow-command-drift", model.SeverityWarning, model.StatusFinding, fmt.Sprintf("Documentation and workflow commands differ for %s", strings.Join(findingClasses, ", ")), evidence, "Review the command difference; DevParity reports bounded, deduplicated evidence without choosing which command is correct.")
}

func uniqueValues(facts []model.Fact) []model.Fact {
	values := make([]model.Fact, 0, len(facts))
	seen := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		if _, exists := seen[fact.Value]; exists {
			continue
		}
		seen[fact.Value] = struct{}{}
		values = append(values, fact)
	}
	return values
}

func differingWorkflowFact(facts []model.Fact, value string) (model.Fact, bool) {
	if len(facts) == 0 {
		return model.Fact{}, false
	}
	if facts[0].Value != value {
		return facts[0], true
	}
	if len(facts) > 1 {
		return facts[1], true
	}
	return model.Fact{}, false
}

func appendEvidence(evidence *[]model.Fact, seen map[model.Fact]struct{}, fact model.Fact) {
	if len(*evidence) >= maxWorkflowDriftEvidence {
		return
	}
	if _, exists := seen[fact]; exists {
		return
	}
	seen[fact] = struct{}{}
	*evidence = append(*evidence, fact)
}

func commandClass(fact model.Fact) string {
	if fact.Subject != "" && fact.Subject != "script" {
		value := fact.Subject
		for _, suffix := range []string{"", ":ci", ":local"} {
			if strings.HasSuffix(value, suffix) && suffix != "" {
				return strings.TrimSuffix(value, suffix)
			}
		}
		return value
	}
	command, ok := nodecmd.Parse(fact.Value)
	if !ok {
		return "unknown"
	}
	switch command.Script {
	case "test", "test:ci":
		return "test"
	case "build", "build:ci":
		return "build"
	case "lint", "lint:ci":
		return "lint"
	default:
		if command.Operation == "install" {
			return "install"
		}
		return command.Script
	}
}

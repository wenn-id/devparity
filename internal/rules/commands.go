package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wenn-id/devparity/internal/model"
	"github.com/wenn-id/devparity/internal/nodecmd"
)

func evaluateWorkflowDrift(facts []model.Fact) model.Finding {
	docs := make(map[string][]model.Fact)
	workflows := make(map[string][]model.Fact)
	for _, fact := range facts {
		switch fact.Kind {
		case model.FactKind("doc.command"):
			docs[commandClass(fact)] = append(docs[commandClass(fact)], fact)
		case model.FactKind("workflow.command"):
			workflows[commandClass(fact)] = append(workflows[commandClass(fact)], fact)
		}
	}
	var evidence []model.Fact
	var classes []string
	for class, docFacts := range docs {
		workflowFacts := workflows[class]
		if len(workflowFacts) == 0 {
			continue
		}
		for _, fact := range docFacts {
			for _, workflow := range workflowFacts {
				if fact.Value != workflow.Value {
					evidence = append(evidence, fact, workflow)
					classes = append(classes, class)
				}
			}
		}
	}
	if len(evidence) == 0 {
		return finding("workflow-command-drift", model.SeverityWarning, model.StatusPass, "Documentation and workflow commands agree or are insufficient for comparison", nil, "No command authority is selected.")
	}
	sort.Strings(classes)
	return finding("workflow-command-drift", model.SeverityWarning, model.StatusFinding, fmt.Sprintf("Documentation and workflow commands differ for %s", strings.Join(unique(classes), ", ")), evidence, "Review the command difference; DevParity reports evidence without choosing which command is correct.")
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

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

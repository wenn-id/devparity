package rules

import (
	"fmt"
	"sort"

	"github.com/wenn-id/devparity/internal/model"
	"github.com/wenn-id/devparity/internal/nodecmd"
)

func evaluatePackageManager(facts []model.Fact) model.Finding {
	var evidence []model.Fact
	managers := make(map[string]struct{})
	for _, fact := range facts {
		if fact.Kind == model.FactKind("package.manager.declared") || fact.Kind == model.FactKind("package.manager.lockfile") || fact.Kind == model.FactKind("package.manager.command") {
			evidence = append(evidence, fact)
			managers[fact.Value] = struct{}{}
		}
	}
	if len(managers) <= 1 {
		return finding("package-manager-conflict", model.SeverityError, model.StatusPass, "Package-manager evidence agrees or is insufficient for comparison", evidence, "No package manager is selected as authoritative.")
	}
	values := make([]string, 0, len(managers))
	for manager := range managers {
		values = append(values, manager)
	}
	sort.Strings(values)
	return finding("package-manager-conflict", model.SeverityError, model.StatusFinding, fmt.Sprintf("Package-manager evidence conflicts: %v", values), evidence, "Use one package manager consistently across package.json, lockfiles, docs, and CI.")
}

func evaluateMissingScripts(facts []model.Fact) []model.Finding {
	available := make(map[string]model.Fact)
	for _, fact := range facts {
		if fact.Kind == model.FactKind("package.script") {
			available[fact.Subject] = fact
		}
	}
	missing := make(map[string][]model.Fact)
	for _, fact := range facts {
		if fact.Kind != model.FactKind("doc.command") && fact.Kind != model.FactKind("workflow.command") {
			continue
		}
		if fact.Subject == "" {
			continue
		}
		command, parsed := nodecmd.Parse(fact.Value)
		if parsed && command.Operation == "install" {
			continue
		}
		if _, ok := available[fact.Subject]; !ok {
			missing[fact.Subject] = append(missing[fact.Subject], fact)
		}
	}
	names := make([]string, 0, len(missing))
	for name := range missing {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]model.Finding, 0, len(names))
	for _, name := range names {
		out = append(out, finding("missing-package-script", model.SeverityError, model.StatusFinding, fmt.Sprintf("Referenced package script %q does not exist", name), missing[name], fmt.Sprintf("Add package script %q or correct the command.", name)))
	}
	return out
}

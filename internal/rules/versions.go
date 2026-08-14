package rules

import (
	"fmt"

	"github.com/wenn-id/devparity/internal/model"
	"github.com/wenn-id/devparity/internal/semverx"
)

func evaluateVersions(facts []model.Fact) model.Finding {
	var constraints []model.Fact
	for _, fact := range facts {
		if fact.Kind == model.FactKind("node.constraint") {
			constraints = append(constraints, fact)
		}
	}
	raw := make([]string, len(constraints))
	for i, fact := range constraints {
		raw[i] = fact.Value
		if _, err := semverx.Normalize(fact.Value); err != nil {
			return finding("node-version-unsupported", model.SeverityWarning, model.StatusInconclusive, fmt.Sprintf("Node version constraint is inconclusive: %v", err), constraints, "Use supported, non-prerelease Node constraints.")
		}
	}
	if len(constraints) < 2 {
		return finding("node-version-conflict", model.SeverityError, model.StatusPass, "Node version constraints are compatible or insufficient for comparison", constraints, "No authoritative Node version is selected.")
	}
	compatible, err := semverx.IntersectsAll(raw)
	if err != nil {
		return finding("node-version-unsupported", model.SeverityWarning, model.StatusInconclusive, fmt.Sprintf("Node version constraints are inconclusive: %v", err), constraints, "Use supported, non-prerelease Node constraints.")
	}
	if compatible {
		return finding("node-version-conflict", model.SeverityError, model.StatusPass, "Node version constraints have a compatible intersection", constraints, "No authoritative Node version is selected.")
	}
	return finding("node-version-conflict", model.SeverityError, model.StatusFinding, "Node version constraints have no compatible intersection", constraints, "Resolve the conflicting Node requirements; DevParity does not choose an authoritative source.")
}

package rules

import (
	"fmt"
	"strings"

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
	for _, fact := range constraints {
		if unsupportedNodeFact(fact) {
			return finding("node-version-unsupported", model.SeverityWarning, model.StatusInconclusive, "Node version constraint uses unsupported prerelease syntax", constraints, "Use a supported non-prerelease Node constraint.")
		}
	}
	if len(constraints) < 2 {
		return finding("node-version-conflict", model.SeverityError, model.StatusPass, "Node version constraints are compatible or insufficient for comparison", constraints, "No authoritative Node version is selected.")
	}
	raw := make([]string, len(constraints))
	for i, fact := range constraints {
		raw[i] = fact.Value
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

func unsupportedNodeFact(fact model.Fact) bool {
	return strings.Contains(fact.Value, "-")
}

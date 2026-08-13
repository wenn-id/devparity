package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wenn-id/devparity/internal/model"
)

func TestTextRendersFindingEvidenceAndSuggestion(t *testing.T) {
	var out bytes.Buffer
	input := fixedReport()

	if err := Text(&out, input); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"node-version-conflict",
		"finding",
		"README.md:2",
		">=20 <22",
		".nvmrc:1",
		"Resolve the conflict",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("text=%q, missing %q", got, want)
		}
	}
}

func fixedReport() model.Report {
	return model.Report{
		SchemaVersion: 1,
		ToolVersion:   "test",
		Repository:    "/repo",
		Summary:       model.Summary{Finding: 1},
		Results: []model.Finding{{
			RuleID:   "node-version-conflict",
			Severity: model.SeverityError,
			Status:   model.StatusFinding,
			Message:  "Node versions conflict",
			Evidence: []model.Fact{
				{Kind: "node.constraint", Value: ">=20 <22", Source: model.SourceRef{Path: "README.md", Line: 2}},
				{Kind: "node.constraint", Value: "22", Source: model.SourceRef{Path: ".nvmrc", Line: 1}},
			},
			Suggestion: "Resolve the conflict",
		}},
	}
}

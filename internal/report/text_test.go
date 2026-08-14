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

func TestTextEscapesControlCharactersAndLineBreaks(t *testing.T) {
	report := model.Report{
		SchemaVersion: 1,
		ToolVersion:   "test",
		Repository:    "/repo\nwith-line",
		Summary:       model.Summary{Finding: 1},
		Results: []model.Finding{{
			RuleID:   "node-version-conflict",
			Severity: model.SeverityError,
			Status:   model.StatusFinding,
			Message:  "line one\nline two\twith tab\x1b[31m",
			Evidence: []model.Fact{
				{Kind: "node.constraint", Value: ">=20 <22\r", Source: model.SourceRef{Path: "bad\npath.md", Line: 2}},
			},
			Suggestion: "fix\x00me",
		}},
	}

	var out bytes.Buffer
	if err := Text(&out, report); err != nil {
		t.Fatal(err)
	}
	got := out.String()

	// Repository-controlled fields must never emit raw control bytes or
	// embedded line breaks that could manipulate the terminal or forge lines.
	for _, raw := range []string{
		"\r",
		"\t",
		"\x00",
		"\x1b",
		"Repository: /repo\nwith-line",
		"line one\nline two",
		"bad\npath.md",
	} {
		if strings.Contains(got, raw) {
			t.Fatalf("text=%q contains raw control/line-break %q", got, raw)
		}
	}

	for _, want := range []string{
		"Repository: /repo\\nwith-line",
		"line one\\nline two\\twith tab\\x1b[31m",
		">=20 <22\\r",
		"bad\\npath.md",
		"fix\\x00me",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("text=%q, missing escaped value %q", got, want)
		}
	}
}

func TestGitHubEscapesMarkdownDelimitersAndNewlines(t *testing.T) {
	report := model.Report{
		SchemaVersion: 1,
		ToolVersion:   "test",
		Repository:    "repo|evil`x`",
		Summary:       model.Summary{Finding: 1},
		Results: []model.Finding{{
			RuleID:   "node-version-conflict",
			Severity: model.SeverityError,
			Status:   model.StatusFinding,
			Message:  "broken\n| cell `code`",
			Evidence: []model.Fact{},
		}},
	}

	var out bytes.Buffer
	if err := GitHub(&out, report); err != nil {
		t.Fatal(err)
	}
	got := out.String()

	// Repository and message must escape pipe/backtick and collapse newlines so
	// a malicious value cannot break or forge table rows.
	if !strings.Contains(got, "repo\\|evil\\`x\\`") {
		t.Fatalf("markdown=%q, want escaped repository", got)
	}
	if !strings.Contains(got, "broken \\| cell \\`code\\`") {
		t.Fatalf("markdown=%q, want escaped message cell", got)
	}
	if strings.Contains(got, "broken\n") {
		t.Fatalf("markdown=%q, raw newline leaked into message cell", got)
	}

	// The rendered table must have exactly header + separator + one finding
	// row; an injected newline would have forged an extra data row.
	rows := strings.Count(got, "\n|")
	if rows != 3 {
		t.Fatalf("markdown=%q, want 3 table lines (header, separator, one finding), got %d", got, rows)
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

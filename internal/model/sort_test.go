package model

import "testing"

func TestSortFindings(t *testing.T) {
	got := []Finding{
		{RuleID: "z", Evidence: []Fact{{Source: SourceRef{Path: "b.md", Line: 1}}}},
		{RuleID: "b", Evidence: []Fact{{Source: SourceRef{Path: "a.md", Line: 4}}}},
		{RuleID: "a", Evidence: []Fact{{Source: SourceRef{Path: "a.md", Line: 4}}}},
	}
	SortFindings(got)
	if got[0].RuleID != "a" || got[1].RuleID != "b" || got[2].RuleID != "z" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestSortFindingsOrdersEmptyEvidenceBeforeSourcedFindings(t *testing.T) {
	got := []Finding{
		{RuleID: "z-empty"},
		{RuleID: "a-sourced", Evidence: []Fact{{Source: SourceRef{Path: "README.md", Line: 1}}}},
		{RuleID: "a-empty"},
	}
	SortFindings(got)
	want := []string{"a-empty", "z-empty", "a-sourced"}
	for index, ruleID := range want {
		if got[index].RuleID != ruleID {
			t.Fatalf("index %d rule=%q, want %q; findings=%#v", index, got[index].RuleID, ruleID, got)
		}
	}
}

func TestSortFindingsUsesRuleIDAfterPathAndLine(t *testing.T) {
	got := []Finding{
		{RuleID: "z", Evidence: []Fact{{Source: SourceRef{Path: "README.md", Line: 4}}}},
		{RuleID: "a", Evidence: []Fact{{Source: SourceRef{Path: "README.md", Line: 4}}}},
		{RuleID: "m", Evidence: []Fact{{Source: SourceRef{Path: "README.md", Line: 3}}}},
		{RuleID: "b", Evidence: []Fact{{Source: SourceRef{Path: "CONTRIBUTING.md", Line: 99}}}},
	}
	SortFindings(got)
	want := []string{"b", "m", "a", "z"}
	for index, ruleID := range want {
		if got[index].RuleID != ruleID {
			t.Fatalf("index %d rule=%q, want %q; findings=%#v", index, got[index].RuleID, ruleID, got)
		}
	}
}

func TestSummarizeCountsNoEvidenceSeparatelyFromPass(t *testing.T) {
	findings := []Finding{
		{Status: StatusPass},
		{Status: StatusNoEvidence},
		{Status: StatusNoEvidence},
		{Status: StatusFinding},
		{Status: StatusInconclusive},
		{Status: StatusSkipped},
	}
	summary := Summarize(findings)
	if summary.Pass != 1 || summary.NoEvidence != 2 || summary.Finding != 1 || summary.Inconclusive != 1 || summary.Skipped != 1 {
		t.Fatalf("summary=%#v, want no-evidence counted separately from pass", summary)
	}
}

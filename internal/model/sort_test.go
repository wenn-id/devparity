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

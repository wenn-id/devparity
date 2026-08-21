package model

import "sort"

func SortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		var left, right SourceRef
		if len(findings[i].Evidence) > 0 {
			left = findings[i].Evidence[0].Source
		}
		if len(findings[j].Evidence) > 0 {
			right = findings[j].Evidence[0].Source
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		return findings[i].RuleID < findings[j].RuleID
	})
}

func Summarize(findings []Finding) Summary {
	var summary Summary
	for _, finding := range findings {
		switch finding.Status {
		case StatusPass:
			summary.Pass++
		case StatusFinding:
			summary.Finding++
		case StatusSkipped:
			summary.Skipped++
		case StatusInconclusive:
			summary.Inconclusive++
		case StatusNoEvidence:
			summary.NoEvidence++
		}
	}
	return summary
}

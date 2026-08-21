package report

import (
	"fmt"
	"io"

	"github.com/wenn-id/devparity/internal/model"
)

func GitHub(w io.Writer, value model.Report) error {
	if _, err := fmt.Fprintf(w, "## DevParity\n\nRepository: %s\n\n| Status | Rule | Message |\n|---|---|---|\n", markdownCell(value.Repository)); err != nil {
		return err
	}
	for _, finding := range value.Results {
		if _, err := fmt.Fprintf(w, "| %s | %s | %s |\n", markdownCell(string(finding.Status)), markdownCell(finding.RuleID), markdownCell(finding.Message)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "\nSummary: pass=%d, finding=%d, skipped=%d, inconclusive=%d, no-evidence=%d\n", value.Summary.Pass, value.Summary.Finding, value.Summary.Skipped, value.Summary.Inconclusive, value.Summary.NoEvidence)
	return err
}

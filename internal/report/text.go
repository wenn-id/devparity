package report

import (
	"fmt"
	"io"

	"github.com/wenn-id/devparity/internal/model"
)

func Text(w io.Writer, value model.Report) error {
	if _, err := fmt.Fprintf(w, "DevParity %s\nRepository: %s\nSummary: pass=%d finding=%d skipped=%d inconclusive=%d\n", textField(value.ToolVersion), textField(value.Repository), value.Summary.Pass, value.Summary.Finding, value.Summary.Skipped, value.Summary.Inconclusive); err != nil {
		return err
	}
	for _, finding := range value.Results {
		if _, err := fmt.Fprintf(w, "\n[%s] %s (%s)\n%s\n", textField(string(finding.Status)), textField(finding.RuleID), textField(string(finding.Severity)), textField(finding.Message)); err != nil {
			return err
		}
		for _, evidence := range finding.Evidence {
			if _, err := fmt.Fprintf(w, "  %s:%d %s=%s\n", textField(evidence.Source.Path), evidence.Source.Line, textField(string(evidence.Kind)), textField(evidence.Value)); err != nil {
				return err
			}
		}
		if finding.Suggestion != "" {
			if _, err := fmt.Fprintf(w, "  suggestion: %s\n", textField(finding.Suggestion)); err != nil {
				return err
			}
		}
	}
	return nil
}

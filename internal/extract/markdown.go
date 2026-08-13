package extract

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/wenn-id/devparity/internal/model"
	"github.com/wenn-id/devparity/internal/repository"
)

const maxMarkdownEvidenceBytes int64 = 1 << 20

var nodeRequirementPrefix = regexp.MustCompile(`(?i)\bnode(?:\.js)?\s+(?:version\s+)?`)
var nodeConstraintValue = regexp.MustCompile(`(?i)^(v?(?:[0-9xX*][0-9A-Za-z.*xX^~<>=| -]*|[<>=~^|]+[0-9xX*][0-9A-Za-z.*xX^~<>=| -]*))`)

func MarkdownVersions(root string, paths []string) ([]model.Fact, []model.Finding) {
	var facts []model.Fact
	var findings []model.Finding
	for _, path := range paths {
		data, err := repository.Read(root, path, maxMarkdownEvidenceBytes)
		if err != nil {
			findings = append(findings, parseFinding(path, err))
			continue
		}
		for lineNumber, line := range splitLines(data) {
			matches := nodeRequirementPrefix.FindAllStringIndex(line, -1)
			if len(matches) == 0 {
				continue
			}
			if len(matches) > 1 {
				findings = append(findings, model.Finding{
					RuleID:     "node-version-unsupported",
					Severity:   model.SeverityWarning,
					Status:     model.StatusInconclusive,
					Message:    fmt.Sprintf("multiple Node version requirements on %s:%d", path, lineNumber+1),
					Evidence:   []model.Fact{{Kind: model.FactKind("node.version.unsupported"), Subject: "node", Source: source(path, lineNumber+1, "")}},
					Suggestion: "Put one unambiguous Node constraint on each line.",
				})
				continue
			}

			start := matches[0][1]
			valueText := line[start:]
			valueMatch := nodeConstraintValue.FindStringSubmatch(valueText)
			if len(valueMatch) != 2 {
				continue
			}
			value := strings.TrimSpace(valueMatch[1])
			value = strings.TrimPrefix(strings.TrimPrefix(value, "v"), "V")
			value = strings.TrimRight(value, ".,;:)]}\"'")
			value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "and"))
			if value != "" {
				facts = append(facts, nodeConstraint(path, lineNumber+1, value))
			}
		}
	}
	return facts, findings
}

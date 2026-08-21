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

// nodeConstraintValue captures the version text that follows the Node
// keyword. The value character class deliberately includes "+", so an
// open-ended prose constraint such as "Node 18+" keeps its suffix instead of
// being truncated to the bare major "18".
var nodeConstraintValue = regexp.MustCompile(`(?i)^(v?(?:[0-9xX*][0-9A-Za-z.*xX^~<>=|+ -]*|[<>=~^|]+[0-9xX*][0-9A-Za-z.*xX^~<>=|+ -]*))`)

// bareMajorConstraint matches a constraint that is exactly one number, which
// is the only base we can safely turn from "N+" into ">=N".
var bareMajorConstraint = regexp.MustCompile(`^\d+$`)

func MarkdownVersions(root string, paths []string) ([]model.Fact, []model.Finding) {
	var facts []model.Fact
	var findings []model.Finding
	for _, path := range paths {
		data, err := repository.Read(root, path, maxMarkdownEvidenceBytes)
		if err != nil {
			findings = append(findings, parseFinding(path, err))
			continue
		}
		fenceChar := byte(0)
		fenceLen := 0
		for lineNumber, line := range splitLines(data) {
			if char, runLen, closing := markdownFence(line); runLen > 0 {
				switch {
				case fenceLen == 0:
					fenceChar, fenceLen = char, runLen
				case char == fenceChar && runLen >= fenceLen && closing:
					fenceChar, fenceLen = 0, 0
				}
				continue
			}
			if fenceLen > 0 {
				continue
			}
			matches := nodeRequirementPrefix.FindAllStringIndex(line, -1)
			if len(matches) == 0 {
				continue
			}
			var lineValues []string
			for _, match := range matches {
				value, ok := nodeProseConstraint(line[match[1]:])
				if ok {
					lineValues = append(lineValues, value)
				}
			}
			// Ordinary prose ("Node.js is a runtime") yields no constraint
			// values at all, so it must not be reported. A line only becomes
			// ambiguous when it carries two or more actual requirements.
			if len(lineValues) > 1 {
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
			if len(lineValues) == 1 {
				facts = append(facts, nodeConstraint(path, lineNumber+1, lineValues[0]))
			}
		}
	}
	return facts, findings
}

// markdownFence reports a backtick/tilde run at the start of a Markdown line.
// A run closes an existing fence only when it uses the same character, is at
// least as long, and has no info string after it.
func markdownFence(line string) (char byte, runLen int, closing bool) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
		return 0, 0, false
	}
	char = trimmed[0]
	for runLen < len(trimmed) && trimmed[runLen] == char {
		runLen++
	}
	if runLen < 3 {
		return 0, 0, false
	}
	return char, runLen, strings.TrimSpace(trimmed[runLen:]) == ""
}

// nodeProseConstraint extracts the Node version constraint at the start of
// text that directly follows a Node keyword. It reports false when the text
// does not start with a version-like token, which is how ordinary prose is
// told apart from an actual requirement.
func nodeProseConstraint(text string) (string, bool) {
	valueMatch := nodeConstraintValue.FindStringSubmatch(text)
	if len(valueMatch) != 2 {
		return "", false
	}
	value := strings.TrimSpace(valueMatch[1])
	value = strings.TrimPrefix(strings.TrimPrefix(value, "v"), "V")
	value = strings.TrimRight(value, ".,;:)]}\"'")
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "and"))
	// In prose, "N+" terminates the constraint ("Node 18+ to build"), so cut
	// the captured value at the "+" before further parsing.
	if plus := strings.IndexByte(value, '+'); plus >= 0 {
		value = strings.TrimSpace(value[:plus+1])
	}
	// Prose such as "Node 18+" means 18 or newer, not exactly 18. Rewrite the
	// open-ended suffix to ">=18" so the constraint keeps the author's
	// meaning; anything else ending in "+" is not a spelling we understand,
	// so drop it rather than misread it.
	if strings.HasSuffix(value, "+") {
		base := strings.TrimSpace(strings.TrimSuffix(value, "+"))
		if !bareMajorConstraint.MatchString(base) {
			return "", false
		}
		value = ">=" + base
	}
	if value == "" {
		return "", false
	}
	return value, true
}

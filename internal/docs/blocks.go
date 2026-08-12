package docs

import (
	"bufio"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/devparity/devparity/internal/model"
	"github.com/devparity/devparity/internal/repository"
)

const (
	docMarker           = "<!-- devparity:run -->"
	maxDocBytes         = int64(1 << 20)
	maxDocScannerBuffer = 1 << 20
)

var supportedShells = map[string]bool{
	"sh":         true,
	"shell":      true,
	"bash":       true,
	"powershell": true,
	"pwsh":       true,
}

func Extract(root string, paths []string) ([]model.DocBlock, []model.Finding) {
	var blocks []model.DocBlock
	var findings []model.Finding
	for _, path := range paths {
		data, err := repository.Read(root, path, maxDocBytes)
		if err != nil {
			findings = append(findings, parseFinding(path, err))
			continue
		}
		foundBlocks, foundFindings := extractFile(filepath.ToSlash(path), data)
		blocks = append(blocks, foundBlocks...)
		findings = append(findings, foundFindings...)
	}
	return blocks, findings
}

func extractFile(path string, data []byte) ([]model.DocBlock, []model.Finding) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), maxDocScannerBuffer)
	state := 0 // 0 normal, 1 marker seen, 2 in fence
	lineNumber := 0
	markerLine := 0
	openingLine := 0
	var shell string
	var script []string
	var blocks []model.DocBlock
	var findings []model.Finding

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		switch state {
		case 0:
			if line == docMarker {
				state = 1
				markerLine = lineNumber
			}
		case 1:
			if strings.HasPrefix(line, "```") {
				shell = strings.TrimSpace(strings.TrimPrefix(line, "```"))
				openingLine = lineNumber
				if !supportedShells[shell] {
					findings = append(findings, docFinding("doc-shell-unsupported", path, openingLine, fmt.Sprintf("unsupported documentation shell %q", shell)))
					state = 0
					continue
				}
				state = 2
				script = nil
			} else {
				state = 0
			}
		case 2:
			if strings.TrimSpace(line) == "```" {
				blocks = append(blocks, model.DocBlock{
					ID:     fmt.Sprintf("%s:%d", path, openingLine),
					Shell:  shell,
					Script: strings.Join(script, "\n"),
					Source: model.SourceRef{Path: path, Line: openingLine},
				})
				state = 0
				script = nil
			} else {
				script = append(script, line)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		findings = append(findings, docFinding("doc-block-too-large", path, lineNumber, err.Error()))
	} else if state == 1 {
		findings = append(findings, docFinding("doc-marker-without-fence", path, markerLine, "documentation marker is not immediately followed by a fence"))
	} else if state == 2 {
		findings = append(findings, docFinding("doc-block-unterminated", path, openingLine, "documentation code fence is not terminated"))
	}
	return blocks, findings
}

func docFinding(ruleID, path string, line int, message string) model.Finding {
	return model.Finding{
		RuleID:     ruleID,
		Severity:   model.SeverityWarning,
		Status:     model.StatusInconclusive,
		Message:    message,
		Evidence:   []model.Fact{{Kind: model.FactKind("documentation"), Subject: path, Value: message, Source: model.SourceRef{Path: path, Line: line}}},
		Suggestion: "Use an adjacent supported fenced shell block with a terminating fence.",
	}
}

func parseFinding(path string, err error) model.Finding {
	return model.Finding{
		RuleID:     "parse-error",
		Severity:   model.SeverityError,
		Status:     model.StatusInconclusive,
		Message:    fmt.Sprintf("could not read %s: %v", path, err),
		Evidence:   []model.Fact{{Kind: model.FactKind("parse.error"), Subject: path, Value: err.Error(), Source: model.SourceRef{Path: path}}},
		Suggestion: "Fix the supported artifact so DevParity can inspect it.",
	}
}

package docs

import (
	"bufio"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wenn-id/devparity/internal/model"
	"github.com/wenn-id/devparity/internal/repository"
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
	state := 0 // 0 normal, 1 marker seen, 2 in marked fence, 3 in outer fence
	lineNumber := 0
	markerLine := 0
	openingLine := 0
	var fenceChar byte
	var fenceLength int
	var outerIndent int
	var shell string
	var script []string
	var blocks []model.DocBlock
	var findings []model.Finding

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		switch state {
		case 0:
			if isDocMarker(line) {
				state = 1
				markerLine = lineNumber
				continue
			}
			if char, length, _, indent, ok := docOuterFence(line); ok {
				state = 3
				fenceChar = char
				fenceLength = length
				outerIndent = indent
			}
		case 1:
			char, length, info, ok := docFence(line)
			if !ok {
				state = 0
				continue
			}
			shell = info
			openingLine = lineNumber
			fenceChar = char
			fenceLength = length
			if !supportedShells[shell] {
				findings = append(findings, docFinding("doc-shell-unsupported", path, openingLine, fmt.Sprintf("unsupported documentation shell %q", shell)))
				state = 3
				outerIndent = 0
				continue
			}
			state = 2
			script = nil
		case 2:
			if closesDocFence(line, fenceChar, fenceLength) {
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
		case 3:
			if closesOuterFence(line, fenceChar, fenceLength, outerIndent) {
				state = 0
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

func isDocMarker(line string) bool {
	content, ok := docIndentedContent(line)
	return ok && strings.TrimRight(content, " 	") == docMarker
}

func docFence(line string) (byte, int, string, bool) {
	var ok bool
	line, ok = docIndentedContent(line)
	if !ok {
		return 0, 0, "", false
	}
	if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
		return 0, 0, "", false
	}
	char := line[0]
	length := 0
	for length < len(line) && line[length] == char {
		length++
	}
	if length < 3 {
		return 0, 0, "", false
	}
	if char == '`' && strings.Contains(strings.TrimSpace(line[length:]), "`") {
		return 0, 0, "", false
	}
	return char, length, strings.TrimSpace(line[length:]), true
}

func docOuterFence(line string) (byte, int, string, int, bool) {
	if fence, length, info, ok := docFence(line); ok {
		return fence, length, info, 0, true
	}
	content, ok := docIndentedContent(line)
	if !ok {
		return 0, 0, "", 0, false
	}
	indent := leadingSpaces(line)
	if len(content) >= 2 && (content[0] == '-' || content[0] == '+' || content[0] == '*') && content[1] == ' ' {
		fence, length, info, ok := docFence(content[2:])
		return fence, length, info, indent + 2, ok
	}
	index := 0
	for index < len(content) && content[index] >= '0' && content[index] <= '9' {
		index++
	}
	if index > 0 && index+1 < len(content) && (content[index] == '.' || content[index] == ')') && content[index+1] == ' ' {
		fence, length, info, ok := docFence(content[index+2:])
		return fence, length, info, indent + index + 2, ok
	}
	return 0, 0, "", 0, false
}

func closesDocFence(line string, char byte, minimumLength int) bool {
	closingChar, length, info, ok := docFence(line)
	return ok && closingChar == char && length >= minimumLength && info == ""
}

func closesOuterFence(line string, char byte, minimumLength, minimumIndent int) bool {
	indent := leadingSpaces(line)
	if indent < minimumIndent || indent > minimumIndent+3 || indent >= len(line) {
		return false
	}
	closingChar, length, info, ok := docFence(line[indent:])
	return ok && closingChar == char && length >= minimumLength && info == ""
}

func leadingSpaces(line string) int {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	return indent
}

func docIndentedContent(line string) (string, bool) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return "", false
	}
	return line[indent:], true
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

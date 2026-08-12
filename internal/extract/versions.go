package extract

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/devparity/devparity/internal/model"
	"github.com/devparity/devparity/internal/repository"
)

const maxVersionEvidenceBytes int64 = 1 << 20

func VersionFiles(root string, paths []string) ([]model.Fact, []model.Finding) {
	var facts []model.Fact
	var findings []model.Finding
	for _, path := range paths {
		data, err := repository.Read(root, path, maxVersionEvidenceBytes)
		if err != nil {
			findings = append(findings, parseFinding(path, err))
			continue
		}

		switch filepath.Base(path) {
		case ".nvmrc", ".node-version":
			value, line, ok := oneVersionLine(data)
			if !ok {
				findings = append(findings, nodeVersionFinding(path, 0, "version file must contain exactly one non-empty line"))
				continue
			}
			facts = append(facts, nodeConstraint(path, line, strings.TrimPrefix(strings.TrimPrefix(value, "v"), "V")))
		case ".tool-versions":
			value, line, found, valid := firstToolVersion(data)
			if !found {
				findings = append(findings, nodeVersionFinding(path, 0, "no nodejs entry found"))
				continue
			}
			if !valid {
				findings = append(findings, nodeVersionFinding(path, line, "nodejs entry must include a version"))
				continue
			}
			facts = append(facts, nodeConstraint(path, line, value))
		}
	}
	return facts, findings
}

func Dockerfile(root, path string) ([]model.Fact, []model.Finding) {
	data, err := repository.Read(root, path, maxVersionEvidenceBytes)
	if err != nil {
		return nil, []model.Finding{parseFinding(path, err)}
	}

	var facts []model.Fact
	var findings []model.Finding
	for lineNumber, line := range splitLines(data) {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "FROM") {
			continue
		}
		image := fields[1]
		if len(image) < len("node:") || !strings.EqualFold(image[:len("node:")], "node:") {
			continue
		}
		tag := image[len("node:"):]
		if tag == "" || strings.Contains(tag, "$") {
			findings = append(findings, nodeVersionFinding(path, lineNumber+1, fmt.Sprintf("unsupported Node image tag %q", tag)))
			continue
		}
		tag = stripDockerImageSuffix(tag)
		if tag == "" {
			findings = append(findings, nodeVersionFinding(path, lineNumber+1, "Node image tag is empty"))
			continue
		}
		facts = append(facts, nodeConstraint(path, lineNumber+1, strings.TrimPrefix(strings.TrimPrefix(tag, "v"), "V")))
	}
	return facts, findings
}

func oneVersionLine(data []byte) (string, int, bool) {
	var value string
	lineNumber := 0
	for number, line := range splitLines(data) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if value != "" {
			return "", 0, false
		}
		value = line
		lineNumber = number + 1
	}
	return value, lineNumber, value != ""
}

func firstToolVersion(data []byte) (string, int, bool, bool) {
	for lineNumber, line := range splitLines(data) {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "nodejs" {
			continue
		}
		if len(fields) < 2 || fields[1] == "" {
			return "", lineNumber + 1, true, false
		}
		return strings.TrimPrefix(strings.TrimPrefix(fields[1], "v"), "V"), lineNumber + 1, true, true
	}
	return "", 0, false, false
}

func splitLines(data []byte) []string {
	return strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
}

func stripDockerImageSuffix(tag string) string {
	if index := strings.IndexByte(tag, '@'); index >= 0 {
		tag = tag[:index]
	}
	if index := strings.IndexByte(tag, '-'); index > 0 {
		tag = tag[:index]
	}
	return tag
}

func nodeConstraint(path string, line int, value string) model.Fact {
	return model.Fact{
		Kind:    model.FactKind("node.constraint"),
		Subject: "node",
		Value:   value,
		Source:  source(path, line, ""),
	}
}

func nodeVersionFinding(path string, line int, message string) model.Finding {
	return model.Finding{
		RuleID:     "node-version-unsupported",
		Severity:   model.SeverityWarning,
		Status:     model.StatusInconclusive,
		Message:    fmt.Sprintf("could not extract Node version from %s: %s", path, message),
		Evidence:   []model.Fact{{Kind: model.FactKind("node.version.unsupported"), Subject: "node", Source: source(path, line, "")}},
		Suggestion: "Use one concrete Node version or an explicitly supported constraint.",
	}
}

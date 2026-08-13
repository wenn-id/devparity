package extract

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wenn-id/devparity/internal/model"
	"github.com/wenn-id/devparity/internal/repository"
)

const maxPackageJSONBytes int64 = 1 << 20

type packageJSON struct {
	Engines struct {
		Node string `json:"node"`
	} `json:"engines"`
	PackageManager string            `json:"packageManager"`
	Scripts        map[string]string `json:"scripts"`
}

var lockfileManager = map[string]string{
	"package-lock.json":   "npm",
	"npm-shrinkwrap.json": "npm",
	"pnpm-lock.yaml":      "pnpm",
	"yarn.lock":           "yarn",
}

func PackageJSON(root, path string) ([]model.Fact, []model.Finding) {
	data, err := repository.Read(root, path, maxPackageJSONBytes)
	if err != nil {
		return nil, []model.Finding{parseFinding(path, err)}
	}

	lines, err := jsonFieldLines(data)
	if err != nil {
		return nil, []model.Finding{parseFinding(path, err)}
	}

	var packageData packageJSON
	if err := json.Unmarshal(data, &packageData); err != nil {
		return nil, []model.Finding{parseFinding(path, err)}
	}

	facts := make([]model.Fact, 0, 2+len(packageData.Scripts))
	if packageData.Engines.Node != "" {
		facts = append(facts, model.Fact{
			Kind:    model.FactKind("node.constraint"),
			Subject: "node",
			Value:   packageData.Engines.Node,
			Source:  source(path, lines["engines.node"], "engines.node"),
		})
	}
	if packageData.PackageManager != "" {
		manager := packageManagerName(packageData.PackageManager)
		field := "packageManager"
		if manager == "bun" {
			return facts, []model.Finding{unsupportedManagerFinding(path, lines[field], packageData.PackageManager)}
		}
		if manager == "npm" || manager == "pnpm" || manager == "yarn" {
			facts = append(facts, model.Fact{
				Kind:    model.FactKind("package.manager.declared"),
				Subject: "package-manager",
				Value:   manager,
				Source:  source(path, lines[field], field),
			})
		}
	}

	scriptNames := make([]string, 0, len(packageData.Scripts))
	for name := range packageData.Scripts {
		scriptNames = append(scriptNames, name)
	}
	// Stable fact order keeps reports deterministic even though JSON objects are unordered.
	for i := 1; i < len(scriptNames); i++ {
		for j := i; j > 0 && scriptNames[j] < scriptNames[j-1]; j-- {
			scriptNames[j], scriptNames[j-1] = scriptNames[j-1], scriptNames[j]
		}
	}
	for _, name := range scriptNames {
		field := "scripts." + name
		facts = append(facts, model.Fact{
			Kind:    model.FactKind("package.script"),
			Subject: name,
			Value:   packageData.Scripts[name],
			Source:  source(path, lines[field], field),
		})
	}
	return facts, nil
}

func Lockfiles(paths []string) []model.Fact {
	facts := make([]model.Fact, 0, len(paths))
	for _, path := range paths {
		manager, ok := lockfileManager[filepath.Base(path)]
		if !ok {
			continue
		}
		facts = append(facts, model.Fact{
			Kind:    model.FactKind("package.manager.lockfile"),
			Subject: "package-manager",
			Value:   manager,
			Source:  model.SourceRef{Path: filepath.ToSlash(path), Line: 1},
		})
	}
	return facts
}

func packageManagerName(value string) string {
	value = strings.TrimSpace(value)
	if at := strings.LastIndexByte(value, '@'); at > 0 {
		return strings.ToLower(value[:at])
	}
	return strings.ToLower(value)
}

func parseFinding(path string, err error) model.Finding {
	return model.Finding{
		RuleID:     "parse-error",
		Severity:   model.SeverityError,
		Status:     model.StatusInconclusive,
		Message:    fmt.Sprintf("could not parse %s: %v", path, err),
		Evidence:   []model.Fact{{Kind: model.FactKind("parse.error"), Subject: path, Value: err.Error(), Source: model.SourceRef{Path: filepath.ToSlash(path)}}},
		Suggestion: "Fix the supported artifact so DevParity can inspect it.",
	}
}

func unsupportedManagerFinding(path string, line int, value string) model.Finding {
	return model.Finding{
		RuleID:     "package-manager-unsupported",
		Severity:   model.SeverityWarning,
		Status:     model.StatusInconclusive,
		Message:    fmt.Sprintf("unsupported package manager %q", value),
		Evidence:   []model.Fact{{Kind: model.FactKind("package.manager.declared"), Subject: "package-manager", Value: value, Source: source(path, line, "packageManager")}},
		Suggestion: "Use npm, pnpm, or Yarn for beta analysis, or treat this result as unsupported.",
	}
}

func source(path string, line int, field string) model.SourceRef {
	return model.SourceRef{Path: filepath.ToSlash(path), Line: line, Field: field}
}

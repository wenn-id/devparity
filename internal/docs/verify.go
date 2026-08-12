package docs

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devparity/devparity/internal/model"
	"github.com/devparity/devparity/internal/nodecmd"
)

func Validate(blocks []model.DocBlock, facts []model.Fact) []model.Finding {
	scripts := make(map[string]struct{})
	for _, fact := range facts {
		if fact.Kind == model.FactKind("package.script") {
			scripts[fact.Subject] = struct{}{}
		}
	}

	var findings []model.Finding
	for _, block := range blocks {
		missing := make(map[string]struct{})
		unsupported := false
		for _, line := range strings.Split(block.Script, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			command, ok := nodecmd.Parse(line)
			if !ok {
				unsupported = true
				continue
			}
			if command.Script == "" {
				continue
			}
			if _, ok := scripts[command.Script]; !ok {
				missing[command.Script] = struct{}{}
			}
		}
		if unsupported {
			findings = append(findings, model.Finding{
				RuleID:     "doc-command-unsupported",
				Severity:   model.SeverityWarning,
				Status:     model.StatusInconclusive,
				Message:    fmt.Sprintf("documentation block %s contains unsupported shell syntax", block.ID),
				Evidence:   []model.Fact{{Kind: model.FactKind("documentation.command"), Subject: block.ID, Value: block.Script, Source: block.Source}},
				Suggestion: "Use one direct npm, pnpm, or Yarn command per line.",
			})
			continue
		}
		missingNames := make([]string, 0, len(missing))
		for name := range missing {
			missingNames = append(missingNames, name)
		}
		sort.Strings(missingNames)
		if len(missingNames) > 0 {
			for _, name := range missingNames {
				findings = append(findings, model.Finding{
					RuleID:     "missing-package-script",
					Severity:   model.SeverityError,
					Status:     model.StatusFinding,
					Message:    fmt.Sprintf("documentation references missing package script %q", name),
					Evidence:   []model.Fact{{Kind: model.FactKind("package.script"), Subject: "missing", Value: name, Source: block.Source}},
					Suggestion: fmt.Sprintf("Add package script %q or correct the documentation.", name),
				})
			}
			continue
		}
		findings = append(findings, model.Finding{
			RuleID:   "doc-script-validation",
			Severity: model.SeverityInfo,
			Status:   model.StatusPass,
			Message:  fmt.Sprintf("recognized package scripts in %s exist", filepath.ToSlash(block.ID)),
			Evidence: []model.Fact{{Kind: model.FactKind("documentation.command"), Subject: block.ID, Value: block.Script, Source: block.Source}},
		})
	}
	return findings
}

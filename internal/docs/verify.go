package docs

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wenn-id/devparity/internal/model"
	"github.com/wenn-id/devparity/internal/nodecmd"
)

func ExecutionFindings(blocks []model.DocBlock, results []model.ExecutionResult) ([]model.Finding, error) {
	byID := make(map[string]model.DocBlock, len(blocks))
	for _, block := range blocks {
		if _, exists := byID[block.ID]; exists {
			return nil, fmt.Errorf("duplicate documentation block ID %q", block.ID)
		}
		byID[block.ID] = block
	}
	seen := make(map[string]struct{}, len(results))
	findings := make([]model.Finding, 0, len(results))
	for _, result := range results {
		block, ok := byID[result.BlockID]
		if !ok {
			return nil, fmt.Errorf("execution result references unknown documentation block %q", result.BlockID)
		}
		if _, exists := seen[result.BlockID]; exists {
			return nil, errors.New("duplicate execution result")
		}
		seen[result.BlockID] = struct{}{}
		finding := model.Finding{
			Evidence: []model.Fact{{Kind: model.FactKind("documentation.command"), Subject: block.ID, Value: block.Script, Source: block.Source}},
		}
		switch result.Status {
		case model.StatusPass:
			finding.RuleID = "docs-command-passed"
			finding.Severity = model.SeverityInfo
			finding.Status = model.StatusPass
			finding.Message = fmt.Sprintf("documentation command %s completed successfully", block.ID)
		case model.StatusFinding:
			finding.RuleID = "docs-command-failed"
			finding.Severity = model.SeverityError
			finding.Status = model.StatusFinding
			finding.Message = fmt.Sprintf("documentation command %s exited with code %d", block.ID, result.ExitCode)
			finding.Suggestion = "Fix the command or update the marked documentation."
		case model.StatusSkipped:
			finding.RuleID = "docs-command-skipped"
			finding.Severity = model.SeverityWarning
			finding.Status = model.StatusSkipped
			finding.Message = fmt.Sprintf("documentation command %s was skipped", block.ID)
			finding.Suggestion = "Install the requested shell or use a supported execution mode."
		default:
			return nil, fmt.Errorf("unknown execution status %q for %q", result.Status, result.BlockID)
		}
		if result.Stdout != "" {
			finding.Evidence = append(finding.Evidence, model.Fact{Kind: model.FactKind("execution.stdout"), Subject: block.ID, Value: result.Stdout, Source: block.Source})
		}
		if result.Stderr != "" {
			finding.Evidence = append(finding.Evidence, model.Fact{Kind: model.FactKind("execution.stderr"), Subject: block.ID, Value: result.Stderr, Source: block.Source})
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

// CanExecute reports whether every non-empty line in a documentation block is
// one of the direct Node package-manager commands accepted by Validate. Keep
// this predicate beside validation so reporting and execution cannot drift.
func CanExecute(block model.DocBlock) bool {
	commands, ok := parseCommands(block)
	if !ok {
		return false
	}
	for _, command := range commands {
		if command.Operation == "builtin" {
			return false
		}
	}
	return true
}

func parseCommands(block model.DocBlock) ([]nodecmd.Command, bool) {
	commands := make([]nodecmd.Command, 0)
	for _, line := range strings.Split(block.Script, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		command, ok := nodecmd.Parse(line)
		if !ok {
			return nil, false
		}
		commands = append(commands, command)
	}
	if len(commands) == 0 {
		return nil, false
	}
	return commands, true
}

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
		commands, supported := parseCommands(block)
		for _, command := range commands {
			if command.Script == "" {
				continue
			}
			if _, ok := scripts[command.Script]; !ok {
				missing[command.Script] = struct{}{}
			}
		}
		if !supported {
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

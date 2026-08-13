package analyze

import (
	"path/filepath"

	"github.com/wenn-id/devparity/internal/docs"
	"github.com/wenn-id/devparity/internal/extract"
	"github.com/wenn-id/devparity/internal/model"
	"github.com/wenn-id/devparity/internal/nodecmd"
	"github.com/wenn-id/devparity/internal/repository"
	"github.com/wenn-id/devparity/internal/rules"
)

func Doctor(root, toolVersion string) (model.Report, error) {
	artifacts, err := repository.Discover(root)
	if err != nil {
		return model.Report{}, err
	}

	var facts []model.Fact
	var findings []model.Finding
	if artifacts.PackageJSON != "" {
		packageFacts, packageFindings := extract.PackageJSON(artifacts.Root, artifacts.PackageJSON)
		facts = append(facts, packageFacts...)
		findings = append(findings, packageFindings...)
	}
	facts = append(facts, extract.Lockfiles(artifacts.Lockfiles)...)
	versionFacts, versionFindings := extract.VersionFiles(artifacts.Root, artifacts.VersionFiles)
	facts = append(facts, versionFacts...)
	findings = append(findings, versionFindings...)
	if artifacts.Dockerfile != "" {
		dockerFacts, dockerFindings := extract.Dockerfile(artifacts.Root, artifacts.Dockerfile)
		facts = append(facts, dockerFacts...)
		findings = append(findings, dockerFindings...)
	}
	markdownFacts, markdownFindings := extract.MarkdownVersions(artifacts.Root, artifacts.Markdown)
	facts = append(facts, markdownFacts...)
	findings = append(findings, markdownFindings...)
	workflowFacts, workflowFindings := extract.Workflows(artifacts.Root, artifacts.Workflows)
	facts = append(facts, workflowFacts...)
	findings = append(findings, workflowFindings...)

	blocks, blockFindings := docs.Extract(artifacts.Root, artifacts.Markdown)
	findings = append(findings, blockFindings...)
	facts = append(facts, docCommandFacts(blocks)...)
	findings = append(findings, docs.Validate(blocks, facts)...)
	facts = append(facts, workflowPackageManagerFacts(workflowFacts)...)

	findings = append(findings, rules.Evaluate(facts)...)
	model.SortFindings(findings)
	return model.Report{
		SchemaVersion: 1,
		ToolVersion:   toolVersion,
		Repository:    filepath.ToSlash(artifacts.Root),
		Summary:       model.Summarize(findings),
		Results:       findings,
	}, nil
}

func docCommandFacts(blocks []model.DocBlock) []model.Fact {
	var facts []model.Fact
	for _, block := range blocks {
		for offset, line := range splitBlockLines(block.Script) {
			command, ok := nodecmd.Parse(line)
			if !ok {
				continue
			}
			subject := command.Script
			if subject == "" {
				subject = command.Operation
			}
			facts = append(facts, model.Fact{
				Kind:    model.FactKind("doc.command"),
				Subject: subject,
				Value:   line,
				Source:  model.SourceRef{Path: block.Source.Path, Line: block.Source.Line + offset, Field: "documentation"},
			})
			if command.Manager != "" {
				facts = append(facts, model.Fact{
					Kind:    model.FactKind("package.manager.command"),
					Subject: command.Operation,
					Value:   command.Manager,
					Source:  model.SourceRef{Path: block.Source.Path, Line: block.Source.Line + offset, Field: "documentation"},
				})
			}
		}
	}
	return facts
}

func workflowPackageManagerFacts(facts []model.Fact) []model.Fact {
	out := make([]model.Fact, 0)
	for _, fact := range facts {
		if fact.Kind != model.FactKind("workflow.command") {
			continue
		}
		command, ok := nodecmd.Parse(fact.Value)
		if !ok {
			continue
		}
		out = append(out, model.Fact{Kind: model.FactKind("package.manager.command"), Subject: command.Operation, Value: command.Manager, Source: fact.Source})
	}
	return out
}

func splitBlockLines(script string) []string {
	if script == "" {
		return nil
	}
	return splitLines(script)
}

func splitLines(value string) []string {
	lines := []string{}
	start := 0
	for i, character := range value {
		if character != '\n' {
			continue
		}
		lines = append(lines, value[start:i])
		start = i + 1
	}
	lines = append(lines, value[start:])
	return lines
}

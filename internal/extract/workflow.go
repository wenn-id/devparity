package extract

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/devparity/devparity/internal/model"
	"github.com/devparity/devparity/internal/nodecmd"
	"github.com/devparity/devparity/internal/repository"
)

const maxWorkflowBytes int64 = 1 << 20

func Workflows(root string, paths []string) ([]model.Fact, []model.Finding) {
	ordered := append([]string(nil), paths...)
	sort.Strings(ordered)
	var facts []model.Fact
	var findings []model.Finding
	for _, path := range ordered {
		data, err := repository.Read(root, path, maxWorkflowBytes)
		if err != nil {
			findings = append(findings, parseFinding(path, err))
			continue
		}
		var document yaml.Node
		if err := yaml.Unmarshal(data, &document); err != nil {
			findings = append(findings, parseFinding(path, err))
			continue
		}
		fileFacts, fileFindings := workflowDocument(filepath.ToSlash(path), &document)
		facts = append(facts, fileFacts...)
		findings = append(findings, fileFindings...)
	}
	return facts, findings
}

func workflowDocument(path string, document *yaml.Node) ([]model.Fact, []model.Finding) {
	root := document
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	jobs := mappingValue(root, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		return nil, nil
	}

	var facts []model.Fact
	var findings []model.Finding
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		jobName := jobs.Content[i].Value
		job := jobs.Content[i+1]
		if job.Kind != yaml.MappingNode {
			findings = append(findings, workflowUnsupported(path, job.Line, fmt.Sprintf("job %q is not a mapping", jobName)))
			continue
		}
		if mappingValue(job, "uses") != nil {
			findings = append(findings, workflowUnsupported(path, job.Line, fmt.Sprintf("job %q is a reusable workflow", jobName)))
			continue
		}
		matrix, matrixFinding := workflowMatrix(path, job)
		if matrixFinding != nil {
			findings = append(findings, *matrixFinding)
		}
		jobFacts, jobFindings := workflowSteps(path, job, matrix)
		facts = append(facts, jobFacts...)
		findings = append(findings, jobFindings...)
	}
	return facts, findings
}

type matrixEntry struct {
	values []string
	line   int
}

type matrixValues map[string]matrixEntry

func workflowMatrix(path string, job *yaml.Node) (matrixValues, *model.Finding) {
	strategy := mappingValue(job, "strategy")
	if strategy == nil {
		return nil, nil
	}
	if strategy.Kind != yaml.MappingNode {
		finding := workflowUnsupported(path, strategy.Line, "job strategy is not a mapping")
		return nil, &finding
	}
	matrix := mappingValue(strategy, "matrix")
	if matrix == nil {
		return nil, nil
	}
	if matrix.Kind != yaml.MappingNode {
		finding := workflowUnsupported(path, matrix.Line, "job matrix must be a literal mapping")
		return nil, &finding
	}
	values := make(matrixValues)
	for i := 0; i+1 < len(matrix.Content); i += 2 {
		key, value := matrix.Content[i], matrix.Content[i+1]
		if value.Anchor != "" || value.Kind != yaml.SequenceNode {
			finding := workflowUnsupported(path, value.Line, fmt.Sprintf("matrix %q is not a literal sequence", key.Value))
			return nil, &finding
		}
		items := make([]string, 0, len(value.Content))
		for _, item := range value.Content {
			if item.Kind != yaml.ScalarNode || item.Tag == "!!null" || strings.Contains(item.Value, "${{") {
				finding := workflowUnsupported(path, item.Line, fmt.Sprintf("matrix %q contains a dynamic value", key.Value))
				return nil, &finding
			}
			items = append(items, item.Value)
		}
		values[key.Value] = matrixEntry{values: items, line: value.Line}
	}
	return values, nil
}

func workflowSteps(path string, job *yaml.Node, matrix matrixValues) ([]model.Fact, []model.Finding) {
	steps := mappingValue(job, "steps")
	if steps == nil {
		return nil, nil
	}
	if steps.Kind != yaml.SequenceNode {
		finding := workflowUnsupported(path, steps.Line, "job steps must be a sequence")
		return nil, []model.Finding{finding}
	}
	var facts []model.Fact
	var findings []model.Finding
	for _, step := range steps.Content {
		if step.Kind != yaml.MappingNode {
			findings = append(findings, workflowUnsupported(path, step.Line, "workflow step must be a mapping"))
			continue
		}
		if uses := mappingValue(step, "uses"); uses != nil {
			if uses.Kind != yaml.ScalarNode {
				findings = append(findings, workflowUnsupported(path, uses.Line, "step uses value must be scalar"))
				continue
			}
			if strings.HasPrefix(uses.Value, "./") {
				findings = append(findings, workflowUnsupported(path, uses.Line, "composite actions are unsupported"))
				continue
			}
			if strings.HasPrefix(uses.Value, "actions/setup-node@") {
				setupFacts, setupFinding := setupNodeFacts(path, step, matrix)
				facts = append(facts, setupFacts...)
				if setupFinding != nil {
					findings = append(findings, *setupFinding)
				}
			}
		}
		if run := mappingValue(step, "run"); run != nil {
			if run.Kind != yaml.ScalarNode {
				findings = append(findings, workflowUnsupported(path, run.Line, "workflow run value must be scalar"))
				continue
			}
			command, ok := nodecmd.Parse(run.Value)
			if !ok {
				findings = append(findings, workflowUnsupported(path, run.Line, fmt.Sprintf("unsupported workflow command %q", run.Value)))
				continue
			}
			facts = append(facts, model.Fact{
				Kind:    model.FactKind("workflow.command"),
				Subject: command.Operation,
				Value:   run.Value,
				Source:  model.SourceRef{Path: path, Line: run.Line, Field: "run"},
			})
		}
	}
	return facts, findings
}

func setupNodeFacts(path string, step *yaml.Node, matrix matrixValues) ([]model.Fact, *model.Finding) {
	with := mappingValue(step, "with")
	if with == nil {
		return nil, nil
	}
	if with.Kind != yaml.MappingNode {
		finding := workflowUnsupported(path, with.Line, "setup-node with must be a mapping")
		return nil, &finding
	}
	version := mappingValue(with, "node-version")
	if version == nil || version.Kind != yaml.ScalarNode {
		finding := workflowUnsupported(path, with.Line, "setup-node node-version must be scalar")
		return nil, &finding
	}
	if name, ok := exactMatrixReference(version.Value); ok {
		entry, exists := matrix[name]
		if !exists {
			finding := workflowUnsupported(path, version.Line, fmt.Sprintf("matrix %q is not a supported local sequence", name))
			return nil, &finding
		}
		facts := make([]model.Fact, 0, len(entry.values))
		for _, value := range entry.values {
			facts = append(facts, nodeConstraint(path, entry.line, value))
		}
		return facts, nil
	}
	if strings.Contains(version.Value, "${{") || strings.Contains(version.Value, "}}") {
		finding := workflowUnsupported(path, version.Line, "dynamic setup-node version is unsupported")
		return nil, &finding
	}
	return []model.Fact{nodeConstraint(path, version.Line, version.Value)}, nil
}

func exactMatrixReference(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "${{ matrix.") || !strings.HasSuffix(value, " }}") {
		return "", false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(value, "${{ matrix."), " }}")
	if name == "" || strings.ContainsAny(name, "[] \t\r\n") {
		return "", false
	}
	return name, true
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func workflowUnsupported(path string, line int, message string) model.Finding {
	return model.Finding{
		RuleID:     "workflow-unsupported",
		Severity:   model.SeverityWarning,
		Status:     model.StatusInconclusive,
		Message:    message,
		Evidence:   []model.Fact{{Kind: model.FactKind("workflow.unsupported"), Subject: "workflow", Value: message, Source: model.SourceRef{Path: path, Line: line}}},
		Suggestion: "Use literal GitHub Actions syntax supported by the beta, or treat this workflow result as inconclusive.",
	}
}

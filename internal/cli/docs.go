package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wenn-id/devparity/internal/docs"
	"github.com/wenn-id/devparity/internal/execute"
	"github.com/wenn-id/devparity/internal/extract"
	"github.com/wenn-id/devparity/internal/model"
	"github.com/wenn-id/devparity/internal/report"
	"github.com/wenn-id/devparity/internal/repository"
)

func runDocs(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "verify" {
		fmt.Fprintln(stderr, "usage: devparity docs verify [path] [--format text|json]")
		return 2
	}
	flags, path, err := normalizePathArgs(args[1:], docsVerifyFlags)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	set := flag.NewFlagSet("docs verify", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	format := set.String("format", "text", "report format")
	executeCommands := set.Bool("execute", false, "execute marked documentation")
	trustRepository := set.Bool("trust-repository", false, "trust the repository for host execution")
	containerMode := set.Bool("container", false, "execute marked documentation in a container")
	allowNetwork := set.Bool("allow-network", false, "allow container network access")
	nodeVersion := set.String("node-version", "", "container Node version")
	var envNames stringList
	set.Var(&envNames, "env", "forward one environment variable")
	timeout := set.Duration("timeout", 0, "execution timeout")
	if err := set.Parse(flags); err != nil {
		fmt.Fprintf(stderr, "%v\n%s\n", err, docsVerifyFlags.Usage())
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "unsupported format %q\n", *format)
		return 2
	}
	if path == "" {
		path = "."
	}
	if *executeCommands && *containerMode {
		fmt.Fprintln(stderr, "--execute and --container are mutually exclusive")
		return 2
	}
	if *trustRepository && !*executeCommands {
		fmt.Fprintln(stderr, "--trust-repository requires --execute")
		return 2
	}
	if *executeCommands != *trustRepository {
		fmt.Fprintln(stderr, "host execution requires both --execute and --trust-repository")
		return 2
	}
	if (*allowNetwork || *nodeVersion != "") && !*containerMode {
		fmt.Fprintln(stderr, "--allow-network and --node-version require --container")
		return 2
	}
	value, blocks, err := staticDocsData(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	executionFailed := false
	if *executeCommands || *containerMode {
		executableBlocks := make([]model.DocBlock, 0, len(blocks))
		for _, block := range blocks {
			if docs.CanExecute(block) {
				executableBlocks = append(executableBlocks, block)
			}
		}
		if len(executableBlocks) > 0 {
			if *executeCommands {
				fmt.Fprintln(stderr, "host execution is not sandboxed")
			}
			var hostGrant execute.Grant
			if *executeCommands {
				hostGrant, err = execute.NewHostGrant(true)
			} else {
				hostGrant = execute.NewContainerGrant()
				if *nodeVersion == "" {
					*nodeVersion, err = concreteNodeVersion(path)
				}
			}
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 2
			}
			environment, environmentErr := execute.SnapshotEnvironment(envNames)
			if environmentErr != nil {
				fmt.Fprintln(stderr, environmentErr)
				return 2
			}
			results := make([]model.ExecutionResult, 0, len(executableBlocks))
			for _, block := range executableBlocks {
				options := execute.Options{Root: value.Repository, Timeout: *timeout, EnvNames: envNames, Environment: &environment, AllowNetwork: *allowNetwork, NodeVersion: *nodeVersion}
				var result model.ExecutionResult
				var runErr error
				if *containerMode {
					result, runErr = execute.RunContainer(context.Background(), hostGrant, block, options)
				} else {
					result, runErr = execute.RunHost(context.Background(), hostGrant, block, options)
				}
				if runErr != nil {
					fmt.Fprintln(stderr, runErr)
					return 2
				}
				results = append(results, result)
			}
			executionFindings, findingErr := docs.ExecutionFindings(blocks, results)
			if findingErr != nil {
				fmt.Fprintln(stderr, findingErr)
				return 2
			}
			value.Results = append(value.Results, executionFindings...)
			for _, finding := range executionFindings {
				if finding.RuleID == "docs-command-failed" && finding.Status == model.StatusFinding {
					executionFailed = true
				}
			}
			model.SortFindings(value.Results)
			value.Summary = model.Summarize(value.Results)
		}
	}
	if *format == "json" {
		err = report.JSON(stdout, value)
	} else {
		err = report.Text(stdout, value)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if executionFailed {
		return 1
	}
	return 0
}

func concreteNodeVersion(root string) (string, error) {
	artifacts, err := repository.Discover(root)
	if err != nil {
		return "", err
	}
	facts, findings := extract.VersionFiles(artifacts.Root, artifacts.VersionFiles)
	if hasRelevantNodeVersionFinding(findings) {
		return "", fmt.Errorf("node version is inconclusive for container execution")
	}
	versionPattern := regexp.MustCompile(`^\d+(?:\.\d+){0,2}$`)
	version := ""
	for _, fact := range facts {
		if !versionPattern.MatchString(fact.Value) {
			continue
		}
		if version != "" && version != fact.Value {
			return "", fmt.Errorf("conflicting concrete node versions for container execution")
		}
		version = fact.Value
	}
	if version == "" {
		return "", fmt.Errorf("container execution requires --node-version or one concrete .nvmrc/.node-version")
	}
	return version, nil
}

func hasRelevantNodeVersionFinding(findings []model.Finding) bool {
	for _, finding := range findings {
		for _, evidence := range finding.Evidence {
			switch filepath.Base(evidence.Source.Path) {
			case ".nvmrc", ".node-version":
				return true
			case ".tool-versions":
				if !strings.Contains(finding.Message, "no nodejs entry found") {
					return true
				}
			}
		}
	}
	return false
}

func staticDocsData(root string) (model.Report, []model.DocBlock, error) {
	artifacts, err := repository.Discover(root)
	if err != nil {
		return model.Report{}, nil, err
	}
	var facts []model.Fact
	var findings []model.Finding
	if artifacts.PackageJSON != "" {
		packageFacts, packageFindings := extract.PackageJSON(artifacts.Root, artifacts.PackageJSON)
		facts = append(facts, packageFacts...)
		findings = append(findings, packageFindings...)
	}
	blocks, blockFindings := docs.Extract(artifacts.Root, artifacts.Markdown)
	findings = append(findings, blockFindings...)
	findings = append(findings, docs.Validate(blocks, facts)...)
	model.SortFindings(findings)
	return model.Report{
		SchemaVersion: 1,
		ToolVersion:   Version,
		Repository:    artifacts.Root,
		Summary:       model.Summarize(findings),
		Results:       findings,
	}, blocks, nil
}

type stringList []string

func (values *stringList) String() string { return strings.Join(*values, ",") }

func (values *stringList) Set(value string) error {
	if strings.TrimSpace(value) == "" || strings.Contains(value, "=") {
		return fmt.Errorf("invalid environment variable name %q", value)
	}
	*values = append(*values, value)
	return nil
}

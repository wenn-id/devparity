package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type Artifacts struct {
	Root         string
	PackageJSON  string
	Lockfiles    []string
	VersionFiles []string
	Dockerfile   string
	Markdown     []string
	Workflows    []string
}

var lockfileNames = []string{
	"package-lock.json",
	"npm-shrinkwrap.json",
	"pnpm-lock.yaml",
	"yarn.lock",
}

var versionFileNames = []string{
	".nvmrc",
	".node-version",
	".tool-versions",
}

var markdownNames = []string{
	"README.md",
	"CONTRIBUTING.md",
}

func Discover(path string) (Artifacts, error) {
	root, err := resolveRoot(path)
	if err != nil {
		return Artifacts{}, fmt.Errorf("resolve repository root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return Artifacts{}, fmt.Errorf("stat repository root: %w", err)
	}
	if !info.IsDir() {
		return Artifacts{}, fmt.Errorf("repository root %q is not a directory", path)
	}

	artifacts := Artifacts{Root: root}
	if artifacts.PackageJSON, err = discoverFile(root, "package.json"); err != nil {
		return Artifacts{}, err
	}
	for _, name := range lockfileNames {
		path, err := discoverFile(root, name)
		if err != nil {
			return Artifacts{}, err
		}
		if path != "" {
			artifacts.Lockfiles = append(artifacts.Lockfiles, path)
		}
	}
	for _, name := range versionFileNames {
		path, err := discoverFile(root, name)
		if err != nil {
			return Artifacts{}, err
		}
		if path != "" {
			artifacts.VersionFiles = append(artifacts.VersionFiles, path)
		}
	}
	if artifacts.Dockerfile, err = discoverFile(root, "Dockerfile"); err != nil {
		return Artifacts{}, err
	}
	for _, name := range markdownNames {
		path, err := discoverFile(root, name)
		if err != nil {
			return Artifacts{}, err
		}
		if path != "" {
			artifacts.Markdown = append(artifacts.Markdown, path)
		}
	}
	sort.Strings(artifacts.Lockfiles)
	sort.Strings(artifacts.VersionFiles)
	sort.Strings(artifacts.Markdown)

	artifacts.Workflows, err = discoverWorkflows(root)
	if err != nil {
		return Artifacts{}, err
	}
	return artifacts, nil
}

func discoverFile(root, relative string) (string, error) {
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	if _, err := os.Lstat(candidate); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("inspect artifact %q: %w", relative, err)
	}

	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve artifact %q: %w", relative, err)
	}
	if !within(root, resolved) {
		return "", fmt.Errorf("artifact %q resolves outside repository", relative)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat artifact %q: %w", relative, err)
	}
	if !info.Mode().IsRegular() {
		return "", nil
	}
	return filepath.ToSlash(relative), nil
}

func discoverWorkflows(root string) ([]string, error) {
	var paths []string
	for _, pattern := range []string{"*.yml", "*.yaml"} {
		matches, err := filepath.Glob(filepath.Join(root, ".github", "workflows", pattern))
		if err != nil {
			return nil, fmt.Errorf("glob workflows: %w", err)
		}
		for _, match := range matches {
			relative, err := filepath.Rel(root, match)
			if err != nil {
				return nil, fmt.Errorf("make workflow path relative: %w", err)
			}
			path, err := discoverFile(root, relative)
			if err != nil {
				return nil, err
			}
			if path != "" {
				paths = append(paths, path)
			}
		}
	}
	sort.Strings(paths)
	return paths, nil
}

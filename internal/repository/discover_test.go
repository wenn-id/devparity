package repository

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverAllowlistedArtifacts(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", "{}")
	writeFixture(t, root, "package-lock.json", "{}")
	writeFixture(t, root, "npm-shrinkwrap.json", "{}")
	writeFixture(t, root, "pnpm-lock.yaml", "lockfileVersion: 9")
	writeFixture(t, root, "yarn.lock", "# yarn lock")
	writeFixture(t, root, ".nvmrc", "22")
	writeFixture(t, root, ".node-version", "20")
	writeFixture(t, root, ".tool-versions", "nodejs 18")
	writeFixture(t, root, "Dockerfile", "FROM node:22")
	writeFixture(t, root, "README.md", "# readme")
	writeFixture(t, root, "CONTRIBUTING.md", "# contributing")
	writeFixture(t, root, "docs/ignored.md", "# ignored")
	writeFixture(t, root, "Dockerfile.dev", "FROM node:22")
	writeFixture(t, root, ".github/workflows/z.yaml", "jobs: {}")
	writeFixture(t, root, ".github/workflows/a.yml", "jobs: {}")
	writeFixture(t, root, ".github/workflows/ignored.txt", "jobs: {}")
	writeFixture(t, root, ".github/workflows/nested/deep.yml", "jobs: {}")

	got, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Root != resolvedRoot {
		t.Fatalf("root=%q, want %q", got.Root, resolvedRoot)
	}
	if got.PackageJSON != "package.json" {
		t.Fatalf("package.json=%q, want package.json", got.PackageJSON)
	}
	if !reflect.DeepEqual(got.Lockfiles, []string{
		"npm-shrinkwrap.json",
		"package-lock.json",
		"pnpm-lock.yaml",
		"yarn.lock",
	}) {
		t.Fatalf("lockfiles=%#v", got.Lockfiles)
	}
	if !reflect.DeepEqual(got.VersionFiles, []string{
		".node-version",
		".nvmrc",
		".tool-versions",
	}) {
		t.Fatalf("version files=%#v", got.VersionFiles)
	}
	if got.Dockerfile != "Dockerfile" {
		t.Fatalf("Dockerfile=%q, want Dockerfile", got.Dockerfile)
	}
	if !reflect.DeepEqual(got.Markdown, []string{"CONTRIBUTING.md", "README.md"}) {
		t.Fatalf("markdown=%#v", got.Markdown)
	}
	if !reflect.DeepEqual(got.Workflows, []string{
		".github/workflows/a.yml",
		".github/workflows/z.yaml",
	}) {
		t.Fatalf("workflows=%#v", got.Workflows)
	}
}

func TestDiscoverRejectsNonDirectoryRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(root, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(root); err == nil {
		t.Fatal("expected non-directory root error")
	}
}

func writeFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

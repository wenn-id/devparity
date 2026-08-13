package cli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocsVerify(t *testing.T) {
	clean := filepath.Join("..", "..", "testdata", "repos", "clean-node")
	before := treeHash(t, clean)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"docs", "verify", clean}, &stdout, &stderr); code != 0 {
		t.Fatalf("clean code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if after := treeHash(t, clean); before != after {
		t.Fatalf("clean fixture changed: before=%s after=%s", before, after)
	}

	tests := []struct {
		name     string
		readme   string
		contains string
	}{
		{name: "missing script", readme: "<!-- devparity:run -->\n```sh\nnpm run missing\n```\n", contains: "missing-package-script"},
		{name: "unsupported shell", readme: "<!-- devparity:run -->\n```javascript\nnpm test\n```\n", contains: "doc-shell-unsupported"},
		{name: "malformed fence", readme: "<!-- devparity:run -->\n```sh\nnpm test\n", contains: "doc-block-unterminated"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeCLIFile(t, root, "package.json", `{"scripts":{"test":"node --test"}}`)
			writeCLIFile(t, root, "README.md", tt.readme)
			var stdout, stderr bytes.Buffer
			if code := Run([]string{"docs", "verify", root}, &stdout, &stderr); code != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.contains) {
				t.Fatalf("stdout=%q, missing %q", stdout.String(), tt.contains)
			}
		})
	}
}

func TestDocsExecutionRequiresCompleteTrustFlags(t *testing.T) {
	clean := filepath.Join("..", "..", "testdata", "repos", "clean-node")
	for _, args := range [][]string{
		{"docs", "verify", clean, "--execute"},
		{"docs", "verify", clean, "--trust-repository"},
	} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("args=%#v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stderr.String(), "trust") {
			t.Fatalf("args=%#v stderr=%q", args, stderr.String())
		}
	}
}

func TestDocsHostExecutionReturnsFailureExitCode(t *testing.T) {
	malicious := filepath.Join("..", "..", "testdata", "repos", "malicious-docs")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"docs", "verify", malicious, "--execute", "--trust-repository", "--timeout", "100ms"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "docs-command-failed") || !strings.Contains(stderr.String(), "not sandboxed") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestDocsContainerModeCanSkipWhenRuntimeIsUnavailable(t *testing.T) {
	clean := filepath.Join("..", "..", "testdata", "repos", "clean-node")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"docs", "verify", clean, "--container", "--node-version", "22"}, &stdout, &stderr)
	if code != 0 && code != 2 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if code == 0 && !strings.Contains(stdout.String(), "docs-command-skipped") {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if code == 2 && !strings.Contains(stdout.String()+stderr.String(), "container runtime failed") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func treeHash(t *testing.T, root string) string {
	t.Helper()
	hash := sha256.New()
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(hash, "%s\x00", strings.TrimPrefix(filepath.ToSlash(path), filepath.ToSlash(root)))
		hash.Write(data)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func writeCLIFile(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

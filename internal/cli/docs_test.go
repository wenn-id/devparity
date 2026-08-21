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

func TestDocsHostExecutionRejectsMissingEnvironmentBeforeRunning(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "must-not-run")
	writeCLIFile(t, root, "package.json", `{"scripts":{"test":"touch must-not-run"}}`)
	writeCLIFile(t, root, "README.md", "<!-- devparity:run -->\n```sh\nnpm test\n```\n")
	const variable = "DEVPARITY_TEST_CLI_MISSING"
	previous, wasSet := os.LookupEnv(variable)
	if err := os.Unsetenv(variable); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(variable, previous)
		} else {
			_ = os.Unsetenv(variable)
		}
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"docs", "verify", root, "--execute", "--trust-repository", "--env", variable}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), `requested environment variable "`+variable+`" is not set`) {
		t.Fatalf("stderr=%q", stderr.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("documentation command ran despite missing environment: %v", err)
	}
}

func TestDocsExecutionSkipsUnsupportedCommands(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "host", args: []string{"--execute", "--trust-repository"}},
		{name: "container", args: []string{"--container", "--node-version", "22"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			marker := filepath.Join(root, "must-not-run")
			writeCLIFile(t, root, "package.json", `{"scripts":{"test":"node --test"}}`)
			writeCLIFile(t, root, "README.md", "<!-- devparity:run -->\n```sh\ntouch "+marker+"\n```\n")

			var stdout, stderr bytes.Buffer
			args := append([]string{"docs", "verify", root}, tc.args...)
			code := Run(args, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), "doc-command-unsupported") {
				t.Fatalf("stdout=%q, missing unsupported-command finding", stdout.String())
			}
			if strings.Contains(stdout.String(), "docs-command-passed") || strings.Contains(stdout.String(), "docs-command-failed") || strings.Contains(stdout.String(), "docs-command-skipped") {
				t.Fatalf("unsupported command received execution finding: stdout=%q", stdout.String())
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("unsupported documentation command ran: %v", err)
			}
		})
	}
}

func TestDocsExecutionIgnoresMarkedBlocksInsideOuterFence(t *testing.T) {
	for _, test := range []struct {
		name     string
		readme   string
		wantLive bool
	}{
		{name: "top level", readme: "````markdown\n<!-- devparity:run -->\n```sh\nnpm test\n```\n````\n"},
		{name: "list item", readme: "- ````markdown\n  <!-- devparity:run -->\n  ```sh\n  npm test\n  ```\n  ````\n"},
		{name: "tab-separated list item", readme: "-	````markdown\n    <!-- devparity:run -->\n    ```sh\n    npm test\n    ```\n    ````\n"},
		{name: "ordered list followed by live block", readme: "12. ````markdown\n    <!-- devparity:run -->\n    ```sh\n    npm test\n    ```\n    ````\n\n<!-- devparity:run -->\n```sh\nnpm test\n```\n", wantLive: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			marker := filepath.Join(root, "nested-example-ran")
			writeCLIFile(t, root, "package.json", `{"scripts":{"test":"touch nested-example-ran"}}`)
			writeCLIFile(t, root, "README.md", test.readme)

			var stdout, stderr bytes.Buffer
			code := Run([]string{"docs", "verify", root, "--execute", "--trust-repository"}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if !test.wantLive && (strings.Contains(stdout.String(), "docs-command-") || strings.Contains(stdout.String(), "doc-script-validation")) {
				t.Fatalf("nested example was treated as live documentation: stdout=%q", stdout.String())
			}
			if test.wantLive {
				if strings.Count(stdout.String(), "docs-command-passed") != 1 || strings.Count(stdout.String(), "doc-script-validation") != 1 {
					t.Fatalf("live block after list fence was not executed exactly once: stdout=%q", stdout.String())
				}
				if _, err := os.Stat(marker); err != nil {
					t.Fatalf("live documentation block did not run: %v", err)
				}
				return
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("nested documentation example ran: %v", err)
			}
		})
	}
}

func TestDocsExecutionRunsSupportedBlocksAndSkipsUnsupportedBlocks(t *testing.T) {
	root := t.TempDir()
	unsupportedMarker := filepath.Join(root, "unsupported-ran")
	supportedMarker := filepath.Join(root, "supported-ran")
	writeCLIFile(t, root, "package.json", `{"scripts":{"test":"touch supported-ran"}}`)
	writeCLIFile(t, root, "README.md", "<!-- devparity:run -->\n```sh\ntouch "+unsupportedMarker+"\n```\n\n<!-- devparity:run -->\n```sh\nnpm test\n```\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"docs", "verify", root, "--execute", "--trust-repository"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Count(stdout.String(), "doc-command-unsupported") != 1 || strings.Count(stdout.String(), "docs-command-passed") != 1 {
		t.Fatalf("mixed execution findings are inconsistent: stdout=%q", stdout.String())
	}
	if _, err := os.Stat(unsupportedMarker); !os.IsNotExist(err) {
		t.Fatalf("unsupported documentation command ran: %v", err)
	}
	if _, err := os.Stat(supportedMarker); err != nil {
		t.Fatalf("supported documentation command did not run: %v", err)
	}
}

func TestDocsHostExecutionReturnsFailureExitCode(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, "package.json", `{"scripts":{"test":"sleep 2"}}`)
	writeCLIFile(t, root, "README.md", "<!-- devparity:run -->\n```sh\nnpm test\n```\n")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"docs", "verify", root, "--execute", "--trust-repository", "--timeout", "100ms"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "docs-command-failed") || !strings.Contains(stderr.String(), "not sandboxed") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestConcreteNodeVersionIgnoresUnrelatedVersionFinding(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, ".nvmrc", "22\n")
	writeCLIFile(t, root, ".tool-versions", "python 3.12\n")

	version, err := concreteNodeVersion(root)
	if err != nil {
		t.Fatalf("concreteNodeVersion() error=%v", err)
	}
	if version != "22" {
		t.Fatalf("concreteNodeVersion()=%q, want 22", version)
	}
}

func TestConcreteNodeVersionRejectsRelevantVersionFinding(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, ".nvmrc", "22\n")
	writeCLIFile(t, root, ".node-version", "20\n21\n")

	if _, err := concreteNodeVersion(root); err == nil || !strings.Contains(err.Error(), "inconclusive") {
		t.Fatalf("concreteNodeVersion() error=%v, want inconclusive Node version error", err)
	}
}

func TestDocsContainerModeCanSkipWhenRuntimeIsUnavailable(t *testing.T) {
	clean := filepath.Join("..", "..", "testdata", "repos", "clean-node")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"docs", "verify", clean, "--container", "--node-version", "22"}, &stdout, &stderr)
	if code != 0 && code != 1 && code != 2 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if code == 0 && !strings.Contains(stdout.String(), "docs-command-skipped") && !strings.Contains(stdout.String(), "docs-command-passed") {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if code == 1 && !strings.Contains(stdout.String(), "docs-command-failed") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if code == 2 && !strings.Contains(stdout.String()+stderr.String(), "container runtime failed") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestDocsContainerRejectsInvalidNodeVersionBeforeRuntimeProbe(t *testing.T) {
	clean := filepath.Join("..", "..", "testdata", "repos", "clean-node")
	for _, version := range []string{"latest", "22-alpine", "22.1.2.3", "v22", "22@sha256:bad"} {
		t.Run(version, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run([]string{"docs", "verify", clean, "--container", "--node-version", version}, &stdout, &stderr)
			if code != 2 || !strings.Contains(stderr.String(), "invalid --node-version") {
				t.Fatalf("version=%q code=%d stdout=%q stderr=%q", version, code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestDocsContainerValidatesNodeImageDigest(t *testing.T) {
	clean := filepath.Join("..", "..", "testdata", "repos", "clean-node")
	for _, digest := range []string{"sha256:bad", strings.Repeat("a", 64), "sha512:" + strings.Repeat("a", 64), "sha256:" + strings.Repeat("A", 64)} {
		t.Run(digest, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run([]string{"docs", "verify", clean, "--container", "--node-version", "22", "--node-image-digest", digest}, &stdout, &stderr)
			if code != 2 || !strings.Contains(stderr.String(), "invalid --node-image-digest") {
				t.Fatalf("digest=%q code=%d stdout=%q stderr=%q", digest, code, stdout.String(), stderr.String())
			}
		})
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

package execute

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyWorkspaceExcludesMutableDirectoriesAndCleansUp(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "README.md", "ok")
	writeWorkspaceFile(t, root, ".git/config", "ignored")
	writeWorkspaceFile(t, root, "node_modules/pkg/index.js", "ignored")
	writeWorkspaceFile(t, root, ".devparity/cache", "ignored")
	copy, cleanup, err := CopyWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(copy, "README.md")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".git", "node_modules", ".devparity"} {
		if _, err := os.Stat(filepath.Join(copy, name)); !os.IsNotExist(err) {
			t.Fatalf("%s was copied", name)
		}
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(copy); !os.IsNotExist(err) {
		t.Fatalf("copy still exists: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "README.md")); err != nil || string(data) != "ok" {
		t.Fatalf("source changed: data=%q err=%v", data, err)
	}
}

func TestCopyWorkspaceRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, _, err := CopyWorkspace(root); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func writeWorkspaceFile(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

package execute

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCopyWorkspaceMakesNestedCopyAccessibleToContainerUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	root := t.TempDir()
	writeWorkspaceFile(t, root, "package.json", "{}")
	writeWorkspaceFile(t, root, "src/index.js", "console.log('ok')")
	copy, cleanup, err := CopyWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup() })
	for _, path := range []string{copy, filepath.Join(copy, "src")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o777 {
			t.Fatalf("directory %q mode=%o, want 777", path, info.Mode().Perm())
		}
	}
	info, err := os.Stat(filepath.Join(copy, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("file mode=%o, want 644", info.Mode().Perm())
	}
	if err := os.WriteFile(filepath.Join(copy, "container-artifact"), []byte("created"), 0o666); err != nil {
		t.Fatalf("workspace is not writable: %v", err)
	}
}

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

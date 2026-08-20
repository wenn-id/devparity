package execute

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
	parent := filepath.Dir(copy)
	if parent == os.TempDir() {
		t.Fatalf("workspace %q is exposed directly beneath the shared temp directory", copy)
	}
	parentInfo, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if parentInfo.Mode().Perm() != 0o700 {
		t.Fatalf("workspace parent %q mode=%o, want 700", parent, parentInfo.Mode().Perm())
	}
	if filepath.Base(copy) != "workspace" {
		t.Fatalf("workspace path=%q, want private-parent/workspace layout", copy)
	}
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
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("private workspace parent still exists after cleanup: %v", err)
	}
}

func TestCopyWorkspaceRejectsOversizedFileAndCleansUp(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "small", "ok")
	writeWorkspaceFile(t, root, "large", strings.Repeat("x", int(workspaceMaxFileBytes)+1))

	copy, cleanup, err := CopyWorkspaceWithContext(context.Background(), root, WorkspaceLimits{MaxFileBytes: workspaceMaxFileBytes, MaxTotalBytes: workspaceMaxTotalBytes, MaxFiles: workspaceMaxFiles})
	if err == nil {
		_ = cleanup()
		t.Fatal("expected oversized-file error")
	}
	if copy != "" || cleanup != nil {
		t.Fatalf("copy=%q cleanup=%v after failure", copy, cleanup != nil)
	}
	if !strings.Contains(err.Error(), "maximum file size") {
		t.Fatalf("err=%v", err)
	}
}

func TestCopyWorkspaceRejectsCumulativeSizeAndCleansUp(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "one", "12")
	writeWorkspaceFile(t, root, "two", "34")

	copy, cleanup, err := CopyWorkspaceWithContext(context.Background(), root, WorkspaceLimits{MaxFileBytes: 2, MaxTotalBytes: 3, MaxFiles: 10})
	if err == nil {
		_ = cleanup()
		t.Fatal("expected cumulative-size error")
	}
	if copy != "" || cleanup != nil {
		t.Fatalf("copy=%q cleanup=%v after failure", copy, cleanup != nil)
	}
	if !strings.Contains(err.Error(), "maximum size 3 bytes") {
		t.Fatalf("err=%v", err)
	}
}

func TestCopyWorkspaceBoundsDirectoryEntries(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "a/b/file", "ok")

	copy, cleanup, err := CopyWorkspaceWithContext(context.Background(), root, WorkspaceLimits{MaxFileBytes: 10, MaxTotalBytes: 10, MaxFiles: 2})
	if err == nil {
		_ = cleanup()
		t.Fatal("expected directory-entry limit error")
	}
	if copy != "" || cleanup != nil {
		t.Fatalf("copy=%q cleanup=%v after failure", copy, cleanup != nil)
	}
	if !strings.Contains(err.Error(), "maximum file count") {
		t.Fatalf("err=%v", err)
	}
}

func TestCopyWorkspaceRejectsFileCountAndCleansUp(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "one", "1")
	writeWorkspaceFile(t, root, "two", "2")

	copy, cleanup, err := CopyWorkspaceWithContext(context.Background(), root, WorkspaceLimits{MaxFileBytes: 10, MaxTotalBytes: 10, MaxFiles: 1})
	if err == nil {
		_ = cleanup()
		t.Fatal("expected file-count error")
	}
	if copy != "" || cleanup != nil {
		t.Fatalf("copy=%q cleanup=%v after failure", copy, cleanup != nil)
	}
	if !strings.Contains(err.Error(), "maximum file count") {
		t.Fatalf("err=%v", err)
	}
}

func TestCopyWorkspaceCancellationCleansUpPartialCopy(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "a-first", strings.Repeat("x", 8<<20))
	writeWorkspaceFile(t, root, "b-second", strings.Repeat("y", 8<<20))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	before, err := workspaceTempDirs()
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	timedOut := make(chan struct{})
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-stop:
				return
			case <-timer.C:
				close(timedOut)
				return
			case <-ticker.C:
				paths, globErr := workspaceTempDirs()
				if globErr != nil {
					close(timedOut)
					return
				}
				for _, parent := range paths {
					if containsPath(before, parent) {
						continue
					}
					workspace := parent
					if info, statErr := os.Stat(filepath.Join(parent, "workspace")); statErr == nil && info.IsDir() {
						workspace = filepath.Join(parent, "workspace")
					}
					if _, statErr := os.Stat(filepath.Join(workspace, "a-first")); statErr == nil {
						close(started)
						cancel()
						return
					}
				}
			}
		}
	}()
	copy, cleanup, copyErr := CopyWorkspaceWithContext(ctx, root, WorkspaceLimits{MaxFileBytes: 16 << 20, MaxTotalBytes: 32 << 20, MaxFiles: 10})
	close(stop)
	<-done
	if copyErr == nil {
		_ = cleanup()
		t.Fatal("expected cancellation error")
	}
	if copy != "" || cleanup != nil {
		t.Fatalf("copy=%q cleanup=%v after failure", copy, cleanup != nil)
	}
	select {
	case <-started:
	case <-timedOut:
		t.Fatal("context was not canceled during workspace copy")
	default:
		t.Fatal("cancellation observer did not finish")
	}
	if !strings.Contains(copyErr.Error(), "workspace copy canceled") {
		t.Fatalf("err=%v", copyErr)
	}
	leftovers, err := workspaceTempDirs()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range leftovers {
		if !containsPath(before, path) {
			t.Fatalf("partial workspace directory remains: %q", path)
		}
	}
}

func workspaceTempDirs() ([]string, error) {
	return filepath.Glob(filepath.Join(os.TempDir(), "devparity-workspace-*"))
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
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

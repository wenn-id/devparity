package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadReturnsContentWithinLimit(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "nested/file.txt", "hello")

	got, err := Read(root, "nested/./file.txt", 5)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("content=%q, want hello", got)
	}
}

func TestReadRejectsAbsoluteAndEscapingPaths(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"../outside.txt", outside} {
		t.Run(path, func(t *testing.T) {
			if _, err := Read(root, path, 1024); err == nil {
				t.Fatalf("Read(%q) unexpectedly succeeded", path)
			}
		})
	}
}

func TestReadRejectsSymlinkEscapingRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation denied: %v", err)
	}

	if _, err := Read(root, "link.txt", 1024); err == nil {
		t.Fatal("expected symlink escape error")
	}
}

func TestReadAllowsSymlinkStayingWithinRoot(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "real/file.txt", "inside")
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "linkdir")); err != nil {
		t.Skipf("symlink creation denied: %v", err)
	}

	got, err := Read(root, "linkdir/file.txt", 6)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "inside" {
		t.Fatalf("content=%q, want inside", got)
	}
}

func TestReadRejectsNonRegularFileAndOversize(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "file.txt", "hello")

	for _, test := range []struct {
		name string
		path string
		max  int64
	}{
		{name: "oversize", path: "file.txt", max: 4},
		{name: "directory", path: ".", max: 1024},
		{name: "negative limit", path: "file.txt", max: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Read(root, test.path, test.max); err == nil {
				t.Fatal("expected read error")
			}
		})
	}
}

func TestReadRejectsDataAddedPastLimit(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "empty.txt", "")
	got, err := Read(root, "empty.txt", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("content=%q, want empty", got)
	}

	writeFixture(t, root, "nonempty.txt", "x")
	if _, err := Read(root, "nonempty.txt", 0); err == nil {
		t.Fatal("expected zero-byte limit error")
	}
}

func TestReadErrorDoesNotExposeOutsideContent(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Read(root, "../"+filepath.Base(filepath.Dir(outside))+"/outside.txt", 1024)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unexpected error=%v", err)
	}
}

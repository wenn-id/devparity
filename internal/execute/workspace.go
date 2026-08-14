package execute

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func CopyWorkspace(root string) (string, func() error, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", nil, err
	}
	target, err := os.MkdirTemp("", "devparity-workspace-")
	if err != nil {
		return "", nil, err
	}
	if err := os.Chmod(target, 0o755); err != nil {
		_ = os.RemoveAll(target)
		return "", nil, err
	}
	cleanup := func() error { return os.RemoveAll(target) }
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() && excludedDirectory(rel) {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace contains unsupported symlink %q", filepath.ToSlash(rel))
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("workspace contains unsupported file %q", filepath.ToSlash(rel))
		}
		destination := filepath.Join(target, rel)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			_, err = copyFile(output, input)
		}
		if closeErr := input.Close(); err == nil {
			err = closeErr
		}
		if closeErr := output.Close(); err == nil && output != nil {
			err = closeErr
		}
		return err
	})
	if err != nil {
		_ = cleanup()
		return "", nil, err
	}
	return target, cleanup, nil
}

func excludedDirectory(rel string) bool {
	first := strings.Split(filepath.ToSlash(rel), "/")[0]
	return first == ".git" || first == "node_modules" || first == ".devparity"
}

func copyFile(output, input *os.File) (int64, error) {
	return output.ReadFrom(input)
}

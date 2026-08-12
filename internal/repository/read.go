package repository

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func Read(root, path string, maxBytes int64) ([]byte, error) {
	if maxBytes < 0 {
		return nil, fmt.Errorf("maximum read size must not be negative")
	}
	if filepath.IsAbs(path) {
		return nil, fmt.Errorf("artifact path %q is absolute", path)
	}

	resolvedRoot, err := resolveRoot(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	clean := filepath.Clean(path)
	if clean == ".." || len(clean) > 2 && clean[:3] == ".."+string(filepath.Separator) {
		return nil, fmt.Errorf("artifact path %q escapes repository", path)
	}
	candidate := filepath.Join(resolvedRoot, clean)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact path %q: %w", path, err)
	}
	if !within(resolvedRoot, resolved) {
		return nil, fmt.Errorf("artifact path %q resolves outside repository", path)
	}

	file, err := os.Open(resolved)
	if err != nil {
		return nil, fmt.Errorf("open artifact %q: %w", path, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat artifact %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("artifact path %q is not a regular file", path)
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("artifact %q exceeds maximum size %d bytes", path, maxBytes)
	}

	limit := maxBytes
	if limit < int64(^uint64(0)>>1) {
		limit++
	}
	data, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return nil, fmt.Errorf("read artifact %q: %w", path, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("artifact %q exceeds maximum size %d bytes", path, maxBytes)
	}
	return data, nil
}

func resolveRoot(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

func within(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." {
		return false
	}
	return len(relative) < 3 || relative[:3] != ".."+string(filepath.Separator)
}

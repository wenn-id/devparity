package execute

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ponytail: use mode-based access until portable UID/GID or ACL mapping exists;
// keep container execution non-root.
const (
	workspaceDirectoryMode = 0o777
	workspaceFileMode      = 0o644

	workspaceMaxFileBytes  int64 = 16 << 20
	workspaceMaxTotalBytes int64 = 512 << 20
	workspaceMaxFiles      int64 = 100_000
)

type WorkspaceLimits struct {
	MaxFileBytes  int64
	MaxTotalBytes int64
	MaxFiles      int64
}

var defaultWorkspaceLimits = WorkspaceLimits{
	MaxFileBytes:  workspaceMaxFileBytes,
	MaxTotalBytes: workspaceMaxTotalBytes,
	MaxFiles:      workspaceMaxFiles,
}

func CopyWorkspace(root string) (string, func() error, error) {
	return CopyWorkspaceWithContext(context.Background(), root, defaultWorkspaceLimits)
}

func CopyWorkspaceWithContext(ctx context.Context, root string, limits WorkspaceLimits) (string, func() error, error) {
	if ctx == nil {
		return "", nil, fmt.Errorf("workspace copy requires a context")
	}
	limits, err := normalizeWorkspaceLimits(limits)
	if err != nil {
		return "", nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", nil, err
	}
	if err := checkWorkspaceContext(ctx); err != nil {
		return "", nil, err
	}
	parent, err := os.MkdirTemp("", "devparity-workspace-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() error { return os.RemoveAll(parent) }
	fail := func(copyErr error) (string, func() error, error) {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return "", nil, fmt.Errorf("%w; workspace cleanup failed: %v", copyErr, cleanupErr)
		}
		return "", nil, copyErr
	}
	target := filepath.Join(parent, "workspace")
	if err := os.Mkdir(target, workspaceDirectoryMode); err != nil {
		return fail(err)
	}
	if err := os.Chmod(target, workspaceDirectoryMode); err != nil {
		return fail(err)
	}

	var entryCount, totalBytes int64
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := checkWorkspaceContext(ctx); err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entryCount >= limits.MaxFiles {
			return fmt.Errorf("workspace exceeds maximum file count %d", limits.MaxFiles)
		}
		entryCount++
		if entry.IsDir() && excludedDirectory(rel) {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			destination := filepath.Join(target, rel)
			if err := os.MkdirAll(destination, workspaceDirectoryMode); err != nil {
				return err
			}
			return os.Chmod(destination, workspaceDirectoryMode)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace contains unsupported symlink %q", filepath.ToSlash(rel))
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("workspace contains unsupported file %q", filepath.ToSlash(rel))
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() < 0 || info.Size() > limits.MaxFileBytes {
			return fmt.Errorf("workspace file %q exceeds maximum file size %d bytes", filepath.ToSlash(rel), limits.MaxFileBytes)
		}
		if totalBytes > limits.MaxTotalBytes-info.Size() {
			return fmt.Errorf("workspace exceeds maximum size %d bytes", limits.MaxTotalBytes)
		}

		destination := filepath.Join(target, rel)
		if err := os.MkdirAll(filepath.Dir(destination), workspaceDirectoryMode); err != nil {
			return err
		}
		// #nosec G304 -- intentional: copy trusted repo files into the size-bounded workspace.
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, workspaceFileMode)
		if err != nil {
			_ = input.Close()
			return err
		}
		copied, copyErr := copyFile(ctx, output, input, limits.MaxFileBytes, limits.MaxTotalBytes-totalBytes, limits.MaxTotalBytes)
		if closeErr := input.Close(); copyErr == nil {
			copyErr = closeErr
		}
		if chmodErr := output.Chmod(workspaceFileMode); copyErr == nil {
			copyErr = chmodErr
		}
		if closeErr := output.Close(); copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			return copyErr
		}
		totalBytes += copied
		return nil
	})
	if err != nil {
		return fail(err)
	}
	if err := checkWorkspaceContext(ctx); err != nil {
		return fail(err)
	}
	return target, cleanup, nil
}

func normalizeWorkspaceLimits(limits WorkspaceLimits) (WorkspaceLimits, error) {
	if limits.MaxFileBytes == 0 {
		limits.MaxFileBytes = workspaceMaxFileBytes
	}
	if limits.MaxTotalBytes == 0 {
		limits.MaxTotalBytes = workspaceMaxTotalBytes
	}
	if limits.MaxFiles == 0 {
		limits.MaxFiles = workspaceMaxFiles
	}
	if limits.MaxFileBytes < 1 || limits.MaxTotalBytes < 1 || limits.MaxFiles < 1 {
		return WorkspaceLimits{}, fmt.Errorf("workspace limits must be positive")
	}
	if limits.MaxFileBytes > limits.MaxTotalBytes {
		return WorkspaceLimits{}, fmt.Errorf("workspace maximum file size exceeds total size")
	}
	return limits, nil
}

func checkWorkspaceContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("workspace copy canceled: %w", err)
	}
	return nil
}

func excludedDirectory(rel string) bool {
	first := strings.Split(filepath.ToSlash(rel), "/")[0]
	return first == ".git" || first == "node_modules" || first == ".devparity"
}

func copyFile(ctx context.Context, output, input *os.File, maxFileBytes, remainingTotalBytes, maxTotalBytes int64) (int64, error) {
	buffer := make([]byte, 32*1024)
	var copied int64
	for {
		if err := checkWorkspaceContext(ctx); err != nil {
			return copied, err
		}
		read, readErr := input.Read(buffer)
		if read > 0 {
			if copied > maxFileBytes-int64(read) {
				return copied, fmt.Errorf("workspace file exceeds maximum file size %d bytes", maxFileBytes)
			}
			if copied > remainingTotalBytes-int64(read) {
				return copied, fmt.Errorf("workspace exceeds maximum size %d bytes", maxTotalBytes)
			}
			written, writeErr := output.Write(buffer[:read])
			copied += int64(written)
			if writeErr != nil {
				return copied, writeErr
			}
			if written != read {
				return copied, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return copied, nil
		}
		if readErr != nil {
			return copied, readErr
		}
	}
}

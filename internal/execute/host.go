package execute

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/wenn-id/devparity/internal/model"
)

const (
	defaultTimeout         = 10 * time.Minute
	defaultMaxOutput int64 = 1 << 20
)

type Options struct {
	Root        string
	Timeout     time.Duration
	MaxOutput   int64
	EnvNames    []string
	Environment *EnvironmentSnapshot

	AllowNetwork bool
	NodeVersion  string
}

// EnvironmentSnapshot contains the exact values selected for forwarding and
// redaction. Its fields are private so callers cannot change the snapshot
// after it has been captured.
type EnvironmentSnapshot struct {
	forwarded []string
	secrets   []string
}

func RunHost(ctx context.Context, grant Grant, block model.DocBlock, opts Options) (model.ExecutionResult, error) {
	if !grant.host {
		return model.ExecutionResult{}, errors.New("host execution requires a host grant")
	}
	environment := opts.Environment
	if environment == nil {
		captured, snapshotErr := SnapshotEnvironment(opts.EnvNames)
		if snapshotErr != nil {
			return model.ExecutionResult{}, snapshotErr
		}
		environment = &captured
	}
	executable, args, err := shellCommand(block)
	if err != nil {
		return model.ExecutionResult{}, err
	}
	if _, err := exec.LookPath(executable); err != nil {
		return model.ExecutionResult{BlockID: block.ID, Mode: "host", Status: model.StatusSkipped, Stderr: err.Error()}, nil
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	if opts.MaxOutput <= 0 {
		opts.MaxOutput = defaultMaxOutput
	}
	commandContext, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	// #nosec G204 -- intentional: execute explicitly trusted documentation commands.
	command := exec.CommandContext(commandContext, executable, args...)
	processTree, err := configureHostProcessTree(command)
	if err != nil {
		return model.ExecutionResult{}, fmt.Errorf("host process tree setup failed: %w", err)
	}
	defer processTree.Close()
	command.Cancel = func() error { return processTree.Cancel(command.Process) }
	command.WaitDelay = hostWaitDelay
	if opts.Root != "" {
		command.Dir = opts.Root
	}
	command.Env = minimalEnvironment(environment.forwarded)
	stdout := &cappedWriter{max: opts.MaxOutput}
	stderr := &cappedWriter{max: opts.MaxOutput}
	command.Stdout = stdout
	command.Stderr = stderr
	start := time.Now()
	var runErr error
	if err := command.Start(); err != nil {
		runErr = err
	} else if err := processTree.Attach(command.Process); err != nil {
		_ = processTree.Cancel(command.Process)
		_ = command.Process.Kill()
		_ = command.Wait()
		return model.ExecutionResult{}, fmt.Errorf("host process tree attach failed: %w", err)
	} else {
		runErr = command.Wait()
	}
	result := model.ExecutionResult{
		BlockID:  block.ID,
		Mode:     "host",
		Duration: time.Since(start).Milliseconds(),
		Stdout:   environment.redactor().Redact(stdout.String()),
		Stderr:   environment.redactor().Redact(stderr.String()),
		Status:   model.StatusPass,
	}
	if runErr == nil {
		result.ExitCode = 0
		return result, nil
	}
	result.Status = model.StatusFinding
	result.ExitCode = exitCode(runErr)
	if commandContext.Err() != nil {
		result.Stderr = strings.TrimSpace(result.Stderr + "\n" + commandContext.Err().Error())
	}
	return result, nil
}

func shellCommand(block model.DocBlock) (string, []string, error) {
	switch strings.ToLower(block.Shell) {
	case "sh", "shell", "bash":
		return "sh", []string{"-eu", "-c", block.Script}, nil
	case "powershell", "pwsh":
		if runtime.GOOS == "windows" {
			return "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", block.Script}, nil
		}
		return "pwsh", []string{"-NoProfile", "-NonInteractive", "-Command", block.Script}, nil
	default:
		return "", nil, fmt.Errorf("unsupported documentation shell %q", block.Shell)
	}
}

func SnapshotEnvironment(names []string) (EnvironmentSnapshot, error) {
	snapshot := EnvironmentSnapshot{
		forwarded: make([]string, 0, len(names)),
		secrets:   make([]string, 0, len(names)),
	}
	for _, name := range names {
		value, ok := os.LookupEnv(name)
		if !ok {
			return EnvironmentSnapshot{}, fmt.Errorf("requested environment variable %q is not set", name)
		}
		// Empty values are explicit and are forwarded as NAME=; they are not
		// useful redaction patterns, so only non-empty values are retained there.
		snapshot.forwarded = append(snapshot.forwarded, name+"="+value)
		if value != "" {
			snapshot.secrets = append(snapshot.secrets, value)
		}
	}
	return snapshot, nil
}

func (snapshot EnvironmentSnapshot) redactor() Redactor {
	return NewRedactor(snapshot.secrets)
}

func minimalEnvironment(forwarded []string) []string {
	names := []string{"PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "TMP", "TEMP", "TMPDIR"}
	environment := make([]string, 0, len(names)+len(forwarded))
	for _, name := range names {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	return append(environment, forwarded...)
}

func exitCode(err error) int {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return -1
}

type cappedWriter struct {
	data []byte
	max  int64
}

func (w *cappedWriter) Write(data []byte) (int, error) {
	total := len(data)
	remaining := w.max - int64(len(w.data))
	if remaining > 0 {
		if int64(len(data)) > remaining {
			data = data[:remaining]
		}
		w.data = append(w.data, data...)
	}
	return total, nil
}

func (w *cappedWriter) String() string { return string(w.data) }

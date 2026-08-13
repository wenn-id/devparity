package execute

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/wenn-id/devparity/internal/model"
)

type CommandFunc func(context.Context, string, []string) ([]byte, []byte, int, error)

var (
	lookPath                = exec.LookPath
	commandFunc CommandFunc = runCommand
)

func NewContainerGrant() Grant { return Grant{container: true} }

func RunContainer(ctx context.Context, grant Grant, block model.DocBlock, opts Options) (result model.ExecutionResult, err error) {
	if !grant.container {
		return model.ExecutionResult{}, errors.New("container execution requires a container grant")
	}
	if opts.Root == "" {
		return model.ExecutionResult{}, errors.New("container execution requires a repository root")
	}
	version := opts.NodeVersion
	if version == "" {
		return model.ExecutionResult{}, errors.New("container execution requires a concrete Node version")
	}
	workspace, cleanup, err := CopyWorkspace(opts.Root)
	if err != nil {
		return model.ExecutionResult{}, err
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			result = model.ExecutionResult{}
			err = fmt.Errorf("container workspace cleanup failed: %w", cleanupErr)
		}
	}()
	runtimeName := ""
	for _, candidate := range []string{"docker", "podman"} {
		if _, lookErr := lookPath(candidate); lookErr == nil {
			runtimeName = candidate
			break
		}
	}
	if runtimeName == "" {
		return model.ExecutionResult{BlockID: block.ID, Mode: "container", Status: model.StatusSkipped, Stderr: "docker or podman is not installed"}, nil
	}
	args := []string{"run", "--rm", "--user", "10001:10001", "--cap-drop", "ALL", "--security-opt", "no-new-privileges"}
	if !opts.AllowNetwork {
		args = append(args, "--network", "none")
	}
	args = append(args, "--cpus", "2", "--memory", "2g")
	if runtime.GOOS != "windows" {
		args = append(args, "--pids-limit", "256")
	}
	args = append(args, "-v", workspace+":/workspace", "-w", "/workspace", "node:"+version)
	shell, shellArgs, err := containerShell(block)
	if err != nil {
		return model.ExecutionResult{BlockID: block.ID, Mode: "container", Status: model.StatusSkipped, Stderr: err.Error()}, nil
	}
	args = append(args, shell)
	args = append(args, shellArgs...)
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	commandContext, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	start := time.Now()
	stdout, stderr, exit, runErr := commandFunc(commandContext, runtimeName, args)
	if runErr != nil {
		return model.ExecutionResult{}, fmt.Errorf("container runtime failed: %w", runErr)
	}
	if runtimeFailure(stderr) {
		return model.ExecutionResult{}, fmt.Errorf("container runtime failed: %s", strings.TrimSpace(string(stderr)))
	}
	result = model.ExecutionResult{BlockID: block.ID, Mode: "container", ExitCode: exit, Duration: time.Since(start).Milliseconds(), Stdout: NewRedactor(nil).Redact(string(stdout)), Stderr: NewRedactor(nil).Redact(string(stderr)), Status: model.StatusPass}
	if exit != 0 || commandContext.Err() != nil {
		result.Status = model.StatusFinding
		if commandContext.Err() != nil {
			result.Stderr = strings.TrimSpace(result.Stderr + "\n" + commandContext.Err().Error())
		}
	}
	return result, nil
}

func runtimeFailure(stderr []byte) bool {
	message := strings.ToLower(string(stderr))
	for _, marker := range []string{"permission denied", "cannot connect", "is the docker daemon running", "error during connect"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func containerShell(block model.DocBlock) (string, []string, error) {
	switch strings.ToLower(block.Shell) {
	case "sh", "shell", "bash":
		return "sh", []string{"-eu", "-c", block.Script}, nil
	case "powershell", "pwsh":
		return "", nil, fmt.Errorf("PowerShell container fences are unsupported")
	default:
		return "", nil, fmt.Errorf("unsupported documentation shell %q", block.Shell)
	}
}

func runCommand(ctx context.Context, name string, args []string) ([]byte, []byte, int, error) {
	command := exec.CommandContext(ctx, name, args...)
	stdout, err := command.Output()
	if err == nil {
		return stdout, nil, 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return stdout, exitError.Stderr, exitError.ExitCode(), nil
	}
	return stdout, nil, -1, err
}

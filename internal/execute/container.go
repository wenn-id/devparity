package execute

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/wenn-id/devparity/internal/model"
)

type CommandFunc func(context.Context, string, []string, int64) ([]byte, []byte, int, error)

var (
	lookPath                                 = exec.LookPath
	commandFunc                  CommandFunc = runCommand
	containerRuntimeProbeTimeout             = 10 * time.Second
)

func probeContainerRuntime(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	failures := make([]string, 0, 2)
	for _, candidate := range []string{"docker", "podman"} {
		if _, err := lookPath(candidate); err != nil {
			failures = append(failures, candidate+": not installed")
			continue
		}
		probeContext, cancel := context.WithTimeout(ctx, containerRuntimeProbeTimeout)
		_, stderr, exit, err := commandFunc(probeContext, candidate, []string{"info"}, defaultMaxOutput)
		probeErr := probeContext.Err()
		cancel()
		if err == nil && exit == 0 {
			return candidate, nil
		}
		message := strings.TrimSpace(string(stderr))
		if err != nil {
			message = err.Error()
		}
		if probeErr != nil {
			message = probeErr.Error()
		}
		if message == "" {
			message = fmt.Sprintf("exit code %d", exit)
		}
		failures = append(failures, candidate+": "+message)
	}
	return "", fmt.Errorf("no usable Docker or Podman runtime: %s", strings.Join(failures, "; "))
}

func NewContainerGrant() Grant { return Grant{container: true} }

func RunContainer(ctx context.Context, grant Grant, block model.DocBlock, opts Options) (result model.ExecutionResult, err error) {
	if !grant.container {
		return model.ExecutionResult{}, errors.New("container execution requires a container grant")
	}
	runtimeName, probeErr := probeContainerRuntime(ctx)
	if probeErr != nil {
		return model.ExecutionResult{BlockID: block.ID, Mode: "container", Status: model.StatusSkipped, Stderr: probeErr.Error()}, nil
	}
	return runContainerWithRuntime(ctx, grant, runtimeName, block, opts)
}

func runContainerWithRuntime(ctx context.Context, grant Grant, runtimeName string, block model.DocBlock, opts Options) (result model.ExecutionResult, err error) {
	if !grant.container {
		return model.ExecutionResult{}, errors.New("container execution requires a container grant")
	}
	environment := opts.Environment
	if environment == nil {
		captured, snapshotErr := SnapshotEnvironment(opts.EnvNames)
		if snapshotErr != nil {
			return model.ExecutionResult{}, snapshotErr
		}
		environment = &captured
	}
	redactor := environment.redactor()
	if opts.Root == "" {
		return model.ExecutionResult{}, errors.New("container execution requires a repository root")
	}
	version := opts.NodeVersion
	if version == "" {
		return model.ExecutionResult{}, errors.New("container execution requires a concrete Node version")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	if opts.MaxOutput <= 0 {
		opts.MaxOutput = defaultMaxOutput
	}
	commandContext, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	if runtimeName == "" {
		return model.ExecutionResult{}, errors.New("container execution requires a selected runtime")
	}
	if err := checkWorkspaceContext(commandContext); err != nil {
		return model.ExecutionResult{}, err
	}
	workspace, workspaceCleanup, err := CopyWorkspaceWithContext(commandContext, opts.Root, defaultWorkspaceLimits)
	if err != nil {
		return model.ExecutionResult{}, fmt.Errorf("container workspace setup failed: %s", redactor.Redact(err.Error()))
	}
	defer func() {
		if cleanupErr := workspaceCleanup(); cleanupErr != nil {
			result = model.ExecutionResult{}
			err = fmt.Errorf("container workspace cleanup failed: %s", redactor.Redact(cleanupErr.Error()))
		}
	}()
	shell, shellArgs, err := containerShell(block)
	if err != nil {
		return model.ExecutionResult{BlockID: block.ID, Mode: "container", Status: model.StatusSkipped, Stderr: redactor.Redact(err.Error())}, nil
	}
	containerName, err := uniqueContainerName()
	if err != nil {
		return model.ExecutionResult{}, fmt.Errorf("container name generation failed: %s", redactor.Redact(err.Error()))
	}
	cleanupArmed := false
	defer func() {
		if !cleanupArmed {
			return
		}
		if cleanupErr := forceRemoveContainer(runtimeName, containerName, opts.MaxOutput, redactor); cleanupErr != nil {
			cleanupFailure := fmt.Errorf("container cleanup failed: %s", redactor.Redact(cleanupErr.Error()))
			priorResult := result
			result = model.ExecutionResult{}
			if err == nil {
				if priorResult.Status == model.StatusFinding {
					err = fmt.Errorf("container runtime failed: exit code %d; %s", priorResult.ExitCode, cleanupFailure.Error())
				} else {
					err = cleanupFailure
				}
			} else {
				err = fmt.Errorf("%s; %s", redactor.Redact(err.Error()), cleanupFailure.Error())
			}
		}
	}()
	args := []string{"run", "--rm", "--name", containerName, "--user", "10001:10001", "--cap-drop", "ALL", "--security-opt", "no-new-privileges"}
	if !opts.AllowNetwork {
		args = append(args, "--network", "none")
	}
	args = append(args, "--cpus", "2", "--memory", "2g")
	if runtime.GOOS != "windows" {
		args = append(args, "--pids-limit", "256")
	}
	for _, variable := range environment.forwarded {
		args = append(args, "-e", variable)
	}
	args = append(args, "-v", workspace+":/workspace", "-w", "/workspace", "node:"+version)
	args = append(args, shell)
	args = append(args, shellArgs...)
	start := time.Now()
	cleanupArmed = true
	stdout, stderr, exit, runErr := commandFunc(commandContext, runtimeName, args, opts.MaxOutput)
	if runErr != nil {
		return model.ExecutionResult{}, fmt.Errorf("container runtime failed: %s", redactor.Redact(runErr.Error()))
	}
	// The container process exit code is authoritative. Repository commands control
	// stderr, so its text must never reclassify a successful runtime invocation.
	result = model.ExecutionResult{BlockID: block.ID, Mode: "container", ExitCode: exit, Duration: time.Since(start).Milliseconds(), Stdout: redactor.Redact(string(stdout)), Stderr: redactor.Redact(string(stderr)), Status: model.StatusPass}
	if exit != 0 || commandContext.Err() != nil {
		result.Status = model.StatusFinding
		if commandContext.Err() != nil {
			result.Stderr = strings.TrimSpace(result.Stderr + "\n" + redactor.Redact(commandContext.Err().Error()))
		}
	}
	return result, nil
}

func uniqueContainerName() (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return "devparity-" + hex.EncodeToString(suffix[:]), nil
}

func forceRemoveContainer(runtimeName, containerName string, maxOutput int64, redactor Redactor) error {
	cleanupContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, stderr, exit, err := commandFunc(cleanupContext, runtimeName, []string{"rm", "-f", containerName}, maxOutput)
	if err != nil {
		return errors.New(redactor.Redact(err.Error()))
	}
	if exit == 0 || containerNotFound(stderr) {
		return nil
	}
	message := strings.TrimSpace(string(stderr))
	if message == "" {
		message = fmt.Sprintf("exit code %d", exit)
	}
	return errors.New(redactor.Redact(message))
}

func containerNotFound(stderr []byte) bool {
	message := strings.ToLower(string(stderr))
	for _, marker := range []string{"no such container", "no container with name", "no container with id", "does not exist"} {
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

func runCommand(ctx context.Context, name string, args []string, maxOutput int64) ([]byte, []byte, int, error) {
	if maxOutput <= 0 {
		maxOutput = defaultMaxOutput
	}
	command := exec.CommandContext(ctx, name, args...)
	stdout := &cappedWriter{max: maxOutput}
	stderr := &cappedWriter{max: maxOutput}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if err == nil {
		return stdout.data, stderr.data, 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return stdout.data, stderr.data, exitError.ExitCode(), nil
	}
	return stdout.data, stderr.data, -1, err
}

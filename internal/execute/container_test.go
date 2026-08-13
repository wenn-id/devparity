package execute

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wenn-id/devparity/internal/model"
)

func TestRunContainerBuildsRestrictedArguments(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(name string) (string, error) {
		if name == "docker" {
			return "/fake/docker", nil
		}
		return "", errors.New("not found")
	}
	var runtimeName string
	var args []string
	var calls int
	commandFunc = func(_ context.Context, name string, got []string, _ int64) ([]byte, []byte, int, error) {
		calls++
		if len(got) > 0 && got[0] == "run" {
			runtimeName = name
			args = append([]string(nil), got...)
		}
		return []byte("ok"), nil, 0, nil
	}
	root := t.TempDir()
	writeWorkspaceFile(t, root, "package.json", "{}")
	grant := NewContainerGrant()
	result, err := RunContainer(context.Background(), grant, model.DocBlock{ID: "README.md:2", Shell: "sh", Script: "echo ok"}, Options{Root: root, NodeVersion: "22"})
	if err != nil {
		t.Fatal(err)
	}
	if runtimeName != "docker" || result.Status != model.StatusPass || result.ExitCode != 0 {
		t.Fatalf("runtime=%q result=%#v", runtimeName, result)
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want run plus cleanup", calls)
	}
	for _, want := range []string{"run", "--rm", "--name", "--user", "10001:10001", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--network", "none", "--cpus", "2", "--memory", "2g", "-w", "/workspace", "node:22", "sh", "-eu", "-c", "echo ok"} {
		if !containsArg(args, want) {
			t.Fatalf("args=%#v, missing %q", args, want)
		}
	}
	name := argValue(args, "--name")
	if name == "" || !strings.HasPrefix(name, "devparity-") {
		t.Fatalf("invalid retained container name %q in args=%#v", name, args)
	}
	if runtime.GOOS == "windows" && containsArg(args, "--pids-limit") {
		t.Fatalf("Windows Docker does not support --pids-limit: args=%#v", args)
	}
	if runtime.GOOS != "windows" && (!containsArg(args, "--pids-limit") || !containsArg(args, "256")) {
		t.Fatalf("POSIX runtime lost process limit: args=%#v", args)
	}
	for _, arg := range args {
		if arg == root {
			t.Fatalf("original root mounted in args=%#v", args)
		}
	}
}

func TestRunCommandCapsBothStreams(t *testing.T) {
	t.Setenv("DEVPARITY_COMMAND_HELPER", "large")
	stdout, stderr, exit, err := runCommand(context.Background(), os.Args[0], []string{"-test.run=TestCommandHelperProcess"}, defaultMaxOutput)
	if err != nil {
		t.Fatal(err)
	}
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	if int64(len(stdout)) > defaultMaxOutput || int64(len(stderr)) > defaultMaxOutput {
		t.Fatalf("stdout=%d stderr=%d limit=%d", len(stdout), len(stderr), defaultMaxOutput)
	}
}

func TestRunCommandUsesConfiguredLimit(t *testing.T) {
	t.Setenv("DEVPARITY_COMMAND_HELPER", "large")
	stdout, stderr, exit, err := runCommand(context.Background(), os.Args[0], []string{"-test.run=TestCommandHelperProcess"}, 64)
	if err != nil {
		t.Fatal(err)
	}
	if exit != 0 || len(stdout) > 64 || len(stderr) > 64 {
		t.Fatalf("exit=%d stdout=%d stderr=%d", exit, len(stdout), len(stderr))
	}
}

func TestRunCommandPreservesSuccessfulStderr(t *testing.T) {
	t.Setenv("DEVPARITY_COMMAND_HELPER", "stderr")
	stdout, stderr, exit, err := runCommand(context.Background(), os.Args[0], []string{"-test.run=TestCommandHelperProcess"}, defaultMaxOutput)
	if err != nil {
		t.Fatal(err)
	}
	if exit != 0 || !strings.Contains(string(stdout), "ok") || !strings.Contains(string(stderr), "warning") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)
	}
}

func TestRunCommandPreservesBothStreamsOnFailure(t *testing.T) {
	t.Setenv("DEVPARITY_COMMAND_HELPER", "failure")
	stdout, stderr, exit, err := runCommand(context.Background(), os.Args[0], []string{"-test.run=TestCommandHelperProcess"}, defaultMaxOutput)
	if err != nil {
		t.Fatal(err)
	}
	if exit != 7 || !strings.Contains(string(stdout), "stdout") || !strings.Contains(string(stderr), "stderr") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)
	}
}

func TestRunContainerDoesNotClassifyCommandStderrAsRuntimeFailure(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "/fake/docker", nil }
	commandFunc = func(_ context.Context, _ string, args []string, _ int64) ([]byte, []byte, int, error) {
		if args[0] == "rm" {
			return nil, nil, 0, nil
		}
		return []byte("ok"), []byte("permission denied ghp_abcd1234"), 0, nil
	}
	result, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{ID: "README.md:2", Shell: "sh", Script: "true"}, Options{Root: t.TempDir(), NodeVersion: "22"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusPass || !strings.Contains(result.Stderr, "[REDACTED]") {
		t.Fatalf("result=%#v", result)
	}
}

func TestRunContainerForceRemovesTimedOutContainerBeforeWorkspaceCleanup(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "/fake/docker", nil }
	root := t.TempDir()
	writeWorkspaceFile(t, root, "sentinel", "unchanged")
	var workspace string
	var containerName string
	var calls [][]string
	commandFunc = func(ctx context.Context, _ string, args []string, _ int64) ([]byte, []byte, int, error) {
		calls = append(calls, append([]string(nil), args...))
		switch args[0] {
		case "run":
			workspace = strings.TrimSuffix(argValue(args, "-v"), ":/workspace")
			containerName = argValue(args, "--name")
			<-ctx.Done()
			return nil, nil, -1, nil
		case "rm":
			if ctx.Err() != nil {
				t.Errorf("cleanup context canceled: %v", ctx.Err())
			}
			if args[1] != "-f" || args[2] != containerName {
				t.Errorf("cleanup args=%#v, want rm -f %q", args, containerName)
			}
			if _, err := os.Stat(workspace); err != nil {
				t.Errorf("workspace removed before container cleanup: %v", err)
			}
			return nil, nil, 0, nil
		default:
			t.Fatalf("unexpected command args=%#v", args)
			return nil, nil, -1, errors.New("unexpected command")
		}
	}
	result, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{Shell: "sh", Script: "sleep 30"}, Options{Root: root, NodeVersion: "22", Timeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusFinding {
		t.Fatalf("result=%#v", result)
	}
	if len(calls) != 2 || calls[1][0] != "rm" {
		t.Fatalf("calls=%#v", calls)
	}
	if workspace == "" || containerName == "" {
		t.Fatalf("workspace=%q container=%q calls=%#v", workspace, containerName, calls)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("workspace still exists after cleanup: %v", err)
	}
}

func TestRunContainerAttemptsCleanupAfterRuntimeError(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "/fake/docker", nil }
	var cleanupCalled bool
	commandFunc = func(_ context.Context, _ string, args []string, _ int64) ([]byte, []byte, int, error) {
		if args[0] == "rm" {
			cleanupCalled = true
			return nil, nil, 0, nil
		}
		return nil, nil, -1, errors.New("daemon unavailable")
	}
	_, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{Shell: "sh", Script: "true"}, Options{Root: t.TempDir(), NodeVersion: "22"})
	if err == nil || !strings.Contains(err.Error(), "container runtime failed") {
		t.Fatalf("err=%v", err)
	}
	if !cleanupCalled {
		t.Fatal("container cleanup was not attempted after runtime error")
	}
}

func TestCommandHelperProcess(t *testing.T) {
	switch os.Getenv("DEVPARITY_COMMAND_HELPER") {
	case "large":
		_, _ = os.Stdout.Write([]byte(strings.Repeat("o", int(defaultMaxOutput*2))))
		_, _ = os.Stderr.Write([]byte(strings.Repeat("e", int(defaultMaxOutput*2))))
		os.Exit(0)
	case "stderr":
		_, _ = os.Stdout.Write([]byte("ok"))
		_, _ = os.Stderr.Write([]byte("warning"))
		os.Exit(0)
	case "failure":
		_, _ = os.Stdout.Write([]byte("stdout"))
		_, _ = os.Stderr.Write([]byte("stderr"))
		os.Exit(7)
	}
}

func TestRunContainerAllowNetworkRemovesOnlyNetworkRestriction(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "/fake/docker", nil }
	var args []string
	commandFunc = func(_ context.Context, _ string, got []string, _ int64) ([]byte, []byte, int, error) {
		if len(got) > 0 && got[0] == "run" {
			args = append([]string(nil), got...)
		}
		return nil, nil, 0, nil
	}
	if _, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{Shell: "sh", Script: "true"}, Options{Root: t.TempDir(), NodeVersion: "22", AllowNetwork: true}); err != nil {
		t.Fatal(err)
	}
	if containsArg(args, "none") {
		t.Fatalf("network restriction remained: %#v", args)
	}
	if !containsArg(args, "--cap-drop") || !containsArg(args, "ALL") {
		t.Fatalf("security flags changed: %#v", args)
	}
}

func TestLiveContainer(t *testing.T) {
	if os.Getenv("DEVPARITY_CONTAINER_TEST") != "1" {
		t.Skip("set DEVPARITY_CONTAINER_TEST=1 to run the Docker/Podman integration test")
	}
	runtimeName := "docker"
	if _, err := exec.LookPath(runtimeName); err != nil {
		runtimeName = "podman"
	}
	if _, err := exec.LookPath(runtimeName); err != nil {
		t.Skip("no usable Docker or Podman runtime")
	}
	infoContext, infoCancel := context.WithTimeout(context.Background(), 10*time.Second)
	infoErr := exec.CommandContext(infoContext, runtimeName, "info").Run()
	infoCancel()
	if infoErr != nil {
		t.Skip("no usable Docker or Podman runtime")
	}
	root := t.TempDir()
	writeWorkspaceFile(t, root, "sentinel", "unchanged")
	result, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{ID: "live", Shell: "sh", Script: "head -c 2097152 /dev/zero; printf warning >&2"}, Options{Root: root, NodeVersion: "22", Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusPass || int64(len(result.Stdout)) > defaultMaxOutput || !strings.Contains(result.Stderr, "warning") {
		t.Fatalf("result=%#v", result)
	}
	data, err := os.ReadFile(filepath.Join(root, "sentinel"))
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("source changed: data=%q err=%v", data, err)
	}
}

func TestLiveContainerTimeoutRemovesContainer(t *testing.T) {
	if os.Getenv("DEVPARITY_CONTAINER_TEST") != "1" {
		t.Skip("set DEVPARITY_CONTAINER_TEST=1 to run the Docker/Podman integration test")
	}
	runtimeName := "docker"
	if _, err := exec.LookPath(runtimeName); err != nil {
		runtimeName = "podman"
	}
	if _, err := exec.LookPath(runtimeName); err != nil {
		t.Skip("no usable Docker or Podman runtime")
	}
	infoContext, infoCancel := context.WithTimeout(context.Background(), 10*time.Second)
	infoErr := exec.CommandContext(infoContext, runtimeName, "info").Run()
	infoCancel()
	if infoErr != nil {
		t.Skip("no usable Docker or Podman runtime")
	}
	oldCommand := commandFunc
	t.Cleanup(func() { commandFunc = oldCommand })
	var containerName string
	commandFunc = func(ctx context.Context, name string, args []string, maxOutput int64) ([]byte, []byte, int, error) {
		if len(args) > 0 && args[0] == "run" {
			containerName = argValue(args, "--name")
		}
		return runCommand(ctx, name, args, maxOutput)
	}
	result, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{ID: "timeout", Shell: "sh", Script: "sleep 30"}, Options{Root: t.TempDir(), NodeVersion: "22", Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusFinding {
		t.Fatalf("result=%#v", result)
	}
	if containerName == "" {
		t.Fatal("container name was not retained")
	}
	listContext, listCancel := context.WithTimeout(context.Background(), 10*time.Second)
	output, err := exec.CommandContext(listContext, runtimeName, "ps", "-a", "--filter", "name="+containerName, "--format", "{{.Names}}").CombinedOutput()
	listCancel()
	if err != nil {
		t.Fatalf("container listing failed: %v; output=%q", err, output)
	}
	if strings.TrimSpace(string(output)) != "" {
		t.Fatalf("timed-out container remains: %q", output)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if strings.EqualFold(arg, want) {
			return true
		}
	}
	return false
}

func argValue(args []string, flag string) string {
	for index, arg := range args {
		if arg == flag {
			if index+1 >= len(args) {
				return ""
			}
			return args[index+1]
		}
	}
	return ""
}

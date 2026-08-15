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

func TestProbeContainerRuntimeFailsClosed(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(name string) (string, error) {
		if name == "docker" || name == "podman" {
			return "/fake/" + name, nil
		}
		return "", errors.New("not found")
	}
	commandFunc = func(_ context.Context, _ string, args []string, _ int64) ([]byte, []byte, int, error) {
		if len(args) != 1 || args[0] != "info" {
			t.Fatalf("probe args=%#v", args)
		}
		return nil, []byte("daemon unavailable"), 1, nil
	}
	if _, err := probeContainerRuntime(context.Background()); err == nil || !strings.Contains(err.Error(), "no usable Docker or Podman runtime") {
		t.Fatalf("err=%v", err)
	}
}

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
		return []byte("permission denied ghp_abcd1234"), []byte("cannot connect ghp_abcd1234"), 0, nil
	}
	result, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{ID: "README.md:2", Shell: "sh", Script: "true"}, Options{Root: t.TempDir(), NodeVersion: "22"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusPass || !strings.Contains(result.Stdout, "permission denied") || !strings.Contains(result.Stderr, "cannot connect") || !strings.Contains(result.Stdout+result.Stderr, "[REDACTED]") || strings.Contains(result.Stdout+result.Stderr, "ghp_abcd1234") {
		t.Fatalf("result=%#v", result)
	}
}

func TestRunContainerDoesNotClassifySpoofedRuntimeStderr(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "/fake/docker", nil }
	commandFunc = func(_ context.Context, _ string, args []string, _ int64) ([]byte, []byte, int, error) {
		if args[0] == "rm" {
			return nil, nil, 0, nil
		}
		return []byte("permission denied ghp_abcdefghijklmnopqrstuvwxyz123456"), []byte("cannot connect ghp_abcdefghijklmnopqrstuvwxyz123456"), 1, nil
	}

	result, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{Shell: "sh", Script: "false"}, Options{Root: t.TempDir(), NodeVersion: "22"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if result.Status != model.StatusFinding || result.ExitCode != 1 {
		t.Fatalf("result=%#v", result)
	}
	if !strings.Contains(result.Stdout, "permission denied") || !strings.Contains(result.Stderr, "cannot connect") || !strings.Contains(result.Stdout+result.Stderr, "[REDACTED]") {
		t.Fatalf("result lost context or redaction=%#v", result)
	}
	if strings.Contains(result.Stdout+result.Stderr, "ghp_abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("token leaked in result=%#v", result)
	}
}

func TestRunContainerRedactsCommandError(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "/fake/docker", nil }
	commandFunc = func(_ context.Context, _ string, args []string, _ int64) ([]byte, []byte, int, error) {
		if args[0] == "rm" {
			return nil, nil, 0, nil
		}
		return nil, nil, -1, errors.New("cannot start runtime ghp_abcdefghijklmnopqrstuvwxyz123456")
	}

	_, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{Shell: "sh", Script: "true"}, Options{Root: t.TempDir(), NodeVersion: "22"})
	if err == nil {
		t.Fatal("expected runtime error")
	}
	if !strings.Contains(err.Error(), "cannot start runtime") || !strings.Contains(err.Error(), "[REDACTED]") || strings.Contains(err.Error(), "ghp_abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("error lost context or redaction=%q", err)
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

func TestRunContainerChecksRuntimeBeforeWorkspaceCopy(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "", errors.New("runtime missing") }
	commandFunc = func(_ context.Context, _ string, _ []string, _ int64) ([]byte, []byte, int, error) {
		t.Fatal("runtime command must not run when runtime is unavailable")
		return nil, nil, -1, nil
	}
	root := t.TempDir()
	writeWorkspaceFile(t, root, "large", strings.Repeat("x", int(workspaceMaxFileBytes)+1))
	result, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{Shell: "sh", Script: "true"}, Options{Root: root, NodeVersion: "22"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusSkipped || !strings.Contains(result.Stderr, "not installed") {
		t.Fatalf("result=%#v", result)
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
	if runtime.GOOS != "linux" || os.Getenv("DEVPARITY_CONTAINER_TEST") != "1" {
		t.Skip("set DEVPARITY_CONTAINER_TEST=1 on Linux to run the Docker/Podman integration test")
	}
	runtimeName, runtimeErr := probeContainerRuntime(context.Background())
	if runtimeErr != nil {
		t.Fatal(runtimeErr)
	}
	root := t.TempDir()
	writeWorkspaceFile(t, root, "package.json", `{"name":"live"}`)
	writeWorkspaceFile(t, root, "sentinel", "unchanged")
	script := `
set -eu
test "$(cat package.json)" = '{"name":"live"}'
printf created > container-artifact
test "$(cat container-artifact)" = created
printf 'live-container-ran\n'
if node -e 'const os=require("os"); const nets=os.networkInterfaces(); const external=Object.values(nets).some(l=>l.some(a=>!a.internal)); process.exit(external ? 0 : 1)'; then
	printf 'network-enabled\n' >&2
	exit 41
fi
node -e 'process.stdout.write("o".repeat(2 * 1024 * 1024))'
node -e 'process.stderr.write("e".repeat(2 * 1024 * 1024))'
`
	result, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{ID: "live", Shell: "sh", Script: script}, Options{Root: root, NodeVersion: "22", Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Stdout, "live-container-ran") {
		t.Fatalf("live container did not emit positive execution marker: result=%#v", result)
	}
	t.Logf("live-container-ran runtime=%s stdout_bytes=%d stderr_bytes=%d", runtimeName, len(result.Stdout), len(result.Stderr))
	if result.Status != model.StatusPass || int64(len(result.Stdout)) > defaultMaxOutput || int64(len(result.Stderr)) > defaultMaxOutput {
		t.Fatalf("result=%#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "container-artifact")); !os.IsNotExist(err) {
		t.Fatalf("container artifact leaked into original repository: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "sentinel"))
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("source changed: data=%q err=%v", data, err)
	}
}

func TestLiveContainerTimeoutRemovesContainer(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("DEVPARITY_CONTAINER_TEST") != "1" {
		t.Skip("set DEVPARITY_CONTAINER_TEST=1 on Linux to run the Docker/Podman integration test")
	}
	runtimeName, runtimeErr := probeContainerRuntime(context.Background())
	if runtimeErr != nil {
		t.Fatal(runtimeErr)
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

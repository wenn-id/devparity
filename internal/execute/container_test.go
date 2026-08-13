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
	commandFunc = func(_ context.Context, name string, got []string, _ int64) ([]byte, []byte, int, error) {
		runtimeName = name
		args = append([]string(nil), got...)
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
	for _, want := range []string{"run", "--rm", "--user", "10001:10001", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--network", "none", "--cpus", "2", "--memory", "2g", "-w", "/workspace", "node:22", "sh", "-eu", "-c", "echo ok"} {
		if !containsArg(args, want) {
			t.Fatalf("args=%#v, missing %q", args, want)
		}
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
	commandFunc = func(_ context.Context, _ string, _ []string, _ int64) ([]byte, []byte, int, error) {
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
		args = append([]string(nil), got...)
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
	if _, err := exec.LookPath(runtimeName); err != nil || exec.Command(runtimeName, "info").Run() != nil {
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

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if strings.EqualFold(arg, want) {
			return true
		}
	}
	return false
}

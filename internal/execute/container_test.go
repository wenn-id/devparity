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
	commandFunc = func(_ context.Context, name string, got []string) ([]byte, []byte, int, error) {
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

func TestRuntimeFailureRecognizesUnavailableContainerRuntime(t *testing.T) {
	for _, stderr := range []string{
		"failed to connect to the docker API",
		"no matching manifest for windows/amd64",
		"Unable to find image 'node:22' locally",
	} {
		if !runtimeFailure([]byte(stderr)) {
			t.Fatalf("runtimeFailure(%q)=false", stderr)
		}
	}
	if runtimeFailure([]byte("npm test failed")) {
		t.Fatal("ordinary command failure classified as runtime failure")
	}
}

func TestRunContainerDoesNotClassifySpoofedRuntimeStderr(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "/fake/docker", nil }
	commandFunc = func(_ context.Context, _ string, _ []string) ([]byte, []byte, int, error) {
		return []byte("permission denied ghp_abcdefghijklmnopqrstuvwxyz123456"), []byte("cannot connect ghp_abcdefghijklmnopqrstuvwxyz123456"), 1, nil
	}

	result, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{Shell: "sh", Script: "false"}, Options{Root: t.TempDir(), NodeVersion: "22"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if result.Status != model.StatusFinding || result.ExitCode != 1 {
		t.Fatalf("result=%#v", result)
	}
	if !strings.Contains(result.Stderr, "cannot connect") {
		t.Fatalf("stderr=%q lost command output", result.Stderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, "ghp_abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("token leaked in result=%#v", result)
	}
}

func TestRunContainerRedactsExit125RuntimeFailure(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "/fake/docker", nil }
	commandFunc = func(_ context.Context, _ string, _ []string) ([]byte, []byte, int, error) {
		return nil, []byte("runtime unavailable ghp_abcdefghijklmnopqrstuvwxyz123456"), 125, nil
	}

	_, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{Shell: "sh", Script: "true"}, Options{Root: t.TempDir(), NodeVersion: "22"})
	if err == nil {
		t.Fatal("expected runtime error")
	}
	if strings.Contains(err.Error(), "ghp_abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("token leaked in error=%q", err)
	}
}

func TestRunContainerRedactsCommandError(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "/fake/docker", nil }
	commandFunc = func(_ context.Context, _ string, _ []string) ([]byte, []byte, int, error) {
		return nil, nil, -1, errors.New("cannot start runtime ghp_abcdefghijklmnopqrstuvwxyz123456")
	}

	_, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{Shell: "sh", Script: "true"}, Options{Root: t.TempDir(), NodeVersion: "22"})
	if err == nil {
		t.Fatal("expected runtime error")
	}
	if strings.Contains(err.Error(), "ghp_abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("token leaked in error=%q", err)
	}
}

func TestRunContainerAllowNetworkRemovesOnlyNetworkRestriction(t *testing.T) {
	oldLookPath, oldCommand := lookPath, commandFunc
	t.Cleanup(func() { lookPath, commandFunc = oldLookPath, oldCommand })
	lookPath = func(string) (string, error) { return "/fake/docker", nil }
	var args []string
	commandFunc = func(_ context.Context, _ string, got []string) ([]byte, []byte, int, error) {
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
	result, err := RunContainer(context.Background(), NewContainerGrant(), model.DocBlock{ID: "live", Shell: "sh", Script: "printf live"}, Options{Root: root, NodeVersion: "22", Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusPass || result.Stdout != "live" {
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

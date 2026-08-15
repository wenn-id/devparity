//go:build windows

package execute

import "testing"

// The container image is always Linux, so the process limit must be attempted
// on every host — including Windows, where Docker Desktop runs Linux containers.
func TestRunContainerWindowsHostIncludesPidsLimit(t *testing.T) {
	args := buildContainerArgs("devparity-test", `C:\workspace`, "22", "sh", []string{"-eu", "-c", "true"}, &EnvironmentSnapshot{}, Options{})
	if !containsArg(args, "--pids-limit") || !containsArg(args, "256") {
		t.Fatalf("Windows host dropped the process limit: args=%#v", args)
	}
}

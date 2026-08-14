package execute

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wenn-id/devparity/internal/model"
)

func TestRunHostRejectsZeroGrant(t *testing.T) {
	_, err := RunHost(context.Background(), Grant{}, model.DocBlock{Shell: "sh", Script: "exit 0"}, Options{Root: t.TempDir()})
	if err == nil {
		t.Fatal("expected grant error")
	}
}

func TestRunHostSuccessUsesMinimalForwardedEnvironment(t *testing.T) {
	grant, err := NewHostGrant(true)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEVPARITY_TEST_SECRET", "exact-secret")
	block := model.DocBlock{ID: "README.md:2", Shell: "sh", Script: `printf '%s|%s|%s' "$DEVPARITY_TEST_SECRET" "${DEVPARITY_TEST_ABSENT-}" "${DEVPARITY_NOT_FORWARDED-}"`}
	result, err := RunHost(context.Background(), grant, block, Options{Root: t.TempDir(), EnvNames: []string{"DEVPARITY_TEST_SECRET", "DEVPARITY_TEST_ABSENT"}, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusPass || result.ExitCode != 0 {
		t.Fatalf("result=%#v", result)
	}
	if strings.Contains(result.Stdout, "exact-secret") || strings.Contains(result.Stdout, "DEVPARITY_NOT_FORWARDED") {
		t.Fatalf("result=%#v", result)
	}
	if !strings.Contains(result.Stdout, "[REDACTED]") {
		t.Fatalf("result=%#v, expected redaction", result)
	}
}

func TestRunHostNonzeroExit(t *testing.T) {
	grant, err := NewHostGrant(true)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunHost(context.Background(), grant, model.DocBlock{Shell: "sh", Script: "exit 7"}, Options{Root: t.TempDir(), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusFinding || result.ExitCode != 7 {
		t.Fatalf("result=%#v", result)
	}
}

func TestRunHostTimeout(t *testing.T) {
	grant, err := NewHostGrant(true)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	result, err := RunHost(context.Background(), grant, model.DocBlock{Shell: "sh", Script: "sleep 1"}, Options{Root: t.TempDir(), Timeout: 100 * time.Millisecond})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusFinding {
		t.Fatalf("result=%#v", result)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("timeout exceeded bound: elapsed=%s result=%#v", elapsed, result)
	}
}

func TestRunHostTimeoutKillsBackgroundChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell process-tree test")
	}
	grant, err := NewHostGrant(true)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "child-finished")
	script := "(sleep 1; printf done > " + shellQuote(marker) + ") & wait"
	started := time.Now()
	result, err := RunHost(context.Background(), grant, model.DocBlock{Shell: "sh", Script: script}, Options{Root: t.TempDir(), Timeout: 100 * time.Millisecond})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusFinding {
		t.Fatalf("result=%#v", result)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("timeout waited for background child: elapsed=%s result=%#v", elapsed, result)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("background child survived timeout")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func TestRunHostCapsOutputPerStream(t *testing.T) {
	grant, err := NewHostGrant(true)
	if err != nil {
		t.Fatal(err)
	}
	block := model.DocBlock{Shell: "sh", Script: "printf '%*s' 2097152 ''"}
	result, err := RunHost(context.Background(), grant, block, Options{Root: t.TempDir(), Timeout: 2 * time.Second, MaxOutput: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(result.Stdout)) > 1<<20 {
		t.Fatalf("stdout=%d bytes", len(result.Stdout))
	}
}

package execute

import (
	"context"
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
	block := model.DocBlock{ID: "README.md:2", Shell: shellName("sh", "powershell"), Script: shellScript(`printf '%s|%s|%s' "$DEVPARITY_TEST_SECRET" "${DEVPARITY_TEST_ABSENT-}" "${DEVPARITY_NOT_FORWARDED-}"`, `$env:DEVPARITY_TEST_SECRET; Write-Output "$env:DEVPARITY_TEST_SECRET|$env:DEVPARITY_TEST_ABSENT|$env:DEVPARITY_NOT_FORWARDED"`)}
	result, err := RunHost(context.Background(), grant, block, Options{Root: t.TempDir(), EnvNames: []string{"DEVPARITY_TEST_SECRET", "DEVPARITY_TEST_ABSENT"}, Timeout: 2 * time.Second})
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
	result, err := RunHost(context.Background(), grant, model.DocBlock{Shell: shellName("sh", "powershell"), Script: shellScript("exit 7", "exit 7")}, Options{Root: t.TempDir(), Timeout: time.Second})
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
	result, err := RunHost(context.Background(), grant, model.DocBlock{Shell: shellName("sh", "powershell"), Script: shellScript("sleep 1", "Start-Sleep -Seconds 1")}, Options{Root: t.TempDir(), Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusFinding {
		t.Fatalf("result=%#v", result)
	}
}

func TestRunHostCapsOutputPerStream(t *testing.T) {
	grant, err := NewHostGrant(true)
	if err != nil {
		t.Fatal(err)
	}
	block := model.DocBlock{Shell: "sh", Script: shellScript("printf '%*s' 2097152 ''", "Write-Output ('x' * 2097152)")}
	result, err := RunHost(context.Background(), grant, block, Options{Root: t.TempDir(), Timeout: 2 * time.Second, MaxOutput: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(result.Stdout)) > 1<<20 {
		t.Fatalf("stdout=%d bytes", len(result.Stdout))
	}
}

func shellName(unix, windows string) string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return unix
}

func shellScript(unix, windows string) string {
	if runtime.GOOS == "windows" {
		return windows
	}
	return unix
}

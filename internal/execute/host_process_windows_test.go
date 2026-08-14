//go:build windows

package execute

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wenn-id/devparity/internal/model"
)

func TestRunHostTimeoutKillsWindowsBackgroundChild(t *testing.T) {
	grant, err := NewHostGrant(true)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "child-finished.txt")
	marker = strings.ReplaceAll(marker, "'", "''")
	script := fmt.Sprintf("$child = Start-Process -FilePath powershell.exe -ArgumentList @('-NoProfile','-NonInteractive','-Command',\"Start-Sleep -Seconds 1; Set-Content -LiteralPath '%s' -Value done\") -PassThru; Wait-Process -Id $child.Id", marker)
	started := time.Now()
	result, err := RunHost(context.Background(), grant, model.DocBlock{Shell: "powershell", Script: script}, Options{Root: t.TempDir(), Timeout: 100 * time.Millisecond})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusFinding {
		t.Fatalf("result=%#v", result)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("timeout waited for Windows background child: elapsed=%s result=%#v", elapsed, result)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("Windows background child survived timeout")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

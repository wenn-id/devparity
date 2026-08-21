package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wenn-id/devparity/internal/model"
)

func TestDoctorCLI(t *testing.T) {
	clean := filepath.Join("..", "..", "testdata", "repos", "clean-node")
	drifted := filepath.Join("..", "..", "testdata", "repos", "drifted-node")
	tests := []struct {
		name     string
		args     []string
		code     int
		contains string
	}{
		{name: "clean", args: []string{"doctor", clean}, code: 0},
		{name: "drifted non strict", args: []string{"doctor", drifted}, code: 0, contains: "node-version-conflict"},
		{name: "drifted strict", args: []string{"doctor", drifted, "--strict"}, code: 1},
		{name: "bad format", args: []string{"doctor", clean, "--format", "bad"}, code: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run(tt.args, &stdout, &stderr); code != tt.code {
				t.Fatalf("code=%d, want %d; stdout=%q stderr=%q", code, tt.code, stdout.String(), stderr.String())
			}
			if tt.contains != "" && !strings.Contains(stdout.String(), tt.contains) {
				t.Fatalf("stdout=%q, missing %q", stdout.String(), tt.contains)
			}
		})
	}
}

func TestDoctorJSONUsesSchemaOne(t *testing.T) {
	var stdout, stderr bytes.Buffer
	clean := filepath.Join("..", "..", "testdata", "repos", "clean-node")
	if code := Run([]string{"doctor", "--format", "json", clean}, &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var report model.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 1 {
		t.Fatalf("report=%#v", report)
	}
}

func TestDoctorAndDocsVerifyReportTheSameRepositoryPath(t *testing.T) {
	// The two subcommands must normalise path separators identically at the
	// reporting boundary so JSON consumers never see both "a/b" and "a\b"
	// for the same repository.
	clean := filepath.Join("..", "..", "testdata", "repos", "clean-node")

	var doctorOut, docsOut bytes.Buffer
	var stderr bytes.Buffer
	if code := Run([]string{"doctor", "--format", "json", clean}, &doctorOut, &stderr); code != 0 {
		t.Fatalf("doctor code=%d stderr=%q", code, stderr.String())
	}
	stderr.Reset()
	if code := Run([]string{"docs", "verify", "--format", "json", clean}, &docsOut, &stderr); code != 0 {
		t.Fatalf("docs verify code=%d stderr=%q", code, stderr.String())
	}

	var doctorReport, docsReport model.Report
	if err := json.Unmarshal(doctorOut.Bytes(), &doctorReport); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(docsOut.Bytes(), &docsReport); err != nil {
		t.Fatal(err)
	}
	if doctorReport.Repository != docsReport.Repository {
		t.Fatalf("doctor repository=%q, docs verify repository=%q; want identical", doctorReport.Repository, docsReport.Repository)
	}
	if strings.Contains(doctorReport.Repository, "\\") {
		t.Fatalf("repository=%q, want forward slashes in the report", doctorReport.Repository)
	}
}

func TestDoctorGitHubWritesStepSummary(t *testing.T) {
	clean := filepath.Join("..", "..", "testdata", "repos", "clean-node")
	summary := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"doctor", clean, "--format", "github"}, &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout=%q, want empty", stdout.String())
	}
	data, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "## DevParity") {
		t.Fatalf("summary=%q", data)
	}
}

func TestDoctorGitHubRequiresSummaryPath(t *testing.T) {
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"doctor", "--format", "github"}, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

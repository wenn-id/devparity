package report

import (
	"bytes"
	"strings"
	"testing"
)

func TestGitHubRendersMarkdown(t *testing.T) {
	var out bytes.Buffer
	if err := GitHub(&out, fixedReport()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "## DevParity") || !strings.Contains(out.String(), "node-version-conflict") {
		t.Fatalf("markdown=%q", out.String())
	}
}

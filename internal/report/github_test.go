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

func TestMarkdownCellEscapesAmpersand(t *testing.T) {
	// Entity sequences in the source must survive as literal text: &lt; may
	// not be decoded into "<" when the step summary renders the cell.
	got := markdownCell("a & b &lt; &amp; c")
	want := "a &amp; b &amp;lt; &amp;amp; c"
	if got != want {
		t.Fatalf("markdownCell=%q, want %q", got, want)
	}
}

package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devparity/devparity/internal/model"
)

func TestExtractRequiresExactAdjacentMarker(t *testing.T) {
	input := "<!-- devparity:run -->\n```sh\nnpm ci\nnpm test\n```\n" +
		"```sh\nnpm run ignored\n```\n"
	blocks, findings := extractFixture(t, input)
	if len(findings) != 0 || len(blocks) != 1 {
		t.Fatalf("blocks=%#v findings=%#v", blocks, findings)
	}
	if blocks[0].ID != "README.md:2" || blocks[0].Shell != "sh" || blocks[0].Script != "npm ci\nnpm test" {
		t.Fatalf("block=%#v", blocks[0])
	}
	if blocks[0].Source.Path != "README.md" || blocks[0].Source.Line != 2 {
		t.Fatalf("source=%#v", blocks[0].Source)
	}
}

func TestExtractRequiresMarkerImmediatelyBeforeFence(t *testing.T) {
	input := "<!-- devparity:run -->\n\n```sh\nnpm run ignored\n```\n" +
		"text <!-- devparity:run -->\n```sh\nnpm run also-ignored\n```\n"
	blocks, findings := extractFixture(t, input)
	if len(findings) != 0 || len(blocks) != 0 {
		t.Fatalf("blocks=%#v findings=%#v", blocks, findings)
	}
}

func TestExtractReportsUnterminatedFence(t *testing.T) {
	blocks, findings := extractFixture(t, "<!-- devparity:run -->\n```bash\nnpm test\n")
	if len(blocks) != 0 || len(findings) != 1 {
		t.Fatalf("blocks=%#v findings=%#v", blocks, findings)
	}
	if findings[0].Status != model.StatusInconclusive || findings[0].RuleID != "doc-block-unterminated" {
		t.Fatalf("finding=%#v", findings[0])
	}
}

func TestExtractReportsUnsupportedLanguage(t *testing.T) {
	blocks, findings := extractFixture(t, "<!-- devparity:run -->\n```python\nprint('no')\n```\n")
	if len(blocks) != 0 || len(findings) != 1 {
		t.Fatalf("blocks=%#v findings=%#v", blocks, findings)
	}
	if findings[0].Status != model.StatusInconclusive || findings[0].RuleID != "doc-shell-unsupported" {
		t.Fatalf("finding=%#v", findings[0])
	}
}

func TestExtractAcceptsPowerShellAndAliases(t *testing.T) {
	for _, shell := range []string{"shell", "bash", "powershell", "pwsh"} {
		t.Run(shell, func(t *testing.T) {
			blocks, findings := extractFixture(t, "<!-- devparity:run -->\n```"+shell+"\nnpm test\n```\n")
			if len(findings) != 0 || len(blocks) != 1 {
				t.Fatalf("blocks=%#v findings=%#v", blocks, findings)
			}
			if blocks[0].Shell != shell || blocks[0].Script != "npm test" {
				t.Fatalf("block=%#v", blocks[0])
			}
		})
	}
}

func TestExtractCapsScannerTokenAtOneMiB(t *testing.T) {
	input := "<!-- devparity:run -->\n```sh\n" + strings.Repeat("x", 1<<20) + "\n```\n"
	blocks, findings := extractFixture(t, input)
	if len(blocks) != 0 || len(findings) != 1 || findings[0].Status != model.StatusInconclusive {
		t.Fatalf("blocks=%#v findings=%#v", blocks, findings)
	}
}

func extractFixture(t *testing.T, input string) ([]model.DocBlock, []model.Finding) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	return Extract(root, []string{"README.md"})
}

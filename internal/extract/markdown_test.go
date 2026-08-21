package extract

import (
	"testing"

	"github.com/wenn-id/devparity/internal/model"
)

func TestMarkdownVersions(t *testing.T) {
	tests := []struct {
		name        string
		contents    string
		wantValue   string
		wantLine    int
		wantFacts   int
		wantFinding bool
	}{
		{
			name:      "requires Node.js range",
			contents:  "# Requirements\nRequires Node.js >=20 <23.\n",
			wantValue: ">=20 <23",
			wantLine:  2,
			wantFacts: 1,
		},
		{
			name:      "accepts optional version word and leading v",
			contents:  "Use node version v22.4.1, today.\n",
			wantValue: "22.4.1",
			wantLine:  1,
			wantFacts: 1,
		},
		{
			name:      "scans each line",
			contents:  "node 20\nNode.js 22\n",
			wantFacts: 2,
		},
		{
			name:        "multiple matches on one line are inconclusive",
			contents:    "Supports Node.js 20 and Node.js 22.\n",
			wantFinding: true,
		},
		{
			name:     "unrelated prose is ignored",
			contents: "This project has no runtime requirement.\n",
		},
		{
			name:     "ordinary prose mentioning Node.js twice is ignored",
			contents: "Node.js is a runtime. Node.js powers everything.\n",
		},
		{
			name:      "open-ended plus suffix means >=N",
			contents:  "Requires Node 18+ to build.\n",
			wantValue: ">=18",
			wantLine:  1,
			wantFacts: 1,
		},
		{
			name:      "open-ended plus suffix after version word",
			contents:  "Use node version 20+ in CI.\n",
			wantValue: ">=20",
			wantLine:  1,
			wantFacts: 1,
		},
		{
			name:     "unsupported plus spelling is dropped",
			contents: "Requires Node 18.4.1+ exactly.\n",
		},
		{
			name:      "fenced code blocks are skipped",
			contents:  "```bash\nNode.js 16 is mentioned in a fenced block.\n```\n\nNode 20.\n",
			wantValue: "20",
			wantLine:  5,
			wantFacts: 1,
		},
		{
			name:     "tilde fenced code blocks are skipped",
			contents: "~~~\nnode 16\n~~~\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeTask5Fixture(t, root, "README.md", test.contents)

			facts, findings := MarkdownVersions(root, []string{"README.md"})
			if test.wantFinding {
				if len(facts) != 0 || len(findings) != 1 || findings[0].Status != model.StatusInconclusive {
					t.Fatalf("facts=%#v findings=%#v, want one inconclusive finding", facts, findings)
				}
				return
			}
			if len(findings) != 0 || len(facts) != test.wantFacts {
				t.Fatalf("facts=%#v findings=%#v, want %d facts", facts, findings, test.wantFacts)
			}
			if test.wantValue != "" {
				assertTask5NodeFact(t, facts[0], "README.md", test.wantValue, test.wantLine)
			}
		})
	}
}

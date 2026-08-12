package extract

import (
	"testing"

	"github.com/devparity/devparity/internal/model"
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

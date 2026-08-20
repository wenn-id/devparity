package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wenn-id/devparity/internal/model"
)

func TestVersionFiles(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		contents    string
		wantValue   string
		wantLine    int
		wantFinding bool
	}{
		{
			name:      "nvmrc strips leading v",
			path:      ".nvmrc",
			contents:  "v22.4.1\n",
			wantValue: "22.4.1",
			wantLine:  1,
		},
		{
			name:      "node version accepts one nonempty line",
			path:      ".node-version",
			contents:  "\n20\n",
			wantValue: "20",
			wantLine:  2,
		},
		{
			name:      "tool versions takes first nodejs pair",
			path:      ".tool-versions",
			contents:  "python 3.12\nnodejs 22.3.0\nnodejs 20\n",
			wantValue: "22.3.0",
			wantLine:  2,
		},
		{
			name:        "multiple logical lines are inconclusive",
			path:        ".nvmrc",
			contents:    "20\n22\n",
			wantFinding: true,
		},
		{
			name:        "tool versions without nodejs is inconclusive",
			path:        ".tool-versions",
			contents:    "python 3.12\n",
			wantFinding: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeTask5Fixture(t, root, test.path, test.contents)

			facts, findings := VersionFiles(root, []string{test.path})
			if test.wantFinding {
				if len(facts) != 0 || len(findings) != 1 || findings[0].Status != model.StatusInconclusive {
					t.Fatalf("facts=%#v findings=%#v, want one inconclusive finding", facts, findings)
				}
				return
			}
			if len(findings) != 0 || len(facts) != 1 {
				t.Fatalf("facts=%#v findings=%#v", facts, findings)
			}
			assertTask5NodeFact(t, facts[0], filepath.ToSlash(test.path), test.wantValue, test.wantLine)
		})
	}
}

func TestDockerfile(t *testing.T) {
	tests := []struct {
		name        string
		contents    string
		wantValue   string
		wantFinding bool
	}{
		{
			name:      "strips image suffix",
			contents:  "FROM node:22-slim\n",
			wantValue: "22",
		},
		{
			name:      "matches case insensitively",
			contents:  "  from NODE:22-alpine AS build\n",
			wantValue: "22",
		},
		{
			name:      "skips leading flags",
			contents:  "FROM --platform=linux/amd64 node:18\n",
			wantValue: "18",
		},
		{
			name:      "strips Docker Hub registry prefix",
			contents:  "FROM docker.io/library/node:20\n",
			wantValue: "20",
		},
		{
			name:      "strips arbitrary registry namespace",
			contents:  "FROM public.ecr.aws/docker/library/node:21-bookworm\n",
			wantValue: "21",
		},
		{
			name:        "variable is inconclusive",
			contents:    "FROM node:${NODE_VERSION}\n",
			wantFinding: true,
		},
		{
			name:        "unclassified image is inconclusive",
			contents:    "FROM alpine:3.20\n",
			wantFinding: true,
		},
		{
			name:        "scratch image is inconclusive",
			contents:    "FROM scratch\n",
			wantFinding: true,
		},
		{
			name:        "missing image after flags is inconclusive",
			contents:    "FROM --platform=linux/amd64\n",
			wantFinding: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeTask5Fixture(t, root, "Dockerfile", test.contents)

			facts, findings := Dockerfile(root, "Dockerfile")
			if test.wantFinding {
				if len(facts) != 0 || len(findings) != 1 || findings[0].Status != model.StatusInconclusive {
					t.Fatalf("facts=%#v findings=%#v, want one inconclusive finding", facts, findings)
				}
				return
			}
			if len(findings) != 0 || len(facts) != 1 {
				t.Fatalf("facts=%#v findings=%#v", facts, findings)
			}
			assertTask5NodeFact(t, facts[0], "Dockerfile", test.wantValue, 1)
		})
	}
}

func assertTask5NodeFact(t *testing.T, fact model.Fact, path, value string, line int) {
	t.Helper()
	if fact.Kind != model.FactKind("node.constraint") || fact.Subject != "node" || fact.Value != value {
		t.Fatalf("fact=%#v, want node.constraint node %q", fact, value)
	}
	if fact.Source.Path != filepath.ToSlash(path) || fact.Source.Line != line {
		t.Fatalf("source=%#v, want path=%q line=%d", fact.Source, filepath.ToSlash(path), line)
	}
}

func writeTask5Fixture(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

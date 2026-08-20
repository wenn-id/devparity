package nodecmd

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		line, manager, operation, script string
	}{
		{"npm ci", "npm", "install", ""},
		{"npm install", "npm", "install", ""},
		{"npm i", "npm", "install", ""},
		{"npm add", "npm", "install", ""},
		{"npm run integration", "npm", "script", "integration"},
		{"npm test", "npm", "test", "test"},
		{"pnpm install", "pnpm", "install", ""},
		{"pnpm i", "pnpm", "install", ""},
		{"pnpm ci", "pnpm", "install", ""},
		{"pnpm build", "pnpm", "build", "build"},
		{"yarn", "yarn", "install", ""},
		{"yarn install", "yarn", "install", ""},
		{"yarn i", "yarn", "i", "i"},
		{"yarn ci", "yarn", "ci", "ci"},
		{"yarn run lint", "yarn", "script", "lint"},
	}
	for _, tt := range tests {
		got, ok := Parse(tt.line)
		if !ok || got.Manager != tt.manager || got.Operation != tt.operation || got.Script != tt.script {
			t.Fatalf("%q => %#v, %v", tt.line, got, ok)
		}
	}
}

func TestParseRejectsShellIndirectionAndOperators(t *testing.T) {
	for _, line := range []string{
		"npm ci && npm test",
		"npm ci | npm test",
		"npm ci > output",
		"npm ci; npm test",
		"npm ci &",
		"npm ci || true",
		"CI=true npm ci",
		"npm ci $(date)",
		"npm ci `date`",
		"npm $COMMAND",
		"npm\nci",
		"npm\r\nci",
	} {
		if got, ok := Parse(line); ok {
			t.Fatalf("%q unexpectedly parsed as %#v", line, got)
		}
	}
}

func TestParseRejectsUnapprovedForms(t *testing.T) {
	for _, line := range []string{
		"",
		"bun install",
		"npm run",
		"npm run a b",
		"npm ci extra",
		"pnpm",
		"pnpm add",
		"yarn run",
		"yarn add",
		"yarn run lint extra",
		"pnpm run build extra",
	} {
		if got, ok := Parse(line); ok {
			t.Fatalf("%q unexpectedly parsed as %#v", line, got)
		}
	}
}

func TestParsePreservesRawInput(t *testing.T) {
	got, ok := Parse("  npm test  ")
	if !ok {
		t.Fatal("expected command")
	}
	if got.Raw != "  npm test  " {
		t.Fatalf("raw=%q", got.Raw)
	}
}

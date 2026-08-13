package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wenn-id/devparity/internal/model"
)

func TestPackageJSONExtractsNodeManagerAndScripts(t *testing.T) {
	root := t.TempDir()
	path := "package.json"
	writePackageFixture(t, root, path, `{
  "engines": { "node": ">=20 <23" },
  "packageManager": "pnpm@10.0.0",
  "scripts": { "test": "node --test", "build": "tsc" }
}`)

	facts, findings := PackageJSON(root, path)
	if len(findings) != 0 {
		t.Fatalf("unexpected findings: %#v", findings)
	}
	assertFact(t, facts, "node.constraint", "node", ">=20 <23", "engines.node", 2)
	assertFact(t, facts, "package.manager.declared", "package-manager", "pnpm", "packageManager", 3)
	assertFact(t, facts, "package.script", "test", "node --test", "scripts.test", 4)
	assertFact(t, facts, "package.script", "build", "tsc", "scripts.build", 4)
}

func TestPackageJSONReportsMalformedAndUnsupportedManager(t *testing.T) {
	root := t.TempDir()
	writePackageFixture(t, root, "malformed.json", `{"scripts":`)
	_, findings := PackageJSON(root, "malformed.json")
	if len(findings) != 1 || findings[0].RuleID != "parse-error" || findings[0].Status != model.StatusInconclusive {
		t.Fatalf("findings=%#v", findings)
	}

	writePackageFixture(t, root, "bun.json", `{"packageManager":"bun@1.0.0"}`)
	facts, findings := PackageJSON(root, "bun.json")
	if len(facts) != 0 {
		t.Fatalf("facts=%#v, want none for unsupported manager", facts)
	}
	if len(findings) != 1 || findings[0].RuleID != "package-manager-unsupported" || findings[0].Status != model.StatusInconclusive {
		t.Fatalf("findings=%#v", findings)
	}
}

func TestLockfilesExtractPackageManagerFacts(t *testing.T) {
	facts := Lockfiles([]string{
		"yarn.lock",
		"package-lock.json",
		"pnpm-lock.yaml",
		"npm-shrinkwrap.json",
		"other.lock",
	})
	if len(facts) != 4 {
		t.Fatalf("facts=%#v", facts)
	}
	assertFact(t, facts, "package.manager.lockfile", "package-manager", "npm", "", 1)
	assertFact(t, facts, "package.manager.lockfile", "package-manager", "pnpm", "", 1)
	assertFact(t, facts, "package.manager.lockfile", "package-manager", "yarn", "", 1)
}

func assertFact(t *testing.T, facts []model.Fact, kind, subject, value, field string, line int) {
	t.Helper()
	for _, fact := range facts {
		if fact.Kind == model.FactKind(kind) && fact.Subject == subject && fact.Value == value && fact.Source.Field == field && fact.Source.Line == line {
			return
		}
	}
	t.Fatalf("missing fact kind=%q subject=%q value=%q field=%q line=%d in %#v", kind, subject, value, field, line, facts)
}

func writePackageFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPackageJSONReportsDuplicateKeys(t *testing.T) {
	root := t.TempDir()
	writePackageFixture(t, root, "duplicate.json", `{"scripts":{"test":"a","test":"b"}}`)
	_, findings := PackageJSON(root, "duplicate.json")
	if len(findings) != 1 || findings[0].RuleID != "parse-error" || findings[0].Status != model.StatusInconclusive {
		t.Fatalf("findings=%#v", findings)
	}
}

func TestPackageJSONMissingOptionalFieldsPasses(t *testing.T) {
	root := t.TempDir()
	writePackageFixture(t, root, "minimal.json", `{}`)
	facts, findings := PackageJSON(root, "minimal.json")
	if len(facts) != 0 || len(findings) != 0 {
		t.Fatalf("facts=%#v findings=%#v", facts, findings)
	}
}

func TestPackageJSONReadFailureIsInconclusive(t *testing.T) {
	_, findings := PackageJSON(t.TempDir(), "missing.json")
	if len(findings) != 1 || findings[0].RuleID != "parse-error" || findings[0].Status != model.StatusInconclusive {
		t.Fatalf("findings=%#v", findings)
	}
}

package analyze

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// staticAnalysisPackages is the single list of packages on the static
// analysis path. The guard below iterates exactly this list, so adding a
// package here (and to the static path) extends the guard automatically.
var staticAnalysisPackages = []string{
	"internal/analyze",
	"internal/docs",
	"internal/extract",
	"internal/model",
	"internal/nodecmd",
	"internal/report",
	"internal/repository",
	"internal/rules",
	"internal/semverx",
}

// forbiddenStaticImports are import paths a static-analysis package must not
// pull in: process spawning, network, direct syscalls, and dynamically
// loaded code.
var forbiddenStaticImports = []string{
	"os/exec",
	"os.StartProcess",
	"net",
	"net/http",
	"syscall",
	"plugin",
	"golang.org/x/sys",
}

func TestStaticPackagesDoNotImportExecutionOrNetwork(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, packagePath := range staticAnalysisPackages {
		matches, err := filepath.Glob(filepath.Join(root, packagePath, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 0 {
			t.Fatalf("static package %s has no Go files; fix the package list in static_safety_test.go", packagePath)
		}
		for _, path := range matches {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, importSpec := range file.Imports {
				pathValue := strings.Trim(importSpec.Path.Value, "\"")
				for _, forbidden := range forbiddenStaticImports {
					// "net" and "net/http" match the prefix and the exact
					// path; the others must match exactly.
					if forbidden == "net" || forbidden == "net/http" {
						if pathValue == "net" || pathValue == "net/http" || strings.HasPrefix(pathValue, "net/") {
							t.Fatalf("static package %s imports forbidden %q", path, pathValue)
						}
						continue
					}
					if pathValue == forbidden {
						t.Fatalf("static package %s imports forbidden %q", path, pathValue)
					}
				}
			}
		}
	}
}

// TestStaticSafetyCoversEveryStaticPackage ensures the guard list stays in
// sync with the packages that actually exist under internal/. A new package
// reachable from the static path must be added to staticAnalysisPackages, or
// this test fails instead of the guard silently not covering it.
func TestStaticSafetyCoversEveryStaticPackage(t *testing.T) {
	root := filepath.Join("..", "..", "internal")
	entries, err := filepath.Glob(filepath.Join(root, "*"))
	if err != nil {
		t.Fatal(err)
	}
	// Packages intentionally outside the static path: they are allowed to
	// spawn processes or use the network.
	exempt := map[string]bool{
		"cli":     true, // CLI layer: parses flags, wires execution
		"execute": true, // runs commands on host and in containers
	}
	guarded := make(map[string]bool, len(staticAnalysisPackages))
	for _, packagePath := range staticAnalysisPackages {
		guarded[strings.TrimPrefix(packagePath, "internal/")] = true
	}
	for _, entry := range entries {
		info, err := filepath.Glob(filepath.Join(entry, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		if len(info) == 0 {
			continue // not a Go package directory
		}
		name := filepath.Base(entry)
		if exempt[name] {
			continue
		}
		if !guarded[name] {
			t.Fatalf("internal/%s is not covered by the static safety guard; add it to staticAnalysisPackages in static_safety_test.go (or to the exempt list if it is intentionally outside the static path)", name)
		}
	}
	// And nothing stale: every guarded package must exist.
	for _, packagePath := range staticAnalysisPackages {
		matches, err := filepath.Glob(filepath.Join("..", "..", packagePath, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 0 {
			t.Fatalf("staticAnalysisPackages lists %s, which has no Go files", packagePath)
		}
	}
}

// TestForbiddenImportListIsSane keeps the forbidden list meaningful.
func TestForbiddenImportListIsSane(t *testing.T) {
	seen := make(map[string]bool)
	for _, forbidden := range forbiddenStaticImports {
		if forbidden == "" {
			t.Fatal("forbidden import list contains an empty entry")
		}
		if seen[forbidden] {
			t.Fatalf("forbidden import list contains duplicate %q", forbidden)
		}
		seen[forbidden] = true
	}
	if !seen["os/exec"] || !seen["syscall"] || !seen["plugin"] {
		t.Fatal("forbidden import list lost a required entry")
	}
}

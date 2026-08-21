package analyze

import (
	"go/ast"
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

// forbiddenStaticImportPrefixes are import-path prefixes a static-analysis
// package must not pull in. A prefix matches the exact path and every
// subpackage ("golang.org/x/sys" also rejects "golang.org/x/sys/unix").
var forbiddenStaticImportPrefixes = []string{
	"os/exec",
	"net",
	"net/http",
	"syscall",
	"plugin",
	"golang.org/x/sys",
}

// forbiddenStaticSymbols are package-qualified identifiers a static-analysis
// package must not reference, regardless of how the import is aliased.
// os.StartProcess spawns processes without importing os/exec.
var forbiddenStaticSymbols = []string{
	"os.StartProcess",
}

func importForbidden(pathValue string) bool {
	for _, prefix := range forbiddenStaticImportPrefixes {
		if pathValue == prefix || strings.HasPrefix(pathValue, prefix+"/") {
			return true
		}
	}
	return false
}

// symbolForbidden reports whether selector references a forbidden symbol.
// The local name of the imported package is resolved so an aliased import
// cannot hide the reference: `alias "os"` + `alias.StartProcess` is caught.
func symbolForbidden(file *ast.File, qualifiedName string) bool {
	pkg, symbol, ok := strings.Cut(qualifiedName, ".")
	if !ok {
		return false
	}
	for _, importSpec := range file.Imports {
		importPath := strings.Trim(importSpec.Path.Value, "\"")
		// Only "os" itself can provide os.StartProcess.
		if importPath != pkg {
			continue
		}
		localName := pkg
		if importSpec.Name != nil {
			localName = importSpec.Name.Name
		}
		if localName == "_" || localName == "." {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if sel, isSel := n.(*ast.SelectorExpr); isSel {
				if ident, isIdent := sel.X.(*ast.Ident); isIdent && ident.Name == localName && sel.Sel.Name == symbol {
					// Found: remember by panicking with a sentinel that is
					// recovered below; ast.Inspect has no early exit.
					panic(symbolFound{})
				}
			}
			return true
		})
	}
	return false
}

type symbolFound struct{}

func symbolForbiddenRecovers(file *ast.File, qualifiedName string) (found bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(symbolFound); ok {
				found = true
				return
			}
			panic(r)
		}
	}()
	symbolForbidden(file, qualifiedName)
	return false
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
				if importForbidden(pathValue) {
					t.Fatalf("static package %s imports forbidden %q", path, pathValue)
				}
			}
			for _, symbol := range forbiddenStaticSymbols {
				if symbolForbiddenRecovers(file, symbol) {
					t.Fatalf("static package %s references forbidden symbol %q", path, symbol)
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

// TestStaticGuardDetectsForbiddenImports feeds synthetic sources through the
// exact check the package guard uses, proving the matcher rejects
// subpackages of forbidden modules and aliased os.StartProcess references.
func TestStaticGuardDetectsForbiddenImports(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		wantErr bool
	}{
		{name: "plain os import", source: `package x
import "os"
var _ = os.Args`, wantErr: false},
		{name: "x/sys root", source: `package x
import "golang.org/x/sys"`, wantErr: true},
		{name: "x/sys unix subpackage", source: `package x
import "golang.org/x/sys/unix"`, wantErr: true},
		{name: "net/http client", source: `package x
import "net/http"`, wantErr: true},
		{name: "os/exec", source: `package x
import "os/exec"`, wantErr: true},
		{name: "aliased os.StartProcess", source: `package x
import alias "os"
var _ = alias.StartProcess`, wantErr: true},
		{name: "plain os.StartProcess", source: `package x
import "os"
var _ = os.StartProcess`, wantErr: true},
		{name: "harmless os.Getenv", source: `package x
import "os"
var _ = os.Getenv`, wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", tc.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			violated := false
			for _, importSpec := range file.Imports {
				if importForbidden(strings.Trim(importSpec.Path.Value, "\"")) {
					violated = true
				}
			}
			for _, symbol := range forbiddenStaticSymbols {
				if symbolForbiddenRecovers(file, symbol) {
					violated = true
				}
			}
			if violated != tc.wantErr {
				t.Fatalf("violation=%v, want %v", violated, tc.wantErr)
			}
		})
	}
}

// TestForbiddenImportListIsSane keeps the forbidden lists meaningful: no
// empty or duplicate entries, and every mandatory entry still present so
// the guard cannot be silently weakened.
func TestForbiddenImportListIsSane(t *testing.T) {
	requiredPrefixes := []string{
		"os/exec",
		"net",
		"net/http",
		"syscall",
		"plugin",
		"golang.org/x/sys",
	}
	seen := make(map[string]bool)
	for _, forbidden := range forbiddenStaticImportPrefixes {
		if forbidden == "" {
			t.Fatal("forbidden import list contains an empty entry")
		}
		if seen[forbidden] {
			t.Fatalf("forbidden import list contains duplicate %q", forbidden)
		}
		seen[forbidden] = true
	}
	for _, required := range requiredPrefixes {
		if !seen[required] {
			t.Fatalf("forbidden import list lost required entry %q", required)
		}
	}
	if len(forbiddenStaticSymbols) == 0 {
		t.Fatal("forbidden symbol list is empty; os.StartProcess would go unguarded")
	}
	for _, symbol := range forbiddenStaticSymbols {
		if symbol == "" || !strings.Contains(symbol, ".") {
			t.Fatalf("forbidden symbol %q must be a package-qualified name", symbol)
		}
	}
}

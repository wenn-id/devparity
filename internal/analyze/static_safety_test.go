package analyze

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticPackagesDoNotImportExecutionOrNetwork(t *testing.T) {
	root := filepath.Join("..", "..")
	packages := []string{"internal/analyze", "internal/extract", "internal/repository", "internal/rules"}
	for _, packagePath := range packages {
		matches, err := filepath.Glob(filepath.Join(root, packagePath, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range matches {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, importSpec := range file.Imports {
				pathValue := strings.Trim(importSpec.Path.Value, "\"")
				if pathValue == "os/exec" || strings.HasPrefix(pathValue, "net") {
					t.Fatalf("static package %s imports forbidden %q", path, pathValue)
				}
				_ = ast.IsExported
			}
		}
	}
}

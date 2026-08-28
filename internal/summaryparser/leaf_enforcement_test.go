// leaf_enforcement_test.go enforces the Summaryparser Sole-Parser Invariant's stdlib-only half:
// production code in internal/summaryparser imports the standard library and nothing else -- never
// internal/lyxcwd, never any feature package.
// Like internal/pattern's leaf_enforcement_test.go, this check is an ALLOWLIST, empty here because
// no non-stdlib import is ever permitted, so a future stray dependency is caught with no list
// maintenance required.

package summaryparser

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// allowedImports is the set of non-stdlib import paths production code in this package may use.
// It is deliberately empty: internal/summaryparser is a stdlib-only leaf.
var allowedImports = map[string]bool{}

// TestLeafInvariant_AllowlistOnly verifies that every non-test .go file in this package directory
// imports only stdlib (no '.' in the first path segment) or an entry in allowedImports.
// It uses go/parser with ImportsOnly so only real import declarations are inspected, never string
// literals in doc comments.
func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine summaryparser source directory location")
	}
	pkgDir := filepath.Dir(file)

	var failures []string

	err := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), "_test.go") || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Logf("warning: failed to parse %s: %v", path, err)
			return nil
		}

		for _, imp := range astFile.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			// A stdlib import path has no '.' in its first path segment
			// (e.g. "fmt", "os", "go/parser") -- a domain that would need a
			// registered TLD (e.g. "github.com/...") always contains one.
			firstSegment := importPath
			if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
				firstSegment = importPath[:idx]
			}
			isStdlib := !strings.Contains(firstSegment, ".")

			if isStdlib || allowedImports[importPath] {
				continue
			}

			relPath, _ := filepath.Rel(pkgDir, path)
			failures = append(failures, relPath+": "+importPath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk summaryparser directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("Summaryparser Sole-Parser Invariant violated; imports outside stdlib found: %v", failures)
	}
}

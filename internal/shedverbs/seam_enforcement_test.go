// seam_enforcement_test.go enforces this package's no-resolver, no-<module>cli import seam: production
// code in internal/shedverbs takes every path it operates on from a told Spec, never derives one, and
// never imports back into a CLI module.
//
// The allowlist below is deliberately a membership list rather than a bare denylist, mirroring
// internal/battenshed's and internal/loomrecipe's own reasoning: it catches the excluded imports
// and anything else that would drag geometry resolution in, with no list maintenance beyond a genuine
// new dependency. shedverbs importing cobra while not being a <module>cli package is deliberate: the
// rule that matters is that an ENGINE never imports cli/cobra, and shedverbs is not an engine.

package shedverbs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// shedverbsAllowedImports are the only non-stdlib import paths production code in this package may
// use.
var shedverbsAllowedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/clihelp":    true,
	"github.com/Knatte18/loomyard/internal/output":     true,
	"github.com/Knatte18/loomyard/internal/state":      true,
	"github.com/Knatte18/loomyard/internal/shedengine": true,
	"github.com/spf13/cobra":                           true,
}

// shedverbsDeniedLyxcwdImport is the exact import path the no-resolver clause excludes from this
// package's production files, named here so a violation of that specific rule is reported by name
// rather than only implied by its absence from the allowlist above.
const shedverbsDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"

// isModuleCLIImportPath reports whether importPath has the <module>cli shape this package must
// never import: an internal/ path whose final segment ends in "cli".
func isModuleCLIImportPath(importPath string) bool {
	const prefix = "github.com/Knatte18/loomyard/internal/"
	if !strings.HasPrefix(importPath, prefix) {
		return false
	}
	rest := strings.TrimPrefix(importPath, prefix)
	// rest may still contain a slash for a nested package; the module name is its first segment.
	if idx := strings.IndexByte(rest, '/'); idx >= 0 {
		rest = rest[:idx]
	}
	return strings.HasSuffix(rest, "cli")
}

// TestNoResolverNoModuleCLIInvariant_AllowlistOnly verifies that every non-test .go file in this
// package imports only stdlib or an entry in shedverbsAllowedImports, and separately asserts that no
// production import path is shedverbsDeniedLyxcwdImport or matches the <module>cli shape.
func TestNoResolverNoModuleCLIInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine shedverbs source directory location")
	}
	pkgDir := filepath.Dir(file)

	var failures []string
	var deniedFound []string
	var moduleCLIFound []string

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

		relPath, _ := filepath.Rel(pkgDir, path)

		for _, imp := range astFile.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			if importPath == shedverbsDeniedLyxcwdImport {
				deniedFound = append(deniedFound, relPath)
			}
			if isModuleCLIImportPath(importPath) {
				moduleCLIFound = append(moduleCLIFound, relPath+": "+importPath)
			}

			firstSegment := importPath
			if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
				firstSegment = importPath[:idx]
			}
			isStdlib := !strings.Contains(firstSegment, ".")

			if isStdlib || shedverbsAllowedImports[importPath] {
				continue
			}

			failures = append(failures, relPath+": "+importPath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk shedverbs directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("no-resolver/no-<module>cli seam violated; imports outside the allowlist found: %v", failures)
	}
	if len(deniedFound) > 0 {
		t.Errorf("no-resolver seam violated; %s imported directly in: %v", shedverbsDeniedLyxcwdImport, deniedFound)
	}
	if len(moduleCLIFound) > 0 {
		t.Errorf("no-<module>cli seam violated; a <module>cli-shaped import was found in: %v", moduleCLIFound)
	}
}

// TestNoResolverInvariant_NoOSGetwd verifies that no production file in this package references
// os.Getwd, matching the no-resolver clause the new Shed Verb-Set Invariant carries in batch 6. A
// selector-expression walk is enough, since the package imports no shell runner through which
// `git rev-parse` could reach it.
func TestNoResolverInvariant_NoOSGetwd(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine shedverbs source directory location")
	}
	pkgDir := filepath.Dir(file)

	var found []string

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
		astFile, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Logf("warning: failed to parse %s: %v", path, err)
			return nil
		}

		relPath, _ := filepath.Rel(pkgDir, path)

		ast.Inspect(astFile, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if pkgIdent.Name == "os" && sel.Sel.Name == "Getwd" {
				found = append(found, relPath)
			}
			return true
		})

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk shedverbs directory: %v", err)
	}

	if len(found) > 0 {
		t.Errorf("no-resolver seam violated; os.Getwd referenced in: %v", found)
	}
}

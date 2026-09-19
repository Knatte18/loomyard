// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
// code in internal/lifecycleshed takes every absolute path it operates on from its caller and has
// no direct production import of internal/lyxcwd.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd
// denylist, mirroring internal/loomrecipe's and internal/shedrecipe's own reasoning: it catches the
// excluded import and anything else that would drag geometry resolution in, with no list
// maintenance beyond a genuine new dependency.

package lifecycleshed

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// lifecycleshedAllowedImports are the only non-stdlib import paths production code in this package
// may use.
var lifecycleshedAllowedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/shedengine": true,
	"github.com/Knatte18/loomyard/internal/logger":     true,
}

// lifecycleshedDeniedLyxcwdImport is the exact import path the Told-Geometry Invariant excludes
// from this package's production files, named here so a violation of that specific rule is
// reported by name rather than only implied by its absence from the allowlist above.
const lifecycleshedDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file in this package
// imports only stdlib or an entry in lifecycleshedAllowedImports, and separately asserts that no
// production import path is lifecycleshedDeniedLyxcwdImport.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine lifecycleshed source directory location")
	}
	pkgDir := filepath.Dir(file)

	var failures []string
	var deniedFound []string

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

			relPath, _ := filepath.Rel(pkgDir, path)
			if importPath == lifecycleshedDeniedLyxcwdImport {
				deniedFound = append(deniedFound, relPath)
			}

			firstSegment := importPath
			if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
				firstSegment = importPath[:idx]
			}
			isStdlib := !strings.Contains(firstSegment, ".")

			if isStdlib || lifecycleshedAllowedImports[importPath] {
				continue
			}

			failures = append(failures, relPath+": "+importPath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk lifecycleshed directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("Told-Geometry Invariant violated; imports outside the allowlist found: %v", failures)
	}
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", lifecycleshedDeniedLyxcwdImport, deniedFound)
	}
}

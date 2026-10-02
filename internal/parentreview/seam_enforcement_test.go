// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production code takes every absolute path from its caller and has no direct production import of internal/lyxcwd.
// The allowlist is a membership list rather than a bare denylist, modelled on internal/loomshed's own.

package parentreview

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// parentreviewAllowedImports are the only non-stdlib import paths production code in this package may use.
var parentreviewAllowedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/state":         true,
	"github.com/Knatte18/loomyard/internal/logger":        true,
	"github.com/Knatte18/loomyard/internal/shuttleengine": true,
}

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file imports only stdlib or an entry in parentreviewAllowedImports.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine parentreview source directory location")
	}
	pkgDir := filepath.Dir(file)

	var failures []string
	err := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasSuffix(d.Name(), "_test.go") || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		astFile, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Logf("warning: failed to parse %s: %v", path, err)
			return nil
		}
		for _, imp := range astFile.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			first := importPath
			if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
				first = importPath[:idx]
			}
			if !strings.Contains(first, ".") || parentreviewAllowedImports[importPath] {
				continue
			}
			rel, _ := filepath.Rel(pkgDir, path)
			failures = append(failures, rel+": "+importPath)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk parentreview directory: %v", err)
	}
	if len(failures) > 0 {
		t.Errorf("Told-Geometry Invariant violated; imports outside the allowlist found: %v", failures)
	}
}

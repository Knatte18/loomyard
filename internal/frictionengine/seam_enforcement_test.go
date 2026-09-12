// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
// code in internal/frictionengine takes every absolute path it operates on from its caller and has
// no direct production import of internal/lyxcwd. Modelled directly on
// internal/mergeresolve/seam_enforcement_test.go.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd denylist:
// it catches the excluded import and anything else that would drag geometry resolution in, with no
// list maintenance beyond a genuine new dependency, and a transitive reach through an allowed entry
// is explicitly fine.

package frictionengine

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// frictionengineAllowedImports are the only non-stdlib import paths production code in this package
// may use: internal/friction (ReportFileName and EnsureDir), the model-spec package (resolving the
// reflection session's model), the shuttle engine (the reflection session's seam), the stencil store
// (reading the reflection prompt off disk), the stencil filler (rendering it), and the logger.
var frictionengineAllowedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/friction":      true,
	"github.com/Knatte18/loomyard/internal/logger":        true,
	"github.com/Knatte18/loomyard/internal/modelspec":     true,
	"github.com/Knatte18/loomyard/internal/shuttleengine": true,
	"github.com/Knatte18/loomyard/internal/stencil":       true,
	"github.com/Knatte18/loomyard/internal/stencilstore":  true,
}

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file in this package
// imports only stdlib or an entry in frictionengineAllowedImports.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine frictionengine source directory location")
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

			firstSegment := importPath
			if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
				firstSegment = importPath[:idx]
			}
			isStdlib := !strings.Contains(firstSegment, ".")

			if isStdlib || frictionengineAllowedImports[importPath] {
				continue
			}

			relPath, _ := filepath.Rel(pkgDir, path)
			failures = append(failures, relPath+": "+importPath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk frictionengine directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("Told-Geometry Invariant violated; imports outside the allowlist found: %v", failures)
	}
}

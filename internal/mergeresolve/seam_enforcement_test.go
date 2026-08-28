// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
// code in internal/mergeresolve takes every absolute path it operates on from its caller and has no
// direct production import of internal/lyxcwd. Modelled directly on
// internal/loomshed/seam_enforcement_test.go.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd denylist:
// it catches the excluded import and anything else that would drag geometry resolution in, with no
// list maintenance beyond a genuine new dependency, and a transitive reach through an allowed entry
// is explicitly fine.

package mergeresolve

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// mergeresolveAllowedImports are the only non-stdlib import paths production code in this package
// may use: the fabric engine (the merge seam), the shuttle engine (the conflict-session seam), the
// model-spec package (resolving the conflict session's model), the stencil store (reading the
// conflict prompt off disk), the stencil filler (rendering it), and the logger.
// internal/logger carries no geometry and opens no seam, so admitting it leaves the Told-Geometry
// Invariant's actual property intact -- the same call CONSTRAINTS.md's Treadle Runner-Seam
// Invariant allowlist already makes.
var mergeresolveAllowedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/fabricengine":  true,
	"github.com/Knatte18/loomyard/internal/shuttleengine": true,
	"github.com/Knatte18/loomyard/internal/modelspec":     true,
	"github.com/Knatte18/loomyard/internal/stencilstore":  true,
	"github.com/Knatte18/loomyard/internal/stencil":       true,
	"github.com/Knatte18/loomyard/internal/logger":        true,
}

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file in this package
// imports only stdlib or an entry in mergeresolveAllowedImports.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine mergeresolve source directory location")
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

			if isStdlib || mergeresolveAllowedImports[importPath] {
				continue
			}

			relPath, _ := filepath.Rel(pkgDir, path)
			failures = append(failures, relPath+": "+importPath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk mergeresolve directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("Told-Geometry Invariant violated; imports outside the allowlist found: %v", failures)
	}
}

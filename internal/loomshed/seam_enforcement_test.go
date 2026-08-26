// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
// code in internal/loomshed takes every absolute path it operates on from its caller and has no
// direct production import of internal/lyxcwd. Named seam_enforcement_test.go rather than
// leaf_enforcement_test.go on purpose -- that second filename pairs with a TestLeafInvariant_
// AllowlistOnly name on genuine zero-or-one-dependency leaves, and loomshed imports seven internal
// packages, so it is structurally a seam, not a leaf, exactly like internal/shedengine's own
// seam_enforcement_test.go this file is modelled on.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd denylist:
// it catches the excluded import and anything else that would drag geometry resolution in, with no
// list maintenance beyond a genuine new dependency. internal/loomengine appearing on the allowlist
// is legal despite loomengine itself importing internal/lyxcwd, because the invariant's membership
// predicate is about a direct production import and transitive is explicitly fine.

package loomshed

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// loomshedAllowedImports are the only non-stdlib import paths production code in this package may
// use.
var loomshedAllowedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/shedengine":       true,
	"github.com/Knatte18/loomyard/internal/shedadapters":     true,
	"github.com/Knatte18/loomyard/internal/websterengine":    true,
	"github.com/Knatte18/loomyard/internal/loomengine":       true,
	"github.com/Knatte18/loomyard/internal/planparser":       true,
	"github.com/Knatte18/loomyard/internal/discussionparser": true,
	"github.com/Knatte18/loomyard/internal/batcher":          true,
	"github.com/Knatte18/loomyard/internal/state":            true,
	// internal/logger is a genuine new dependency rather than a loosened rule: the gate producers
	// here surface the determined findings they used to discard, and it resolves no geometry of any
	// kind, so it cannot drag cwd resolution in. internal/shedadapters, already on this list,
	// imports it too.
	"github.com/Knatte18/loomyard/internal/logger": true,
}

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file in this package
// imports only stdlib or an entry in loomshedAllowedImports.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine loomshed source directory location")
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

			if isStdlib || loomshedAllowedImports[importPath] {
				continue
			}

			relPath, _ := filepath.Rel(pkgDir, path)
			failures = append(failures, relPath+": "+importPath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk loomshed directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("Told-Geometry Invariant violated; imports outside the allowlist found: %v", failures)
	}
}

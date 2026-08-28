// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
// code in internal/landingshed takes every absolute path it operates on from its caller and has no
// direct production import of internal/lyxcwd. Modelled directly on internal/loomshed's own
// seam_enforcement_test.go: same walk, same imports-only parse, same stdlib rule, same
// allowlist-membership shape.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd denylist:
// it catches the excluded import and anything else that would drag geometry resolution in, with no
// list maintenance beyond a genuine new dependency. A transitive reach through an allowlisted
// dependency (internal/fabricengine and internal/githubclient both import internal/lyxcwd
// themselves) is explicitly fine -- the invariant's membership predicate is about a direct
// production import, and transitive is never policed.

package landingshed

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// landingshedAllowedImports are the only non-stdlib import paths production code in this package
// may use.
var landingshedAllowedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/fabricengine":  true,
	"github.com/Knatte18/loomyard/internal/mergeresolve":  true,
	"github.com/Knatte18/loomyard/internal/modelspec":     true,
	"github.com/Knatte18/loomyard/internal/configengine":  true,
	"github.com/Knatte18/loomyard/internal/logger":        true,
	"github.com/Knatte18/loomyard/internal/shedengine":    true,
	"github.com/Knatte18/loomyard/internal/githubclient":  true,
	"github.com/Knatte18/loomyard/internal/gitrepo":       true,
	"github.com/Knatte18/loomyard/internal/summaryparser": true,
	"github.com/google/go-github/v75/github":              true,
	"gopkg.in/yaml.v3":                                    true,
}

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file in this package
// imports only stdlib or an entry in landingshedAllowedImports.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine landingshed source directory location")
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

			if isStdlib || landingshedAllowedImports[importPath] {
				continue
			}

			relPath, _ := filepath.Rel(pkgDir, path)
			failures = append(failures, relPath+": "+importPath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk landingshed directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("Told-Geometry Invariant violated; imports outside the allowlist found: %v", failures)
	}
}

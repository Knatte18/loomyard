// seam_enforcement_test.go enforces this package's half of the Told-Geometry Invariant: production
// code in internal/cliwire takes every absolute path it operates on from its caller and has no
// direct production import of internal/lyxcwd.
//
// cliwire/doc.go already makes this claim in prose ("cliwire's production dependency set is fixed
// ... internal/lyxcwd is barred by the Told-Geometry Invariant and is never needed here"); this test
// is the mechanical pin behind that prose, mirroring internal/shedrecipe's own
// seam_enforcement_test.go (crucible round sonnet-xhigh-r8, CW-3). Several other Told-Geometry
// "Bound packages" (reedengine, burlerengine, websterengine, planparser, planglyph, configengine)
// carry the same unenforced claim today with no dedicated test of their own -- a genuine, repo-wide
// gap this one test closes only for cliwire, the package this round's own mandate is auditing, not
// for the whole Bound-packages set.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd denylist,
// mirroring internal/shedrecipe's own reasoning: it catches the excluded import and anything else
// that would drag geometry resolution in, with no list maintenance beyond a genuine new dependency.

package cliwire

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// cliwireAllowedImports are the only non-stdlib import paths production code in this package may
// use -- exactly the six named in doc.go's own "production dependency set is fixed" claim.
var cliwireAllowedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/standalonestate": true,
	"github.com/Knatte18/loomyard/internal/standalonegeom":  true,
	"github.com/Knatte18/loomyard/internal/logger":          true,
	"github.com/Knatte18/loomyard/internal/stencilstore":    true,
	"github.com/Knatte18/loomyard/internal/buildinfo":       true,
	"github.com/Knatte18/loomyard/contracts/stencils":       true,
}

// cliwireDeniedLyxcwdImport is the exact import path the Told-Geometry Invariant excludes from this
// package's production files, named here so a violation of that specific rule is reported by name
// rather than only implied by its absence from the allowlist above.
const cliwireDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file in this package
// imports only stdlib or an entry in cliwireAllowedImports, and separately asserts that no
// production import path is cliwireDeniedLyxcwdImport.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine cliwire source directory location")
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
			if importPath == cliwireDeniedLyxcwdImport {
				deniedFound = append(deniedFound, relPath)
			}

			firstSegment := importPath
			if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
				firstSegment = importPath[:idx]
			}
			isStdlib := !strings.Contains(firstSegment, ".")

			if isStdlib || cliwireAllowedImports[importPath] {
				continue
			}

			failures = append(failures, relPath+": "+importPath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk cliwire directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("Told-Geometry Invariant violated; imports outside the allowlist found: %v", failures)
	}
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", cliwireDeniedLyxcwdImport, deniedFound)
	}
}

// seam_enforcement_test.go enforces this package's told-geometry rule, mirroring
// internal/shedrecipe/seam_enforcement_test.go's shape exactly: production code in
// internal/shedbuild takes every absolute path it operates on from its caller and has no direct
// production import of internal/lyxcwd.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd
// denylist, for the same reason the sibling's is: it catches the excluded import and anything else
// that would drag geometry resolution in, with no list maintenance beyond a genuine new dependency.
// This package's allowlist is deliberately short: it reaches the registry, the producer-definition
// type, and the checker, and nothing else.

package shedbuild

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// shedbuildAllowedImports are the only non-stdlib import paths production code in this package may
// use.
var shedbuildAllowedImports = map[string]bool{
	"gopkg.in/yaml.v3": true,
	"github.com/Knatte18/loomyard/internal/shedrecipe": true,
	"github.com/Knatte18/loomyard/internal/shedengine": true,
	"github.com/Knatte18/loomyard/internal/shedcheck":  true,
}

// shedbuildDeniedLyxcwdImport is the exact import path the Told-Geometry Invariant excludes from
// this package's production files, named here so a violation of that specific rule is reported by
// name rather than only implied by its absence from the allowlist above.
const shedbuildDeniedLyxcwdImport = "github.com/Knatte18/loomyard/internal/lyxcwd"

// TestToldGeometryInvariant_AllowlistOnly verifies that every non-test .go file in this package
// imports only stdlib or an entry in shedbuildAllowedImports, and separately asserts that no
// production import path is shedbuildDeniedLyxcwdImport.
func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine shedbuild source directory location")
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
			if importPath == shedbuildDeniedLyxcwdImport {
				deniedFound = append(deniedFound, relPath)
			}

			firstSegment := importPath
			if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
				firstSegment = importPath[:idx]
			}
			isStdlib := !strings.Contains(firstSegment, ".")

			if isStdlib || shedbuildAllowedImports[importPath] {
				continue
			}

			failures = append(failures, relPath+": "+importPath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk shedbuild directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("Told-Geometry Invariant violated; imports outside the allowlist found: %v", failures)
	}
	if len(deniedFound) > 0 {
		t.Errorf("Told-Geometry Invariant violated; %s imported directly in: %v", shedbuildDeniedLyxcwdImport, deniedFound)
	}
}

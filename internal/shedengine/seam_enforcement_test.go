// seam_enforcement_test.go enforces the Shed Producer-Seam Invariant: production code in
// internal/shedengine imports ONLY the standard library, internal/state, and internal/lock — never
// internal/loomengine, never any adapter package, never internal/lyxcwd, and never internal/logger.
// Like internal/treadleengine's seam_enforcement_test.go, this check is an ALLOWLIST: any import
// outside the allowed set fails the test, so a future stray dependency is caught with no list
// maintenance required.
// One observation worth recording about today's allowlist, though this test checks direct imports
// only and enforces nothing beyond that: the exclusion of internal/lyxcwd happens to hold
// transitively too, because internal/lock imports no internal package at all and internal/state
// imports only internal/fsx and internal/lock.

package shedengine

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// shedengineAllowedImports are the only non-stdlib import paths production code in this package
// may use.
var shedengineAllowedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/state": true,
	"github.com/Knatte18/loomyard/internal/lock":  true,
}

// TestProducerSeamInvariant_AllowlistOnly verifies that every non-test .go file in this package
// imports only stdlib or an entry in shedengineAllowedImports.
func TestProducerSeamInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine shedengine source directory location")
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

			if isStdlib || shedengineAllowedImports[importPath] {
				continue
			}

			relPath, _ := filepath.Rel(pkgDir, path)
			failures = append(failures, relPath+": "+importPath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk shedengine directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("Shed Producer-Seam Invariant violated; imports outside the allowlist (stdlib + internal/state + internal/lock) found: %v", failures)
	}
}

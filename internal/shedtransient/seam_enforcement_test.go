// seam_enforcement_test.go enforces this package's import allowlist: production code in
// internal/shedtransient may import only the stdlib and the four packages whose classifications
// it translates, so shedengine stays stdlib-only and no leaf package ever imports shedengine.

package shedtransient

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// shedtransientAllowedImports are the only non-stdlib import paths production code in this package may use.
var shedtransientAllowedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/gitexec":       true,
	"github.com/Knatte18/loomyard/internal/githubclient":  true,
	"github.com/Knatte18/loomyard/internal/shedengine":    true,
	"github.com/Knatte18/loomyard/internal/shuttleengine": true,
}

// TestImportAllowlistOnly verifies that every non-test .go file imports only stdlib or an entry in shedtransientAllowedImports.
func TestImportAllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine shedtransient source directory location")
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
			return err
		}
		relPath, _ := filepath.Rel(pkgDir, path)
		for _, imp := range astFile.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			firstSegment := importPath
			if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
				firstSegment = importPath[:idx]
			}
			if !strings.Contains(firstSegment, ".") || shedtransientAllowedImports[importPath] {
				continue
			}
			failures = append(failures, relPath+": "+importPath)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk shedtransient directory: %v", err)
	}
	if len(failures) > 0 {
		t.Errorf("import seam violated; imports outside the allowlist found: %v", failures)
	}
}

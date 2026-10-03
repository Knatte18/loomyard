// leaf_enforcement_test.go enforces the gitoracle independence rule:
// production code in internal/gitrepo/internal/gitoracle imports only the standard library and internal/gitexec, never internal/gitrepo or internal/gitkit.
// The oracle is a second implementation of gitrepo's reads, so an import of gitrepo would turn every parity test into a tautology.

package gitoracle

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/gitexec": true,
}

// minScannedFiles is the vacuous-scan floor: the package's production file set is never empty.
const minScannedFiles = 1

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine gitoracle source directory location")
	}
	dir := filepath.Dir(file)

	var failures []string
	scanned := 0

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), "_test.go") || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		scanned++

		astFile, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range astFile.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			// A stdlib import path has no '.' in its first path segment.
			firstSegment, _, _ := strings.Cut(importPath, "/")
			if !strings.Contains(firstSegment, ".") || allowedImports[importPath] {
				continue
			}

			relPath, _ := filepath.Rel(dir, path)
			failures = append(failures, relPath+": "+importPath)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk gitoracle directory: %v", err)
	}

	if scanned < minScannedFiles {
		t.Fatalf("scanned %d production file(s), want at least %d: the walk may be misconfigured", scanned, minScannedFiles)
	}
	if len(failures) > 0 {
		t.Errorf("gitoracle independence violated; imports outside stdlib and internal/gitexec: %v", failures)
	}
}

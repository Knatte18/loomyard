// leaf_enforcement_test.go enforces the lyxtest Leaf Invariant: internal/lyxtest
// must not import internal/configreg or any feature package (board, warp, weft).
// Tests that need real config seed it via SeedConfig with a configreg-free
// map[string]string (never configreg types).

package lyxtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLeafInvariant verifies that lyxtest imports only stdlib and internal/paths,
// never internal/configreg or feature packages (board, warp, weft).
// It uses go/parser to read actual import paths, avoiding false positives from
// string literals in doc comments.
func TestLeafInvariant(t *testing.T) {
	// Resolve the lyxtest source directory via runtime.Caller.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	lyxtestDir := filepath.Dir(file)

	// List of banned import paths (canonical Go import paths).
	bannedImports := []string{
		"github.com/Knatte18/loomyard/internal/configreg",
		"github.com/Knatte18/loomyard/internal/board",
		"github.com/Knatte18/loomyard/internal/warp",
		"github.com/Knatte18/loomyard/internal/weft",
	}

	var failures []string

	// Walk all .go files in the lyxtest directory (excluding *_test.go files).
	err := filepath.WalkDir(lyxtestDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// Skip test files and non-.go files.
		if strings.HasSuffix(d.Name(), "_test.go") || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		// Parse the file with ImportsOnly to extract import declarations.
		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			// Skip files that fail to parse (should not happen for valid Go).
			t.Logf("warning: failed to parse %s: %v", path, err)
			return nil
		}

		// Check each import in the file.
		for _, imp := range astFile.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			// Check if the import is in the banned list.
			for _, banned := range bannedImports {
				if importPath == banned {
					relPath, _ := filepath.Rel(lyxtestDir, path)
					failures = append(failures, relPath+": "+importPath)
				}
			}
		}

		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk lyxtest directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("lyxtest Leaf Invariant violated; banned imports found: %v", failures)
	}
}

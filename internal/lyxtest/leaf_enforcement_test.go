// leaf_enforcement_test.go enforces the lyxtest Leaf Invariant: production code in internal/lyxtest
// imports ONLY the standard library and internal/configengine, internal/lyxcwd, internal/weftname,
// internal/lyxdirs — never internal/configreg or any feature package (boardengine/boardcli,
// ideengine/idecli, selfreportengine/selfreportcli, fabricengine/fabriccli).
// Tests that need real config seed it via SeedConfig with a configreg-free map[string]string (never
// configreg types).

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

// allowedImports are the only non-stdlib import paths production code in
// this package may use.
var allowedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/configengine": true,
	"github.com/Knatte18/loomyard/internal/lyxcwd":       true,
	"github.com/Knatte18/loomyard/internal/lyxdirs":      true,
	"github.com/Knatte18/loomyard/internal/weftname":     true,
}

// TestLeafInvariant_AllowlistOnly verifies that every non-test .go file in this package directory
// imports only stdlib (no '.'
// in the first path segment) or an entry in allowedImports.
// The allowlist must never be widened to admit a feature package: feature packages' own tests import
// lyxtest, so a reverse import would close a test-build cycle.
// It uses go/parser with ImportsOnly so only real import declarations are inspected, never string
// literals in doc comments.
func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	// Resolve the lyxtest source directory via runtime.Caller.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine lyxtest source directory location")
	}
	lyxtestDir := filepath.Dir(file)

	var failures []string

	// Walk all .go files in the lyxtest directory (excluding *_test.go files).
	err := filepath.WalkDir(lyxtestDir, func(path string, d fs.DirEntry, err error) error {
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

			// A stdlib import path has no '.' in its first path segment
			// (e.g. "fmt", "os", "go/parser") — a domain that would need a
			// registered TLD (e.g. "github.com/...") always contains one.
			firstSegment := importPath
			if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
				firstSegment = importPath[:idx]
			}
			isStdlib := !strings.Contains(firstSegment, ".")

			if isStdlib || allowedImports[importPath] {
				continue
			}

			relPath, _ := filepath.Rel(lyxtestDir, path)
			failures = append(failures, relPath+": "+importPath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk lyxtest directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("lyxtest Leaf Invariant violated; imports outside the allowlist (stdlib + configengine, lyxcwd, weftname, lyxdirs) found: %v", failures)
	}
}

// seam_enforcement_test.go enforces the Shared Decision "provider-seam
// import rule": internal/shuttleengine must never import
// internal/shuttleengine/claudeengine. The interface (Engine) and its value
// types live here; claudeengine imports shuttleengine and implements the
// interface, never the reverse — this is what lets a second provider engine
// be added without ever touching this package.

package shuttleengine

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestProviderSeamImportRule verifies that no non-test file in
// internal/shuttleengine imports internal/shuttleengine/claudeengine. It
// uses go/parser to read actual import paths (avoiding false positives from
// string literals in doc comments), in the style of
// internal/lyxtest/leaf_enforcement_test.go's TestLeafInvariant.
func TestProviderSeamImportRule(t *testing.T) {
	// Resolve this package's source directory via runtime.Caller so the
	// test walks the real package tree regardless of the working directory
	// go test is invoked from.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	shuttleengineDir := filepath.Dir(file)

	const bannedImport = "github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"

	var failures []string

	// Walk only this directory's own files (not subpackages like
	// claudeengine itself) so the scan matches the rule's scope: the seam
	// package, not everything beneath it.
	entries, err := os.ReadDir(shuttleengineDir)
	if err != nil {
		t.Fatalf("read shuttleengine directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), "_test.go") || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(shuttleengineDir, entry.Name())

		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Logf("warning: failed to parse %s: %v", path, err)
			continue
		}

		for _, imp := range astFile.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if importPath == bannedImport {
				failures = append(failures, entry.Name())
			}
		}
	}

	if len(failures) > 0 {
		t.Errorf("provider-seam import rule violated (Shared Decision \"provider-seam import rule\"): "+
			"internal/shuttleengine must never import internal/shuttleengine/claudeengine, but found it imported in: %v", failures)
	}
}

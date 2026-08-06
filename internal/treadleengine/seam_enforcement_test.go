// seam_enforcement_test.go enforces the Treadle Runner-Seam Invariant: production code in internal/treadleengine imports ONLY the standard library, internal/lock, internal/logger, internal/state, internal/stencil, internal/shuttleengine, and gopkg.in/yaml.v3 — never internal/burlerengine, never internal/lyxcwd as a direct import, and never any internal/*cli package.
// Like internal/modelspec's leaf_enforcement_test.go, this check is an ALLOWLIST: any import outside the allowed set fails the test, so a future stray dependency (a round-runner's own type leaking upward, a convenience lyxcwd import) is caught with no list maintenance required.

package treadleengine

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
	"github.com/Knatte18/loomyard/internal/lock":          true,
	"github.com/Knatte18/loomyard/internal/logger":        true,
	"github.com/Knatte18/loomyard/internal/state":         true,
	"github.com/Knatte18/loomyard/internal/stencil":       true,
	"github.com/Knatte18/loomyard/internal/shuttleengine": true,
	"gopkg.in/yaml.v3": true,
}

// TestRunnerSeamInvariant_AllowlistOnly verifies that every non-test .go file imports only stdlib or an entry in allowedImports.
func TestRunnerSeamInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine treadleengine source directory location")
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

			if isStdlib || allowedImports[importPath] {
				continue
			}

			relPath, _ := filepath.Rel(pkgDir, path)
			failures = append(failures, relPath+": "+importPath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk treadleengine directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("Treadle Runner-Seam Invariant violated; imports outside the allowlist (stdlib + lock + logger + state + stencil + shuttleengine + yaml.v3) found: %v", failures)
	}
}

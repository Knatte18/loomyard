// leaf_enforcement_test.go enforces the GitHub Auth Invariant's leaf half:
// production code in internal/githubclient imports ONLY the standard
// library, go-github, golang.org/x/sys, and internal/proc -- never
// internal/output, cobra, internal/gitexec, internal/gitrepo, or
// golang.org/x/oauth2. Like modelspec's and tokenvocab's
// leaf_enforcement_test.go, this check is an ALLOWLIST: any import outside
// the allowed set fails the test, so a future stray dependency is caught
// with no list maintenance required.

package githubclient

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// allowedImports are the only non-stdlib imports allowed in production code.
var allowedImports = map[string]bool{
	"github.com/google/go-github/v75/github":     true,
	"golang.org/x/sys/windows":                   true,
	"github.com/Knatte18/loomyard/internal/proc": true,
}

// TestLeafInvariant_AllowlistOnly verifies every non-test .go file imports
// only stdlib or allowedImports. Uses go/parser with ImportsOnly.
func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine githubclient source directory location")
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
		t.Fatalf("failed to walk githubclient directory: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("GitHub Auth Invariant's leaf property violated; imports outside the allowlist (stdlib + go-github + x/sys + internal/proc) found: %v", failures)
	}
}

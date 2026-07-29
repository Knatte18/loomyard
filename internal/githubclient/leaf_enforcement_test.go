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

// allowedImports are the only non-stdlib import paths production code in
// this package may use. github.com/google/go-github/v75's importable
// package is .../v75/github (not the bare module path), and the sys
// dependency used is golang.org/x/sys/windows -- both entries are the exact
// import strings the production files carry, not shorthand.
var allowedImports = map[string]bool{
	"github.com/google/go-github/v75/github":     true,
	"golang.org/x/sys/windows":                   true,
	"github.com/Knatte18/loomyard/internal/proc": true,
}

// TestLeafInvariant_AllowlistOnly verifies that every non-test .go file in
// this package directory imports only stdlib (no '.' in the first path
// segment) or an entry in allowedImports. It uses go/parser with
// ImportsOnly so only real import declarations are inspected, never string
// literals in doc comments. It walks the whole directory rather than a
// single file so the allowlist covers the union of both cache_windows.go's
// and cache_other.go's import sets, regardless of which one the local GOOS
// happens to compile.
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

			// A stdlib import path has no '.' in its first path segment
			// (e.g. "fmt", "os", "go/parser") -- a domain that would need a
			// registered TLD (e.g. "github.com/...") always contains one.
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

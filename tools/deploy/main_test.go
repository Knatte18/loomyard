// main_test.go covers resolveDest, the destination-directory resolution logic for the -dev / -dest
// flags, plus a source-level guard that the -ldflags -X path in run() still names a variable that
// actually exists at internal/buildinfo.
// Tests are Go-native and Tier-1 pure: no go build / go env spawns, so the goBinDir() fallback
// (which shells out to `go env`) is intentionally left uncovered here.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/tools/internal/devbin"
)

// TestResolveDest_DevAndDestMutuallyExclusive verifies -dev and -dest are mutually exclusive.
func TestResolveDest_DevAndDestMutuallyExclusive(t *testing.T) {
	_, err := resolveDest(true, "/x")
	if err == nil {
		t.Error("resolveDest(true, \"/x\") = nil error; want mutual-exclusion error")
	}
}

// TestResolveDest_DevUsesDerivedDevBinDir verifies -dev resolves to devbin.Dir().
func TestResolveDest_DevUsesDerivedDevBinDir(t *testing.T) {
	want, err := devbin.Dir()
	if err != nil {
		t.Fatalf("devbin.Dir() error: %v", err)
	}

	got, err := resolveDest(true, "")
	if err != nil {
		t.Fatalf("resolveDest(true, \"\") error: %v", err)
	}
	if got != want {
		t.Errorf("resolveDest(true, \"\") = %q; want %q", got, want)
	}
}

// TestResolveDest_DestPassedThrough verifies non-empty -dest is returned verbatim.
func TestResolveDest_DestPassedThrough(t *testing.T) {
	want := "/some/dir"

	got, err := resolveDest(false, want)
	if err != nil {
		t.Fatalf("resolveDest(false, %q) error: %v", want, err)
	}
	if got != want {
		t.Errorf("resolveDest(false, %q) = %q; want %q", want, got, want)
	}
}

// repoRoot locates the module root relative to this test file's own source location: tools/deploy
// is two levels below the root, so three filepath.Dir calls from runtime.Caller(0)'s file (which
// removes the filename itself, then "deploy", then "tools") land on the root.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

// TestLdflagsPath_MatchesBuildinfoChannel guards the -X argument tools/deploy/main.go's run() passes
// to the linker against silent drift: Go's linker does not error on an unmatched -X, so a rename of
// either the variable or its package leaves a -dev build behaving as production, with no build
// error, no test failure, and no visible symptom until someone notices stencils refreshing when
// they should not.
func TestLdflagsPath_MatchesBuildinfoChannel(t *testing.T) {
	root := repoRoot(t)

	const wantLdflag = "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev"

	mainSrc, err := os.ReadFile(filepath.Join(root, "tools", "deploy", "main.go"))
	if err != nil {
		t.Fatalf("read tools/deploy/main.go: %v", err)
	}
	if !strings.Contains(string(mainSrc), wantLdflag) {
		t.Errorf("tools/deploy/main.go does not contain %q; the linker silently ignores an "+
			"unmatched -X, so a rename of internal/buildinfo.Channel or its package would leave "+
			"a -dev build behaving as production with no build error, no test failure, and no "+
			"visible symptom until someone notices stencils refreshing when they should not", wantLdflag)
	}

	buildinfoPath := filepath.Join(root, "internal", "buildinfo", "buildinfo.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, buildinfoPath, nil, 0)
	if err != nil {
		t.Fatalf("parse internal/buildinfo/buildinfo.go: %v", err)
	}

	if !declaresExportedVar(file, "Channel") {
		t.Errorf("internal/buildinfo/buildinfo.go declares no exported var Channel; the linker "+
			"silently ignores an unmatched -X, so if this symbol is renamed or moved, tools/deploy's "+
			"%q stamp would stop matching and a -dev build would behave as production with no build "+
			"error, no test failure, and no visible symptom until someone notices stencils "+
			"refreshing when they should not", wantLdflag)
	}
}

// declaresExportedVar reports whether file declares a package-level exported var named name.
func declaresExportedVar(file *ast.File, name string) bool {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, ident := range valueSpec.Names {
				if ident.Name == name && ident.IsExported() {
					return true
				}
			}
		}
	}
	return false
}

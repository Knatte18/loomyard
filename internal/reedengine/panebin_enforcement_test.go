// panebin_enforcement_test.go enforces the CONSTRAINTS.md Pane Binary Resolution clause's two-sided
// guarantee: no pane-creation site exists outside the named allowlist, and the one allowlisted
// chokepoint still composes the prelude rather than silently becoming a plain pass-through. It is
// modelled directly on selvagepane_enforcement_test.go: runtime.Caller(0) resolves this file's
// directory, then the repository root, then go/parser walks every non-_test.go .go file directly
// inside internal/reedengine -- never internal/reedengine/render/, and never a sibling package. It
// spawns no process, so it carries no build tag.
//
// Honest residual, recorded here as this package's other enforcement test records its own: the scan
// is a tripwire over string literals in this one package, so a pane created by a helper in another
// package, or by a "split-window" value assembled from fragments or held in a variable defined
// elsewhere, is not seen. It narrows the gap the invariant exists to close rather than closing it.

package reedengine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// panebinScanMinFiles is the plausible floor for how many non-test .go files
// internal/reedengine holds. Guards against a misconfigured directory read reporting as a vacuous
// pass instead of a failure, the way tools/sandbox/pathresolve_guard_test.go's own floor does.
const panebinScanMinFiles = 10

// paneCreationAllowlist names the files in this package permitted to contain the "split-window"
// string literal, each with a comment recording why.
var paneCreationAllowlist = map[string]bool{
	// spawn.go is the chokepoint itself: the one strand-pane split, which composes the pane-binary
	// prelude via composePaneLaunchLine before every send-keys.
	"spawn.go": true,
	// selvagepane.go is Selvage's own split, exempt by name per the Pane Binary Resolution clause:
	// Selvage is a one-row status band with no operator input and no lyx invocation.
	"selvagepane.go": true,
	// probe.go lists "split-window" as a required subcommand NAME in requiredSubcommands -- it
	// issues no split of its own.
	"probe.go": true,
}

// TestPaneCreationSitesRouteThroughThePreludeChokepoint walks each parsed file for an *ast.BasicLit
// whose value is the string "split-window" and fails for any occurrence in a file outside
// paneCreationAllowlist. Scanning the AST rather than raw bytes is what keeps a doc comment
// mentioning split-window from tripping the check -- go/parser turns a comment into neither an
// identifier nor a basic literal.
func TestPaneCreationSitesRouteThroughThePreludeChokepoint(t *testing.T) {
	scanDir := panebinScanDir(t)

	entries, err := os.ReadDir(scanDir)
	if err != nil {
		t.Fatalf("read %s: %v", scanDir, err)
	}

	var scanned int
	var failures []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++

		path := filepath.Join(scanDir, name)
		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		if !fileContainsSplitWindowLiteral(astFile) {
			continue
		}
		if paneCreationAllowlist[name] {
			continue
		}
		failures = append(failures, name)
	}

	if scanned < panebinScanMinFiles {
		t.Fatalf("panebin enforcement: only scanned %d non-test .go file(s) in %s; expected at least %d -- the directory read may be misconfigured", scanned, scanDir, panebinScanMinFiles)
	}

	if len(failures) > 0 {
		t.Errorf("pane-creation chokepoint violated (see CONSTRAINTS.md's Pane Binary Resolution clause): %v carries a \"split-window\" literal outside the named allowlist %v -- route the split through launchStrandLocked instead", failures, paneCreationAllowlist)
	}
}

// TestLaunchStrandLockedStillComposesThePrelude parses spawn.go, locates the launchStrandLocked
// function declaration, and fails unless its body contains a call to composePaneLaunchLine. This is
// what keeps the allowlisted chokepoint from silently becoming a plain pass-through: the previous test
// proves no second pane-creation site exists, and this one proves the single site still composes the
// prelude.
func TestLaunchStrandLockedStillComposesThePrelude(t *testing.T) {
	scanDir := panebinScanDir(t)
	path := filepath.Join(scanDir, "spawn.go")

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var fn *ast.FuncDecl
	for _, decl := range astFile.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "launchStrandLocked" {
			continue
		}
		fn = fd
		break
	}
	if fn == nil {
		t.Fatal("spawn.go: launchStrandLocked function declaration not found")
	}

	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if ok && ident.Name == "composePaneLaunchLine" {
			found = true
		}
		return true
	})
	if !found {
		t.Error("launchStrandLocked's body contains no call to composePaneLaunchLine -- the chokepoint no longer composes the pane-binary prelude")
	}
}

// fileContainsSplitWindowLiteral reports whether astFile contains an *ast.BasicLit whose value is
// the quoted string "split-window".
func fileContainsSplitWindowLiteral(astFile *ast.File) bool {
	found := false
	ast.Inspect(astFile, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if lit.Value == `"split-window"` {
			found = true
		}
		return true
	})
	return found
}

// panebinScanDir resolves the internal/reedengine source directory from this test file's own
// location, exactly as selvagepane_enforcement_test.go does.
func panebinScanDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine reedengine source directory location")
	}
	reedengineDir := filepath.Dir(thisFile)
	repoRoot := filepath.Dir(filepath.Dir(reedengineDir)) // internal/reedengine -> internal -> repo root
	return filepath.Join(repoRoot, "internal", "reedengine")
}

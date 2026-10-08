// panebin_enforcement_test.go enforces PATTERN-pane-binary-resolution's two-sided
// guarantee: no pane-creation site exists outside the named allowlist, and the one allowlisted
// chokepoint still composes the prelude rather than silently becoming a plain pass-through.
// It is modelled directly on selvagepane_enforcement_test.go: scankit walks every non-_test.go .go file directly inside internal/reedengine -- never internal/reedengine/render/, and never a sibling package.
// It spawns no process, so it carries no build tag.
//
// Honest residual, recorded here as this package's other enforcement test records its own: the scan
// is a tripwire over string literals in this one package, so a pane created by a helper in another
// package, or by a "split-window" value assembled from fragments or held in a variable defined
// elsewhere, is not seen. It narrows the gap the invariant exists to close rather than closing it.

package reedengine

import (
	"go/ast"
	"go/token"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// panebinScanMinFiles is the plausible floor for how many non-test .go files
// internal/reedengine holds. Guards against a misconfigured directory read reporting as a vacuous
// pass instead of a failure, the way tools/sandbox/pathresolve_guard_test.go's own floor does.
const panebinScanMinFiles = 10

// reedengineScanDir is the module-relative package directory both reedengine enforcement scans walk.
const reedengineScanDir = "internal/reedengine"

// paneCreationAllowlist names the files in this package permitted to contain the "split-window" string literal, each with the reason.
var paneCreationAllowlist = []scankit.Entry{
	{
		Key: "internal/reedengine/spawn.go",
		Why: "the chokepoint itself: the one strand-pane split, which composes the pane-binary prelude via composePaneLaunchLine before every send-keys",
	},
	{
		Key: "internal/reedengine/selvagepane.go",
		Why: "Selvage's own split, exempt by name per the Pane Binary Resolution clause: Selvage is a one-row status band with no operator input and no lyx invocation",
	},
	{
		Key: "internal/reedengine/probe.go",
		Why: "lists \"split-window\" as a required subcommand NAME in requiredSubcommands; it issues no split of its own",
	},
}

// windowCreationAllowlist names the files in this package permitted to contain the "new-window" string literal, each with the reason.
var windowCreationAllowlist = []scankit.Entry{
	{
		Key: "internal/reedengine/window.go",
		Why: "the one detached-window op: it runs a `lyx` command composed by composeWindowCommand, which carries the pane-binary prelude",
	},
	{
		Key: "internal/reedengine/probe.go",
		Why: "lists \"new-window\" as a required subcommand NAME in requiredSubcommands; it issues no window of its own",
	},
}

// TestPaneCreationSitesRouteThroughThePreludeChokepoint walks each parsed file for an *ast.BasicLit whose value is the string "split-window" or "new-window", and fails for any occurrence in a file outside its allowlist (paneCreationAllowlist, windowCreationAllowlist).
// Scanning the AST rather than raw bytes is what keeps a doc comment mentioning split-window from tripping the check, since go/parser turns a comment into neither an identifier nor a basic literal.
//
//testtiming:keep pins that no file outside the pane-creation allowlist holds the "split-window" string literal, so every strand pane is created through the prelude chokepoint; its covering tests run this code without asserting it
func TestPaneCreationSitesRouteThroughThePreludeChokepoint(t *testing.T) {
	scans := []struct {
		literal   string
		allowlist []scankit.Entry
		advice    string
	}{
		{`"split-window"`, paneCreationAllowlist, "route the split through launchStrandLocked instead"},
		{`"new-window"`, windowCreationAllowlist, "route the window through OpenWindow instead"},
	}
	for _, scan := range scans {
		allow := scankit.NewAllowlist(scan.allowlist)

		var failures []string
		scanned := scankit.Walk(t, scankit.Options{Roots: []string{reedengineScanDir}, Shallow: true}, func(f *scankit.File) {
			if !fileContainsStringLiteral(f.AST(t, 0), scan.literal) {
				return
			}
			if allow.Allowed(f.Rel) {
				return
			}
			failures = append(failures, f.Rel)
		})

		scankit.RequireFloor(t, scanned, panebinScanMinFiles, "panebin enforcement")
		allow.RequireNoStale(t)

		if len(failures) > 0 {
			t.Errorf("pane-creation chokepoint violated (see PATTERN-pane-binary-resolution): %v carries a %s literal outside the named allowlist -- %s", failures, scan.literal, scan.advice)
		}
	}
}

// TestLaunchStrandLockedStillComposesThePrelude parses spawn.go, locates the launchStrandLocked
// function declaration, and fails unless its body contains a call to composePaneLaunchLine. This is
// what keeps the allowlisted chokepoint from silently becoming a plain pass-through: the previous test
// proves no second pane-creation site exists, and this one proves the single site still composes the
// prelude.
//
//testtiming:keep pins launchStrandLocked still calling composePaneLaunchLine, so the allowlisted chokepoint cannot become a plain pass-through; its covering tests run this code without asserting it
func TestLaunchStrandLockedStillComposesThePrelude(t *testing.T) {
	var fn *ast.FuncDecl
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{reedengineScanDir}, Shallow: true}, func(f *scankit.File) {
		if f.Rel != reedengineScanDir+"/spawn.go" {
			return
		}
		for _, decl := range f.AST(t, 0).Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Name.Name != "launchStrandLocked" {
				continue
			}
			fn = fd
			break
		}
	})
	scankit.RequireFloor(t, scanned, panebinScanMinFiles, "launchStrandLocked scan")
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

// fileContainsStringLiteral reports whether astFile contains an *ast.BasicLit whose value is quoted, the string literal as written in source.
func fileContainsStringLiteral(astFile *ast.File, quoted string) bool {
	found := false
	ast.Inspect(astFile, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if lit.Value == quoted {
			found = true
		}
		return true
	})
	return found
}

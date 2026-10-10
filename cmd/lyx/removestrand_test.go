// removestrand_test.go enforces `PATTERN-shuttle-stop`: a strand is removed from Go only inside the packages that own the removal.
// The scan matches the selector, so a method value handed to a wrapper is caught as well as a call.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// removeStrandAllowlist names the package directories permitted to select RemoveStrand, each with its reason.
// `internal/reedengine` and `internal/testkit/shuttlefake` declare RemoveStrand and select it nowhere, so they carry no entry: a stale entry fails the scan.
var removeStrandAllowlist = []scankit.Entry{
	{Key: "internal/reedcli/", Why: "reed's own remove verb"},
	{Key: "internal/shuttleengine/", Why: "the stop verb settles the run's record before it removes the strand"},
	{Key: "internal/loomcli/", Why: "removes the loom driver's strand, which no attach probe or Wait reads"},
	{Key: "internal/orchcli/", Why: "removes the orch strand, which no attach probe or Wait reads"},
}

// removeStrandSelectors returns "file:line" for every selector expression selecting RemoveStrand in a non-test Go file under base's internal and cmd, skipping the files allow admits.
func removeStrandSelectors(t *testing.T, base string, allow *scankit.Allowlist) []string {
	t.Helper()
	var found []string
	scankit.Walk(t, scankit.Options{Base: base, Roots: []string{"internal", "cmd"}}, func(f *scankit.File) {
		tree := f.AST(t, parser.SkipObjectResolution)
		ast.Inspect(tree, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "RemoveStrand" {
				return true
			}
			if !allow.Allowed(f.Rel) {
				found = append(found, fmt.Sprintf("%s:%d", f.Rel, f.FileSet().Position(sel.Sel.Pos()).Line))
			}
			return true
		})
	})
	return found
}

// TestShuttleStop_RemoveStrandOnlyInAllowedPackages fails when a package outside removeStrandAllowlist selects RemoveStrand.
// The scan sees the selector, not whose strand it removes.
//
//lyx:guard
func TestShuttleStop_RemoveStrandOnlyInAllowedPackages(t *testing.T) {
	t.Parallel()

	t.Run("fixture tree", func(t *testing.T) {
		t.Parallel()
		base := t.TempDir()
		for rel, src := range map[string]string{
			"internal/rogue/call.go":  "package rogue\n\nfunc stop(r interface{ RemoveStrand() }) { r.RemoveStrand() }\n",
			"internal/rogue/value.go": "package rogue\n\nfunc wrap(r interface{ RemoveStrand() }) { run(r.RemoveStrand) }\n\nfunc run(func()) {}\n",
			"internal/orchcli/ok.go":  "package orchcli\n\nfunc stop(r interface{ RemoveStrand() }) { r.RemoveStrand() }\n",
		} {
			path := filepath.Join(base, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(filepath.Join(base, "cmd"), 0o755); err != nil {
			t.Fatal(err)
		}

		got := removeStrandSelectors(t, base, scankit.NewAllowlist(removeStrandAllowlist))
		want := []string{"internal/rogue/call.go:3", "internal/rogue/value.go:3"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("selectors = %v; want %v", got, want)
		}
	})

	t.Run("module tree", func(t *testing.T) {
		t.Parallel()
		allow := scankit.NewAllowlist(removeStrandAllowlist)
		found := removeStrandSelectors(t, scankit.Root(t), allow)
		allow.RequireNoStale(t)
		if len(found) > 0 {
			t.Errorf("`PATTERN-shuttle-stop` violated: RemoveStrand is selected outside the allowlist; stop the strand through shuttle's stop verb:\n%s", strings.Join(found, "\n"))
		}
	})
}

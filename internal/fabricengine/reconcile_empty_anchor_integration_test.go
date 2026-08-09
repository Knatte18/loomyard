//go:build integration

// reconcile_empty_anchor_integration_test.go pins Reconcile's refusal to wire against a
// present-but-empty lyx-anchor marker.
// lyxcwd treats an empty marker as absent, so Resolve falls back to the "." anchor and succeeds at
// the warp worktree root; on a subpath-anchored hub whose marker was truncated, reconcile then wired
// a full SECOND junction set at the repo root beside the live set at the real anchor — the exact
// damage the stale-marker guard exists to prevent for the pre-rename spelling.
//
// Package fabricengine_test to reuse newFabricFixture from
// reconcile_stale_registration_test.go; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestReconcile_RefusesEmptyAnchorMarkerInsteadOfWiringAtTheRoot anchors a hub at a subpath, wires
// it, truncates the marker, and asserts a root-resolved reconcile refuses and leaves the warp
// worktree root free of junctions.
func TestReconcile_RefusesEmptyAnchorMarkerInsteadOfWiringAtTheRoot(t *testing.T) {
	t.Parallel()

	const anchor = "sub"

	fixture := newFabricFixture(t)
	l := fixture.Layout

	subDir := filepath.Join(l.WorktreePath(), anchor)
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", subDir, err)
	}
	markerPath := filepath.Join(fabricengine.BoardDir(l.HubPath), lyxcwd.AnchorFileName)
	if err := os.WriteFile(markerPath, []byte(anchor+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", markerPath, err)
	}

	anchoredLayout, err := lyxcwd.Resolve(subDir)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", subDir, err)
	}
	names, err := fabricengine.RepoWiredNames(anchoredLayout)
	if err != nil {
		t.Fatalf("RepoWiredNames: %v", err)
	}
	slug := filepath.Base(anchoredLayout.WorktreePath())
	if err := fabricengine.WireJunctions(anchoredLayout, slug, names); err != nil {
		t.Fatalf("WireJunctions at the anchor: %v", err)
	}

	// Truncate the marker: Resolve now answers "." and succeeds at the worktree root.
	if err := os.WriteFile(markerPath, []byte("   \n"), 0o644); err != nil {
		t.Fatalf("truncate %s: %v", markerPath, err)
	}
	rootLayout, err := lyxcwd.Resolve(l.WorktreePath())
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s) after truncation: %v", l.WorktreePath(), err)
	}
	if rootLayout.AnchorRel != "." {
		t.Fatalf("AnchorRel after truncation = %q; want %q (the empty-is-absent fallback must stay)", rootLayout.AnchorRel, ".")
	}

	topology := fabricengine.NewTopology(fabricengine.Config{})
	_, reconcileErr := topology.Reconcile(rootLayout)
	if reconcileErr == nil {
		t.Fatal("Reconcile() = nil error against an empty anchor marker; want a refusal")
	}
	if !strings.Contains(reconcileErr.Error(), lyxcwd.AnchorFileName) {
		t.Errorf("Reconcile() error = %v; want it to name %s", reconcileErr, lyxcwd.AnchorFileName)
	}

	for _, name := range []string{lyxdirs.LyxDirName, lyxdirs.DotLyxDirName, fabricengine.BoardDirName} {
		rootLink := filepath.Join(l.WorktreePath(), name)
		if _, statErr := os.Lstat(rootLink); statErr == nil {
			t.Errorf("a second junction %s was wired at the warp worktree root; want none", rootLink)
		}
	}

	// The real junction set at the anchor is untouched.
	anchoredLink := filepath.Join(subDir, lyxdirs.LyxDirName)
	if _, statErr := os.Lstat(anchoredLink); statErr != nil {
		t.Errorf("anchored junction %s went missing: %v", anchoredLink, statErr)
	}
}

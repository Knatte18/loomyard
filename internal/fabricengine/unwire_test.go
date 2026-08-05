//go:build integration

// unwire_test.go ports initengine/undo_test.go's coverage to
// fabricengine.Unwire, the new per-worktree teardown verb: full on-disk
// junction removal (including a stale junction absent from the current
// pathspec, proving on-disk-scan enumeration rather than a config
// name-set), the deliberate _lyx-only / never-_pattern weft-clear
// asymmetry, the .gitignore revert, idempotency on a never-wired host, and
// preservation of the repo-wide weft:main records for a later reconcile
// re-wire.
//
// Package fabricengine_test to reuse newFabricFixture/seedRepoWideFabricConfig
// from reconcile_stale_registration_test.go; shares the single TestMain in
// testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitignore"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestUnwire_RemovesOnDiskJunctionsIncludingStale proves the on-disk-scan
// enumeration property: Unwire removes every fabric junction present on
// disk for the worktree, including a junction (`_extra`) absent from the
// repo-wide pathspec that a config-driven name-set would have left behind.
func TestUnwire_RemovesOnDiskJunctionsIncludingStale(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "unwire-removes-stale"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	hostLayout, err := hubgeometry.Resolve(l.WorktreePath(slug))
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(host): %v", err)
	}

	// Wire the desired pair (_lyx, _pattern) plus a stale name (_extra) that
	// is present on disk but absent from any pathspec — Unwire's on-disk
	// scan must remove it too, unlike a config-name-set-driven teardown.
	if err := fabricengine.WireJunctions(hostLayout, slug, []string{"_lyx", "_pattern", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	res, err := fabricengine.Unwire(hostLayout.WorktreeRoot)
	if err != nil {
		t.Fatalf("Unwire() = %v; want nil", err)
	}

	got := slices.Clone(res.JunctionsRemoved)
	sort.Strings(got)
	want := []string{"_extra", configengine.LyxDirName, hubgeometry.PatternDirName}
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("res.JunctionsRemoved (sorted) = %v; want %v", got, want)
	}

	for _, name := range []string{"_extra", configengine.LyxDirName, hubgeometry.PatternDirName} {
		link := filepath.Join(hostLayout.WorktreeRoot, name)
		if _, statErr := os.Lstat(link); !os.IsNotExist(statErr) {
			t.Errorf("junction %s still exists after Unwire (stat err: %v)", link, statErr)
		}
	}
}

// TestUnwire_ClearsWeftLyxOnlyNeverPattern verifies the deliberate asymmetry
// Unwire ports from the deleted initengine.Undo: weft _lyx content is
// cleared (WeftContent == "cleared"), while weft _pattern content survives
// on disk untouched.
func TestUnwire_ClearsWeftLyxOnlyNeverPattern(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "unwire-clears-lyx-only"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	hostLayout, err := hubgeometry.Resolve(l.WorktreePath(slug))
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(host): %v", err)
	}
	if err := fabricengine.WireJunctions(hostLayout, slug, []string{"_lyx", "_pattern"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Seed real content on the weft side of both junctions: WireJunctions
	// only materializes the target directories, it writes no files.
	weftLyxDir := hostLayout.WeftLyxDirFor(slug)
	if err := os.WriteFile(filepath.Join(weftLyxDir, "marker.txt"), []byte("lyx state"), 0o644); err != nil {
		t.Fatalf("seed weft _lyx content: %v", err)
	}
	weftPatternDir := hostLayout.WeftPatternDirFor(slug)
	patternFile := filepath.Join(weftPatternDir, "PATTERN.md")
	if err := os.WriteFile(patternFile, []byte("# constraints\n"), 0o644); err != nil {
		t.Fatalf("seed weft _pattern content: %v", err)
	}

	res, err := fabricengine.Unwire(hostLayout.WorktreeRoot)
	if err != nil {
		t.Fatalf("Unwire() = %v; want nil", err)
	}

	if res.WeftContent != "cleared" {
		t.Errorf("res.WeftContent = %q; want %q", res.WeftContent, "cleared")
	}
	if _, statErr := os.Stat(weftLyxDir); !os.IsNotExist(statErr) {
		t.Errorf("weft _lyx dir %s still exists after Unwire (stat err: %v)", weftLyxDir, statErr)
	}

	// _pattern content is deliberately never touched by Unwire.
	content, err := os.ReadFile(patternFile)
	if err != nil {
		t.Fatalf("read PATTERN.md after Unwire: %v", err)
	}
	if string(content) != "# constraints\n" {
		t.Errorf("PATTERN.md content changed after Unwire: %q", string(content))
	}
}

// TestUnwire_RevertsGitignore verifies Unwire reverts the managed .gitignore
// block's ".lyx/" entry.
func TestUnwire_RevertsGitignore(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "unwire-reverts-gitignore"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	hostLayout, err := hubgeometry.Resolve(l.WorktreePath(slug))
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(host): %v", err)
	}
	if err := fabricengine.WireJunctions(hostLayout, slug, []string{"_lyx"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}
	if _, err := gitignore.Ensure(hostLayout.WorktreeRoot, ".lyx/"); err != nil {
		t.Fatalf("seed .gitignore block: %v", err)
	}

	res, err := fabricengine.Unwire(hostLayout.WorktreeRoot)
	if err != nil {
		t.Fatalf("Unwire() = %v; want nil", err)
	}

	if res.Gitignore != "reverted" {
		t.Errorf("res.Gitignore = %q; want %q", res.Gitignore, "reverted")
	}
}

// TestUnwire_NeverWiredHostIsIdempotentNoOp verifies that Unwire is a clean,
// error-free no-op on a host worktree that was never fabric-paired at all —
// no weft sibling, no junctions — mirroring initengine.Undo's
// TestUndo_NoWeftPairing coverage.
func TestUnwire_NeverWiredHostIsIdempotentNoOp(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	host := lyxtest.CopyHostHub(t)

	res, err := fabricengine.Unwire(host.Hub)
	if err != nil {
		t.Fatalf("Unwire() = %v; want nil", err)
	}
	if res.WeftContent != "not_present" {
		t.Errorf("res.WeftContent = %q; want %q", res.WeftContent, "not_present")
	}
	if len(res.JunctionsRemoved) != 0 {
		t.Errorf("res.JunctionsRemoved = %v; want empty", res.JunctionsRemoved)
	}

	// Running it again is a clean, identical no-op.
	res2, err := fabricengine.Unwire(host.Hub)
	if err != nil {
		t.Fatalf("second Unwire() = %v; want nil", err)
	}
	if res2.WeftContent != "not_present" {
		t.Errorf("second res.WeftContent = %q; want %q", res2.WeftContent, "not_present")
	}
}

// TestUnwire_PreservesRepoWideRecords proves Unwire's per-worktree scope: the
// repo-wide weft:main records (.fabric-anchor, <BoardDir>/_lyx/config/fabric.yaml)
// survive a worktree's Unwire untouched, so a later `lyx fabric reconcile`
// can still re-wire it.
func TestUnwire_PreservesRepoWideRecords(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")

	const slug = "unwire-preserves-repo-wide-records"
	fixture := newFabricFixture(t)
	l := fixture.Layout

	// Record the anchor marker alongside the repo-wide fabric.yaml
	// newFabricFixture already seeded, mirroring what fabric clone commits
	// onto weft:main.
	boardDir := hubgeometry.BoardDir(l.Hub)
	anchorPath := filepath.Join(boardDir, hubgeometry.FabricAnchorName)
	if err := os.WriteFile(anchorPath, []byte(".\n"), 0o644); err != nil {
		t.Fatalf("seed .fabric-anchor: %v", err)
	}
	fabricConfigPath := configengine.ConfigFile(boardDir, "fabric")

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	hostLayout, err := hubgeometry.Resolve(l.WorktreePath(slug))
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(host): %v", err)
	}
	if err := fabricengine.WireJunctions(hostLayout, slug, []string{"_lyx", "_pattern"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	if _, err := fabricengine.Unwire(hostLayout.WorktreeRoot); err != nil {
		t.Fatalf("Unwire() = %v; want nil", err)
	}

	if _, statErr := os.Stat(anchorPath); statErr != nil {
		t.Errorf(".fabric-anchor missing after Unwire: %v", statErr)
	}
	if _, statErr := os.Stat(fabricConfigPath); statErr != nil {
		t.Errorf("repo-wide fabric.yaml missing after Unwire: %v", statErr)
	}
	hostLyxLink := filepath.Join(hostLayout.WorktreeRoot, configengine.LyxDirName)
	if _, statErr := os.Lstat(hostLyxLink); !os.IsNotExist(statErr) {
		t.Errorf("host _lyx junction %s still exists after Unwire (stat err: %v)", hostLyxLink, statErr)
	}
}

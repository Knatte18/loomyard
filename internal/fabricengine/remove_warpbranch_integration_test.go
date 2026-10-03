//go:build integration

// remove_warpbranch_integration_test.go covers Remove's local warp-branch deletion: a pair's task
// branch goes when the destructive gate proves its work is pushed or landed on the parent recorded in
// the pair's origin record, and is kept with a reason otherwise.
//
// Every hub is built through hubforge.NewHub (via newFabricFixture) with an empty branch_prefix, so
// the pair's warp branch is the bare slug.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// warpBranchCommit commits a new file in the pair's warp worktree.
func warpBranchCommit(t *testing.T, l *lyxcwd.Location, slug, name string) {
	t.Helper()
	dir := fabricengine.WorktreePath(l, slug)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	gitkit.MustRun(t, dir, "git", "add", name)
	gitkit.MustRun(t, dir, "git", "commit", "-m", name)
}

func TestRemove_PushedWarpBranchIsDeleted(t *testing.T) {
	t.Parallel()

	const slug = "wb-pushed"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("setup Add(%q): %v", slug, err)
	}
	warpBranchCommit(t, l, slug, "work.txt")
	gitkit.MustRun(t, fabricengine.WorktreePath(l, slug), "git", "push", "origin", slug)

	res, err := topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove(%q) error = %v", slug, err)
	}
	if !res.WarpBranchDeleted || res.WarpBranchKeptReason != "" {
		t.Errorf("WarpBranchDeleted = %v, kept reason = %q; want deleted", res.WarpBranchDeleted, res.WarpBranchKeptReason)
	}
	if gitkit.BranchExists(t, l.WorktreePath(), slug) {
		t.Errorf("warp branch %q still exists after Remove", slug)
	}
}

func TestRemove_SquashLandedWarpBranchIsDeleted(t *testing.T) {
	t.Parallel()

	const slug = "wb-landed"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("setup Add(%q): %v", slug, err)
	}
	warpBranchCommit(t, l, slug, "work.txt")

	prime := l.WorktreePath()
	gitkit.MustRun(t, prime, "git", "merge", "--squash", slug)
	gitkit.MustRun(t, prime, "git", "commit", "-m", "land "+slug)

	res, err := topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove(%q) error = %v", slug, err)
	}
	if !res.WarpBranchDeleted || res.WarpBranchKeptReason != "" {
		t.Errorf("WarpBranchDeleted = %v, kept reason = %q; want deleted", res.WarpBranchDeleted, res.WarpBranchKeptReason)
	}
	if gitkit.BranchExists(t, prime, slug) {
		t.Errorf("warp branch %q still exists after Remove", slug)
	}
}

func TestRemove_UnlandedWarpBranchIsKept(t *testing.T) {
	t.Parallel()

	for _, force := range []bool{false, true} {
		name := "no-force"
		if force {
			name = "force"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			slug := "wb-unlanded-" + name
			fixture := newFabricFixture(t)
			l := fixture.Layout
			topology := fabricengine.NewTopology(fabricengine.Config{})
			if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
				t.Fatalf("setup Add(%q): %v", slug, err)
			}
			warpBranchCommit(t, l, slug, "work.txt")

			res, err := topology.Remove(l, slug, force, false)
			if err != nil {
				t.Fatalf("Remove(%q, force=%v) error = %v; a kept branch is not a failure", slug, force, err)
			}
			if res.WarpBranchDeleted {
				t.Errorf("WarpBranchDeleted = true; want the unlanded branch kept")
			}
			if res.WarpBranchKeptReason == "" {
				t.Errorf("WarpBranchKeptReason is empty; want the gate's reason")
			}
			if !gitkit.BranchExists(t, l.WorktreePath(), slug) {
				t.Errorf("warp branch %q was deleted despite unlanded work", slug)
			}
		})
	}
}

func TestRemove_MissingWeftWorktreeRecreatesNothing(t *testing.T) {
	t.Parallel()

	const slug = "wb-noweft"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("setup Add(%q): %v", slug, err)
	}

	weftPath := fabricengine.WeftWorktreePath(l, slug)
	if err := os.RemoveAll(weftPath); err != nil {
		t.Fatalf("remove weft worktree: %v", err)
	}
	if _, found, err := fabricengine.ReadOriginFor(l, slug); err != nil || found {
		t.Fatalf("ReadOriginFor with no weft worktree = (found=%v, err=%v); want (false, nil)", found, err)
	}
	if _, err := os.Stat(weftPath); !os.IsNotExist(err) {
		t.Fatalf("ReadOriginFor recreated %s (stat err = %v)", weftPath, err)
	}

	if _, err := topology.Remove(l, slug, false, false); err != nil {
		t.Fatalf("Remove(%q) error = %v", slug, err)
	}
	if _, err := os.Stat(weftPath); !os.IsNotExist(err) {
		t.Errorf("Remove left a stray path at %s (stat err = %v)", weftPath, err)
	}
}

func TestRemove_AddSucceedsAfterWarpBranchDeleted(t *testing.T) {
	t.Parallel()

	const slug = "wb-readd"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("setup Add(%q): %v", slug, err)
	}

	res, err := topology.Remove(l, slug, false, false)
	if err != nil {
		t.Fatalf("Remove(%q) error = %v", slug, err)
	}
	if !res.WarpBranchDeleted {
		t.Fatalf("WarpBranchDeleted = false (kept reason %q); a workless branch should be deleted", res.WarpBranchKeptReason)
	}
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("second Add(%q) after a deleting Remove: %v", slug, err)
	}
}

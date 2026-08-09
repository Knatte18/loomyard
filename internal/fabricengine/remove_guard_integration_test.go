//go:build integration

// remove_guard_integration_test.go pins the two guards that keep `lyx fabric remove` from deleting a
// directory git itself declined to remove.
// Before them, `git worktree remove` refusing a main working tree (or any path that is not a linked
// worktree of the repo) fell through to an unconditional os.RemoveAll, so removing the prime slug
// deleted the whole warp clone — gitdir included — on a clean hub with no --force.
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
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestRemove_RefusesPrimeWorktreeAndLeavesItIntact asserts Remove refuses the hub's prime slug and
// that the warp clone — worktree contents and gitdir alike — survives the refusal untouched.
func TestRemove_RefusesPrimeWorktreeAndLeavesItIntact(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	primeSlug := filepath.Base(l.WorktreePath())
	topology := fabricengine.NewTopology(fabricengine.Config{})

	_, err := topology.Remove(l, primeSlug, false)
	if err == nil {
		t.Fatalf("Remove(%q) = nil error; want a refusal naming the prime worktree", primeSlug)
	}
	if !strings.Contains(err.Error(), "prime worktree") {
		t.Errorf("Remove(%q) error = %v; want it to name the prime worktree", primeSlug, err)
	}

	if _, statErr := os.Stat(l.WorktreePath()); statErr != nil {
		t.Fatalf("prime warp worktree %s was destroyed by a refused Remove: %v", l.WorktreePath(), statErr)
	}
	gitPath := filepath.Join(l.WorktreePath(), ".git")
	if _, statErr := os.Stat(gitPath); statErr != nil {
		t.Fatalf("prime warp gitdir %s was destroyed by a refused Remove: %v", gitPath, statErr)
	}
}

// TestRemove_RefusesForeignWorktreeWithoutDeletingIt drives the other half of the same data-loss
// path: a hub directory that is a clean git worktree of the WEFT repo, not of warp.
// `git worktree remove` run against the warp repo refuses it ("not a working tree"), which is the
// exact shape `<hub>/_board` has — and the old fallback deleted the directory anyway, taking
// .lyx-anchor, .lyx-warp and the repo-wide fabric.yaml with it while reporting success.
// The slug is deliberately NOT a reserved name, so this test exercises removeWarpWorktreeDir's
// registered-linked-worktree rule rather than the slug guard in front of it.
func TestRemove_RefusesForeignWorktreeWithoutDeletingIt(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout

	const slug = "sidecar"
	foreign := filepath.Join(l.HubPath, slug)
	lyxtest.MustRun(t, fixture.WeftPrime, "git", "worktree", "add", "--detach", foreign)

	marker := filepath.Join(foreign, "content-marker")
	if err := os.WriteFile(marker, []byte("keep me\n"), 0o644); err != nil {
		t.Fatalf("seed marker: %v", err)
	}
	lyxtest.MustRun(t, foreign, "git", "add", "content-marker")
	lyxtest.MustRun(t, foreign, "git", "commit", "-m", "seed marker")

	topology := fabricengine.NewTopology(fabricengine.Config{})
	_, err := topology.Remove(l, slug, false)
	if err == nil {
		t.Fatalf("Remove(%q) = nil error; want a refusal naming git's own reason", slug)
	}
	if !strings.Contains(err.Error(), "not a linked worktree of this repo") {
		t.Errorf("Remove(%q) error = %v; want it to say fabric will not delete a directory git declined to remove", slug, err)
	}

	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("foreign worktree content at %s was destroyed by a refused Remove: %v", marker, statErr)
	}
}

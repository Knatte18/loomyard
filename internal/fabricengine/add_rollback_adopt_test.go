//go:build integration

// add_rollback_adopt_test.go proves Add's rollback never deletes a
// pre-existing (adopted) weft branch: when Add merely adopted an existing
// <slug>-weft branch and then failed at a later step, the rollback must tear
// down only the worktree Add created — the branch, and any unpushed history
// it carries, survives. A live review round reproduced the pre-fix behavior
// (branch and its unique commit destroyed after a host-push failure), so this
// test injects a deterministic post-adopt failure (a portal blocker file, the
// same injection TestAddRollback_DifferentialEquivalence uses) instead of a
// network failure.
//
// Package fabricengine_test to reuse the external-test-package fixture idiom
// of lifecycle_differential_test.go; shares the single TestMain in
// testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// shaOf returns the commit SHA rev names in the repo at dir, failing the test
// on any git error.
func shaOf(t *testing.T, dir, rev string) string {
	t.Helper()

	out, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", rev}, dir)
	if err != nil || exitCode != 0 {
		t.Fatalf("rev-parse %s in %s: err=%v exit=%d", rev, dir, err, exitCode)
	}
	return strings.TrimSpace(out)
}

// TestAddRollback_AdoptedWeftBranchSurvives pre-creates a weft branch carrying
// a unique commit, forces Add to fail after adopting it, and asserts the
// rollback removed the weft worktree but left the branch — still pointing at
// the unique commit — untouched, alongside the usual zero host-side residue.
func TestAddRollback_AdoptedWeftBranchSurvives(t *testing.T) {
	t.Parallel()

	const slug = "adopt-rollback-keep"
	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})
	// Mirror CloneHub's post-clone state so the fixture matches a real fabric
	// hub: the weft primary sits on the suffixed sibling of the host's branch.
	lyxtest.MustRun(t, fixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))

	l := fixture.Layout
	weftBranch := fabricengine.WeftBranchName(slug)

	// Pre-create the weft branch with a unique commit that predates the Add —
	// the history the rollback must not destroy. The seeding worktree is
	// removed again so the branch is free for Add to adopt.
	seedDir := filepath.Join(t.TempDir(), "seed")
	lyxtest.MustRun(t, l.WeftRepoRoot(), "git", "worktree", "add", "-b", weftBranch, seedDir, fabricengine.WeftBranchName("main"))
	if err := os.WriteFile(filepath.Join(seedDir, "precious.txt"), []byte("pre-existing weft work\n"), 0o644); err != nil {
		t.Fatalf("write precious.txt: %v", err)
	}
	lyxtest.MustRun(t, seedDir, "git", "add", "precious.txt")
	lyxtest.MustRun(t, seedDir, "git", "commit", "-m", "precious pre-existing weft work")
	preciousSHA := shaOf(t, seedDir, "HEAD")
	lyxtest.MustRun(t, l.WeftRepoRoot(), "git", "worktree", "remove", seedDir)

	// Inject a deterministic failure AFTER the adopt: a blocker file at the
	// portal location makes step 9 (createPortal) fail, triggering rollback.
	portalLink := filepath.Join(l.PortalsDir(), slug)
	if err := os.MkdirAll(filepath.Dir(portalLink), 0o755); err != nil {
		t.Fatalf("mkdir portal parent: %v", err)
	}
	if err := os.WriteFile(portalLink, []byte("blocker"), 0o644); err != nil {
		t.Fatalf("create blocker: %v", err)
	}

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("Add should have failed (portal blocker)")
	}

	// The adopted branch survives the rollback, still at its unique commit.
	if !branchExistsAt(t, l.WeftRepoRoot(), weftBranch) {
		t.Fatalf("adopted weft branch %q was deleted by Add's rollback; want it preserved", weftBranch)
	}
	branchSHA := shaOf(t, l.WeftRepoRoot(), "refs/heads/"+weftBranch)
	if branchSHA != preciousSHA {
		t.Errorf("adopted weft branch %q = %s; want the pre-existing commit %s", weftBranch, branchSHA, preciousSHA)
	}

	// Everything Add itself created is rolled back: no weft worktree dir, no
	// host worktree dir, no host branch.
	if _, err := os.Stat(l.WeftWorktreePath(slug)); !os.IsNotExist(err) {
		t.Errorf("weft worktree dir still exists at %s", l.WeftWorktreePath(slug))
	}
	if _, err := os.Stat(l.WorktreePath(slug)); !os.IsNotExist(err) {
		t.Errorf("host worktree dir still exists at %s", l.WorktreePath(slug))
	}
	if branchExistsAt(t, l.WorktreeRoot, slug) {
		t.Errorf("host branch %q still exists", slug)
	}
}

//go:build integration

// warpforward_integration_test.go is the Tier-2 real-git coverage for the
// four warp-only Fabric methods added in warpforward.go: CheckoutDetached,
// RestoreBranch, CurrentBranch, and ResetHard. Each test drives a real paired
// Fabric built from newFabricFixture's warp worktree and asserts the
// resulting git state directly — no fake, no mock — since the whole point of
// this file is proving the thin delegation actually reaches real git.
//
// Package fabricengine_test to reuse newFabricFixture/currentBranchOf from
// reconcile_stale_registration_test.go, exactly as checkout_rollback_test.go
// does; shares the single TestMain in testmain_test.go.

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

// currentSHAOf returns the full SHA of HEAD at dir via git rev-parse HEAD,
// failing the test on any git error. Shared by every test in this file that
// needs to capture or assert a specific commit.
func currentSHAOf(t *testing.T, dir string) string {
	t.Helper()

	out, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "HEAD"}, dir)
	if err != nil || exitCode != 0 {
		t.Fatalf("rev-parse HEAD in %s: err=%v exit=%d", dir, err, exitCode)
	}
	return strings.TrimSpace(out)
}

// commitFile writes name/content in dir and commits it there with msg,
// returning the new HEAD SHA. Used to build up warp history the four
// warp-only Fabric methods can then checkout/reset against.
func commitFile(t *testing.T, dir, name, content, msg string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	lyxtest.MustRun(t, dir, "git", "add", "--", name)
	lyxtest.MustRun(t, dir, "git", "commit", "-m", msg)
	return currentSHAOf(t, dir)
}

// TestFabricWarp_DetachVerifyRestoreRoundTrip proves CheckoutDetached and
// RestoreBranch round-trip: capture the current branch, make a new commit,
// detach HEAD to the commit before it, then restore the original branch and
// confirm HEAD is back where it started.
func TestFabricWarp_DetachVerifyRestoreRoundTrip(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	f, err := fabricengine.New(fixture.Layout.WorktreePath(), fabricengine.WeftWorktree(fixture.Layout))
	if err != nil {
		t.Fatalf("fabricengine.New: %v", err)
	}

	originalBranch, err := f.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch (before detach): %v", err)
	}
	olderSHA := currentSHAOf(t, fixture.Layout.WorktreePath())

	// A later commit on warp gives CheckoutDetached somewhere to detach FROM,
	// and something the eventual RestoreBranch must land back on top of.
	commitFile(t, fixture.Layout.WorktreePath(), "round-trip.txt", "v1", "round-trip commit")

	if err := f.CheckoutDetached(olderSHA); err != nil {
		t.Fatalf("CheckoutDetached(%q): %v", olderSHA, err)
	}
	if got := currentSHAOf(t, fixture.Layout.WorktreePath()); got != olderSHA {
		t.Errorf("HEAD SHA after CheckoutDetached = %q; want %q", got, olderSHA)
	}
	// A detached HEAD reports the literal "HEAD" for --abbrev-ref, never a
	// branch name; this is the same signal gitrepo.Repo.CurrentBranch itself
	// rejects.
	if got := currentBranchOf(t, fixture.Layout.WorktreePath()); got != "HEAD" {
		t.Errorf("HEAD ref after CheckoutDetached = %q; want %q (detached)", got, "HEAD")
	}

	if err := f.RestoreBranch(originalBranch); err != nil {
		t.Fatalf("RestoreBranch(%q): %v", originalBranch, err)
	}
	if got := currentBranchOf(t, fixture.Layout.WorktreePath()); got != originalBranch {
		t.Errorf("branch after RestoreBranch = %q; want %q (original)", got, originalBranch)
	}
}

// TestFabricWarp_RestoreBranchInvalidRefErrors proves RestoreBranch returns a
// non-nil error when handed a ref that does not exist — the shape a caller
// would hit if the branch it captured earlier was deleted out from under it.
func TestFabricWarp_RestoreBranchInvalidRefErrors(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	f, err := fabricengine.New(fixture.Layout.WorktreePath(), fabricengine.WeftWorktree(fixture.Layout))
	if err != nil {
		t.Fatalf("fabricengine.New: %v", err)
	}

	if err := f.RestoreBranch("does-not-exist-anywhere"); err == nil {
		t.Fatalf("RestoreBranch(non-existent ref) error = nil; want non-nil")
	}
}

// TestFabricWarp_ResetHardDiscardsCommitsAndWorktreeChanges proves ResetHard
// discards both a later commit AND an uncommitted working-tree change,
// landing HEAD exactly at the older sha with the later file gone from disk.
func TestFabricWarp_ResetHardDiscardsCommitsAndWorktreeChanges(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	f, err := fabricengine.New(fixture.Layout.WorktreePath(), fabricengine.WeftWorktree(fixture.Layout))
	if err != nil {
		t.Fatalf("fabricengine.New: %v", err)
	}

	olderSHA := currentSHAOf(t, fixture.Layout.WorktreePath())

	// A committed change past olderSHA, then an uncommitted change on top —
	// ResetHard must discard both in one call.
	laterPath := filepath.Join(fixture.Layout.WorktreePath(), "reset-hard-later.txt")
	commitFile(t, fixture.Layout.WorktreePath(), "reset-hard-later.txt", "committed", "later commit past olderSHA")
	if err := os.WriteFile(laterPath, []byte("uncommitted edit"), 0o644); err != nil {
		t.Fatalf("write uncommitted change: %v", err)
	}

	if err := f.ResetHard(olderSHA); err != nil {
		t.Fatalf("ResetHard(%q): %v", olderSHA, err)
	}

	if got := currentSHAOf(t, fixture.Layout.WorktreePath()); got != olderSHA {
		t.Errorf("HEAD SHA after ResetHard = %q; want %q", got, olderSHA)
	}
	if _, err := os.Stat(laterPath); !os.IsNotExist(err) {
		t.Errorf("file %s still present after ResetHard; want discarded (err=%v)", laterPath, err)
	}
}

// TestFabricWarp_CurrentBranchErrorsOnDetachedHead proves CurrentBranch
// returns a non-nil error when warp's HEAD is already detached, matching
// gitrepo.Repo.CurrentBranch's documented detached-HEAD rejection.
func TestFabricWarp_CurrentBranchErrorsOnDetachedHead(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	f, err := fabricengine.New(fixture.Layout.WorktreePath(), fabricengine.WeftWorktree(fixture.Layout))
	if err != nil {
		t.Fatalf("fabricengine.New: %v", err)
	}

	lyxtest.MustRun(t, fixture.Layout.WorktreePath(), "git", "checkout", "--detach")

	if _, err := f.CurrentBranch(); err == nil {
		t.Fatalf("CurrentBranch() on detached HEAD error = nil; want non-nil")
	}
}

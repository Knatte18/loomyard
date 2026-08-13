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
	"github.com/Knatte18/loomyard/internal/gitkit"
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
	gitkit.MustRun(t, dir, "git", "add", "--", name)
	gitkit.MustRun(t, dir, "git", "commit", "-m", msg)
	return currentSHAOf(t, dir)
}

// TestFabricWarp_DetachVerifyRestoreRoundTrip proves CheckoutDetached and RestoreBranch round-trip:
// capture the current branch, make a new commit, detach HEAD to the commit before it, then restore
// the original branch and confirm HEAD is back where it started.
func TestFabricWarp_DetachVerifyRestoreRoundTrip(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	f, err := fabricengine.Open(fixture.Layout)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
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

// TestFabricWarp_RestoreBranchInvalidRefErrors proves RestoreBranch returns a non-nil error when
// handed a ref that does not exist — the shape a caller would hit if the branch it captured earlier
// was deleted out from under it.
func TestFabricWarp_RestoreBranchInvalidRefErrors(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	f, err := fabricengine.Open(fixture.Layout)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}

	if err := f.RestoreBranch("does-not-exist-anywhere"); err == nil {
		t.Fatalf("RestoreBranch(non-existent ref) error = nil; want non-nil")
	}
}

// TestFabricWarp_ResetHardDiscardsCommitsOnCleanWorktree proves ResetHard discards a later commit,
// landing HEAD exactly at the older sha, when the warp checkout has no uncommitted changes.
// This is the half of ResetHard's contract that is unaffected by the gate: a clean tracked
// worktree is never dirty, so dirtyScopeTracked never refuses it.
func TestFabricWarp_ResetHardDiscardsCommitsOnCleanWorktree(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	f, err := fabricengine.Open(fixture.Layout)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}

	olderSHA := currentSHAOf(t, fixture.Layout.WorktreePath())

	// A committed change past olderSHA, with no uncommitted change on top —
	// ResetHard must still discard the committed history.
	laterPath := filepath.Join(fixture.Layout.WorktreePath(), "reset-hard-later.txt")
	commitFile(t, fixture.Layout.WorktreePath(), "reset-hard-later.txt", "committed", "later commit past olderSHA")

	if err := f.ResetHard(fabricengine.NewMutations(""), olderSHA); err != nil {
		t.Fatalf("ResetHard(%q): %v", olderSHA, err)
	}

	if got := currentSHAOf(t, fixture.Layout.WorktreePath()); got != olderSHA {
		t.Errorf("HEAD SHA after ResetHard = %q; want %q", got, olderSHA)
	}
	if _, err := os.Stat(laterPath); !os.IsNotExist(err) {
		t.Errorf("file %s still present after ResetHard; want discarded (err=%v)", laterPath, err)
	}
}

// TestFabricWarp_ResetHardRefusesDirtyWarpCheckout proves ResetHard refuses to run, leaving both
// the later commit and the uncommitted working-tree change on disk, when the warp checkout has
// uncommitted tracked changes. This is card 11's deliberate hardening of ResetHard's contract:
// it no longer unconditionally discards, matching Pull's own pre-existing ErrWarpDirty check but
// enforced at the ResetHard call site itself rather than only by callers who wrap it in Pull.
func TestFabricWarp_ResetHardRefusesDirtyWarpCheckout(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	f, err := fabricengine.Open(fixture.Layout)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}

	olderSHA := currentSHAOf(t, fixture.Layout.WorktreePath())

	// A committed change past olderSHA, then an uncommitted change on top —
	// ResetHard must refuse rather than discard either one.
	laterPath := filepath.Join(fixture.Layout.WorktreePath(), "reset-hard-later.txt")
	commitFile(t, fixture.Layout.WorktreePath(), "reset-hard-later.txt", "committed", "later commit past olderSHA")
	const uncommittedContent = "uncommitted edit"
	if err := os.WriteFile(laterPath, []byte(uncommittedContent), 0o644); err != nil {
		t.Fatalf("write uncommitted change: %v", err)
	}

	err = f.ResetHard(fabricengine.NewMutations(""), olderSHA)
	if err == nil {
		t.Fatalf("ResetHard(%q) on dirty warp checkout error = nil; want a refusal", olderSHA)
	}
	if !strings.Contains(err.Error(), "dirtiness check failed") {
		t.Errorf("ResetHard(%q) error = %q; want a dirtiness-gate refusal", olderSHA, err)
	}

	if got := currentSHAOf(t, fixture.Layout.WorktreePath()); got == olderSHA {
		t.Errorf("HEAD SHA after refused ResetHard = %q; want the later commit to remain (refusal must not discard history)", got)
	}
	gotContent, err := os.ReadFile(laterPath)
	if err != nil {
		t.Fatalf("read %s after refused ResetHard: %v", laterPath, err)
	}
	if string(gotContent) != uncommittedContent {
		t.Errorf("content of %s after refused ResetHard = %q; want uncommitted change left on disk (%q)", laterPath, gotContent, uncommittedContent)
	}
}

// TestFabricWarp_CurrentBranchErrorsOnDetachedHead proves CurrentBranch returns a non-nil error
// when warp's HEAD is already detached, matching gitrepo.Repo.CurrentBranch's documented
// detached-HEAD rejection.
func TestFabricWarp_CurrentBranchErrorsOnDetachedHead(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	f, err := fabricengine.Open(fixture.Layout)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}

	gitkit.MustRun(t, fixture.Layout.WorktreePath(), "git", "checkout", "--detach")

	if _, err := f.CurrentBranch(); err == nil {
		t.Fatalf("CurrentBranch() on detached HEAD error = nil; want non-nil")
	}
}

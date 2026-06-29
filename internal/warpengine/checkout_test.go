//go:build integration

// checkout_test.go covers coordinated host+weft checkout: happy path, dirty-weft
// precondition refusal, host rollback on weft-side failure, and checkout onto an
// unmanaged branch that causes the weft branch to be forked from the parent's weft.

package warpengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// setupCheckoutFixture prepares a CopyPairedLocal fixture with a warp config seeded
// into the weft prime and the host _lyx junction created so that Checkout can call
// WireJunctions and LoadConfig successfully.
//
// In production, the host _lyx is a junction to the weft's _lyx. This fixture
// replicates that: warp config is seeded into the weft prime, then a junction is
// created from the host _lyx to the weft _lyx so config resolves through the junction.
// Returns the fixture.
func setupCheckoutFixture(t *testing.T) lyxtest.PairedFixture {
	t.Helper()

	f := lyxtest.CopyPairedLocal(t)

	slug := filepath.Base(f.Hub)

	// The weft prime's _lyx/config already has a placeholder from the template.
	// Seed the warp config into the weft prime so LoadConfig resolves through the junction.
	lyxtest.SeedConfig(t, f.WeftPrime, map[string]string{
		"warp": ConfigTemplate(),
	})

	// Create the host _lyx junction pointing to the weft's _lyx dir.
	// This replicates the production topology: host _lyx → weft _lyx.
	// WireJunctions enforces that host _lyx must not be a real directory; the fixture
	// hub has no _lyx yet (only the weft prime does from the template), so it succeeds.
	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	return f
}

// TestCheckout_HappyPath asserts that Checkout switches both the host and the weft
// worktree to the target branch and that the host _lyx junction still exists.
func TestCheckout_HappyPath(t *testing.T) {
	t.Parallel()

	f := setupCheckoutFixture(t)

	const targetBranch = "target"
	slug := filepath.Base(f.Hub)

	// Create the target branch on the host.
	lyxtest.MustRun(t, f.Hub, "git", "branch", targetBranch)

	// Create the matching target branch on the weft prime so Checkout finds it.
	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "branch", targetBranch)

	w := New(Config{})
	result, err := w.Checkout(f.Layout, targetBranch)
	if err != nil {
		t.Fatalf("Checkout(%q) error = %v; want nil", targetBranch, err)
	}

	if result.Branch != targetBranch {
		t.Errorf("Checkout(%q).Branch = %q; want %q", targetBranch, result.Branch, targetBranch)
	}
	if result.WeftWorktree != f.Layout.WeftWorktree() {
		t.Errorf("Checkout(%q).WeftWorktree = %q; want %q", targetBranch, result.WeftWorktree, f.Layout.WeftWorktree())
	}

	// Assert the host worktree is now on the target branch.
	hostBranchOut, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		f.Hub,
	)
	if err != nil || exitCode != 0 {
		t.Fatalf("get host branch: %v (exit %d)", err, exitCode)
	}
	if got := strings.TrimSpace(hostBranchOut); got != targetBranch {
		t.Errorf("host branch = %q; want %q", got, targetBranch)
	}

	// Assert the weft worktree is now on the target branch.
	weftBranchOut, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		f.Layout.WeftWorktree(),
	)
	if err != nil || exitCode != 0 {
		t.Fatalf("get weft branch: %v (exit %d)", err, exitCode)
	}
	if got := strings.TrimSpace(weftBranchOut); got != targetBranch {
		t.Errorf("weft branch = %q; want %q", got, targetBranch)
	}

	// Assert the host _lyx junction still exists (re-pointed by WireJunctions).
	hostLink := f.Layout.HostLyxLink(slug)
	isLink, err := fslink.IsLink(hostLink)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("fslink.IsLink(%s): %v", hostLink, err)
	}
	if !isLink {
		t.Errorf("Checkout(%q): host _lyx junction missing at %s; want junction present", targetBranch, hostLink)
	}
}

// TestCheckout_DirtyWeftRefusal asserts that Checkout refuses with an error when the
// weft worktree has uncommitted tracked changes, and that neither the host nor the
// weft worktree changes branch.
func TestCheckout_DirtyWeftRefusal(t *testing.T) {
	t.Parallel()

	f := setupCheckoutFixture(t)

	const targetBranch = "target"

	// Create the target branch on the host so the host switch would succeed if allowed.
	lyxtest.MustRun(t, f.Hub, "git", "branch", targetBranch)

	// Dirty the weft worktree with an uncommitted tracked change. The fixture's weft
	// prime contains a placeholder file in _lyx/config; modify it so git status
	// --porcelain shows a tracked change.
	placeholderPath := filepath.Join(f.Layout.WeftWorktree(), "_lyx", "config", "placeholder")
	if err := os.WriteFile(placeholderPath, []byte("dirty-content"), 0o644); err != nil {
		t.Fatalf("dirty weft: %v", err)
	}
	// Stage the file so it shows up in --porcelain without --untracked-files=no.
	lyxtest.MustRun(t, f.Layout.WeftWorktree(), "git", "add", placeholderPath)

	w := New(Config{})
	_, err := w.Checkout(f.Layout, targetBranch)

	if err == nil {
		t.Fatalf("Checkout(%q) error = nil; want dirty-weft refusal", targetBranch)
	}
	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("Checkout(%q) error = %q; want substring %q", targetBranch, err.Error(), "uncommitted changes")
	}

	// Assert the host worktree did NOT switch — it must still be on main.
	hostBranchOut, _, exitCode, err2 := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		f.Hub,
	)
	if err2 != nil || exitCode != 0 {
		t.Fatalf("get host branch: %v (exit %d)", err2, exitCode)
	}
	if got := strings.TrimSpace(hostBranchOut); got != "main" {
		t.Errorf("host branch after refusal = %q; want %q (no switch occurred)", got, "main")
	}

	// Assert the weft worktree did NOT switch.
	weftBranchOut, _, exitCode, err2 := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		f.Layout.WeftWorktree(),
	)
	if err2 != nil || exitCode != 0 {
		t.Fatalf("get weft branch: %v (exit %d)", err2, exitCode)
	}
	if got := strings.TrimSpace(weftBranchOut); got != "main" {
		t.Errorf("weft branch after refusal = %q; want %q (no switch occurred)", got, "main")
	}
}

// TestCheckout_HostRollback asserts that when the weft switch fails after the host has
// already switched, the host is rolled back to its original branch so the pair is
// never left half-switched.
//
// The failure is injected by creating a separate weft worktree that checks out the
// target branch (locking it), so git switch <target> on the prime weft worktree
// fails with "branch already checked out".
func TestCheckout_HostRollback(t *testing.T) {
	t.Parallel()

	f := setupCheckoutFixture(t)

	const targetBranch = "target"

	// Create the target branch on the host so the host switch succeeds.
	lyxtest.MustRun(t, f.Hub, "git", "branch", targetBranch)

	// Create the target branch on the weft prime so weftBranchExists returns true
	// (routes to git switch <branch>, not git switch -c).
	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "branch", targetBranch)

	// Lock the target branch in the weft by creating a separate weft worktree that
	// has it checked out. This causes git switch <target> in the prime weft worktree
	// to fail with "already checked out".
	lockPath := filepath.Join(f.Layout.Hub, "lock-weft-target")
	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "worktree", "add", lockPath, targetBranch)
	t.Cleanup(func() {
		// Best-effort cleanup of the lock worktree; TempDir cleanup handles the directory itself.
		_, _, _, _ = gitexec.RunGit([]string{"worktree", "remove", "--force", lockPath}, f.Layout.WeftRepoRoot())
	})

	w := New(Config{})
	_, err := w.Checkout(f.Layout, targetBranch)

	// Checkout must return an error because the weft switch failed.
	if err == nil {
		t.Fatalf("Checkout(%q) error = nil; want weft-side failure triggering rollback", targetBranch)
	}

	// The host must be rolled back to main — it must NOT be on target.
	hostBranchOut, _, exitCode, err2 := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		f.Hub,
	)
	if err2 != nil || exitCode != 0 {
		t.Fatalf("get host branch after rollback: %v (exit %d)", err2, exitCode)
	}
	if got := strings.TrimSpace(hostBranchOut); got != "main" {
		t.Errorf("host branch after rollback = %q; want %q (rollback must restore original)", got, "main")
	}

	// The weft must still be on its original branch (main) — the switch was blocked.
	weftBranchOut, _, exitCode, err2 := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		f.Layout.WeftWorktree(),
	)
	if err2 != nil || exitCode != 0 {
		t.Fatalf("get weft branch after rollback: %v (exit %d)", err2, exitCode)
	}
	if got := strings.TrimSpace(weftBranchOut); got != "main" {
		t.Errorf("weft branch after rollback = %q; want %q (pair must be untouched)", got, "main")
	}
}

// TestCheckout_UnmanagedBranch asserts that when the host switches to a branch that
// has no corresponding weft branch, Checkout forks a new weft branch from the current
// parent weft branch (matching the adopt-or-create fork-point of warp add).
// The resulting pair is managed: both host and weft are on the target branch.
func TestCheckout_UnmanagedBranch(t *testing.T) {
	t.Parallel()

	f := setupCheckoutFixture(t)

	const targetBranch = "new-feature"

	// Create the target branch on the host only — no corresponding weft branch.
	lyxtest.MustRun(t, f.Hub, "git", "branch", targetBranch)

	// Verify no weft branch exists for targetBranch before the call.
	if weftBranchExists(f.Layout, targetBranch) {
		t.Fatalf("pre-condition: weft branch %q must not exist before Checkout", targetBranch)
	}

	w := New(Config{})
	result, err := w.Checkout(f.Layout, targetBranch)
	if err != nil {
		t.Fatalf("Checkout(%q) error = %v; want nil (fork-on-unmanaged)", targetBranch, err)
	}

	if result.Branch != targetBranch {
		t.Errorf("Checkout(%q).Branch = %q; want %q", targetBranch, result.Branch, targetBranch)
	}

	// Assert the host is now on the target branch.
	hostBranchOut, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		f.Hub,
	)
	if err != nil || exitCode != 0 {
		t.Fatalf("get host branch: %v (exit %d)", err, exitCode)
	}
	if got := strings.TrimSpace(hostBranchOut); got != targetBranch {
		t.Errorf("host branch = %q; want %q", got, targetBranch)
	}

	// Assert the weft worktree is now on the forked target branch (managed pair).
	weftBranchOut, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		f.Layout.WeftWorktree(),
	)
	if err != nil || exitCode != 0 {
		t.Fatalf("get weft branch: %v (exit %d)", err, exitCode)
	}
	if got := strings.TrimSpace(weftBranchOut); got != targetBranch {
		t.Errorf("weft branch = %q; want %q (forked managed pair)", got, targetBranch)
	}

	// Assert the new weft branch now exists in the weft repo.
	if !weftBranchExists(f.Layout, targetBranch) {
		t.Errorf("Checkout(%q): weft branch %q does not exist in weft repo; want forked managed branch", targetBranch, targetBranch)
	}
}

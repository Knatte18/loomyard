//go:build integration

// prune_test.go covers the Prune verb: an orphaned/stale pair is reported in dry-run
// and removed under --apply; a live pair is never touched.

package warpengine

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// setupPruneFixture prepares a CopyPairedLocal fixture with warp config seeded
// and the host _lyx junction created, mirroring the production topology for prune tests.
func setupPruneFixture(t *testing.T) lyxtest.PairedFixture {
	t.Helper()

	f := lyxtest.CopyPairedLocal(t)
	slug := filepath.Base(f.Hub)

	lyxtest.SeedConfig(t, f.WeftPrime, map[string]string{
		"warp": ConfigTemplate(),
	})

	// Wire the host _lyx junction to replicate the active production topology.
	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	return f
}

// TestPrune_StaleWeft asserts that Prune correctly handles stale weft worktrees
// (orphaned ones whose host siblings have been removed from git).
// The test runs sequentially on a shared fixture: first in dry-run mode to verify
// reporting without deletion, then in apply mode to verify actual removal.
func TestPrune_StaleWeft(t *testing.T) {
	t.Parallel()

	f := setupPruneFixture(t)

	// Add a real paired worktree so we have a weft directory to make stale.
	const testSlug = "feature-prune-stale"
	w := New(Config{BranchPrefix: ""})
	_, err := w.Add(f.Layout, testSlug, AddOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", testSlug, err)
	}

	weftPath := f.Layout.WeftWorktreePath(testSlug)
	hostPath := f.Layout.WorktreePath(testSlug)

	// Confirm both sides exist before removing the host.
	if _, statErr := os.Stat(hostPath); statErr != nil {
		t.Fatalf("pre-condition: host worktree %s must exist after Add", hostPath)
	}
	if _, statErr := os.Stat(weftPath); statErr != nil {
		t.Fatalf("pre-condition: weft worktree %s must exist after Add", weftPath)
	}

	// Remove only the host worktree via git so the registration is gone but the
	// weft directory remains — creating a stale/orphaned condition for Prune to detect.
	lyxtest.MustRun(t, f.Hub, "git", "worktree", "remove", "--force", hostPath)
	lyxtest.MustRun(t, f.Hub, "git", "branch", "-D", testSlug)

	// The weft directory must still exist (we didn't remove it).
	if _, statErr := os.Stat(weftPath); statErr != nil {
		t.Fatalf("pre-condition: weft worktree %s must still exist after host removal", weftPath)
	}

	// Step 1: dry-run (apply=false). Prune must report but not remove the stale weft.
	r, err := w.Prune(f.Layout, false)
	if err != nil {
		t.Fatalf("Prune(apply=false) error = %v; want nil", err)
	}

	// At least one entry must be reported for the orphaned weft.
	if len(r.Entries) == 0 {
		t.Fatalf("Prune(apply=false).Entries is empty; want at least one orphaned entry")
	}

	// Find the entry for our test pair.
	var found *PruneEntry
	for i := range r.Entries {
		if filepath.Clean(r.Entries[i].WeftWorktree) == filepath.Clean(weftPath) {
			found = &r.Entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Prune(): no entry for weft path %s; entries = %+v", weftPath, r.Entries)
	}

	// JSON-boundary paths must be forward-slash even on Windows (issue #37). Check the
	// raw field value directly -- filepath.Clean would re-normalize forward slashes back
	// to OS-native backslash and silently defeat this assertion.
	if strings.Contains(found.HostWorktree, "\\") {
		t.Errorf("PruneEntry.HostWorktree = %q; want no backslash separators", found.HostWorktree)
	}
	if strings.Contains(found.WeftWorktree, "\\") {
		t.Errorf("PruneEntry.WeftWorktree = %q; want no backslash separators", found.WeftWorktree)
	}

	// Dry-run must not mark Removed, and the weft directory must still exist.
	if found.Removed {
		t.Errorf("PruneEntry.Removed = true on dry-run; want false")
	}
	if found.Error != "" {
		t.Errorf("PruneEntry.Error = %q on dry-run; want empty", found.Error)
	}

	// Weft directory must not have been deleted on a dry-run.
	if _, statErr := os.Stat(weftPath); statErr != nil {
		t.Errorf("weft worktree %s was deleted by dry-run Prune; want intact", weftPath)
	}

	// Step 2: apply (apply=true). Prune must remove the stale weft on the same fixture.
	r, err = w.Prune(f.Layout, true)
	if err != nil {
		t.Fatalf("Prune(apply=true) error = %v; want nil", err)
	}

	// Find the entry for the stale weft again after apply.
	found = nil
	for i := range r.Entries {
		if filepath.Clean(r.Entries[i].WeftWorktree) == filepath.Clean(weftPath) {
			found = &r.Entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Prune(apply=true): no entry for weft path %s", weftPath)
	}

	// Entry must be marked Removed without error.
	if !found.Removed {
		t.Errorf("PruneEntry.Removed = false after apply; want true")
	}
	if found.Error != "" {
		t.Errorf("PruneEntry.Error = %q; want empty (removal should succeed)", found.Error)
	}

	// The weft directory must be gone after apply.
	if _, statErr := os.Stat(weftPath); !os.IsNotExist(statErr) {
		t.Errorf("weft worktree %s still exists after apply Prune; want deleted", weftPath)
	}
}

// TestPrune_DoubleRemovalFailureNoStderrLeak asserts that when both the git-level
// removal (`git worktree remove --force`) AND the os.RemoveAll fallback fail,
// pe.Error is composed from local context (the weft path and git exit code) rather
// than git's own stderr text. The git-level failure is forced by locking the weft
// worktree (`git worktree lock`, which even --force cannot override); the fallback
// failure is forced OS-appropriately — an open file handle inside the weft worktree
// on Windows (which refuses to unlink an open file), a write-stripped worktree dir on
// POSIX (which refuses to unlink a directory's contents without write permission),
// since POSIX unlinks open files freely and would otherwise let the fallback succeed.
func TestPrune_DoubleRemovalFailureNoStderrLeak(t *testing.T) {
	t.Parallel()

	f := setupPruneFixture(t)

	const testSlug = "feature-prune-double-fail"
	w := New(Config{BranchPrefix: ""})
	_, err := w.Add(f.Layout, testSlug, AddOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", testSlug, err)
	}

	weftPath := f.Layout.WeftWorktreePath(testSlug)
	hostPath := f.Layout.WorktreePath(testSlug)

	// Remove only the host worktree so the pair becomes stale/orphaned.
	lyxtest.MustRun(t, f.Hub, "git", "worktree", "remove", "--force", hostPath)
	lyxtest.MustRun(t, f.Hub, "git", "branch", "-D", testSlug)

	// Force the git-level removal to fail: a locked worktree is refused even by
	// `git worktree remove --force` (double -f or an explicit unlock is required).
	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "worktree", "lock", weftPath, "--reason", "test-lock")

	// Force the os.RemoveAll fallback to also fail. The mechanism is OS-specific:
	// Windows refuses to unlink an open-for-read file out from under an active handle;
	// POSIX allows that, so instead strip write permission from the weft worktree dir,
	// which makes unlinking its contents fail. releaseBlock restores state so the
	// fixture teardown (t.Cleanup TempDir removal) succeeds cleanly.
	var releaseBlock func()
	if runtime.GOOS == "windows" {
		blockerPath := filepath.Join(weftPath, "prune-double-fail-blocker")
		if err := os.WriteFile(blockerPath, []byte("blocker"), 0o644); err != nil {
			t.Fatalf("write blocker file: %v", err)
		}
		blocker, err := os.Open(blockerPath)
		if err != nil {
			t.Fatalf("open blocker file: %v", err)
		}
		releaseBlock = func() { blocker.Close() }
	} else {
		if err := os.Chmod(weftPath, 0o555); err != nil {
			t.Fatalf("chmod weft worktree read-only: %v", err)
		}
		releaseBlock = func() { _ = os.Chmod(weftPath, 0o755) }
	}
	defer releaseBlock()

	r, err := w.Prune(f.Layout, true)
	if err != nil {
		t.Fatalf("Prune(apply=true) error = %v; want nil", err)
	}

	var found *PruneEntry
	for i := range r.Entries {
		if filepath.Clean(r.Entries[i].WeftWorktree) == filepath.Clean(weftPath) {
			found = &r.Entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Prune(apply=true): no entry for weft path %s; entries = %+v", weftPath, r.Entries)
	}

	if found.Removed {
		t.Errorf("PruneEntry.Removed = true despite lock + open handle; want false (both removal paths should fail)")
	}
	if found.Error == "" {
		t.Fatalf("PruneEntry.Error = \"\"; want non-empty (both removal paths should fail)")
	}
	if strings.Contains(found.Error, "fatal:") {
		t.Errorf("PruneEntry.Error = %q; want no %q substring (raw git stderr leak)", found.Error, "fatal:")
	}

	// Release the block and unlock/remove the worktree so t.Cleanup (TempDir removal)
	// does not fail, and so the fixture teardown succeeds cleanly.
	releaseBlock()
	_, _, _, _ = gitexec.RunGit([]string{"worktree", "unlock", weftPath}, f.Layout.WeftRepoRoot())
	_, _, _, _ = gitexec.RunGit([]string{"worktree", "remove", "--force", weftPath}, f.Layout.WeftRepoRoot())
}

// TestPrune_LivePairNeverTouched asserts that Prune, whether in dry-run or apply mode,
// does not include or remove a healthy live pair in its output.
func TestPrune_LivePairNeverTouched(t *testing.T) {
	t.Parallel()

	f := setupPruneFixture(t)

	// Add a real paired worktree that will remain live throughout.
	const testSlug = "feature-prune-live"
	w := New(Config{BranchPrefix: ""})
	_, err := w.Add(f.Layout, testSlug, AddOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", testSlug, err)
	}

	weftPath := f.Layout.WeftWorktreePath(testSlug)
	hostPath := f.Layout.WorktreePath(testSlug)

	// Both sides exist — this is a live pair.
	if _, statErr := os.Stat(hostPath); statErr != nil {
		t.Fatalf("pre-condition: host %s must exist", hostPath)
	}
	if _, statErr := os.Stat(weftPath); statErr != nil {
		t.Fatalf("pre-condition: weft %s must exist", weftPath)
	}

	// Run Prune with apply=true; the live pair must not appear in entries.
	r, err := w.Prune(f.Layout, true)
	if err != nil {
		t.Fatalf("Prune(apply=true) error = %v; want nil", err)
	}

	for _, entry := range r.Entries {
		if filepath.Clean(entry.WeftWorktree) == filepath.Clean(weftPath) {
			t.Errorf("Prune reported live weft worktree %s as stale; want not reported", weftPath)
		}
		if filepath.Clean(entry.HostWorktree) == filepath.Clean(hostPath) {
			t.Errorf("Prune reported live host worktree %s as stale; want not reported", hostPath)
		}
	}

	// Both directories must still be intact.
	if _, statErr := os.Stat(hostPath); statErr != nil {
		t.Errorf("live host worktree %s was deleted by Prune; want intact", hostPath)
	}
	if _, statErr := os.Stat(weftPath); statErr != nil {
		t.Errorf("live weft worktree %s was deleted by Prune; want intact", weftPath)
	}
}

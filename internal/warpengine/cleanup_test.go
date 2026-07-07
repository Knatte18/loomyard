//go:build integration

// cleanup_test.go covers the Cleanup verb: the full flag matrix — no flag reports
// only; --apply deletes a non-task orphan but skips a gate-protected task branch
// (reported protected); --apply --force deletes the task branch; --force alone
// reports only; the board repo/branch is never a deletion candidate.

package warpengine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// setupCleanupFixture prepares a CopyPairedLocal fixture with warp config seeded
// so the weft repo is ready for cleanup tests.
func setupCleanupFixture(t *testing.T) lyxtest.PairedFixture {
	t.Helper()

	f := lyxtest.CopyPairedLocal(t)
	slug := filepath.Base(f.Hub)

	lyxtest.SeedConfig(t, f.WeftPrime, map[string]string{
		"warp": ConfigTemplate(),
	})

	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	return f
}

// createOrphanWeftBranch creates a branch in the weft repo that has no corresponding
// host worktree, simulating an orphaned weft branch. Returns the branch name.
func createOrphanWeftBranch(t *testing.T, f lyxtest.PairedFixture, branchName string) string {
	t.Helper()

	// Create the branch directly in the weft repo without a host counterpart.
	// This is the production scenario: the host branch was deleted but the weft branch
	// was never cleaned up.
	_, stderr, exitCode, err := gitexec.RunGit(
		[]string{"branch", branchName},
		f.Layout.WeftRepoRoot(),
	)
	if err != nil || exitCode != 0 {
		t.Fatalf("create orphan weft branch %q: err=%v stderr=%s", branchName, err, stderr)
	}
	return branchName
}

// TestCleanup_ReportOnlyModes asserts that both report-only flag combinations
// (apply=false force=false and apply=false force=true) report an orphaned weft branch
// without deleting it. The test runs sequentially on a shared fixture to verify that
// the branch survives multiple report-only operations.
func TestCleanup_ReportOnlyModes(t *testing.T) {
	t.Parallel()

	f := setupCleanupFixture(t)
	orphanBranch := createOrphanWeftBranch(t, f, "orphan-report-only")

	w := New(Config{})

	// Step 1: apply=false, force=false (default dry-run).
	r, err := w.Cleanup(f.Layout, false, false)
	if err != nil {
		t.Fatalf("Cleanup(apply=false, force=false) error = %v; want nil", err)
	}

	// The orphaned branch must appear in the report.
	var found *CleanupBranchEntry
	for i := range r.Entries {
		if r.Entries[i].Branch == orphanBranch {
			found = &r.Entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Cleanup(apply=false, force=false): orphan branch %q not found in entries %+v", orphanBranch, r.Entries)
	}

	// Dry-run: must not be marked deleted and must not be marked protected.
	if found.Deleted {
		t.Errorf("CleanupBranchEntry.Deleted = true on dry-run (apply=false force=false); want false")
	}
	if found.Error != "" {
		t.Errorf("CleanupBranchEntry.Error = %q on dry-run; want empty", found.Error)
	}

	// Branch must still exist in the weft repo after dry-run.
	if !weftBranchExists(f.Layout, orphanBranch) {
		t.Errorf("orphan branch %q was deleted by dry-run Cleanup; want intact", orphanBranch)
	}

	// Step 2: apply=false, force=true (force without apply = report only).
	// This verifies that force does not imply apply.
	r, err = w.Cleanup(f.Layout, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=false, force=true) error = %v; want nil", err)
	}

	// The orphaned branch should appear in the report (dry-run path).
	found = nil
	for i := range r.Entries {
		if r.Entries[i].Branch == orphanBranch {
			found = &r.Entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Cleanup(apply=false, force=true): orphan branch %q not found in entries", orphanBranch)
	}

	// force alone (without apply) = report only; Deleted must be false.
	if found.Deleted {
		t.Errorf("CleanupBranchEntry.Deleted = true with force-alone; want false (report only)")
	}

	// Branch must still exist after force-alone Cleanup.
	if !weftBranchExists(f.Layout, orphanBranch) {
		t.Errorf("orphan branch %q was deleted by force-alone Cleanup; want intact", orphanBranch)
	}
}

// TestCleanup_ApplySkipsProtectedBranch asserts that Cleanup with apply=true (no force)
// does not delete a branch that the gate marks as protected.
//
// raddleFoldedBack returns false for all branches (conservative), so every orphan
// branch is gate-protected even when apply=true. The test verifies that Protected=true
// and Deleted=false for an orphaned weft branch, and that the branch still exists
// after Cleanup returns.
func TestCleanup_ApplySkipsProtectedBranch(t *testing.T) {
	t.Parallel()

	f := setupCleanupFixture(t)
	orphanBranch := createOrphanWeftBranch(t, f, "orphan-apply-skip")

	w := New(Config{})
	r, err := w.Cleanup(f.Layout, true, false)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, force=false) error = %v; want nil", err)
	}

	// The orphaned branch must appear as protected (raddleFoldedBack returns false).
	var found *CleanupBranchEntry
	for i := range r.Entries {
		if r.Entries[i].Branch == orphanBranch {
			found = &r.Entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Cleanup(apply=true, force=false): orphan branch %q not found in entries %+v", orphanBranch, r.Entries)
	}

	// Must be protected (gate not passed) and not deleted.
	if !found.Protected {
		t.Errorf("CleanupBranchEntry.Protected = false; want true (raddleFoldedBack returns false)")
	}
	if found.Deleted {
		t.Errorf("CleanupBranchEntry.Deleted = true for protected branch; want false")
	}
	if found.Error != "" {
		t.Errorf("CleanupBranchEntry.Error = %q for protected branch; want empty", found.Error)
	}

	// Branch must still exist — it was protected, not deleted.
	if !weftBranchExists(f.Layout, orphanBranch) {
		t.Errorf("protected branch %q was deleted; want intact", orphanBranch)
	}
}

// TestCleanup_ApplyForceDeletesTaskBranch asserts that Cleanup with apply=true and
// force=true deletes a gate-protected orphaned weft branch.
func TestCleanup_ApplyForceDeletesTaskBranch(t *testing.T) {
	t.Parallel()

	f := setupCleanupFixture(t)
	taskBranch := createOrphanWeftBranch(t, f, "task-force-delete")

	w := New(Config{})
	r, err := w.Cleanup(f.Layout, true, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, force=true) error = %v; want nil", err)
	}

	// The task branch must appear as deleted.
	var found *CleanupBranchEntry
	for i := range r.Entries {
		if r.Entries[i].Branch == taskBranch {
			found = &r.Entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Cleanup(apply=true, force=true): task branch %q not found in entries %+v", taskBranch, r.Entries)
	}

	// Must be deleted, not protected.
	if !found.Deleted {
		t.Errorf("CleanupBranchEntry.Deleted = false after apply+force; want true")
	}
	if found.Protected {
		t.Errorf("CleanupBranchEntry.Protected = true after force delete; want false")
	}
	if found.Error != "" {
		t.Errorf("CleanupBranchEntry.Error = %q; want empty (deletion should succeed)", found.Error)
	}

	// Branch must be gone from the weft repo.
	if weftBranchExists(f.Layout, taskBranch) {
		t.Errorf("task branch %q still exists after force Cleanup; want deleted", taskBranch)
	}
}

// TestCleanup_DeleteFailureNoStderrLeak asserts that when deleteWeftBranch fails
// (the orphaned branch is locked by being checked out in another weft worktree),
// entry.Error is composed from local context (the branch name and git exit code)
// rather than git's own stderr text.
func TestCleanup_DeleteFailureNoStderrLeak(t *testing.T) {
	t.Parallel()

	f := setupCleanupFixture(t)
	orphanBranch := createOrphanWeftBranch(t, f, "orphan-delete-fails")

	// Lock the orphan branch by checking it out in a separate weft worktree so that
	// `git branch -D` on it fails (branch checked out elsewhere).
	lockPath := filepath.Join(f.Layout.Hub, "lock-cleanup-orphan")
	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "worktree", "add", lockPath, orphanBranch)
	t.Cleanup(func() {
		_, _, _, _ = gitexec.RunGit([]string{"worktree", "remove", "--force", lockPath}, f.Layout.WeftRepoRoot())
	})

	w := New(Config{})
	r, err := w.Cleanup(f.Layout, true, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, force=true) error = %v; want nil", err)
	}

	var found *CleanupBranchEntry
	for i := range r.Entries {
		if r.Entries[i].Branch == orphanBranch {
			found = &r.Entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Cleanup(apply=true, force=true): locked orphan branch %q not found in entries %+v", orphanBranch, r.Entries)
	}

	if found.Deleted {
		t.Errorf("CleanupBranchEntry.Deleted = true for a locked branch; want false (delete should fail)")
	}
	if found.Error == "" {
		t.Fatalf("CleanupBranchEntry.Error = \"\"; want non-empty (delete should fail)")
	}
	if !strings.Contains(found.Error, orphanBranch) {
		t.Errorf("CleanupBranchEntry.Error = %q; want substring %q (branch name)", found.Error, orphanBranch)
	}
	if strings.Contains(found.Error, "fatal:") {
		t.Errorf("CleanupBranchEntry.Error = %q; want no %q substring (raw git stderr leak)", found.Error, "fatal:")
	}

	// Branch must still exist — the deletion failed.
	if !weftBranchExists(f.Layout, orphanBranch) {
		t.Errorf("orphan branch %q was deleted despite lock; want intact", orphanBranch)
	}
}

// TestCleanup_LiveBranchNeverDeleted asserts that weft branches with corresponding
// live host worktrees are never reported or deleted by Cleanup. The test runs sequentially
// on a shared fixture with both no-prefix and prefixed branch cases to preserve regression
// coverage for the prefix-mismatch bug.
func TestCleanup_LiveBranchNeverDeleted(t *testing.T) {
	t.Parallel()

	f := setupCleanupFixture(t)

	// Add a live pair with no branch prefix.
	const noPrefixSlug = "live-task"
	wNoPrefix := New(Config{BranchPrefix: ""})
	_, err := wNoPrefix.Add(f.Layout, noPrefixSlug, AddOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("Add(%q) with no prefix: %v", noPrefixSlug, err)
	}

	// Add a live pair with a branch prefix. This is the regression test case:
	// when BranchPrefix is non-empty, the weft branch name is "prefix/slug" while
	// hostSlugs contains only "slug". Without stripping the prefix before the lookup,
	// the live weft branch appears as an orphan and would be deleted under --apply --force.
	const prefix = "hanf/"
	const prefixedSlug = "feature-prefix-live"
	wPrefixed := New(Config{BranchPrefix: prefix})
	_, err = wPrefixed.Add(f.Layout, prefixedSlug, AddOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("Add(%q) with prefix %q: %v", prefixedSlug, prefix, err)
	}

	weftBranchPrefixed := prefix + prefixedSlug

	// Run Cleanup in its most aggressive mode with the prefixed config.
	// The prefixed config is required so that the "hanf/"-prefixed branch
	// is recognized as live (an empty-prefix config would report/delete it).
	r, err := wPrefixed.Cleanup(f.Layout, true, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, force=true) error = %v; want nil", err)
	}

	// Neither live branch must appear in Cleanup entries.
	for _, entry := range r.Entries {
		if entry.Branch == noPrefixSlug {
			t.Errorf("Cleanup included live branch %q (no prefix) in entries; want not reported", noPrefixSlug)
		}
		if entry.Branch == weftBranchPrefixed {
			t.Errorf("Cleanup included live branch %q (prefix %q) in entries; want not reported", weftBranchPrefixed, prefix)
		}
	}

	// Both weft branches must still exist after Cleanup.
	if !weftBranchExists(f.Layout, noPrefixSlug) {
		t.Errorf("live weft branch %q (no prefix) was deleted by Cleanup; want intact", noPrefixSlug)
	}
	if !weftBranchExists(f.Layout, weftBranchPrefixed) {
		t.Errorf("live weft branch %q (prefix %q) was deleted by Cleanup; want intact", weftBranchPrefixed, prefix)
	}
}

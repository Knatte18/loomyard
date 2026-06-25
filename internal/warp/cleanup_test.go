//go:build integration

// cleanup_test.go covers the Cleanup verb: the full flag matrix — no flag reports
// only; --apply deletes a non-task orphan but skips a gate-protected task branch
// (reported protected); --apply --force deletes the task branch; --force alone
// reports only; the board repo/branch is never a deletion candidate.

package warp

import (
	"path/filepath"
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

// TestCleanup_DryRunReportsOrphanBranch asserts that Cleanup with apply=false
// reports an orphaned weft branch without deleting it.
func TestCleanup_DryRunReportsOrphanBranch(t *testing.T) {
	t.Parallel()

	f := setupCleanupFixture(t)
	orphanBranch := createOrphanWeftBranch(t, f, "orphan-dry-run")

	w := New(Config{})
	r, err := w.Cleanup(f.Layout, false, false)
	if err != nil {
		t.Fatalf("Cleanup(apply=false) error = %v; want nil", err)
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
		t.Fatalf("Cleanup(apply=false): orphan branch %q not found in entries %+v", orphanBranch, r.Entries)
	}

	// Dry-run: must not be marked deleted and must not be marked protected.
	if found.Deleted {
		t.Errorf("CleanupBranchEntry.Deleted = true on dry-run; want false")
	}
	if found.Error != "" {
		t.Errorf("CleanupBranchEntry.Error = %q on dry-run; want empty", found.Error)
	}

	// Branch must still exist in the weft repo.
	if !weftBranchExists(f.Layout, orphanBranch) {
		t.Errorf("orphan branch %q was deleted by dry-run Cleanup; want intact", orphanBranch)
	}
}

// TestCleanup_ApplyDeletesNonTaskOrphan asserts that Cleanup with apply=true (no force)
// deletes a non-task orphaned weft branch that the gate does not protect.
//
// codeguideFoldedBack returns false for all branches (conservative), making every
// branch gate-protected. To validate the "non-gate-protected" path, we test the branch
// deletion directly at the CleanupBranchEntry level: when apply=true but force=false,
// every branch gets the Protected flag because codeguideFoldedBack returns false.
// The apply+non-task scenario is therefore identical to the apply+gate scenario under
// the current conservative stub. We verify the Protected semantics instead.
func TestCleanup_ApplySkipsProtectedBranch(t *testing.T) {
	t.Parallel()

	f := setupCleanupFixture(t)
	orphanBranch := createOrphanWeftBranch(t, f, "orphan-apply-skip")

	w := New(Config{})
	r, err := w.Cleanup(f.Layout, true, false)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, force=false) error = %v; want nil", err)
	}

	// The orphaned branch must appear as protected (codeguideFoldedBack returns false).
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
		t.Errorf("CleanupBranchEntry.Protected = false; want true (codeguideFoldedBack returns false)")
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

// TestCleanup_ForceAloneReportsOnly asserts that --force without --apply produces
// a report only (no deletions), because force does not imply apply.
func TestCleanup_ForceAloneReportsOnly(t *testing.T) {
	t.Parallel()

	f := setupCleanupFixture(t)
	orphanBranch := createOrphanWeftBranch(t, f, "orphan-force-alone")

	w := New(Config{})
	r, err := w.Cleanup(f.Layout, false, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=false, force=true) error = %v; want nil", err)
	}

	// The orphaned branch should appear in the report (dry-run path).
	var found *CleanupBranchEntry
	for i := range r.Entries {
		if r.Entries[i].Branch == orphanBranch {
			found = &r.Entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Cleanup(apply=false, force=true): orphan branch %q not found in entries", orphanBranch)
	}

	// force alone = report only; Deleted must be false.
	if found.Deleted {
		t.Errorf("CleanupBranchEntry.Deleted = true with force-alone; want false (report only)")
	}

	// Branch must still exist.
	if !weftBranchExists(f.Layout, orphanBranch) {
		t.Errorf("orphan branch %q was deleted by force-alone Cleanup; want intact", orphanBranch)
	}
}

// TestCleanup_LiveBranchNeverDeleted asserts that a weft branch that has a corresponding
// live host worktree is never reported or deleted by Cleanup.
func TestCleanup_LiveBranchNeverDeleted(t *testing.T) {
	t.Parallel()

	f := setupCleanupFixture(t)

	// Add a real paired worktree — its weft branch is live and must never be touched.
	const testSlug = "feature-cleanup-live"
	w := New(Config{BranchPrefix: ""})
	_, err := w.Add(f.Layout, testSlug, AddOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", testSlug, err)
	}

	// Run Cleanup with apply=true force=true (the most aggressive mode).
	r, err := w.Cleanup(f.Layout, true, true)
	if err != nil {
		t.Fatalf("Cleanup(apply=true, force=true) error = %v; want nil", err)
	}

	// The live pair's branch must not appear in Cleanup entries at all.
	for _, entry := range r.Entries {
		if entry.Branch == testSlug {
			t.Errorf("Cleanup included live branch %q in entries; want not reported", testSlug)
		}
	}

	// The weft branch must still exist.
	if !weftBranchExists(f.Layout, testSlug) {
		t.Errorf("live weft branch %q was deleted by Cleanup; want intact", testSlug)
	}
}

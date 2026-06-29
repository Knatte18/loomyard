//go:build integration

// prune_test.go covers the Prune verb: an orphaned/stale pair is reported in dry-run
// and removed under --apply; a live pair is never touched.

package warpengine

import (
	"os"
	"path/filepath"
	"testing"

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

//go:build integration

// reconcile_test.go covers the warp Reconcile repair-and-adopt sweep: missing weft worktree
// with branch present is recreated, broken junction is re-pointed, raw (non-lyx) host
// worktree is adopted dormant, and an unmanaged-branch worktree is reported and untouched.

package warpengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// setupReconcileFixture prepares a CopyPairedLocal fixture with warp config seeded into the
// weft prime and the host _lyx junction created, mirroring the production topology.
func setupReconcileFixture(t *testing.T) lyxtest.PairedFixture {
	t.Helper()

	f := lyxtest.CopyPairedLocal(t)
	slug := filepath.Base(f.Hub)

	// Seed warp config so LoadConfig resolves through the junction.
	lyxtest.SeedConfig(t, f.WeftPrime, map[string]string{
		"warp": ConfigTemplate(),
	})

	// Wire the host _lyx junction to replicate the active production topology.
	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	return f
}

// TestReconcile_MissingWeftWorktreeRecreated asserts that Reconcile recreates a weft worktree
// that was removed while its branch still exists in the weft repo.
func TestReconcile_MissingWeftWorktreeRecreated(t *testing.T) {
	t.Parallel()

	f := setupReconcileFixture(t)
	slug := filepath.Base(f.Hub)
	weftPath := f.Layout.WeftWorktreePath(slug)

	// Use warp add (SkipPush + SkipGit) to create a real paired worktree, then remove
	// only the weft worktree directory (simulating an accidental rm -rf).
	const testSlug = "feature-x"
	w := New(Config{BranchPrefix: ""})
	_, err := w.Add(f.Layout, testSlug, AddOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", testSlug, err)
	}

	featureWeftPath := f.Layout.WeftWorktreePath(testSlug)

	// Verify the weft worktree was created.
	if _, statErr := os.Stat(featureWeftPath); statErr != nil {
		t.Fatalf("pre-condition: weft worktree %s should exist after Add; got %v", featureWeftPath, statErr)
	}

	// Remove the weft worktree directory to simulate accidental deletion.
	// We must first tell git that this worktree is gone so git worktree add can recreate it.
	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "worktree", "remove", "--force", featureWeftPath)

	// Confirm the weft worktree directory is now gone.
	if _, statErr := os.Stat(featureWeftPath); !os.IsNotExist(statErr) {
		t.Fatalf("pre-condition: weft worktree %s should be absent after removal; got %v", featureWeftPath, statErr)
	}

	// Confirm the weft branch still exists (otherwise the recreate path won't be taken).
	// weftBranchExists uses the weft repo root derived from f.Layout.
	if !weftBranchExists(f.Layout, testSlug) {
		t.Fatalf("pre-condition: weft branch %q must exist for recreate path", testSlug)
	}

	// Run Reconcile and assert the weft worktree is recreated.
	r, err := w.Reconcile(f.Layout)
	if err != nil {
		t.Fatalf("Reconcile() error = %v; want nil", err)
	}

	var found *ReconcilePairResult
	for i := range r.Pairs {
		if filepath.Clean(r.Pairs[i].WeftWorktree) == filepath.Clean(featureWeftPath) {
			found = &r.Pairs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Reconcile(): no pair result for weft path %s", featureWeftPath)
	}
	if found.Action != ReconcileActionWeftRecreated {
		t.Errorf("Action = %q; want %q", found.Action, ReconcileActionWeftRecreated)
	}
	if found.Error != "" {
		t.Errorf("Error = %q; want empty (recreate should succeed)", found.Error)
	}

	// Assert the weft worktree directory now exists.
	if _, statErr := os.Stat(featureWeftPath); statErr != nil {
		t.Errorf("weft worktree %s does not exist after Reconcile; want recreated", featureWeftPath)
	}

	// Suppress "declared and not used" for weftPath which is the prime weft, not the feature weft.
	_ = weftPath
}

// TestReconcile_BrokenJunctionRepointed asserts that Reconcile re-points a host _lyx junction
// that was removed (broken/dangling) while the weft worktree is still present.
func TestReconcile_BrokenJunctionRepointed(t *testing.T) {
	t.Parallel()

	f := setupReconcileFixture(t)
	slug := filepath.Base(f.Hub)

	// Break the junction by removing it directly.
	hostLink := f.Layout.HostLyxLink(slug)
	if err := fslink.Remove(hostLink); err != nil {
		t.Fatalf("Remove junction: %v", err)
	}

	// Confirm junction is gone.
	isLink, _ := fslink.IsLink(hostLink)
	if isLink {
		t.Fatalf("pre-condition: junction at %s should be absent after Remove", hostLink)
	}

	w := New(Config{})
	r, err := w.Reconcile(f.Layout)
	if err != nil {
		t.Fatalf("Reconcile() error = %v; want nil", err)
	}

	var found *ReconcilePairResult
	for i := range r.Pairs {
		if filepath.Clean(r.Pairs[i].HostWorktree) == filepath.Clean(f.Hub) {
			found = &r.Pairs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Reconcile(): no pair result for hub worktree %s", f.Hub)
	}
	if found.Action != ReconcileActionJunctionRepointed {
		t.Errorf("Action = %q; want %q", found.Action, ReconcileActionJunctionRepointed)
	}
	if found.Error != "" {
		t.Errorf("Error = %q; want empty (repoint should succeed)", found.Error)
	}

	// JSON-boundary paths must be forward-slash even on Windows (issue #37). Check the
	// raw field value directly -- filepath.Clean would re-normalize forward slashes back
	// to OS-native backslash and silently defeat this assertion.
	if strings.Contains(found.HostWorktree, "\\") {
		t.Errorf("ReconcilePairResult.HostWorktree = %q; want no backslash separators", found.HostWorktree)
	}
	if strings.Contains(found.WeftWorktree, "\\") {
		t.Errorf("ReconcilePairResult.WeftWorktree = %q; want no backslash separators", found.WeftWorktree)
	}

	// Assert the junction was restored.
	isLink, err = fslink.IsLink(hostLink)
	if err != nil || !isLink {
		t.Errorf("junction at %s is not a link after Reconcile; want re-pointed junction", hostLink)
	}
}

// TestReconcile_RawHostWorktreeAdopted asserts that Reconcile creates a dormant weft side
// for a host worktree that was created outside lyx (no _lyx, no weft branch).
func TestReconcile_RawHostWorktreeAdopted(t *testing.T) {
	t.Parallel()

	f := setupReconcileFixture(t)

	// Create a raw host worktree by using plain git (no lyx warp add), so it has no
	// _lyx junction and no corresponding weft branch.
	const rawSlug = "raw-host"
	rawBranch := rawSlug
	rawHostPath := filepath.Join(f.Layout.Hub, rawSlug)

	lyxtest.MustRun(t, f.Hub, "git", "worktree", "add", "-b", rawBranch, rawHostPath)
	t.Cleanup(func() {
		// Best-effort cleanup; TempDir removes the directory.
		lyxtest.MustRun(t, f.Hub, "git", "worktree", "remove", "--force", rawHostPath)
		_, _, _, _ = f.Layout, f.Hub, f.Bare, f.WeftPrime
	})

	// Confirm no _lyx in the raw worktree and no weft branch.
	rawLayout, err := hubgeometry.Resolve(rawHostPath)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(rawHostPath): %v", err)
	}
	if weftBranchExists(rawLayout, rawBranch) {
		t.Fatalf("pre-condition: weft branch %q must not exist for raw-adopt path", rawBranch)
	}
	if !isRawHostWorktree(rawHostPath) {
		t.Fatalf("pre-condition: raw host worktree %s should have no _lyx", rawHostPath)
	}

	w := New(Config{})
	r, err := w.Reconcile(f.Layout)
	if err != nil {
		t.Fatalf("Reconcile() error = %v; want nil", err)
	}

	weftPath := filepath.Join(f.Layout.Hub, rawSlug+"-weft")

	var found *ReconcilePairResult
	for i := range r.Pairs {
		if filepath.Clean(r.Pairs[i].HostWorktree) == filepath.Clean(rawHostPath) {
			found = &r.Pairs[i]
			break
		}
	}
	if found == nil {
		// Print all pairs to aid diagnosis.
		for _, p := range r.Pairs {
			t.Logf("pair: %s → %s (%s)", p.HostWorktree, p.WeftWorktree, p.Action)
		}
		t.Fatalf("Reconcile(): no pair result for raw host worktree %s", rawHostPath)
	}
	if found.Action != ReconcileActionRawAdopted {
		t.Errorf("Action = %q; want %q", found.Action, ReconcileActionRawAdopted)
	}
	if found.Error != "" {
		t.Errorf("Error = %q; want empty (adopt should succeed)", found.Error)
	}

	// The weft worktree must now exist (dormant — no junction).
	if _, statErr := os.Stat(weftPath); statErr != nil {
		t.Errorf("weft worktree %s does not exist after raw-adopt Reconcile; want created dormant", weftPath)
	}

	// The host _lyx must still be absent (dormant = no junction; lyx init wires it).
	lyxPath := filepath.Join(rawHostPath, hubgeometry.LyxDirName)
	if _, statErr := os.Lstat(lyxPath); !os.IsNotExist(statErr) {
		t.Errorf("host _lyx at %s exists after raw-adopt; want absent (lyx init wires the junction)", lyxPath)
	}
}

// TestReconcile_UnmanagedBranchReportedUntouched asserts that Reconcile reports an
// unmanaged-branch worktree without creating any weft side or modifying the host.
//
// An "unmanaged branch" worktree here is one where the host branch already has a
// weft counterpart but the worktree itself is not being managed (simulated by using a
// host worktree with a _lyx placeholder dir so it is not detected as raw, and without
// any weft worktree or branch).
func TestReconcile_UnmanagedBranchReportedUntouched(t *testing.T) {
	t.Parallel()

	f := setupReconcileFixture(t)

	// Create a host worktree with a real _lyx directory (not a junction) so it is not
	// classified as raw by isRawHostWorktree. And ensure no weft branch exists for it.
	const unmanagedSlug = "unmanaged-wt"
	unmanagedBranch := unmanagedSlug
	unmanagedHostPath := filepath.Join(f.Layout.Hub, unmanagedSlug)

	lyxtest.MustRun(t, f.Hub, "git", "worktree", "add", "-b", unmanagedBranch, unmanagedHostPath)
	t.Cleanup(func() {
		lyxtest.MustRun(t, f.Hub, "git", "worktree", "remove", "--force", unmanagedHostPath)
	})

	// Place a real _lyx directory (not a junction) so the worktree is not "raw".
	fakeLyx := filepath.Join(unmanagedHostPath, hubgeometry.LyxDirName)
	if err := os.MkdirAll(fakeLyx, 0o755); err != nil {
		t.Fatalf("MkdirAll fake _lyx: %v", err)
	}

	// Confirm pre-conditions: no weft branch, not raw (has _lyx dir).
	unmanagedLayout, err := hubgeometry.Resolve(unmanagedHostPath)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(unmanagedHostPath): %v", err)
	}
	if weftBranchExists(unmanagedLayout, unmanagedBranch) {
		t.Fatalf("pre-condition: weft branch %q must not exist", unmanagedBranch)
	}
	if isRawHostWorktree(unmanagedHostPath) {
		t.Fatalf("pre-condition: worktree %s should not be raw (has _lyx dir)", unmanagedHostPath)
	}

	weftPath := filepath.Join(f.Layout.Hub, unmanagedSlug+"-weft")

	w := New(Config{})
	r, err := w.Reconcile(f.Layout)
	if err != nil {
		t.Fatalf("Reconcile() error = %v; want nil", err)
	}

	var found *ReconcilePairResult
	for i := range r.Pairs {
		if filepath.Clean(r.Pairs[i].HostWorktree) == filepath.Clean(unmanagedHostPath) {
			found = &r.Pairs[i]
			break
		}
	}
	if found == nil {
		for _, p := range r.Pairs {
			t.Logf("pair: %s → %s (%s)", p.HostWorktree, p.WeftWorktree, p.Action)
		}
		t.Fatalf("Reconcile(): no pair result for unmanaged worktree %s", unmanagedHostPath)
	}
	if found.Action != ReconcileActionUnmanagedReported {
		t.Errorf("Action = %q; want %q", found.Action, ReconcileActionUnmanagedReported)
	}

	// The weft worktree must NOT have been created — reported only, untouched.
	if _, statErr := os.Stat(weftPath); !os.IsNotExist(statErr) {
		t.Errorf("weft worktree %s exists after unmanaged report; want absent (touch nothing)", weftPath)
	}

	// The Detail string must suggest the remediation commands.
	if !strings.Contains(found.Detail, "lyx warp add") && !strings.Contains(found.Detail, "lyx init") {
		t.Errorf("Detail = %q; want reference to 'lyx warp add' or 'lyx init'", found.Detail)
	}
}

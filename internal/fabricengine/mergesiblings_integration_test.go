//go:build integration

// mergesiblings_integration_test.go asserts the sibling-verb disposition table card 13 wires: every
// mutating verb whose write would corrupt or be corrupted by a live merge refuses with the single
// typed, side-free *ErrMergeInProgress while a fabric merge record exists on the pair, Commit
// additionally refuses foreign git-level merge state with no record — pre-empting git's own raw
// "cannot do a partial commit during a merge" — Cleanup needs no new guard because its own structure
// already protects a mid-merge pair, PushWeft and every read-only verb stay unaffected, and every
// guarded verb works again once MergeAbort clears the record.
//
// Package fabricengine_test, reusing newMergePairFixture and its sibling helpers (commitOnBranch,
// setupConflictingDivergence, branchAtCurrentHEAD, currentBranchName) from
// mergein_integration_test.go, and readBranchForTest from healthreason_integration_test.go; shares
// the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// newMergeSiblingsFixture builds a real hubforge pair — deliberately NOT the hub's prime, since
// Topology.Remove refuses the prime slug unconditionally and this file needs Remove's own
// merge-in-progress refusal to be reachable — plus the *fabricengine.Fabric handle open on it and its
// own *lyxcwd.Location, for driving the Topology verbs (Checkout, Remove, Cleanup) this file also
// covers.
func newMergeSiblingsFixture(t *testing.T) (h *hubforge.Hub, f *fabricengine.Fabric, l *lyxcwd.Location, slug string) {
	t.Helper()

	h = hubforge.NewHub(t, ".")
	slug = "merge-siblings-pair"
	hubforge.AddPair(t, h, slug)

	var err error
	l, err = lyxcwd.ResolveWorktree(h.PairWarpWorktree(slug))
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(%s): %v", h.PairWarpWorktree(slug), err)
	}
	f, err = fabricengine.Open(l)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}
	return h, f, l, slug
}

// TestMergeSiblings_Dispositions drives a real conflicted MergeIn on a non-prime pair to establish a
// live recorded merge, then asserts the full sibling-verb disposition table in sequence: every
// mutating verb refuses with *ErrMergeInProgress and mutates nothing, Cleanup succeeds unaffected in
// every apply/force combination and never touches the pair's own weft branch, PushWeft and every
// read-only verb succeed unaffected, and every guarded verb works again once MergeAbort clears the
// record.
func TestMergeSiblings_Dispositions(t *testing.T) {
	h, f, l, slug := newMergeSiblingsFixture(t)
	warpDir := h.PairWarpWorktree(slug)
	weftDir := h.PairWeftSibling(slug)

	// The pair's own branches — captured before MergeIn ever runs, since these (not the "feature"/
	// "feature-weft" merge-source branches) are what Cleanup's existing liveWarpBranches skip
	// protects, and what every other guarded verb must leave untouched.
	pairWarpBranch, err := readBranchForTest(t, warpDir)
	if err != nil {
		t.Fatalf("readBranchForTest(warp) before MergeIn: %v", err)
	}
	pairWeftBranch, err := readBranchForTest(t, weftDir)
	if err != nil {
		t.Fatalf("readBranchForTest(weft) before MergeIn: %v", err)
	}

	setupConflictingDivergence(t, warpDir, "feature", "conflict.txt")
	branchAtCurrentHEAD(t, weftDir, fabricengine.WeftBranchName("feature"))

	preWarpSHA := fabricengine.CurrentSHAForTest(t, warpDir)
	preWeftSHA := fabricengine.CurrentSHAForTest(t, weftDir)

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("MergeIn(feature) produced no conflicts; want the seeded warp conflict")
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || !exists {
		t.Fatalf("MergeRecordExistsForTest() after conflicted MergeIn = (%v, %v); want (true, nil)", exists, err)
	}

	assertHEADsUnchanged := func(t *testing.T, label string) {
		t.Helper()
		if got := fabricengine.CurrentSHAForTest(t, warpDir); got != preWarpSHA {
			t.Errorf("%s: warp HEAD = %q; want unchanged %q", label, got, preWarpSHA)
		}
		if got := fabricengine.CurrentSHAForTest(t, weftDir); got != preWeftSHA {
			t.Errorf("%s: weft HEAD = %q; want unchanged %q", label, got, preWeftSHA)
		}
	}

	t.Run("Commit", func(t *testing.T) {
		commitRes, err := f.Commit(nil, "should be refused", nil, fabricengine.SyncOptions{})
		var refused *fabricengine.ErrMergeInProgress
		if !errors.As(err, &refused) {
			t.Fatalf("Commit() error = %v (%T); want *ErrMergeInProgress", err, err)
		}
		if commitRes.Mutated().Len() != 0 {
			t.Errorf("Commit() mutation record = %+v; want empty", commitRes.Mutated().Entries())
		}
		assertHEADsUnchanged(t, "Commit")
	})

	t.Run("Pull", func(t *testing.T) {
		pullRes, err := f.Pull(fabricengine.SyncOptions{})
		var refused *fabricengine.ErrMergeInProgress
		if !errors.As(err, &refused) {
			t.Fatalf("Pull() error = %v (%T); want *ErrMergeInProgress", err, err)
		}
		if pullRes.Mutated().Len() != 0 {
			t.Errorf("Pull() mutation record = %+v; want empty", pullRes.Mutated().Entries())
		}
		assertHEADsUnchanged(t, "Pull")
	})

	t.Run("Checkout", func(t *testing.T) {
		_, err := h.Topology.Checkout(l, "main")
		var refused *fabricengine.ErrMergeInProgress
		if !errors.As(err, &refused) {
			t.Fatalf("Checkout() error = %v (%T); want *ErrMergeInProgress", err, err)
		}
		if got, err := readBranchForTest(t, warpDir); err != nil || got != pairWarpBranch {
			t.Errorf("warp branch after refused Checkout = (%q, %v); want unchanged %q", got, err, pairWarpBranch)
		}
		if got, err := readBranchForTest(t, weftDir); err != nil || got != pairWeftBranch {
			t.Errorf("weft branch after refused Checkout = (%q, %v); want unchanged %q", got, err, pairWeftBranch)
		}
		assertHEADsUnchanged(t, "Checkout")
	})

	t.Run("RemoveWithoutForce", func(t *testing.T) {
		_, err := h.Topology.Remove(l, slug, false)
		var refused *fabricengine.ErrMergeInProgress
		if !errors.As(err, &refused) {
			t.Fatalf("Remove(force=false) error = %v (%T); want *ErrMergeInProgress", err, err)
		}
		if _, statErr := os.Stat(warpDir); statErr != nil {
			t.Errorf("warp worktree %s missing after refused Remove: %v", warpDir, statErr)
		}
		if _, statErr := os.Stat(weftDir); statErr != nil {
			t.Errorf("weft worktree %s missing after refused Remove: %v", weftDir, statErr)
		}
	})

	t.Run("RemoveWithForce", func(t *testing.T) {
		// force answers dirtiness only, never a live merge record — the gate's own rule (card 13).
		_, err := h.Topology.Remove(l, slug, true)
		var refused *fabricengine.ErrMergeInProgress
		if !errors.As(err, &refused) {
			t.Fatalf("Remove(force=true) error = %v (%T); want *ErrMergeInProgress even with --force", err, err)
		}
		if _, statErr := os.Stat(warpDir); statErr != nil {
			t.Errorf("warp worktree %s missing after refused Remove --force: %v", warpDir, statErr)
		}
		if _, statErr := os.Stat(weftDir); statErr != nil {
			t.Errorf("weft worktree %s missing after refused Remove --force: %v", weftDir, statErr)
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		for _, tt := range []struct {
			name  string
			apply bool
			force bool
		}{
			{"DryRun", false, false},
			{"DryRunForce", false, true},
			{"Apply", true, false},
			{"ApplyForce", true, true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				cleanupRes, err := h.Topology.Cleanup(l, tt.apply, tt.force)
				if err != nil {
					t.Fatalf("Cleanup(apply=%v, force=%v) error = %v", tt.apply, tt.force, err)
				}
				for _, entry := range cleanupRes.Entries {
					if entry.Branch == pairWeftBranch {
						t.Errorf("Cleanup(apply=%v, force=%v) entries = %+v; the mid-merge pair's own weft branch %q must never appear (it is live, skipped before entry construction)", tt.apply, tt.force, cleanupRes.Entries, pairWeftBranch)
					}
				}
				if _, statErr := os.Stat(warpDir); statErr != nil {
					t.Errorf("warp worktree %s missing after Cleanup(apply=%v, force=%v): %v", warpDir, tt.apply, tt.force, statErr)
				}
				if _, statErr := os.Stat(weftDir); statErr != nil {
					t.Errorf("weft worktree %s missing after Cleanup(apply=%v, force=%v): %v", weftDir, tt.apply, tt.force, statErr)
				}
				if got, err := readBranchForTest(t, warpDir); err != nil || got != pairWarpBranch {
					t.Errorf("warp branch after Cleanup(apply=%v, force=%v) = (%q, %v); want unchanged %q", tt.apply, tt.force, got, err, pairWarpBranch)
				}
				if got, err := readBranchForTest(t, weftDir); err != nil || got != pairWeftBranch {
					t.Errorf("weft branch after Cleanup(apply=%v, force=%v) = (%q, %v); want unchanged %q", tt.apply, tt.force, got, err, pairWeftBranch)
				}
				if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || !exists {
					t.Errorf("MergeRecordExistsForTest() after Cleanup(apply=%v, force=%v) = (%v, %v); want (true, nil)", tt.apply, tt.force, exists, err)
				}
			})
		}
	})

	t.Run("PushWeftUnaffected", func(t *testing.T) {
		if _, err := f.PushWeft(fabricengine.SyncOptions{}); err != nil {
			t.Errorf("PushWeft() error = %v; want nil during a live merge record", err)
		}
	})

	t.Run("ReadOnlyVerbsUnaffected", func(t *testing.T) {
		if _, err := f.Status(); err != nil {
			t.Errorf("Status() error = %v; want nil during a live merge record", err)
		}
		if _, err := f.Diff(preWarpSHA); err != nil {
			t.Errorf("Diff(%s) error = %v; want nil during a live merge record", preWarpSHA, err)
		}
		if _, err := h.Topology.List(warpDir); err != nil {
			t.Errorf("List() error = %v; want nil during a live merge record", err)
		}
		if _, err := h.Topology.Status(l); err != nil {
			t.Errorf("Status(pairs) error = %v; want nil during a live merge record", err)
		}
	})

	t.Run("AfterMergeAbort_CommitWorksAgain", func(t *testing.T) {
		if _, err := f.MergeAbort(); err != nil {
			t.Fatalf("MergeAbort() error = %v", err)
		}
		if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
			t.Fatalf("MergeRecordExistsForTest() after MergeAbort = (%v, %v); want (false, nil)", exists, err)
		}

		marker := filepath.Join(warpDir, "post-abort-marker.txt")
		if err := os.WriteFile(marker, []byte("post-abort\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(post-abort-marker.txt): %v", err)
		}
		commitRes, err := f.Commit([]string{"post-abort-marker.txt"}, "post-abort commit", nil, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() after MergeAbort error = %v; want the sibling guard cleared", err)
		}
		if !commitRes.WarpCommitted {
			t.Errorf("Commit() after MergeAbort WarpCommitted = false; want true")
		}
	})
}

// TestMergeSiblings_CommitRefusesForeignMergeState covers Commit's foreign-git-state disposition: a
// plain-git conflicted merge in the warp checkout, driven with no MergeIn involved and so no fabric
// merge record, still refuses with the same typed *ErrMergeInProgress rather than surfacing git's own
// raw "cannot do a partial commit during a merge".
func TestMergeSiblings_CommitRefusesForeignMergeState(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	warpDir := h.PrimeWorktree()

	setupConflictingDivergence(t, warpDir, "foreign-feature", "foreign-conflict.txt")

	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Fatalf("MergeRecordExistsForTest() before any merge = (%v, %v); want (false, nil)", exists, err)
	}

	mergeCmd := exec.Command("git", "merge", "foreign-feature")
	mergeCmd.Dir = warpDir
	_ = mergeCmd.Run() // a conflict is expected here — nonzero exit, deliberately ignored.

	foreign, err := fabricengine.ForeignMergeStatePresentForTest(f)
	if err != nil {
		t.Fatalf("ForeignMergeStatePresentForTest() error = %v", err)
	}
	if !foreign {
		t.Fatal("ForeignMergeStatePresentForTest() = false; want true (a plain-git conflicted merge is present)")
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Fatalf("MergeRecordExistsForTest() after plain-git merge = (%v, %v); want (false, nil) -- no fabric record was ever written", exists, err)
	}

	_, err = f.Commit(nil, "should be refused", nil, fabricengine.SyncOptions{})
	var refused *fabricengine.ErrMergeInProgress
	if !errors.As(err, &refused) {
		t.Fatalf("Commit() error = %v (%T); want *ErrMergeInProgress even though no fabric record exists", err, err)
	}
}

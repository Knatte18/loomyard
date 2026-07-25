// revert_test.go — untagged unit tests for classifyCorrespondence, the pure
// gap-classification core RevertWithWeft's resolution step builds on. No
// git spawn: every case works against a hand-built *corrIndex.

package fabricengine

import (
	"errors"
	"path/filepath"
	"testing"
)

// buildIndexFromEntries returns a corrIndex populated with entries, backed
// by a fresh t.TempDir() file (corrIndex.record always persists — see
// corrindex.go), sorted by WarpSeq via loadCorrIndex's own sort so the
// fixture matches what a real on-disk index would look like once loaded.
// This is still a Tier-1, git-free test — only corrIndex's own file I/O is
// exercised, no git spawn.
func buildIndexFromEntries(t *testing.T, entries []corrEntry) *corrIndex {
	t.Helper()

	path := filepath.Join(t.TempDir(), "corr.json")
	ix, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}
	for _, e := range entries {
		if err := ix.record(e); err != nil {
			t.Fatalf("record(%+v) error = %v", e, err)
		}
	}
	return ix
}

// TestClassifyCorrespondence_ExactHit asserts that an exact index entry for
// targetSHA wins outright, regardless of targetSeq.
func TestClassifyCorrespondence_ExactHit(t *testing.T) {
	ix := buildIndexFromEntries(t, []corrEntry{
		{WarpSHA: "w10", WeftSHA: "f10", WarpSeq: 10},
		{WarpSHA: "w20", WeftSHA: "f20", WarpSeq: 20},
	})

	got, err := classifyCorrespondence(ix, 20, "w20")
	if err != nil {
		t.Fatalf("classifyCorrespondence() error = %v", err)
	}
	if !got.Exact {
		t.Errorf("classifyCorrespondence() Exact = false; want true")
	}
	if got.Entry.WeftSHA != "f20" {
		t.Errorf("classifyCorrespondence() Entry.WeftSHA = %q; want %q", got.Entry.WeftSHA, "f20")
	}
}

// TestClassifyCorrespondence_GapFallsBackToNearestOlder asserts that a
// target with no exact entry falls back to the nearest at-or-before entry
// and reports Exact = false.
func TestClassifyCorrespondence_GapFallsBackToNearestOlder(t *testing.T) {
	ix := buildIndexFromEntries(t, []corrEntry{
		{WarpSHA: "w10", WeftSHA: "f10", WarpSeq: 10},
		{WarpSHA: "w30", WeftSHA: "f30", WarpSeq: 30},
	})

	// "w25" has no recorded entry itself; its WarpSeq (25) sits between the
	// two recorded entries, so the nearest older one (seq 10) is used.
	got, err := classifyCorrespondence(ix, 25, "w25")
	if err != nil {
		t.Fatalf("classifyCorrespondence() error = %v", err)
	}
	if got.Exact {
		t.Errorf("classifyCorrespondence() Exact = true; want false (gap)")
	}
	if got.Entry.WarpSHA != "w10" {
		t.Errorf("classifyCorrespondence() Entry.WarpSHA = %q; want %q (nearest older)", got.Entry.WarpSHA, "w10")
	}
}

// TestClassifyCorrespondence_NoOlderEntryErrors asserts that a target whose
// WarpSeq precedes every recorded entry — and which has no exact entry
// either — returns wrapped ErrNoCorrespondence.
func TestClassifyCorrespondence_NoOlderEntryErrors(t *testing.T) {
	ix := buildIndexFromEntries(t, []corrEntry{
		{WarpSHA: "w10", WeftSHA: "f10", WarpSeq: 10},
	})

	_, err := classifyCorrespondence(ix, 5, "w5")
	if !errors.Is(err, ErrNoCorrespondence) {
		t.Errorf("classifyCorrespondence() error = %v; want errors.Is(err, ErrNoCorrespondence)", err)
	}
}

// TestClassifyCorrespondence_EmptyIndexErrors asserts the degenerate case:
// an entirely empty index always reports ErrNoCorrespondence, exact or not.
func TestClassifyCorrespondence_EmptyIndexErrors(t *testing.T) {
	ix := buildIndexFromEntries(t, nil)

	_, err := classifyCorrespondence(ix, 1, "w1")
	if !errors.Is(err, ErrNoCorrespondence) {
		t.Errorf("classifyCorrespondence() error = %v; want errors.Is(err, ErrNoCorrespondence)", err)
	}
}

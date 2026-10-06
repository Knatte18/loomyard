// revert_test.go — untagged unit tests for classifyCorrespondence, the pure gap-classification core
// resolveRevertTarget's resolution step builds on.
// No git spawn: every case works against a hand-built *corrIndex.

package fabricengine

import (
	"errors"
	"path/filepath"
	"testing"
)

// buildIndexFromEntries returns a corrIndex populated with entries, sorted by
// WarpSeq to match what a real on-disk index would look like. Only corrIndex's
// own file I/O is exercised, no git spawn.
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

// TestClassifyCorrespondence covers an exact index entry for targetSHA (wins outright, regardless
// of targetSeq), a gap (no exact entry: the nearest at-or-before entry is used and Exact is false),
// a target whose WarpSeq precedes every recorded entry (wrapped ErrNoCorrespondence) and an empty
// index (ErrNoCorrespondence, exact or not).
//
//testtiming:keep an exact index hit, a gap falling back to the nearest older entry with Exact false, and the ErrNoCorrespondence cases; coverage of its blocks by other tests does not show an assertion of this
func TestClassifyCorrespondence(t *testing.T) {
	tests := []struct {
		name        string
		entries     []corrEntry
		targetSeq   int
		targetSHA   string
		wantErr     bool
		wantExact   bool
		wantWarpSHA string
	}{
		{
			name: "exact_hit",
			entries: []corrEntry{
				{WarpSHA: "w10", WeftSHA: "f10", WarpSeq: 10},
				{WarpSHA: "w20", WeftSHA: "f20", WarpSeq: 20},
			},
			targetSeq: 20, targetSHA: "w20",
			wantExact: true, wantWarpSHA: "w20",
		},
		{
			// "w25" has no recorded entry itself; its WarpSeq (25) sits between the
			// two recorded entries, so the nearest older one (seq 10) is used.
			name: "gap_falls_back_to_nearest_older",
			entries: []corrEntry{
				{WarpSHA: "w10", WeftSHA: "f10", WarpSeq: 10},
				{WarpSHA: "w30", WeftSHA: "f30", WarpSeq: 30},
			},
			targetSeq: 25, targetSHA: "w25",
			wantExact: false, wantWarpSHA: "w10",
		},
		{
			name:      "no_older_entry_errors",
			entries:   []corrEntry{{WarpSHA: "w10", WeftSHA: "f10", WarpSeq: 10}},
			targetSeq: 5, targetSHA: "w5",
			wantErr: true,
		},
		{
			name:      "empty_index_errors",
			targetSeq: 1, targetSHA: "w1",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := buildIndexFromEntries(t, tt.entries)

			got, err := classifyCorrespondence(ix, tt.targetSeq, tt.targetSHA)
			if tt.wantErr {
				if !errors.Is(err, ErrNoCorrespondence) {
					t.Errorf("classifyCorrespondence() error = %v; want errors.Is(err, ErrNoCorrespondence)", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("classifyCorrespondence() error = %v", err)
			}
			if got.Exact != tt.wantExact {
				t.Errorf("classifyCorrespondence() Exact = %v; want %v", got.Exact, tt.wantExact)
			}
			if got.Entry.WarpSHA != tt.wantWarpSHA {
				t.Errorf("classifyCorrespondence() Entry.WarpSHA = %q; want %q", got.Entry.WarpSHA, tt.wantWarpSHA)
			}
		})
	}
}

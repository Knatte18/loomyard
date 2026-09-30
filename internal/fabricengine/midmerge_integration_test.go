//go:build integration

// midmerge_integration_test.go covers MidMerge against a real hubforge pair, one row per state:
// clean, fabric-parked with and without remaining conflicts, each foreign shape on each side, and a
// pair that cannot be opened at all.

package fabricengine_test

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestMidMerge_CleanPair_None(t *testing.T) {
	h, _, _, _, _, _ := newMergePairFixture(t, ".")

	got, err := fabricengine.MidMerge(h.Location)
	if err != nil {
		t.Fatalf("MidMerge() error = %v", err)
	}
	if got.Kind != fabricengine.MidMergeNone || len(got.Conflicts) != 0 || got.Conflicts == nil {
		t.Errorf("MidMerge() = %+v; want MidMergeNone with empty non-nil Conflicts", got)
	}
}

func TestMidMerge_FabricParkedWithConflicts_Parked(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "clash.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatalf("MergeIn(feature).Conflicts is empty; want a conflict to park on")
	}

	got, err := fabricengine.MidMerge(h.Location)
	if err != nil {
		t.Fatalf("MidMerge() error = %v", err)
	}
	if got.Kind != fabricengine.MidMergeParked {
		t.Errorf("MidMerge().Kind = %v; want MidMergeParked", got.Kind)
	}
	if !reflect.DeepEqual(got.Conflicts, res.Conflicts) {
		t.Errorf("MidMerge().Conflicts = %v; want MergeIn's own %v", got.Conflicts, res.Conflicts)
	}
}

func TestMidMerge_FabricParkedResolved_ParkedNoConflicts(t *testing.T) {
	h, _ := setupResolvedConflictedMergeIn(t)

	got, err := fabricengine.MidMerge(h.Location)
	if err != nil {
		t.Fatalf("MidMerge() error = %v", err)
	}
	if got.Kind != fabricengine.MidMergeParked || len(got.Conflicts) != 0 || got.Conflicts == nil {
		t.Errorf("MidMerge() = %+v; want MidMergeParked with empty non-nil Conflicts", got)
	}
}

func TestMidMerge_ForeignState_EverySideAndShape(t *testing.T) {
	tests := []struct {
		name   string
		onWeft bool
		shape  foreignShape
	}{
		{name: "WarpConflictedIndexAndMergeHead", onWeft: false, shape: shapeConflictedAndMergeHead},
		{name: "WarpMergeHeadOnlyResolvedNotConcluded", onWeft: false, shape: shapeMergeHeadOnly},
		{name: "WarpConflictedIndexOnlyFromSquash", onWeft: false, shape: shapeConflictedIndexOnly},
		{name: "WeftConflictedIndexAndMergeHead", onWeft: true, shape: shapeConflictedAndMergeHead},
		{name: "WeftMergeHeadOnlyResolvedNotConcluded", onWeft: true, shape: shapeMergeHeadOnly},
		{name: "WeftConflictedIndexOnlyFromSquash", onWeft: true, shape: shapeConflictedIndexOnly},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, _, _, _, _ := newMergePairFixture(t, ".")

			dir := h.PrimeWorktree()
			branch, conflictPath := "other", "plain-conflict.txt"
			if tt.onWeft {
				dir = h.PrimeWeft()
				branch, conflictPath = "other-weft", "_lyx/plain-conflict.txt"
			}

			setupConflictingDivergence(t, dir, branch, conflictPath)
			if tt.shape == shapeConflictedIndexOnly {
				gitMergeSquashAllowConflict(t, dir, branch)
			} else {
				gitMergeAllowConflict(t, dir, branch)
			}
			if tt.shape == shapeMergeHeadOnly {
				gitkit.MustRun(t, dir, "git", "add", "--", conflictPath)
			}

			want := []string{}
			if tt.shape != shapeMergeHeadOnly {
				want = []string{conflictPath}
			}

			got, err := fabricengine.MidMerge(h.Location)
			if err != nil {
				t.Fatalf("MidMerge() error = %v", err)
			}
			if got.Kind != fabricengine.MidMergeForeign {
				t.Errorf("MidMerge().Kind = %v; want MidMergeForeign", got.Kind)
			}
			if !reflect.DeepEqual(got.Conflicts, want) {
				t.Errorf("MidMerge().Conflicts = %#v; want %#v", got.Conflicts, want)
			}
		})
	}
}

func TestMidMerge_UnopenablePair_Errors(t *testing.T) {
	l := &lyxcwd.Location{RepoName: "ghost", HubPath: t.TempDir(), WorktreeName: "ghost", AnchorRel: "."}

	if _, err := fabricengine.MidMerge(l); err == nil {
		t.Fatalf("MidMerge() over an unopenable pair error = nil; want an error, never MidMergeNone")
	}
}

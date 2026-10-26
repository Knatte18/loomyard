//go:build tier0

// finalize_land_test.go covers Finalize's land check against a scripted blocker seam: each blocker's own ways forward, and the runs that land.

package landingshed

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// TestFinalize_LandCheck pins that a beam's landing halts while a member or a forked pair remains under it, each blocker naming its own ways forward and the halt ending in the resume, and that no halt names the landing of a member or merges anything.
// An empty list, no seam and a parent already holding the beam's work land as before.
func TestFinalize_LandCheck(t *testing.T) {
	tests := []struct {
		name        string
		nilSeam     bool
		blockers    []LandBlocker
		readErr     error
		holds       bool
		wantOutcome shedengine.Outcome
		// wantInReason names what a halting reason must carry, behind LandHaltPrefix.
		wantInReason []string
		// wantNotInReason names what a halting reason must not carry.
		wantNotInReason []string
	}{
		{
			name:         "a member with a pair names the cancel in the parent worktree, then the board move",
			blockers:     []LandBlocker{{Slug: "m1", HasPair: true}},
			wantOutcome:  shedengine.Stuck,
			wantInReason: []string{"m1", "`lyx batten cancel m1 --remove`, run in the parent worktree base", "then move its `project` off the beam with `lyx board upsert`", "`draft` member"},
		},
		{
			name:         "a member in flight with no pair here names the cancel on the machine holding its pair, then the board move",
			blockers:     []LandBlocker{{Slug: "m2", Running: true}},
			wantOutcome:  shedengine.Stuck,
			wantInReason: []string{"m2", "run in flight", "`lyx batten cancel m2 --remove` on the machine holding its pair", "`lyx batten status m2`", "then move its `project` off the beam with `lyx board upsert`"},
		},
		{
			name:            "a member with no run status names the board move alone",
			blockers:        []LandBlocker{{Slug: "m3"}},
			wantOutcome:     shedengine.Stuck,
			wantInReason:    []string{"m3", "no run status", "move its `project` off the beam with `lyx board upsert`"},
			wantNotInReason: []string{"batten cancel"},
		},
		{
			name:            "a forked pair names its cancel in the parent worktree",
			blockers:        []LandBlocker{{Slug: "c1", Pair: true}},
			wantOutcome:     shedengine.Stuck,
			wantInReason:    []string{"the pair c1", "`lyx batten cancel c1 --remove`, run in the parent worktree base"},
			wantNotInReason: []string{"lyx board upsert"},
		},
		{
			name:         "every blocker is named",
			blockers:     []LandBlocker{{Slug: "m1", HasPair: true}, {Slug: "c1", Pair: true}},
			wantOutcome:  shedengine.Stuck,
			wantInReason: []string{"m1", "c1"},
		},
		{
			name:         "a failed read halts naming the cause and the resume",
			readErr:      errors.New("board unreadable"),
			wantOutcome:  shedengine.Stuck,
			wantInReason: []string{"board unreadable"},
		},
		{name: "an empty list lands", wantOutcome: shedengine.Done},
		{name: "no seam lands", nilSeam: true, wantOutcome: shedengine.Done},
		{
			name:        "a parent already holding the work lands past the blockers",
			blockers:    []LandBlocker{{Slug: "m1", HasPair: true}},
			holds:       true,
			wantOutcome: shedengine.Done,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Slug = "beam-x"
			deps.ParentWorktree = "base"
			if !tt.nilSeam {
				deps.LandBlockers = func() ([]LandBlocker, error) { return tt.blockers, tt.readErr }
			}
			deps.ParentHoldsTaskWork = func() (bool, error) { return tt.holds, nil }
			deps.Reconcile = func() (fabricengine.OriginReport, error) { return fabricengine.OriginReport{}, nil }
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
			fz := &Finalize{deps: deps, resolver: &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}, parentOpener: func() (parentMerger, error) { return merger, nil }}

			ptr := shedfake.RequireOutcome(t, fz, tt.wantOutcome)
			if tt.wantOutcome == shedengine.Done {
				if len(merger.calls) != 1 {
					t.Errorf("parent-side merge calls = %d; want 1", len(merger.calls))
				}
				return
			}
			reason := requireFinalizeReason(t, ptr)
			if !strings.HasPrefix(reason, LandHaltPrefix) {
				t.Errorf("reason = %q; want it to start with %q", reason, LandHaltPrefix)
			}
			for _, want := range append(tt.wantInReason, "`lyx loom resume` in the beam's worktree beam-x") {
				if !strings.Contains(reason, want) {
					t.Errorf("reason = %q; want it to carry %q", reason, want)
				}
			}
			for _, unwanted := range append(tt.wantNotInReason, "lyx batten run") {
				if strings.Contains(reason, unwanted) {
					t.Errorf("reason = %q; want it to omit %q", reason, unwanted)
				}
			}
			if !ptr.Halt {
				t.Error("OutputPointer.Halt = false; want a land halt to block the run")
			}
			if len(merger.calls) != 0 {
				t.Errorf("parent-side merge calls = %d; want 0", len(merger.calls))
			}
		})
	}
}

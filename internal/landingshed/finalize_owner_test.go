//go:build tier0

// finalize_owner_test.go covers Finalize's owner checks against scripted reconcile reports and owner seams.

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

// TestFinalize_OwnerChecks pins that a landing happens only when this machine owns the run and its parent, and that each halt names its owners and the adopt commands that move them.
// A parent already holding the task's work lands whatever the owners and origin say, and a Finalize halted then resumed meets both checks again.
func TestFinalize_OwnerChecks(t *testing.T) {
	ownedElsewhere := func(owner string) *fabricengine.OriginReport {
		return &fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: "me", State: fabricengine.EntryOwnedElsewhere, Owner: owner}}}
	}
	active := &fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: "me", State: fabricengine.EntryActive}}}
	unreachable := &fabricengine.OriginReport{Unreachable: true}
	here := &ParentOwnership{Here: true}
	primeElsewhere := &ParentOwnership{Owner: "laptop"}
	pairElsewhere := &ParentOwnership{Slug: "base", Owner: "laptop"}

	tests := []struct {
		name        string
		report      *fabricengine.OriginReport
		holds       bool
		holdsErr    error
		parent      *ParentOwnership
		parentErr   error
		wantOutcome shedengine.Outcome
		// wantInReason names what a halting reason must carry, behind OwnerHaltPrefix.
		wantInReason []string
	}{
		{name: "run and parent owned here lands", report: active, parent: here, wantOutcome: shedengine.Done},
		{
			name: "run owned elsewhere halts naming the owner and keep-local", report: ownedElsewhere("desk"), parent: here, wantOutcome: shedengine.Stuck,
			wantInReason: []string{`machine "desk"`, `"lyx adopt me --keep-local"`},
		},
		{
			name: "prime owned elsewhere halts naming its owner and both adopt forms", report: active, parent: primeElsewhere, wantOutcome: shedengine.Stuck,
			wantInReason: []string{"the prime", `machine "laptop"`, `"lyx adopt --prime"`, `"lyx adopt me" on machine "laptop"`, `"--keep-local" and "--drop-local"`},
		},
		{
			name: "parent pair owned elsewhere halts naming its slug in the adopt form", report: active, parent: pairElsewhere, wantOutcome: shedengine.Stuck,
			wantInReason: []string{`the parent pair "base"`, `machine "laptop"`, `"lyx adopt base"`, `"lyx adopt me" on machine "laptop"`},
		},
		{
			name: "run and prime both elsewhere name both owners and both forms", report: ownedElsewhere("desk"), parent: primeElsewhere, wantOutcome: shedengine.Stuck,
			wantInReason: []string{`machine "desk"`, `machine "laptop"`, `"lyx adopt me" on machine "laptop"`, `"lyx adopt --prime me --keep-local"`},
		},
		{
			name: "run and parent pair both elsewhere name the parent slug in the run's form", report: ownedElsewhere("desk"), parent: pairElsewhere, wantOutcome: shedengine.Stuck,
			wantInReason: []string{`machine "desk"`, `machine "laptop"`, `"lyx adopt base me --keep-local"`},
		},
		{
			name: "an unreachable origin halts naming the re-run", report: unreachable, parent: here, wantOutcome: shedengine.Stuck,
			wantInReason: []string{"origin was unreachable", `"lyx loom resume"`, "once origin answers"},
		},
		{name: "a parent holding the work lands under an owned-elsewhere report", report: ownedElsewhere("desk"), holds: true, parent: primeElsewhere, wantOutcome: shedengine.Done},
		{name: "a parent holding the work lands under an unreachable report", report: unreachable, holds: true, parent: here, wantOutcome: shedengine.Done},
		{
			name: "a failed read of whether the parent holds the work halts", report: active, holdsErr: errors.New("no origin record"), parent: here, wantOutcome: shedengine.Stuck,
			wantInReason: []string{"no origin record", `"lyx loom resume"`},
		},
		{
			name: "a failed read of the parent's owner halts", report: active, parentErr: errors.New("board unreadable"), wantOutcome: shedengine.Stuck,
			wantInReason: []string{"board unreadable", `"lyx loom resume"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Slug = "me"
			deps.Reconcile = func() (fabricengine.OriginReport, error) { return *tt.report, nil }
			deps.ParentHoldsTaskWork = func() (bool, error) { return tt.holds, tt.holdsErr }
			deps.ParentOwner = func() (ParentOwnership, error) {
				if tt.parent == nil {
					return ParentOwnership{}, tt.parentErr
				}
				return *tt.parent, tt.parentErr
			}
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
			if !strings.HasPrefix(reason, OwnerHaltPrefix) {
				t.Errorf("reason = %q; want it to start with %q", reason, OwnerHaltPrefix)
			}
			for _, want := range tt.wantInReason {
				if !strings.Contains(reason, want) {
					t.Errorf("reason = %q; want it to carry %q", reason, want)
				}
			}
			if !ptr.Halt {
				t.Error("OutputPointer.Halt = false; want an owner halt to block the run")
			}
			if len(merger.calls) != 0 {
				t.Errorf("parent-side merge calls = %d; want 0", len(merger.calls))
			}
		})
	}

	t.Run("a halted Finalize resumed with the parent not holding the work meets both checks again", func(t *testing.T) {
		report, parent := *ownedElsewhere("desk"), *pairElsewhere
		deps := newTestDeps(t)
		deps.Slug = "me"
		deps.Reconcile = func() (fabricengine.OriginReport, error) { return report, nil }
		deps.ParentHoldsTaskWork = func() (bool, error) { return false, nil }
		deps.ParentOwner = func() (ParentOwnership, error) { return parent, nil }
		merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
		fz := &Finalize{deps: deps, resolver: &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}, parentOpener: func() (parentMerger, error) { return merger, nil }}

		shedfake.RequireOutcome(t, fz, shedengine.Stuck)
		report = *active
		ptr := shedfake.RequireOutcome(t, fz, shedengine.Stuck)
		if reason := requireFinalizeReason(t, ptr); !strings.Contains(reason, `the parent pair "base"`) {
			t.Errorf("reason = %q; want the resumed run to meet the parent check", reason)
		}
		parent = *here
		shedfake.RequireOutcome(t, fz, shedengine.Done)
		if len(merger.calls) != 1 {
			t.Errorf("parent-side merge calls = %d; want 1", len(merger.calls))
		}
	})
}

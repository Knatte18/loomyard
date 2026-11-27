// finalize_owner_test.go covers Finalize's owner checks against scripted reconcile reports and owner seams.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestFinalize_OwnerChecks pins that a landing happens only when this machine owns the run and its parent,
// and that each halt names its owners and the adopt commands that move them.
// A parent already holding the task's work lands whatever the owners and origin say,
// and a Finalize halted then resumed meets both checks again.
func TestFinalize_OwnerChecks(t *testing.T) {
	tiergate.Require(t, 0)
	ownedElsewhere := func(owner string) *fabriclayout.OriginReport {
		return &fabriclayout.OriginReport{Entries: []fabriclayout.EntryOutcome{{Slug: "me", State: fabriclayout.EntryOwnedElsewhere, Owner: owner}}}
	}
	active := &fabriclayout.OriginReport{Entries: []fabriclayout.EntryOutcome{{Slug: "me", State: fabriclayout.EntryActive}}}
	unreachable := &fabriclayout.OriginReport{Unreachable: true}
	here := &ParentOwnership{Here: true}
	primeElsewhere := &ParentOwnership{Owner: "laptop"}
	pairElsewhere := &ParentOwnership{Slug: "base", Owner: "laptop"}

	tests := []struct {
		name      string
		report    *fabriclayout.OriginReport
		holds     bool
		holdsErr  error
		parent    *ParentOwnership
		parentErr error
		// subject is the run's own adopt subject; an empty subject reads as its slug.
		subject     string
		wantOutcome shedengine.Outcome
		// wantInReason names what a halting reason must carry, behind OwnerHaltPrefix.
		wantInReason []string
	}{
		{name: "run and parent owned here lands", report: active, parent: here, wantOutcome: shedengine.Done},
		{
			name: "run owned elsewhere halts naming the owner and the adopt of the run's beam", report: ownedElsewhere("desk"), parent: here, subject: "beam", wantOutcome: shedengine.Stuck,
			wantInReason: []string{`machine "desk"`, `"lym adopt beam"`},
		},
		{
			name: "run with no adopt subject names its slug", report: ownedElsewhere("desk"), parent: here, wantOutcome: shedengine.Stuck,
			wantInReason: []string{`machine "desk"`, `"lym adopt me"`},
		},
		{
			name: "prime owned elsewhere halts naming its owner and both adopt forms with root", report: active, parent: primeElsewhere, subject: "root", wantOutcome: shedengine.Stuck,
			wantInReason: []string{"the prime", `machine "laptop"`, `"lym adopt root"`, `"lym adopt root" on machine "laptop"`, "once the check is green"},
		},
		{
			name: "parent pair owned elsewhere halts naming its slug in the adopt form", report: active, parent: pairElsewhere, subject: "base", wantOutcome: shedengine.Stuck,
			wantInReason: []string{`the parent pair "base"`, `machine "laptop"`, `"lym adopt base"`, `"lym adopt base" on machine "laptop"`},
		},
		{
			name: "root run and prime both elsewhere name both owners and the one adopt of root", report: ownedElsewhere("desk"), parent: primeElsewhere, subject: "root", wantOutcome: shedengine.Stuck,
			wantInReason: []string{`machine "desk"`, `machine "laptop"`, `"lym adopt root" on machine "laptop"`, `or run "lym adopt root" here`},
		},
		{
			name: "beam run and prime both elsewhere name root and the beam in the run's form", report: ownedElsewhere("desk"), parent: primeElsewhere, subject: "me", wantOutcome: shedengine.Stuck,
			wantInReason: []string{`machine "desk"`, `machine "laptop"`, `"lym adopt me" on machine "laptop"`, `"lym adopt root me"`},
		},
		{
			name: "member run and its beam both elsewhere name the beam once", report: ownedElsewhere("desk"), parent: pairElsewhere, subject: "base", wantOutcome: shedengine.Stuck,
			wantInReason: []string{`machine "desk"`, `machine "laptop"`, `or run "lym adopt base" here`},
		},
		{
			name: "beam run and parent pair both elsewhere name the parent slug in the run's form", report: ownedElsewhere("desk"), parent: pairElsewhere, subject: "me", wantOutcome: shedengine.Stuck,
			wantInReason: []string{`machine "desk"`, `machine "laptop"`, `"lym adopt base me"`},
		},
		{
			name: "an unreachable origin halts naming the re-run", report: unreachable, parent: here, wantOutcome: shedengine.Stuck,
			wantInReason: []string{"origin was unreachable", `"lym loom resume"`, "once origin answers"},
		},
		{name: "a parent holding the work lands under an owned-elsewhere report", report: ownedElsewhere("desk"), holds: true, parent: primeElsewhere, wantOutcome: shedengine.Done},
		{name: "a parent holding the work lands under an unreachable report", report: unreachable, holds: true, parent: here, wantOutcome: shedengine.Done},
		{
			name: "a failed read of whether the parent holds the work halts", report: active, holdsErr: errors.New("no origin record"), parent: here, wantOutcome: shedengine.Stuck,
			wantInReason: []string{"no origin record", `"lym loom resume"`},
		},
		{
			name: "a failed read of the parent's owner halts", report: active, parentErr: errors.New("board unreadable"), wantOutcome: shedengine.Stuck,
			wantInReason: []string{"board unreadable", `"lym loom resume"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Slug = "me"
			if tt.subject != "" {
				deps.AdoptSubject = func() (string, error) { return tt.subject, nil }
			}
			deps.Reconcile = func() (fabriclayout.OriginReport, error) { return *tt.report, nil }
			deps.ParentHoldsTaskWork = func() (bool, error) { return tt.holds, tt.holdsErr }
			deps.ParentOwner = func() (ParentOwnership, error) {
				if tt.parent == nil {
					return ParentOwnership{}, tt.parentErr
				}
				return *tt.parent, tt.parentErr
			}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabriclayout.MergeResult{Committed: true}}}}
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

	type handOverRun struct {
		fz        *Finalize
		merger    *recordingParentMerger
		handOvers []string
	}
	newHandOverRun := func(t *testing.T, report *fabriclayout.OriginReport, configure func(*Deps)) *handOverRun {
		t.Helper()
		run := &handOverRun{merger: &recordingParentMerger{results: []mergeCallResult{{result: fabriclayout.MergeResult{Committed: true}}}}}
		deps := newTestDeps(t)
		deps.Slug = "me"
		deps.AdoptSubject = func() (string, error) { return "beam", nil }
		deps.Reconcile = func() (fabriclayout.OriginReport, error) { return *report, nil }
		deps.ParentOwner = func() (ParentOwnership, error) { return *here, nil }
		deps.HandOver = func(owner, subject string) error {
			run.handOvers = append(run.handOvers, owner+"/"+subject)
			return fmt.Errorf("%w: machine %q holds the run %q now", fabricengine.ErrHandedOver, owner, "me")
		}
		configure(&deps)
		run.fz = &Finalize{deps: deps, resolver: &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}, parentOpener: func() (parentMerger, error) { return run.merger, nil }}
		return run
	}
	requireHandOver := func(t *testing.T, run *handOverRun, ptr shedengine.OutputPointer) {
		t.Helper()
		if want := []string{"desk/beam"}; !slices.Equal(run.handOvers, want) {
			t.Errorf("hand-overs = %v; want %v", run.handOvers, want)
		}
		reason := requireFinalizeReason(t, ptr)
		if !strings.HasPrefix(reason, OwnerHaltPrefix) || !strings.Contains(reason, `machine "desk"`) || !strings.Contains(reason, "the landing finishes on that machine") {
			t.Errorf("reason = %q; want an owner halt naming machine %q and where the landing finishes", reason, "desk")
		}
		if !ptr.Halt {
			t.Error("OutputPointer.Halt = false; want a hand-over to block the run")
		}
	}

	t.Run("a run the reconcile reads owned elsewhere is handed over and merges nothing", func(t *testing.T) {
		run := newHandOverRun(t, ownedElsewhere("desk"), func(*Deps) {})

		ptr := shedfake.RequireOutcome(t, run.fz, shedengine.Stuck)

		requireHandOver(t, run, ptr)
		if len(run.merger.calls) != 0 {
			t.Errorf("parent-side merge calls = %d; want 0", len(run.merger.calls))
		}
	})

	t.Run("an owner read after the merge is handed over before the parent push", func(t *testing.T) {
		run := newHandOverRun(t, active, func(d *Deps) {
			d.OwnedElsewhere = func() (string, string, bool, error) { return "desk", "beam", true, nil }
		})

		ptr := shedfake.RequireOutcome(t, run.fz, shedengine.Stuck)

		requireHandOver(t, run, ptr)
		if len(run.merger.calls) != 1 || len(run.merger.pushCalls) != 0 {
			t.Errorf("parent-side merge calls = %d and parent pushes = %d; want 1 and 0", len(run.merger.calls), len(run.merger.pushCalls))
		}
	})

	t.Run("an unreadable owner returns its error as a transient failure with nothing pushed", func(t *testing.T) {
		readErr := errors.New("origin unreachable")
		run := newHandOverRun(t, active, func(d *Deps) {
			d.OwnedElsewhere = func() (string, string, bool, error) { return "", "", false, readErr }
		})

		_, _, err := run.fz.Call(context.Background())

		if !errors.Is(err, readErr) {
			t.Fatalf("Call() error = %v; want it to wrap %v", err, readErr)
		}
		if shedengine.TransientOf(err) == "" {
			t.Errorf("Call() error = %v; want a transient mark, so the driver re-steps it", err)
		}
		if len(run.handOvers) != 0 || len(run.merger.pushCalls) != 0 {
			t.Errorf("hand-overs = %v and parent pushes = %d; want none", run.handOvers, len(run.merger.pushCalls))
		}
	})

	t.Run("a halted Finalize resumed with the parent not holding the work meets both checks again", func(t *testing.T) {
		report, parent := *ownedElsewhere("desk"), *pairElsewhere
		deps := newTestDeps(t)
		deps.Slug = "me"
		deps.Reconcile = func() (fabriclayout.OriginReport, error) { return report, nil }
		deps.ParentHoldsTaskWork = func() (bool, error) { return false, nil }
		deps.ParentOwner = func() (ParentOwnership, error) { return parent, nil }
		merger := &recordingParentMerger{results: []mergeCallResult{{result: fabriclayout.MergeResult{Committed: true}}}}
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

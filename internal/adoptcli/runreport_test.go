// runreport_test.go pins the states of the per-run report of an adopt over the fake world: one row for each state, the precedence between them, the sort and a read that fails.

package adoptcli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fronttask"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRunReports(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	run := boardengine.RunStatus("running", "Code-Write")
	draft := boardengine.StatusDraft
	hold := pauserecord.Entry{Scope: "t1", Caller: pauserecord.HoldCaller}

	// each row builds the world and the input of one run, `t1` under the beam `alpha` this machine owns, and names the report it wants.
	tests := []struct {
		name  string
		setup func(w *world, in *reportInput)
		want  runReport
		// wantInLine are the texts the report's line holds.
		wantInLine []string
	}{
		{
			name: "OwnedElsewhereNamesItsOwner",
			setup: func(w *world, in *reportInput) {
				in.Owners.Entries["t1"] = boardengine.OwnerEntry{IsTask: true}
				w.parents["t1"] = "beta"
				in.Before = []string{"t1"}
			},
			want:       runReport{Slug: "t1", State: runStateOwnedElsewhere, OwnedBy: there},
			wantInLine: []string{"`lym adopt beta`"},
		},
		{
			name: "LandedWhenItsEntryIsDone",
			setup: func(_ *world, in *reportInput) {
				in.Owners.Entries["t1"] = boardengine.OwnerEntry{IsTask: true, Done: true}
				in.Before = []string{"t1"}
			},
			want:       runReport{Slug: "t1", State: runStateLanded},
			wantInLine: []string{"done or gone"},
		},
		{
			name:  "LandedWhenItsEntryIsPruned",
			setup: func(_ *world, in *reportInput) { in.Before = []string{"t1"} },
			want:  runReport{Slug: "t1", State: runStateLanded},
		},
		{
			name: "LandedOutranksHeld",
			setup: func(w *world, in *reportInput) {
				in.Owners.Entries["t1"] = boardengine.OwnerEntry{IsTask: true, Done: true}
				in.Before = []string{"t1"}
				w.record.Live = []pauserecord.Entry{hold}
				in.Held = map[string][]string{"alpha": {"t1"}}
			},
			want: runReport{Slug: "t1", State: runStateLanded},
		},
		{
			name: "HeldFailedCheckForAFailedRecordWhenNoCheckStartedNow",
			setup: func(w *world, in *reportInput) {
				heldUnderAlpha(w, in)
				w.checkStates["alpha"] = checkFailed
			},
			want:       runReport{Slug: "t1", State: runStateHeldFailedCheck},
			wantInLine: []string{"failed", "`lym adopt alpha`"},
		},
		{
			name: "HeldFailedCheckForARunningRecordWhoseProcessIsGone",
			setup: func(w *world, in *reportInput) {
				heldUnderAlpha(w, in)
				w.checkStates["alpha"] = checkStateEnded
			},
			want:       runReport{Slug: "t1", State: runStateHeldFailedCheck},
			wantInLine: []string{"ended without a verdict"},
		},
		{
			name: "WaitingForCheckWhenThisCallStartedTheSubjectsCheckOverAFailedRecord",
			setup: func(w *world, in *reportInput) {
				heldUnderAlpha(w, in)
				w.checkStates["alpha"] = checkFailed
				in.Started = map[string]bool{"alpha": true}
			},
			want: runReport{Slug: "t1", State: runStateWaitingForCheck},
		},
		{
			name: "WaitingForCheckNamesTheRowOfAHeldRun",
			setup: func(w *world, in *reportInput) {
				heldUnderAlpha(w, in)
				w.runs["t1"] = shedengine.Status{State: shedengine.StatePaused, CurrentProducer: "Code-Bouncer"}
			},
			want:       runReport{Slug: "t1", State: runStateWaitingForCheck, Row: "Code-Bouncer"},
			wantInLine: []string{"t1: waiting for the adopt check at Code-Bouncer"},
		},
		{
			name:  "WaitingForCheckHasNoRowForAHeldReleasedTask",
			setup: heldUnderAlpha,
			want:  runReport{Slug: "t1", State: runStateWaitingForCheck, Line: "t1: waiting for the adopt check"},
		},
		{
			name: "PausedNamesTheCaller",
			setup: func(w *world, in *reportInput) {
				heldUnderAlpha(w, in)
				w.record.Live = []pauserecord.Entry{{Scope: "t1", Caller: "ly:orch"}}
			},
			want:       runReport{Slug: "t1", State: runStatePaused, Caller: "ly:orch"},
			wantInLine: []string{"`lym tender resume t1`"},
		},
		{
			name: "HaltedForBlockedWithItsReason",
			setup: func(w *world, in *reportInput) {
				ownedUnderAlpha(w, in)
				w.runs["t1"] = shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Finalize", Error: "owner check: parent elsewhere"}
			},
			want:       runReport{Slug: "t1", State: runStateHalted, Row: "Finalize"},
			wantInLine: []string{"halted at Finalize", "owner check: parent elsewhere"},
		},
		{
			name: "HaltedForFailed",
			setup: func(w *world, in *reportInput) {
				ownedUnderAlpha(w, in)
				w.runs["t1"] = shedengine.Status{State: shedengine.StateFailed, CurrentProducer: "Code-Write", Error: "boom"}
			},
			want: runReport{Slug: "t1", State: runStateHalted, Row: "Code-Write"},
		},
		{
			name: "HaltedForAwaitingWithNoAdmittedEscalation",
			setup: func(w *world, in *reportInput) {
				ownedUnderAlpha(w, in)
				w.runs["t1"] = shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: "Code-Bouncer", PauseReason: "waiting for the operator"}
			},
			want:       runReport{Slug: "t1", State: runStateHalted, Row: "Code-Bouncer"},
			wantInLine: []string{"waiting for the operator"},
		},
		{
			name: "AwaitingWithAnAdmittedEscalationReadsRunningHere",
			setup: func(w *world, in *reportInput) {
				ownedUnderAlpha(w, in)
				w.runs["t1"] = shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: "Code-Bouncer"}
				w.escalation["t1"] = true
			},
			want: runReport{Slug: "t1", State: runStateRunningHere, Row: "Code-Bouncer"},
		},
		{
			name: "HaltedFixTaskStoppedOnAMachineFailureNamesTheRerun",
			setup: func(w *world, in *reportInput) {
				ownedUnderAlpha(w, in)
				in.Tasks = []boardengine.Task{{Slug: "t1", Kind: boardengine.KindTask, Front: true, OnLand: fronttask.ActionAdoptCheck, Project: "alpha", Status: &run}}
				w.runs["t1"] = shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Code-Write", Error: "the machine lacks a tool"}
			},
			want:       runReport{Slug: "t1", State: runStateHalted, Row: "Code-Write"},
			wantInLine: []string{"the machine lacks a tool", "`lym adopt alpha`"},
		},
		{
			name: "EndedNamesTheStatusAHandCancelLeft",
			setup: func(_ *world, in *reportInput) {
				in.Owners.Entries["t1"] = boardengine.OwnerEntry{IsTask: true}
				in.Tasks = []boardengine.Task{{Slug: "t1", Kind: boardengine.KindTask, Status: &draft}}
				in.Before = []string{"t1"}
			},
			want:       runReport{Slug: "t1", State: runStateEnded},
			wantInLine: []string{"draft"},
		},
		{
			name: "RunningHereForARunThisMachineOwns",
			setup: func(w *world, in *reportInput) {
				ownedUnderAlpha(w, in)
				w.runs["t1"] = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Code-Write"}
			},
			want: runReport{Slug: "t1", State: runStateRunningHere, Row: "Code-Write", Line: "t1: running here at Code-Write"},
		},
		{
			name: "RunningHereForAnUnstartedFixTaskWithNoRow",
			setup: func(_ *world, in *reportInput) {
				in.Owners.Entries["t1"] = boardengine.OwnerEntry{IsTask: true}
				in.Tasks = []boardengine.Task{{Slug: "t1", Kind: boardengine.KindTask, Front: true, OnLand: fronttask.ActionAdoptCheck, Project: "alpha"}}
				in.Subjects = []string{"alpha"}
			},
			want: runReport{Slug: "t1", State: runStateRunningHere, Line: "t1: running here"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld()
			in := reportInput{
				Machine: here,
				Owners:  boardengine.Owners{Prime: here, Entries: map[string]boardengine.OwnerEntry{"alpha": {Owner: here, IsProject: true, Beam: true}, "beta": {Owner: there, IsProject: true, Beam: true}}},
			}
			tt.setup(w, &in)

			reports, err := runReports(w.deps(), in)
			if err != nil || len(reports) != 1 {
				t.Fatalf("runReports() = %+v, %v; want one report", reports, err)
			}
			got := reports[0]
			line := got.Line
			if tt.want.Line == "" {
				got.Line = ""
			}
			if got != tt.want {
				t.Errorf("runReports() = %+v; want %+v", got, tt.want)
			}
			for _, text := range tt.wantInLine {
				if !strings.Contains(line, text) {
					t.Errorf("line = %q; want it to hold %q", line, text)
				}
			}
		})
	}

	t.Run("ReportsAreSortedBySlug", func(t *testing.T) {
		t.Parallel()
		w := newWorld()
		in := reportInput{Machine: here, Owners: boardengine.Owners{Prime: here, Entries: map[string]boardengine.OwnerEntry{"b": {IsTask: true}, "a": {IsTask: true}, "c": {IsTask: true}}}, Before: []string{"c", "a"}, Moved: []string{"b", "a"}}

		reports, err := runReports(w.deps(), in)
		var slugs []string
		for _, report := range reports {
			slugs = append(slugs, report.Slug)
		}
		if err != nil || !slices.Equal(slugs, []string{"a", "b", "c"}) {
			t.Errorf("runReports() slugs = %q, %v; want a, b, c once each", slugs, err)
		}
	})

	t.Run("AReadThatFailsReturnsItsErrorAndNoReport", func(t *testing.T) {
		t.Parallel()
		w := newWorld()
		deps := w.deps()
		deps.RunRecord = func(string) (shedengine.Status, bool, error) {
			return shedengine.Status{}, false, errors.New("status unreadable")
		}

		reports, err := runReports(deps, reportInput{Machine: here, Owners: w.owners, Moved: []string{"alpha"}})
		if err == nil || !strings.Contains(err.Error(), "status unreadable") || len(reports) != 0 {
			t.Errorf("runReports() = %+v, %v; want the error and no report", reports, err)
		}
	})
}

// ownedUnderAlpha makes `t1` a task of the beam `alpha`, which this machine owns, so t1 reads owned here.
func ownedUnderAlpha(w *world, in *reportInput) {
	in.Owners.Entries["t1"] = boardengine.OwnerEntry{IsTask: true}
	w.parents["t1"] = "alpha"
	in.Moved = []string{"t1"}
}

// heldUnderAlpha makes `t1` a task of alpha held for alpha's adopt check by a standing hold entry.
func heldUnderAlpha(w *world, in *reportInput) {
	ownedUnderAlpha(w, in)
	w.record.Live = []pauserecord.Entry{{Scope: "t1", Caller: pauserecord.HoldCaller}}
	in.Held = map[string][]string{"alpha": {"t1"}}
}

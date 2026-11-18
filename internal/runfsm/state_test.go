// state_test.go pins the run state derivation and the board display mapping of a status transition.

package runfsm_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// battenWith returns a found batten record in state with the cancel and pause intents given.
func battenWith(state shedengine.State, cancel *shedengine.CancelIntent, pause *shedengine.PauseIntent) shedengine.Status {
	return shedengine.Status{State: state, Cancel: cancel, PauseIntent: pause}
}

func TestDerive(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	finishedKeep := &shedengine.CancelIntent{Finish: &shedengine.CancelFinish{Remove: false}}
	finishedRemove := &shedengine.CancelIntent{Finish: &shedengine.CancelFinish{Remove: true}}
	unfinished := &shedengine.CancelIntent{Machine: "neve"}
	pauseIntent := &shedengine.PauseIntent{By: "orch"}
	child := func(state shedengine.State) shedengine.Status { return shedengine.Status{State: state} }

	tests := []struct {
		name     string
		records  runfsm.Records
		want     runfsm.State
		wantKept bool
	}{
		{"Landed", runfsm.Records{BattenFound: true, Batten: battenWith(shedengine.StateDone, nil, nil)}, runfsm.Landed, false},
		{"LandedFromBoardWithNoBatten", runfsm.Records{BoardDone: true}, runfsm.Landed, false},
		{"CancelledKeepsPair", runfsm.Records{BattenFound: true, TaskBranchOnOrigin: true, Batten: battenWith(shedengine.StateCancelled, finishedKeep, nil)}, runfsm.Cancelled, true},
		{"CancelledRemovedPair", runfsm.Records{BattenFound: true, TaskBranchOnOrigin: true, Batten: battenWith(shedengine.StateCancelled, finishedRemove, nil)}, runfsm.Cancelled, false},
		{"Cancelling", runfsm.Records{BattenFound: true, Batten: battenWith(shedengine.StateRunning, unfinished, nil)}, runfsm.Cancelling, false},
		{"PausedByIntent", runfsm.Records{BattenFound: true, Batten: battenWith(shedengine.StateRunning, nil, pauseIntent)}, runfsm.Paused, false},
		{"PausedByBattenShedState", runfsm.Records{BattenFound: true, Batten: battenWith(shedengine.StatePaused, nil, nil)}, runfsm.Paused, false},
		{"PausedByChildShedState", runfsm.Records{BattenFound: true, ChildFound: true, Child: child(shedengine.StatePaused)}, runfsm.Paused, false},
		{"HaltedBlocked", runfsm.Records{BattenFound: true, ChildFound: true, Child: child(shedengine.StateBlocked)}, runfsm.Halted, false},
		{"HaltedFailed", runfsm.Records{BattenFound: true, ChildFound: true, Child: child(shedengine.StateFailed)}, runfsm.Halted, false},
		{"HaltedAwaiting", runfsm.Records{BattenFound: true, ChildFound: true, Child: child(shedengine.StateAwaiting)}, runfsm.Halted, false},
		{"Landing", runfsm.Records{BattenFound: true, ChildLanding: true}, runfsm.Landing, false},
		{"Running", runfsm.Records{BattenFound: true, BattenStarted: true}, runfsm.Running, false},
		{"Starting", runfsm.Records{BattenFound: true}, runfsm.Starting, false},
		{"Queued", runfsm.Records{Released: true}, runfsm.Queued, false},
		{"NoRunForADraft", runfsm.Records{}, runfsm.NoRun, false},

		{"LandedOverCancelled", runfsm.Records{BattenFound: true, TaskBranchOnOrigin: true, Batten: battenWith(shedengine.StateDone, finishedKeep, nil)}, runfsm.Landed, false},
		{"CancelledOverPaused", runfsm.Records{BattenFound: true, TaskBranchOnOrigin: true, Batten: battenWith(shedengine.StateCancelled, finishedRemove, pauseIntent)}, runfsm.Cancelled, false},
		{"CancellingOverPaused", runfsm.Records{BattenFound: true, Batten: battenWith(shedengine.StateRunning, unfinished, pauseIntent)}, runfsm.Cancelling, false},
		{"PausedOverHalted", runfsm.Records{BattenFound: true, ChildFound: true, Batten: battenWith(shedengine.StatePaused, nil, nil), Child: child(shedengine.StateBlocked)}, runfsm.Paused, false},
		{"HaltedOverLanding", runfsm.Records{BattenFound: true, ChildFound: true, ChildLanding: true, Child: child(shedengine.StateFailed)}, runfsm.Halted, false},
		{"LandingOverRunning", runfsm.Records{BattenFound: true, BattenStarted: true, ChildLanding: true}, runfsm.Landing, false},
		{"RunningOverStarting", runfsm.Records{BattenFound: true, BattenStarted: true}, runfsm.Running, false},
		{"QueuedOverNoRun", runfsm.Records{Released: true}, runfsm.Queued, false},

		{"CancelledRecordWithTaskBranchGoneReadsQueued", runfsm.Records{Released: true, BattenFound: true, Batten: battenWith(shedengine.StateCancelled, finishedRemove, nil)}, runfsm.Queued, false},
		{"CancelledRecordWithTaskBranchOnOriginReadsCancelled", runfsm.Records{Released: true, BattenFound: true, TaskBranchOnOrigin: true, Batten: battenWith(shedengine.StateCancelled, finishedKeep, nil)}, runfsm.Cancelled, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := runfsm.Derive(tt.records)
			want := runfsm.Reading{State: tt.want, PairKept: tt.wantKept}
			if got != want {
				t.Errorf("Derive() = %+v; want %+v", got, want)
			}
		})
	}
}

func TestTransitionState(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name             string
		state            shedengine.State
		started, landing bool
		want             runfsm.State
	}{
		{"Paused", shedengine.StatePaused, true, true, runfsm.Paused},
		{"Blocked", shedengine.StateBlocked, false, false, runfsm.Halted},
		{"Failed", shedengine.StateFailed, false, false, runfsm.Halted},
		{"Awaiting", shedengine.StateAwaiting, false, false, runfsm.Halted},
		{"Landing", shedengine.StateRunning, true, true, runfsm.Landing},
		{"Started", shedengine.StateRunning, true, false, runfsm.Running},
		{"Neither", shedengine.StateRunning, false, false, runfsm.Starting},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := runfsm.TransitionState(tt.state, tt.started, tt.landing); got != tt.want {
				t.Errorf("TransitionState(%q, %v, %v) = %q; want %q", tt.state, tt.started, tt.landing, got, tt.want)
			}
		})
	}
}

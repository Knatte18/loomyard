// looplock_test.go drives the loop's parent lock hooks over a temporary lock store.

package loomcli

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

const lockedBranch = "main"

// loopOwner is the loop process of the run the hook tests lock for.
var loopOwner = parentlock.Owner{Slug: "task", Pair: "/hub/task", RunID: "run-1", PID: 100, StartTime: "100-start"}

// lockedStore returns a store holding lockedBranch's lock for owner.
func lockedStore(t *testing.T, owner parentlock.Owner) *parentlock.Store {
	t.Helper()
	store := &parentlock.Store{Dir: t.TempDir()}
	attempt, err := store.TryAcquire(lockedBranch, owner)
	if err != nil || !attempt.Granted {
		t.Fatalf("TryAcquire() = %+v, %v; want a granted lock", attempt, err)
	}
	return store
}

// lockOwnerOf returns the owner of lockedBranch's record, and whether the record exists.
func lockOwnerOf(t *testing.T, store *parentlock.Store) (parentlock.Owner, bool) {
	t.Helper()
	record, found, err := store.Read(lockedBranch)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	return record.Owner, found
}

// TestLoopLockExit asserts the exit hook releases the loop's record at every stop, hands it to a live batten at an awaiting PR-Gate, and leaves a record of another process alone.
func TestLoopLockExit(t *testing.T) {
	t.Parallel()
	battenPID := os.Getpid()
	battenStart, _ := proc.StartTime(battenPID)
	handedOff := loopOwner
	handedOff.PID, handedOff.StartTime = battenPID, battenStart
	otherProcess := loopOwner
	otherProcess.PID, otherProcess.StartTime = 200, "200-start"
	watched := func() (int, bool) { return battenPID, true }
	unwatched := func() (int, bool) { return 0, false }

	tests := []struct {
		name      string
		recordOf  parentlock.Owner
		stop      shedverbs.LoopStop
		status    shedengine.Status
		batten    func() (int, bool)
		wantOwner parentlock.Owner
		wantFound bool
	}{
		{"blocked", loopOwner, shedverbs.LoopStopHalted, shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Webster"}, watched, parentlock.Owner{}, false},
		{"failed", loopOwner, shedverbs.LoopStopError, shedengine.Status{State: shedengine.StateFailed, CurrentProducer: "Publish"}, watched, parentlock.Owner{}, false},
		{"paused", loopOwner, shedverbs.LoopStopHalted, shedengine.Status{State: shedengine.StatePaused, CurrentProducer: "Publish"}, watched, parentlock.Owner{}, false},
		{"stop condition", loopOwner, shedverbs.LoopStopCondition, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Publish"}, watched, parentlock.Owner{}, false},
		{"done", loopOwner, shedverbs.LoopStopHalted, shedengine.Status{State: shedengine.StateDone, CurrentProducer: "Finalize"}, watched, parentlock.Owner{}, false},
		{"awaiting at Describe", loopOwner, shedverbs.LoopStopHalted, shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: "Describe"}, watched, parentlock.Owner{}, false},
		{"awaiting at PR-Gate hands off to a live batten", loopOwner, shedverbs.LoopStopHalted, shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: loomshed.NamePRGate}, watched, handedOff, true},
		{"awaiting at PR-Gate releases when no batten watches", loopOwner, shedverbs.LoopStopHalted, shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: loomshed.NamePRGate}, unwatched, parentlock.Owner{}, false},
		{"another process of the run keeps its record", otherProcess, shedverbs.LoopStopHalted, shedengine.Status{State: shedengine.StateDone, CurrentProducer: "Finalize"}, watched, otherProcess, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := lockedStore(t, tt.recordOf)

			loopLockExit(store, func() parentlock.Owner { return loopOwner }, tt.batten)(tt.stop, tt.status)

			gotOwner, gotFound := lockOwnerOf(t, store)
			if gotFound != tt.wantFound || gotOwner != tt.wantOwner {
				t.Errorf("record after exit = %+v, found %v; want %+v, found %v", gotOwner, gotFound, tt.wantOwner, tt.wantFound)
			}
		})
	}
}

// TestLoopIdleRelease asserts the wake hook releases the loop's record once every agent has waited past the bound with none busy, and keeps it below the bound, beside a busy agent and on a failed read.
func TestLoopIdleRelease(t *testing.T) {
	t.Parallel()
	const bound = 10 * time.Minute
	waiting := []parentlock.AgentState{{Name: "writer", Waiting: true}}
	busyBeside := []parentlock.AgentState{{Name: "writer", Waiting: true}, {Name: "fixer", Busy: true}}

	tests := []struct {
		name    string
		states  []parentlock.AgentState
		readErr error
		wakeAt  []time.Duration
		// covered reports whether a pause covers the run at the nth wake; nil never covers it.
		covered      func(wake int) bool
		wantReleased bool
	}{
		{"released once the wait passes the bound", waiting, nil, []time.Duration{0, bound}, nil, true},
		{"kept below the bound", waiting, nil, []time.Duration{0, bound - time.Second}, nil, false},
		{"kept beside a busy agent", busyBeside, nil, []time.Duration{0, bound}, nil, false},
		{"kept on a failed read", waiting, errors.New("unreadable"), []time.Duration{0, bound}, nil, false},
		{"kept while every wake is covered", waiting, nil, []time.Duration{0, bound, 2 * bound}, func(int) bool { return true }, false},
		{"kept until the bound passes again after the resume", waiting, nil, []time.Duration{0, bound, bound + bound/2}, func(wake int) bool { return wake < 2 }, false},
		{"released once the bound passed again after the resume", waiting, nil, []time.Duration{0, bound, bound + bound/2, bound + bound/2 + bound}, func(wake int) bool { return wake < 2 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := lockedStore(t, loopOwner)
			var now time.Duration
			var wakeNumber int
			wake := idleReleaseOn(store, func() parentlock.Owner { return loopOwner },
				func() ([]parentlock.AgentState, error) { return tt.states, tt.readErr },
				bound, func() time.Duration { return now },
				func() bool { return tt.covered != nil && tt.covered(wakeNumber) })

			for i, at := range tt.wakeAt {
				now, wakeNumber = at, i
				wake()
			}

			if _, found := lockOwnerOf(t, store); found == tt.wantReleased {
				t.Errorf("record found after the wakes = %v; want %v", found, !tt.wantReleased)
			}
		})
	}
}

// looplock_test.go drives the loop's parent lock hooks over a temporary lock store.

package loomcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
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

// countLogLines counts the lines of log at level that contain text.
func countLogLines(log, level, text string) int {
	count := 0
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, "level="+level) && strings.Contains(line, text) {
			count++
		}
	}
	return count
}

// TestIdleReleaseLogLevels asserts the wake hook logs a repeating list or agent read failure once at Warn, Debug while it lasts and Info at recovery, and logs a release at Info.
// It is serial: it captures the process-global logger.
func TestIdleReleaseLogLevels(t *testing.T) {
	const bound = 10 * time.Minute
	waiting := []parentlock.AgentState{{Name: "writer", Waiting: true}}
	type logLine struct {
		level, text string
		want        int
	}

	tests := []struct {
		name string
		// brokenListWakes is how many leading wakes find the store's directory unreadable.
		brokenListWakes int
		// failingAgentReads is how many leading agent reads fail.
		failingAgentReads int
		wakeAt            []time.Duration
		wantLines         []logLine
		wantHeld          bool
	}{
		{
			name: "a list failure warns once and logs its recovery", brokenListWakes: 3,
			wakeAt: []time.Duration{0, 0, 0, 0}, wantHeld: true,
			wantLines: []logLine{
				{"WARN", "could not list the parent lock records", 1},
				{"DEBUG", "could not list the parent lock records", 2},
				{"INFO", "the parent lock records list again", 1},
			},
		},
		{
			name: "an agent read failure warns once and logs its recovery", failingAgentReads: 3,
			wakeAt: []time.Duration{0, 0, 0, 0}, wantHeld: true,
			wantLines: []logLine{
				{"WARN", "could not read the agents' states", 1},
				{"DEBUG", "could not read the agents' states", 2},
				{"INFO", "read again for the parent lock's idle rule", 1},
			},
		},
		{
			name:   "a release past the bound logs at Info",
			wakeAt: []time.Duration{0, bound},
			wantLines: []logLine{
				{"INFO", "released the parent lock after every agent waited past the idle bound", 1},
				{"WARN", "", 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.CaptureVerbose(t)
			logger.SetVerbosity(2)
			store := lockedStore(t, loopOwner)
			goodDir := store.Dir
			brokenDir := filepath.Join(t.TempDir(), "not-a-directory")
			if err := os.WriteFile(brokenDir, nil, 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			var now time.Duration
			var agentReads int
			wake := idleReleaseOn(store, func() parentlock.Owner { return loopOwner },
				func() ([]parentlock.AgentState, error) {
					agentReads++
					if agentReads <= tt.failingAgentReads {
						return nil, errors.New("unreadable")
					}
					return waiting, nil
				},
				bound, func() time.Duration { return now },
				func() bool { return false })

			for i, at := range tt.wakeAt {
				now = at
				store.Dir = goodDir
				if i < tt.brokenListWakes {
					store.Dir = brokenDir
				}
				wake()
			}

			for _, line := range tt.wantLines {
				if got := countLogLines(buf.String(), line.level, line.text); got != line.want {
					t.Errorf("%s lines containing %q = %d; want %d\n%s", line.level, line.text, got, line.want, buf.String())
				}
			}
			store.Dir = goodDir
			if _, found := lockOwnerOf(t, store); found != tt.wantHeld {
				t.Errorf("record found after the wakes = %v; want %v", found, tt.wantHeld)
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

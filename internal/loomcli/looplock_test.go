// looplock_test.go drives the loop's parent lock hooks over a temporary lock store.

package loomcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/verifytree"
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

// TestLoopLockExit asserts the exit hook releases the loop's record at every stop, an awaiting one included, and leaves a record of another process alone.
func TestLoopLockExit(t *testing.T) {
	t.Parallel()
	otherProcess := loopOwner
	otherProcess.PID, otherProcess.StartTime = 200, "200-start"

	tests := []struct {
		name      string
		recordOf  parentlock.Owner
		stop      shedverbs.LoopStop
		status    shedengine.Status
		wantOwner parentlock.Owner
		wantFound bool
	}{
		{"blocked", loopOwner, shedverbs.LoopStopHalted, shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Webster"}, parentlock.Owner{}, false},
		{"failed", loopOwner, shedverbs.LoopStopError, shedengine.Status{State: shedengine.StateFailed, CurrentProducer: "Publish"}, parentlock.Owner{}, false},
		{"paused", loopOwner, shedverbs.LoopStopHalted, shedengine.Status{State: shedengine.StatePaused, CurrentProducer: "Publish"}, parentlock.Owner{}, false},
		{"stop condition", loopOwner, shedverbs.LoopStopCondition, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Publish"}, parentlock.Owner{}, false},
		{"done", loopOwner, shedverbs.LoopStopHalted, shedengine.Status{State: shedengine.StateDone, CurrentProducer: "Finalize"}, parentlock.Owner{}, false},
		{"awaiting at Describe", loopOwner, shedverbs.LoopStopHalted, shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: "Describe"}, parentlock.Owner{}, false},
		{"another process of the run keeps its record", otherProcess, shedverbs.LoopStopHalted, shedengine.Status{State: shedengine.StateDone, CurrentProducer: "Finalize"}, otherProcess, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := lockedStore(t, tt.recordOf)

			loopLockExit(store, func() parentlock.Owner { return loopOwner })(tt.stop, tt.status)

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
				func() bool { return false },
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

// TestFinalizeKeepsLock asserts the check holds the lock while the run's status names Finalize running or a verify marker of a live process stands, and on a read that fails, and holds nothing for any other status or no file at all.
func TestFinalizeKeepsLock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// status is the producer and state the status file holds; an empty producer writes no file.
		producer string
		state    shedengine.State
		// corruptStatus writes bytes that do not decode in place of the status file.
		corruptStatus bool
		// marker is the bytes of the verify marker; empty writes none.
		marker string
		want   bool
	}{
		{name: "no status and no marker holds nothing"},
		{name: "Finalize running holds it", producer: loomshed.NameFinalize, state: shedengine.StateRunning, want: true},
		{name: "Finalize blocked holds nothing", producer: loomshed.NameFinalize, state: shedengine.StateBlocked},
		{name: "another row running holds nothing", producer: loomshed.NameMergeIn, state: shedengine.StateRunning},
		{name: "a live verify marker holds it", producer: loomshed.NameMergeIn, state: shedengine.StateRunning, marker: "site: landing\npid: %d\n", want: true},
		{name: "a verify marker of a dead process holds nothing", producer: loomshed.NameMergeIn, state: shedengine.StateRunning, marker: "site: landing\npid: 2147483646\n"},
		{name: "a malformed marker holds it", producer: loomshed.NameMergeIn, state: shedengine.StateRunning, marker: "pid: [", want: true},
		{name: "an undecodable status holds it", corruptStatus: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			statusPath := filepath.Join(dir, "status.json")
			statusLockPath := filepath.Join(dir, "status.lock")
			verifyDir := filepath.Join(dir, "verify")
			if err := os.MkdirAll(verifyDir, 0o755); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			switch {
			case tt.corruptStatus:
				if err := os.WriteFile(statusPath, []byte("{"), 0o644); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			case tt.producer != "":
				err := state.UpdateJSON(statusPath, statusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
					cur.CurrentProducer = tt.producer
					cur.State = tt.state
					return cur, nil
				})
				if err != nil {
					t.Fatalf("seed status: %v", err)
				}
			}
			if tt.marker != "" {
				body := tt.marker
				if strings.Contains(body, "%d") {
					body = fmt.Sprintf(body, os.Getpid())
				}
				if err := os.WriteFile(verifytree.NewPaths("", verifyDir).Marker, []byte(body), 0o644); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			}

			if got := finalizeKeepsLock(statusPath, statusLockPath, verifyDir)(); got != tt.want {
				t.Errorf("finalizeKeepsLock()() = %v; want %v", got, tt.want)
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
		covered func(wake int) bool
		// keep reports whether a landing in flight holds the lock at the nth wake; nil never holds it.
		keep         func(wake int) bool
		wantReleased bool
	}{
		{"released once the wait passes the bound", waiting, nil, []time.Duration{0, bound}, nil, nil, true},
		{"kept below the bound", waiting, nil, []time.Duration{0, bound - time.Second}, nil, nil, false},
		{"kept beside a busy agent", busyBeside, nil, []time.Duration{0, bound}, nil, nil, false},
		{"kept on a failed read", waiting, errors.New("unreadable"), []time.Duration{0, bound}, nil, nil, false},
		{"kept while every wake is covered", waiting, nil, []time.Duration{0, bound, 2 * bound}, func(int) bool { return true }, nil, false},
		{"kept until the bound passes again after the resume", waiting, nil, []time.Duration{0, bound, bound + bound/2}, func(wake int) bool { return wake < 2 }, nil, false},
		{"released once the bound passed again after the resume", waiting, nil, []time.Duration{0, bound, bound + bound/2, bound + bound/2 + bound}, func(wake int) bool { return wake < 2 }, nil, true},
		{"kept while a landing holds it past the bound", waiting, nil, []time.Duration{0, bound, 2 * bound}, nil, func(int) bool { return true }, false},
		{"released once the bound passed again after the landing", waiting, nil, []time.Duration{0, bound, bound + bound/2, bound + bound/2 + bound}, nil, func(wake int) bool { return wake < 2 }, true},
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
				func() bool { return tt.covered != nil && tt.covered(wakeNumber) },
				func() bool { return tt.keep != nil && tt.keep(wakeNumber) })

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

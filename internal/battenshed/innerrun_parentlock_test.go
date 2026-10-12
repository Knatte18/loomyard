// innerrun_parentlock_test.go covers the inner-run watch's parent lock duties over a fake clock and fake seams:
// keeping or releasing the lock of the child's run on every probe check, and resuming a child awaiting at the gate row whose parent moved.

package battenshed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

const (
	lockGateRow  = "PR-Gate"
	lockSelfPID  = 4242
	lockLoopPID  = 100
	lockBound    = 10 * time.Minute
	lockCheckGap = time.Minute
)

// lockHarness holds the fake child, the fake lock seams of the InnerRun wait and what they recorded.
// Every check advances the fake clock by lockCheckGap and is a probe check.
type lockHarness struct {
	t     *testing.T
	clock *fakeClock

	status   shedengine.Status
	decision *ChildDecision
	record   *parentlock.Record
	released []parentlock.Record

	agents     []parentlock.AgentState
	agentsErr  error
	moved      bool
	movedErr   error
	movedCalls int
	spawns     *int
	notices    []string

	producer shedengine.ShedProducer
}

func handedRecord() *parentlock.Record {
	return &parentlock.Record{Branch: "main", Owner: parentlock.Owner{Slug: "myslug", Pair: "/pair", RunID: "run-1", PID: lockSelfPID, StartTime: "self-start"}}
}

func loopRecord() *parentlock.Record {
	return &parentlock.Record{Branch: "main", Owner: parentlock.Owner{Slug: "myslug", Pair: "/pair", RunID: "run-1", PID: lockLoopPID, StartTime: "loop-start"}}
}

// newLockHarness builds a producer over a child at status, with the parent lock seams wired to the harness.
func newLockHarness(t *testing.T, status shedengine.Status) *lockHarness {
	t.Helper()
	h := &lockHarness{t: t, clock: &fakeClock{}, status: status}
	started := h.clock.Now()
	_, spawns, deps := newInnerRunDeps(t, nil, nil, []statusResult{{found: true}}, h.clock)
	h.spawns = spawns
	deps.NoticeProbe = lockCheckGap
	deps.Awake = func() time.Duration { return h.clock.Now().Sub(started) }
	deps.ReadStatus = func(string, string) (shedengine.Status, bool, error) { return h.status, true, nil }
	deps.ReadDecision = func() (ChildDecision, bool, error) {
		if h.decision == nil {
			return ChildDecision{}, false, nil
		}
		return *h.decision, true, nil
	}
	deps.PairDir = func() (string, error) { return "/pair", nil }
	deps.Notify = func(_ context.Context, line string) (bool, error) {
		h.notices = append(h.notices, line)
		return true, nil
	}
	deps.ParentLock = &ChildParentLock{
		Read: func() (parentlock.Record, bool, error) {
			if h.record == nil {
				return parentlock.Record{}, false, nil
			}
			return *h.record, true, nil
		},
		Release: func(record parentlock.Record) (bool, error) {
			if h.record == nil || h.record.Owner != record.Owner {
				return false, nil
			}
			h.released = append(h.released, record)
			h.record = nil
			return true, nil
		},
		Agents:    func() ([]parentlock.AgentState, error) { return h.agents, h.agentsErr },
		SelfPID:   lockSelfPID,
		IdleBound: lockBound,
		ParentMoved: func() (bool, error) {
			h.movedCalls++
			return h.moved, h.movedErr
		},
		GateRow: lockGateRow,
	}
	h.producer = NewInnerRun("innerrun", "myslug", deps, lockCheckGap, t.TempDir(), testGrace)
	return h
}

// runChecks runs one Call that ends at check n, the pause the fake batten status raises.
func (h *lockHarness) runChecks(n int) {
	h.t.Helper()
	h.clock.pauseAtSleep = n
	shedfake.CallOK(h.t, h.producer)
}

func awaitingAt(row string) shedengine.Status {
	return shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: row, Error: "waiting on review"}
}

// TestInnerRun_ParentLockKeepsOrReleasesAHandedRecord asserts a record handed to batten is kept while the child awaits at the gate row within the idle bound, awaits a decision to resume on, or runs.
// It asserts the record is released once the awaiting outlasts the bound with no decision, or when the child is in any other state or awaits at another row.
func TestInnerRun_ParentLockKeepsOrReleasesAHandedRecord(t *testing.T) {
	t.Parallel()
	decided := &ChildDecision{Kind: DecisionApprove, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}
	tests := []struct {
		name         string
		status       shedengine.Status
		decision     *ChildDecision
		checks       int
		wantReleased bool
	}{
		{"kept within the bound while awaiting", awaitingAt(lockGateRow), nil, 10, false},
		{"released once the awaiting outlasts the bound", awaitingAt(lockGateRow), nil, 11, true},
		{"kept past the bound while a decision is resumed on", awaitingAt(lockGateRow), decided, 30, false},
		{"kept while the resumed child runs", shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Finalize"}, nil, 30, false},
		{"released when the resumed child halted", shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Finalize"}, nil, 1, true},
		{"released when the child awaits at another row", awaitingAt("Plan-Bouncer"), nil, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newLockHarness(t, tt.status)
			h.decision = tt.decision
			h.record = handedRecord()

			h.runChecks(tt.checks)

			if got := len(h.released) == 1; got != tt.wantReleased || (h.record == nil) != tt.wantReleased {
				t.Errorf("released = %v (record left: %v); want released %v", h.released, h.record != nil, tt.wantReleased)
			}
		})
	}
}

// TestInnerRun_ParentLockReleasesAStalledLoopsRecord asserts a record owned by the child's loop is released once every agent has waited for the idle bound with none busy.
// It asserts the record is kept when the owner changed meanwhile, beside a busy agent and on a failed agent read.
func TestInnerRun_ParentLockReleasesAStalledLoopsRecord(t *testing.T) {
	t.Parallel()
	waiting := []parentlock.AgentState{{Name: "writer", Waiting: true}}
	busyBeside := []parentlock.AgentState{{Name: "writer", Waiting: true}, {Name: "fixer", Busy: true}}
	tests := []struct {
		name         string
		agents       []parentlock.AgentState
		agentsErr    error
		newOwnerAt   int
		wantReleased bool
	}{
		{name: "released once the agents waited the bound", agents: waiting, wantReleased: true},
		{name: "kept when the owner changed meanwhile", agents: waiting, newOwnerAt: 6},
		{name: "kept beside a busy agent", agents: busyBeside},
		{name: "kept on a failed read", agents: waiting, agentsErr: errors.New("unreadable")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newLockHarness(t, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Webster"})
			h.record = loopRecord()
			h.agents, h.agentsErr = tt.agents, tt.agentsErr
			if tt.newOwnerAt > 0 {
				h.clock.onSleep = func(call int) {
					if call == tt.newOwnerAt {
						h.record.Owner.PID, h.record.Owner.StartTime = lockLoopPID+1, "fresh-loop-start"
					}
				}
			}

			h.runChecks(11)

			if got := len(h.released) == 1; got != tt.wantReleased {
				t.Errorf("released = %v; want released %v", h.released, tt.wantReleased)
			}
		})
	}
}

// TestInnerRun_StaleBaseResumesAnAwaitingChildOnce asserts a child awaiting at the gate row with no decision is resumed once when its parent moved, with no awaiting notice,
// and that it is left alone when the parent did not move, when the move cannot be read, when a decision is pending and when it awaits at another row, where the parent is not even read.
func TestInnerRun_StaleBaseResumesAnAwaitingChildOnce(t *testing.T) {
	t.Parallel()
	decided := &ChildDecision{Kind: DecisionReject, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}
	tests := []struct {
		name       string
		status     shedengine.Status
		decision   *ChildDecision
		moved      bool
		movedErr   error
		wantSpawns int
		wantRead   bool
		wantNotice bool
	}{
		{name: "moved parent resumes the child once", status: awaitingAt(lockGateRow), moved: true, wantSpawns: 1, wantRead: true},
		{name: "unmoved parent leaves the child awaiting and notices it", status: awaitingAt(lockGateRow), wantRead: true, wantNotice: true},
		{name: "unreadable move resumes nothing", status: awaitingAt(lockGateRow), moved: true, movedErr: errors.New("fetch failed"), wantRead: true, wantNotice: true},
		{name: "pending decision is resumed on, not as stale", status: awaitingAt(lockGateRow), decision: decided, moved: true, wantSpawns: 1, wantRead: false, wantNotice: true},
		{name: "another awaiting row is never read", status: awaitingAt("Plan-Bouncer"), moved: true, wantNotice: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newLockHarness(t, tt.status)
			h.decision = tt.decision
			h.moved, h.movedErr = tt.moved, tt.movedErr

			h.runChecks(4)

			if *h.spawns != tt.wantSpawns {
				t.Errorf("Spawn calls = %d; want %d", *h.spawns, tt.wantSpawns)
			}
			if (h.movedCalls > 0) != tt.wantRead {
				t.Errorf("ParentMoved calls = %d; want read %v", h.movedCalls, tt.wantRead)
			}
			if (len(h.notices) > 0) != tt.wantNotice {
				t.Errorf("notices = %q; want a notice: %v", h.notices, tt.wantNotice)
			}
		})
	}
}

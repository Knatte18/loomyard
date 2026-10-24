// innerrun_parentlock_test.go covers the inner-run watch's parent lock duties over a fake clock and fake seams:
// keeping or releasing the lock of the child's run on every probe check.

package battenshed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

const (
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

	agents    []parentlock.AgentState
	agentsErr error
	// keeps is the fake KeepsLock's answer to its nth call, counted from one; nil never keeps the lock.
	keeps      func(n int) (bool, error)
	keepsCalls int
	spawns     *int
	notices    []string
	// coveredThrough makes the fake pause record cover the slug until that many Sleep calls have been made; zero never covers it.
	coveredThrough int
	// coverage, when set, is the fake pause record's reading after that many Sleep calls, in place of coveredThrough and the owner pause.
	coverage func(call int) pauserecord.Coverage
	// scratchDir is the producer's scratch directory.
	scratchDir string
	// asleep is how far the awake clock trails the wall clock, which sleep advances to stage a machine sleep.
	asleep time.Duration

	// verdict is the fake owner check's answer to its nth call, counted from one; nil reads as owned here.
	verdict    func(n int) (OwnerVerdict, error)
	ownerCalls int
	// pauseErr is the error the fake owner pause answers its nth call with, counted from one; nil never fails.
	pauseErr func(n int) error
	// ownerPauses is the owner every fake owner pause was asked for, and heldByOwner whether one landed, which the fake pause record then reports as an owner hold.
	ownerPauses []string
	heldByOwner bool
	// parentHolds, restepped and their errors answer the Finalize carve-out seams.
	parentHolds    bool
	parentHoldsErr error
	restepped      bool
	restepErr      error
	// spawnErr is what the fake spawn returns.
	spawnErr error

	producer shedengine.ShedProducer
}

// sleep advances the wall clock by d without the awake clock, which the owner check reads as a machine sleep of d.
func (h *lockHarness) sleep(d time.Duration) {
	h.clock.advance(d)
	h.asleep += d
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
	deps.Awake = func() time.Duration { return h.clock.Now().Sub(started) - h.asleep }
	deps.Spawn = func(context.Context) error {
		*h.spawns++
		return h.spawnErr
	}
	deps.OwnerCheck = func(context.Context) (OwnerVerdict, error) {
		h.ownerCalls++
		if h.verdict == nil {
			return OwnerVerdict{}, nil
		}
		return h.verdict(h.ownerCalls)
	}
	deps.PauseOwned = func(_ context.Context, owner string) error {
		h.ownerPauses = append(h.ownerPauses, owner)
		if h.pauseErr != nil {
			if err := h.pauseErr(len(h.ownerPauses)); err != nil {
				return err
			}
		}
		h.heldByOwner = true
		return nil
	}
	deps.ParentHoldsTaskWork = func() (bool, error) { return h.parentHolds, h.parentHoldsErr }
	deps.Restepped = func(shedengine.Status) (bool, error) { return h.restepped, h.restepErr }
	deps.ReadStatus = func(string, string) (shedengine.Status, bool, error) { return h.status, true, nil }
	deps.ReadDecision = func() (ChildDecision, bool, error) {
		if h.decision == nil {
			return ChildDecision{}, false, nil
		}
		return *h.decision, true, nil
	}
	deps.PairDir = func() (string, error) { return "/pair", nil }
	deps.Pause = func() (pauserecord.Coverage, error) {
		if h.coverage != nil {
			return h.coverage(h.clock.sleepCalls), nil
		}
		if h.heldByOwner {
			return pauserecord.Coverage{Covered: true, OwnerHeld: true, Entry: pauserecord.Entry{Scope: "myslug", Caller: pauserecord.OwnerCaller}}, nil
		}
		return pauserecord.Coverage{Covered: h.clock.sleepCalls < h.coveredThrough, Entry: pauserecord.Entry{Scope: pauserecord.ScopeHub}}, nil
	}
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
		IdleBound: lockBound,
		KeepsLock: func() (bool, error) {
			h.keepsCalls++
			if h.keeps == nil {
				return false, nil
			}
			return h.keeps(h.keepsCalls)
		},
	}
	h.scratchDir = t.TempDir()
	h.producer = NewInnerRun("innerrun", "myslug", deps, lockCheckGap, h.scratchDir, testGrace)
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

// TestInnerRun_ParentLockReleasesAStalledLoopsRecord asserts a record owned by the child's loop is released once every agent has waited for the idle bound with none busy.
// It asserts the record is kept when the owner changed meanwhile, beside a busy agent and on a failed agent read.
// It asserts the record is kept while KeepsLock answers true or fails, and released once the bound passes again after it answers false.
func TestInnerRun_ParentLockReleasesAStalledLoopsRecord(t *testing.T) {
	t.Parallel()
	waiting := []parentlock.AgentState{{Name: "writer", Waiting: true}}
	busyBeside := []parentlock.AgentState{{Name: "writer", Waiting: true}, {Name: "fixer", Busy: true}}
	keepsThrough := func(n int) func(int) (bool, error) {
		return func(call int) (bool, error) { return call <= n, nil }
	}
	tests := []struct {
		name         string
		agents       []parentlock.AgentState
		agentsErr    error
		keeps        func(n int) (bool, error)
		newOwnerAt   int
		coveredUntil int
		// checks is how many checks the Call makes; zero means eleven, just past the bound.
		checks       int
		wantReleased bool
	}{
		{name: "released once the agents waited the bound", agents: waiting, wantReleased: true},
		{name: "kept while a pause covers it", agents: waiting, coveredUntil: 12},
		{name: "kept when the bound has not passed again since the resume", agents: waiting, coveredUntil: 6},
		{name: "released once the bound passes again after the resume", agents: waiting, coveredUntil: 6, checks: 18, wantReleased: true},
		{name: "kept when the owner changed meanwhile", agents: waiting, newOwnerAt: 6},
		{name: "kept beside a busy agent", agents: busyBeside},
		{name: "kept on a failed read", agents: waiting, agentsErr: errors.New("unreadable")},
		{name: "kept while a landing holds it", agents: waiting, keeps: keepsThrough(100), checks: 18},
		{name: "released once the bound passes again after the landing", agents: waiting, keeps: keepsThrough(6), checks: 18, wantReleased: true},
		{name: "kept when the landing hold cannot be read", agents: waiting, keeps: func(int) (bool, error) { return false, errors.New("unreadable") }, checks: 18},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newLockHarness(t, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Webster"})
			h.record = loopRecord()
			h.agents, h.agentsErr = tt.agents, tt.agentsErr
			h.keeps = tt.keeps
			h.coveredThrough = tt.coveredUntil
			if tt.newOwnerAt > 0 {
				h.clock.onSleep = func(call int) {
					if call == tt.newOwnerAt {
						h.record.Owner.PID, h.record.Owner.StartTime = lockLoopPID+1, "fresh-loop-start"
					}
				}
			}

			checks := tt.checks
			if checks == 0 {
				checks = 11
			}
			h.runChecks(checks)

			if got := len(h.released) == 1; got != tt.wantReleased {
				t.Errorf("released = %v; want released %v", h.released, tt.wantReleased)
			}
		})
	}
}

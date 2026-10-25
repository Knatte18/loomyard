//go:build tier0

// innerrun_memory_test.go pins what the memory record adds to InnerRun: each pass is a new producer over one scratch directory, and the record alone carries the episode across them.

package battenshed

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// TestInnerRun_MemoryOutlivesTheProcess builds a new producer per Call over one scratch directory and pins only what the record adds.
func TestInnerRun_MemoryOutlivesTheProcess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	running := shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Review"}

	t.Run("AnUnpostedNoticeIsRetriedAProbeApartAndDroppedAfterItsThirdAttempt", func(t *testing.T) {
		t.Parallel()
		const probe = 30 * time.Second
		h := newNoticeHarness(t)
		h.probe, h.poll = probe, 10*time.Second
		h.notifyQueued = false
		h.setStatus(parked(shedengine.StateBlocked))
		h.writeReport(h.clock.Now())

		h.run(h.producer(), 15)

		if len(h.notifyTimes) != noticeMaxAttempts {
			t.Fatalf("Notify calls at %v; want %d across the passes", h.notifyTimes, noticeMaxAttempts)
		}
		for i := 1; i < len(h.notifyTimes); i++ {
			if gap := h.notifyTimes[i].Sub(h.notifyTimes[i-1]); gap < probe {
				t.Errorf("Notify calls %d and %d are %s apart; want at least %s", i-1, i, gap, probe)
			}
		}
		deliveries := h.memory().Episode.Deliveries
		if len(deliveries) != 1 {
			t.Fatalf("recorded deliveries = %+v; want the one notice", deliveries)
		}
		for _, d := range deliveries {
			if d.Attempts != noticeMaxAttempts || !d.Dropped {
				t.Errorf("recorded delivery = %+v; want %d attempts and dropped", d, noticeMaxAttempts)
			}
		}
	})

	t.Run("AnIdleOwnersParentLockIsReleasedOnThePassAfterTheIdleBound", func(t *testing.T) {
		t.Parallel()
		h := newLockHarness(t, running)
		h.record = loopRecord()
		h.agents = []parentlock.AgentState{{Name: "writer", Waiting: true}}
		h.clock.pauseAtSleep = 0

		// The agents are first seen waiting at the first pass; the bound passes on the pass lockBound later.
		for pass := 0; pass <= int(lockBound/lockCheckGap); pass++ {
			if len(h.released) != 0 {
				t.Fatalf("record released before pass %d; want it kept until the idle bound passed", pass)
			}
			if outcome, _, err := h.newProducer().Call(ctx); err != nil || outcome != shedengine.Yield {
				t.Fatalf("pass %d = %v, %v; want a Yield", pass, outcome, err)
			}
			h.clock.Sleep(ctx, lockCheckGap)
		}
		if len(h.released) != 1 {
			t.Errorf("released = %v; want the record released on the pass after the bound", h.released)
		}
	})

	t.Run("ASeamFailingEveryPassIsOneEpisodeUntilItRecovers", func(t *testing.T) {
		t.Parallel()
		h := newNoticeHarness(t)
		h.activityErr = errors.New("shuttle config unreadable")
		pass := func() {
			t.Helper()
			if outcome, _, err := h.newProducer().Call(ctx); err != nil || outcome != shedengine.Yield {
				t.Fatalf("pass = %v, %v; want a Yield", outcome, err)
			}
		}

		for want := 1; want <= 3; want++ {
			pass()
			if got := h.memory().Failures[failAgentActivity]; got != want {
				t.Fatalf("failed ticks after pass %d = %d; want the episode to count on", want, got)
			}
		}
		h.activityErr = nil
		pass()
		if ticks, open := h.memory().Failures[failAgentActivity]; open {
			t.Errorf("failed ticks = %d after the seam read again; want the episode closed", ticks)
		}
	})

	t.Run("ARunningChildWithNothingToDoYieldsAndLeavesBattensStatusUntouched", func(t *testing.T) {
		t.Parallel()
		clock := &fakeClock{}
		scratchDir := t.TempDir()
		_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{{status: running, found: true}}, clock)
		clock.pauseAtSleep = 0
		recordSpawnConfirmed("innerrun", "myslug", scratchDir, SpawnConfirmedFile(scratchDir, "innerrun"))
		shed := newRunShed(t, freshPerCall(func() shedengine.ShedProducer {
			return NewInnerRun("innerrun", "myslug", deps, time.Second, scratchDir, testGrace)
		}), 3)
		read := func() shedengine.Status {
			t.Helper()
			status, found, err := state.ReadJSON[shedengine.Status](shed.StatusPath, shed.StatusLockPath)
			if err != nil || !found {
				t.Fatalf("read batten's status = %v found=%v", err, found)
			}
			return status
		}

		before := read()
		for pass := 0; pass < 3; pass++ {
			if res, err := shed.Step(ctx); err != nil || res.Outcome != shedengine.Yield {
				t.Fatalf("Step %d = %+v, %v; want a Yield", pass, res, err)
			}
			clock.Sleep(ctx, time.Second)
		}
		if after := read(); !reflect.DeepEqual(before, after) {
			t.Errorf("batten's status changed across yields:\nbefore %+v\nafter  %+v", before, after)
		}
	})

	t.Run("ADoneChildUnderACoveringPauseYieldsAndNeverReturnsDone", func(t *testing.T) {
		t.Parallel()
		clock := &fakeClock{}
		scratchDir := t.TempDir()
		_, _, deps := newInnerRunDeps(t, nil, nil, doneStatus(), clock)
		clock.pauseAtSleep = 0
		deps.NoticeProbe = 0
		covered := true
		deps.Pause = func() (pauserecord.Coverage, error) {
			return pauserecord.Coverage{Covered: covered, Entry: pauserecord.Entry{Scope: pauserecord.ScopeHub}}, nil
		}
		pass := func() shedengine.Outcome {
			t.Helper()
			outcome, _, err := NewInnerRun("innerrun", "myslug", deps, time.Second, scratchDir, testGrace).Call(ctx)
			if err != nil {
				t.Fatalf("pass error = %v", err)
			}
			return outcome
		}

		for i := 0; i < 5; i++ {
			if outcome := pass(); outcome != shedengine.Yield {
				t.Fatalf("pass %d under the pause = %v; want a Yield", i, outcome)
			}
			clock.Sleep(ctx, time.Second)
		}
		covered = false
		if outcome := pass(); outcome != shedengine.Done {
			t.Errorf("pass after the pause lifted = %v; want Done", outcome)
		}
	})

	t.Run("ARunningPassThenADonePassEndsOnTheChangeBeforeAnyPassReturnsDone", func(t *testing.T) {
		t.Parallel()
		clock := &fakeClock{}
		scratchDir := t.TempDir()
		_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{{status: running, found: true}}, clock)
		clock.pauseAtSleep = 0
		deps.NoticeProbe = 0
		recordSpawnConfirmed("innerrun", "myslug", scratchDir, SpawnConfirmedFile(scratchDir, "innerrun"))
		current := running
		deps.ReadStatus = func(string, string) (shedengine.Status, bool, error) { return current, true, nil }
		pass := func() (shedengine.Outcome, shedengine.OutputPointer) {
			t.Helper()
			outcome, pointer, err := NewInnerRun("innerrun", "myslug", deps, time.Second, scratchDir, testGrace).Call(ctx)
			if err != nil {
				t.Fatalf("pass error = %v", err)
			}
			return outcome, pointer
		}

		if outcome, _ := pass(); outcome != shedengine.Yield {
			t.Fatalf("running pass = %v; want a Yield", outcome)
		}
		current = shedengine.Status{State: shedengine.StateDone, CurrentProducer: "Plan-Review"}
		if outcome, pointer := pass(); outcome != shedengine.Stuck || pointer.Path != "child running → done at Plan-Review" {
			t.Fatalf("first done pass = %v %+v; want the Stuck of the change out of running", outcome, pointer)
		}
		if outcome, _ := pass(); outcome != shedengine.Done {
			t.Errorf("second done pass = %v; want Done once the driver is gone", outcome)
		}
	})
}

// innerrun_wait_test.go covers the inner-run watch's in-call wait: what ends it, what it ignores, and when it decodes the child's status file.
// The subtest that captures log output replaces the process-global logger output, so the test does not run in parallel.

package battenshed

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// waitChild is a fake child whose status the test rewrites between checks.
type waitChild struct {
	current shedengine.Status
	reads   int
}

func (c *waitChild) read(string, string) (shedengine.Status, bool, error) {
	c.reads++
	return c.current, true, nil
}

// TestInnerRun_WaitEndsOnlyOnAnEvent drives one running child through the events that end the wait and the writes it must notice.
func TestInnerRun_WaitEndsOnlyOnAnEvent(t *testing.T) {
	running := shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Review"}
	blocked := shedengine.Status{State: shedengine.StateBlocked, Error: "halted", CurrentProducer: "Plan-Review"}

	// newWait builds the deps of a running fake child whose clock advances one second per check, and returns a constructor of its producer, to be called once the test has adjusted the deps.
	newWait := func(t *testing.T, probe time.Duration) (*fakeClock, *waitChild, *InnerRunDeps, func() shedengine.ShedProducer) {
		t.Helper()
		clock := &fakeClock{}
		child := &waitChild{current: running}
		_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{{status: running, found: true}}, clock)
		deps.ReadStatus = child.read
		deps.NoticeProbe = probe
		clock.pauseAtSleep = 0
		scratchDir := t.TempDir()
		// The spawn is already confirmed, so the start read is the only one before the first check.
		if err := os.WriteFile(SpawnConfirmedFile(scratchDir, "innerrun"), pidMarker(os.Getpid()), 0o644); err != nil {
			t.Fatal(err)
		}
		return clock, child, &deps, func() shedengine.ShedProducer {
			return NewInnerRun("innerrun", "myslug", deps, time.Second, scratchDir, testGrace)
		}
	}
	call := func(t *testing.T, p shedengine.ShedProducer) shedengine.OutputPointer {
		t.Helper()
		outcome, ptr, err := p.Call(context.Background())
		if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt || ptr.Path != ptr.Reason {
			t.Fatalf("Call() = %v %+v %v; want a budget-exempt Stuck whose Path mirrors its Reason", outcome, ptr, err)
		}
		return ptr
	}

	t.Run("RunningReturnsNothingUntilTheStateChanges", func(t *testing.T) {
		clock, child, _, build := newWait(t, time.Hour)
		p := build()
		clock.onSleep = func(call int) {
			if call == 50 {
				child.current = blocked
				clock.bumpStatus()
			}
		}
		first := call(t, p)
		if clock.sleepCalls != 50 || first.Path != "child running → blocked at Plan-Review" {
			t.Fatalf("returned after %d checks with Path %q; want the change at check 50 named in its Path", clock.sleepCalls, first.Path)
		}
		if child.reads != 2 {
			t.Errorf("status reads = %d; want one at the start and one for the change, none between", child.reads)
		}

		clock.onSleep = func(call int) {
			if call == 60 {
				child.current = running
				clock.bumpStatus()
			}
		}
		second := call(t, p)
		if second.Path != "child blocked → running at Plan-Review" || second.Path == first.Path {
			t.Errorf("second Path = %q; want a Path distinct from %q", second.Path, first.Path)
		}
	})

	t.Run("AWriteBetweenTheStartReadAndTheFirstCheckIsSeen", func(t *testing.T) {
		clock, child, deps, build := newWait(t, time.Hour)
		// The write lands while the start read is returning: after the baseline stat, before the first check.
		deps.ReadStatus = func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
			status, found, err := child.read(statusPath, statusLockPath)
			if child.reads == 1 {
				child.current = blocked
				clock.bumpStatus()
			}
			return status, found, err
		}
		ptr := call(t, build())
		if clock.sleepCalls != 1 || ptr.Path != "child running → blocked at Plan-Review" {
			t.Errorf("returned after %d checks with Path %q; want the racing write seen at the first check", clock.sleepCalls, ptr.Path)
		}
	})

	t.Run("AWriteThatKeepsTheModificationTimeIsSeenWithinTheNoticeProbe", func(t *testing.T) {
		clock, child, _, build := newWait(t, 3*time.Second)
		clock.onSleep = func(call int) {
			if call == 1 {
				child.current = blocked
			}
		}
		ptr := call(t, build())
		if clock.sleepCalls != 3 || ptr.Path != "child running → blocked at Plan-Review" {
			t.Errorf("returned after %d checks with Path %q; want the unstamped write seen at the probe, the third check", clock.sleepCalls, ptr.Path)
		}
	})

	t.Run("APauseReturnsWithinOneCheckAndEachPauseIsItsOwnPath", func(t *testing.T) {
		clock, _, deps, build := newWait(t, time.Hour)
		paused := false
		deps.PauseRequested = func() (bool, error) { return paused, nil }
		p := build()
		clock.onSleep = func(call int) {
			if call == 5 {
				paused = true
			}
		}
		first := call(t, p)
		if clock.sleepCalls != 5 || first.Path != "pause requested at 2026-01-01T12:00:05Z" {
			t.Fatalf("returned after %d checks with Path %q; want the pause seen at the check it was set", clock.sleepCalls, first.Path)
		}

		paused = false
		clock.onSleep = func(call int) {
			if call == 8 {
				paused = true
			}
		}
		second := call(t, p)
		if second.Path != "pause requested at 2026-01-01T12:00:08Z" || second.Path == first.Path {
			t.Errorf("second Path = %q; want a Path distinct from %q", second.Path, first.Path)
		}
	})

	t.Run("APauseFlagReadErrorIsWarnedOnceAndReadsAsNotPaused", func(t *testing.T) {
		var buf bytes.Buffer
		logger.SetOutput(&buf)
		t.Cleanup(func() { logger.SetOutput(os.Stderr) })

		clock, child, deps, build := newWait(t, time.Hour)
		deps.PauseRequested = func() (bool, error) { return false, errors.New("status lock busy") }
		clock.onSleep = func(call int) {
			if call == 4 {
				child.current = blocked
				clock.bumpStatus()
			}
		}
		call(t, build())
		if got := strings.Count(buf.String(), "could not read batten's own pause flag"); got != 1 {
			t.Errorf("pause flag warnings over four checks = %d; want 1\nlog: %s", got, buf.String())
		}
	})

	t.Run("AFailedStatReturnsAnExemptStuckNamingTheFile", func(t *testing.T) {
		clock, _, _, build := newWait(t, time.Hour)
		clock.onSleep = func(call int) {
			if call == 2 {
				if err := os.Remove(clock.statusPath); err != nil {
					t.Fatal(err)
				}
			}
		}
		ptr := call(t, build())
		if clock.sleepCalls != 2 || !strings.Contains(ptr.Path, clock.statusPath) {
			t.Errorf("returned after %d checks with Path %q; want the failed stat at check 2 naming %s", clock.sleepCalls, ptr.Path, clock.statusPath)
		}
	})
}

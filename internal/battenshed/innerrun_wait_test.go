//go:build tier0

// innerrun_wait_test.go covers the inner-run's one check per pass: what ends it, what it ignores, and when it decodes the child's status file.
// The subtest that captures log output replaces the process-global logger output, so the test does not run in parallel.

package battenshed

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// waitChild is a fake child whose status the test rewrites between passes.
type waitChild struct {
	current shedengine.Status
	reads   int
}

func (c *waitChild) read(string, string) (shedengine.Status, bool, error) {
	c.reads++
	return c.current, true, nil
}

// TestInnerRun_WaitEndsOnlyOnAnEvent drives one running child through the events that end a pass and the writes it must notice.
func TestInnerRun_WaitEndsOnlyOnAnEvent(t *testing.T) {
	running := shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Review"}
	blocked := shedengine.Status{State: shedengine.StateBlocked, Error: "halted", CurrentProducer: "Plan-Review"}

	// newWait builds the deps of a running fake child whose clock advances one second per pass, and returns a constructor of its producer, to be called once the test has adjusted the deps.
	newWait := func(t *testing.T, probe time.Duration) (*fakeClock, *waitChild, *InnerRunDeps, func() shedengine.ShedProducer) {
		t.Helper()
		clock := &fakeClock{}
		child := &waitChild{current: running}
		_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{{status: running, found: true}}, clock)
		deps.ReadStatus = child.read
		deps.NoticeProbe = probe
		clock.pauseAtSleep = 0
		scratchDir := t.TempDir()
		// The spawn is already confirmed, so the start read is the only one in a pass before its check.
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

	t.Run("RunningYieldsUntilTheStateChanges", func(t *testing.T) {
		clock, child, _, build := newWait(t, time.Hour)
		p := clock.passes(build, time.Second)
		clock.onSleep = func(call int) {
			if call == 50 {
				child.current = blocked
				clock.bumpStatus()
			}
		}
		first := call(t, p)
		if clock.sleepCalls != 50 || first.Path != "child running → blocked at Plan-Review" {
			t.Fatalf("returned after %d passes with Path %q; want the change at pass 50 named in its Path", clock.sleepCalls, first.Path)
		}
		if child.reads != 51 {
			t.Errorf("status reads = %d; want one per pass and none besides", child.reads)
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

	t.Run("ACheckThatEndsNothingYieldsWithAnEmptyPointerAndNoSleep", func(t *testing.T) {
		clock, _, _, build := newWait(t, time.Hour)
		outcome, ptr, err := build().Call(context.Background())
		if outcome != shedengine.Yield || !reflect.DeepEqual(ptr, shedengine.OutputPointer{}) || err != nil {
			t.Fatalf("Call() = %v %+v %v; want a Yield with an empty pointer and no error", outcome, ptr, err)
		}
		if clock.sleepCalls != 0 {
			t.Errorf("sleep calls = %d; want a pass to wait for nothing", clock.sleepCalls)
		}
	})

	t.Run("AWriteBetweenTheStartReadAndTheCheckIsSeen", func(t *testing.T) {
		clock, child, deps, build := newWait(t, time.Hour)
		// The write lands while the start read is returning: after the baseline stat, before the check.
		deps.ReadStatus = func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
			status, found, err := child.read(statusPath, statusLockPath)
			if child.reads == 1 {
				child.current = blocked
				clock.bumpStatus()
			}
			return status, found, err
		}
		ptr := call(t, build())
		if clock.sleepCalls != 0 || ptr.Path != "child running → blocked at Plan-Review" {
			t.Errorf("returned after %d sleeps with Path %q; want the racing write seen by the first pass's own check", clock.sleepCalls, ptr.Path)
		}
	})

	t.Run("AWriteThatKeepsTheModificationTimeIsSeenAtTheNextPass", func(t *testing.T) {
		clock, child, _, build := newWait(t, 3*time.Second)
		clock.onSleep = func(call int) {
			if call == 1 {
				child.current = blocked
			}
		}
		ptr := call(t, clock.passes(build, time.Second))
		if clock.sleepCalls != 1 || ptr.Path != "child running → blocked at Plan-Review" {
			t.Errorf("returned after %d sleeps with Path %q; want the unstamped write seen at the pass after it", clock.sleepCalls, ptr.Path)
		}
	})

	t.Run("APauseReturnsAtThePassItWasSetAndEachPauseIsItsOwnPath", func(t *testing.T) {
		clock, _, deps, build := newWait(t, time.Hour)
		paused := false
		deps.PauseRequested = func() (bool, error) { return paused, nil }
		p := clock.passes(build, time.Second)
		clock.onSleep = func(call int) {
			if call == 5 {
				paused = true
			}
		}
		first := call(t, p)
		if clock.sleepCalls != 5 || first.Path != "pause requested at 2026-01-01T12:00:05Z" {
			t.Fatalf("returned after %d sleeps with Path %q; want the pause seen at the pass after it was set", clock.sleepCalls, first.Path)
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

	t.Run("APauseFlagReadErrorIsWarnedOnceAcrossPassesAndReadsAsNotPaused", func(t *testing.T) {
		var buf bytes.Buffer
		logger.SetOutput(&buf)
		t.Cleanup(func() { logger.SetOutput(logger.DefaultOutput()) })

		clock, child, deps, build := newWait(t, time.Hour)
		deps.PauseRequested = func() (bool, error) { return false, errors.New("status lock busy") }
		clock.onSleep = func(call int) {
			if call == 4 {
				child.current = blocked
				clock.bumpStatus()
			}
		}
		call(t, clock.passes(build, time.Second))
		if got := strings.Count(buf.String(), "could not read batten's own pause flag"); got != 1 {
			t.Errorf("pause flag warnings over four passes = %d; want 1\nlog: %s", got, buf.String())
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
		ptr := call(t, clock.passes(build, time.Second))
		if clock.sleepCalls != 2 || !strings.Contains(ptr.Path, clock.statusPath) {
			t.Errorf("returned after %d sleeps with Path %q; want the failed stat at pass 3 naming %s", clock.sleepCalls, ptr.Path, clock.statusPath)
		}
	})
}

// TestInnerRun_RecordsWhetherItHoldsTheWatchedMarker asserts the producer keeps the batten-watched marker's answer: asked once at the start of every pass, an error read as not held with one Warn per change of answer across passes, and a nil seam read as not held.
func TestInnerRun_RecordsWhetherItHoldsTheWatchedMarker(t *testing.T) {
	// passesUntilPause runs one fresh producer per pass until the pause ends the run, and returns the last producer.
	passesUntilPause := func(t *testing.T, probe time.Duration, markWatched func(context.Context) (bool, error), pauseAt int) *innerRunProducer {
		t.Helper()
		clock := &fakeClock{}
		_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{{status: shedengine.Status{State: shedengine.StateRunning}, found: true}}, clock)
		scratchDir := t.TempDir()
		if err := os.WriteFile(SpawnConfirmedFile(scratchDir, "innerrun"), pidMarker(os.Getpid()), 0o644); err != nil {
			t.Fatal(err)
		}
		deps.NoticeProbe = probe
		deps.MarkWatched = markWatched
		clock.pauseAtSleep = pauseAt
		for {
			p := NewInnerRun("innerrun", "myslug", deps, 10*time.Second, scratchDir, testGrace).(*innerRunProducer)
			outcome, _, err := p.Call(context.Background())
			if err != nil {
				t.Fatalf("Call() error = %v", err)
			}
			if outcome != shedengine.Yield {
				return p
			}
			clock.Sleep(context.Background(), 10*time.Second)
		}
	}

	t.Run("AsksAtTheStartOfEveryPass", func(t *testing.T) {
		asks := 0
		p := passesUntilPause(t, 30*time.Second, func(context.Context) (bool, error) { asks++; return true, nil }, 7)
		if asks != 8 || !p.watched {
			t.Errorf("asked %d times and recorded held=%v over eight passes; want one ask per pass and held", asks, p.watched)
		}
	})

	t.Run("AnErrorReadsAsNotHeldWithOneWarnPerChangeOfAnswer", func(t *testing.T) {
		var buf bytes.Buffer
		logger.SetOutput(&buf)
		t.Cleanup(func() { logger.SetOutput(logger.DefaultOutput()) })

		asks := 0
		p := passesUntilPause(t, 0, func(context.Context) (bool, error) {
			asks++
			if asks == 4 {
				return true, nil
			}
			return false, errors.New("task worktree gone")
		}, 3)
		if got := strings.Count(buf.String(), "could not write or remove the batten-watched marker"); got != 1 {
			t.Errorf("warnings over three failed asks = %d; want 1\nlog: %s", got, buf.String())
		}
		if !p.watched {
			t.Error("recorded held = false after the fourth ask answered held; want true")
		}
	})

	t.Run("ANilSeamReadsAsNotHeld", func(t *testing.T) {
		p := passesUntilPause(t, 0, nil, 2)
		if p.watched {
			t.Error("recorded held = true with no seam; want false")
		}
	})
}

// TestInnerRun_SpawnHoldYields asserts a pass at Run-Shed under a covering pause yields at once with the pause reason and spawns nothing, and that a pause lifted between two Calls lets the second one spawn.
func TestInnerRun_SpawnHoldYields(t *testing.T) {
	t.Parallel()
	covering := pauserecord.Coverage{Covered: true, Entry: pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)}}
	running := shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Review"}

	tests := []struct {
		name string
		// covered is the pause record's reading at each Call, the last one holding once exhausted.
		covered []bool
		// wantSpawns is the number of spawns after each Call.
		wantSpawns []int
		// wantReason is the prefix of the reason file after the last Call; empty skips the check.
		wantReason string
	}{
		{name: "a covered absent-status child yields at once with the pause reason and no spawn", covered: []bool{true}, wantSpawns: []int{0}, wantReason: "held by the pause of hub since 2026-01-01T11:00:00Z"},
		{name: "a pause lifted between two Calls spawns once at the second", covered: []bool{true, false}, wantSpawns: []int{0, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clock := &fakeClock{}
			// The child has no status until the spawn, so every Call meets the spawn hold first.
			spawned := false
			_, spawns, deps := newInnerRunDeps(t, nil, nil, []statusResult{{found: false}}, clock)
			deps.Spawn = func(context.Context) error { *spawns++; spawned = true; return nil }
			deps.ReadStatus = func(string, string) (shedengine.Status, bool, error) { return running, spawned, nil }
			call := 0
			deps.Pause = func() (pauserecord.Coverage, error) {
				if tt.covered[min(call, len(tt.covered)-1)] {
					return covering, nil
				}
				return pauserecord.Coverage{}, nil
			}
			scratchDir := t.TempDir()
			producer := NewInnerRun("innerrun", "myslug", deps, time.Second, scratchDir, testGrace)

			for call = 0; call < len(tt.wantSpawns); call++ {
				outcome, pointer, err := producer.Call(context.Background())
				if err != nil || outcome != shedengine.Yield || !reflect.DeepEqual(pointer, shedengine.OutputPointer{}) {
					t.Fatalf("Call %d = %q %+v %v; want a Yield with an empty pointer", call, outcome, pointer, err)
				}
				if *spawns != tt.wantSpawns[call] {
					t.Errorf("Spawn calls after Call %d = %d; want %d", call, *spawns, tt.wantSpawns[call])
				}
			}
			if tt.wantReason == "" {
				return
			}
			reason, err := os.ReadFile(StuckReasonFile(scratchDir, "innerrun"))
			if err != nil || !strings.HasPrefix(string(reason), tt.wantReason) {
				t.Errorf("reason file = %q, %v; want it to start with %q", reason, err, tt.wantReason)
			}
		})
	}
}

// innerrun_failures_test.go covers how the inner-run wait logs a polled seam that keeps failing, and the attempts it repeats:
// one Warn per episode with Debug lines while it lasts and one Info at recovery, and Info only for the first attempt of a retried goto, resume or spawn.
// The tests capture the process-global logger, so they do not run in parallel.

package battenshed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// failFirstCalls returns a seam body that fails on its first n calls and succeeds after.
func failFirstCalls(n int) func() error {
	calls := 0
	return func() error {
		calls++
		if calls <= n {
			return errors.New("seam boom")
		}
		return nil
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

// assertRepeatedFailureEpisode requires the log to hold one Warn of failure, at least one Debug line of it, and one Info naming the recovery of it.
func assertRepeatedFailureEpisode(t *testing.T, log, failure string) {
	t.Helper()
	if got := countLogLines(log, "WARN", failure); got != 1 {
		t.Errorf("WARN lines of %q = %d; want 1\n%s", failure, got, log)
	}
	if got := countLogLines(log, "DEBUG", failure); got < 1 {
		t.Errorf("DEBUG lines of %q = %d; want at least 1\n%s", failure, got, log)
	}
	recovered := 0
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, "level=INFO") && strings.Contains(line, "a repeated failure recovered") && strings.Contains(line, failure) {
			recovered++
		}
	}
	if recovered != 1 {
		t.Errorf("INFO recovery lines naming %q = %d; want 1\n%s", failure, recovered, log)
	}
}

// failureLockSeams is a parent lock whose seams find no record and move nothing.
func failureLockSeams() *ChildParentLock {
	return &ChildParentLock{
		Read:        func() (parentlock.Record, bool, error) { return parentlock.Record{}, false, nil },
		Release:     func(parentlock.Record) (bool, error) { return false, nil },
		Agents:      func() ([]parentlock.AgentState, error) { return nil, nil },
		SelfPID:     lockSelfPID,
		IdleBound:   lockBound,
		ParentMoved: func() (bool, error) { return false, nil },
		GateRow:     lockGateRow,
	}
}

// TestInnerRun_ReadFailuresLogOncePerEpisode asserts each polled seam whose failure repeats on every check warns once, logs Debug while it keeps failing and logs one Info when it reads again.
// It also asserts the demoted lines: a parent lock release logs Info, and a retried goto or resume logs Info on its first attempt and Debug after each refusal.
func TestInnerRun_ReadFailuresLogOncePerEpisode(t *testing.T) {
	blocked := parked(shedengine.StateBlocked)
	awaitingNoticed := parked(shedengine.StateAwaiting)
	awaitingNoticed.ParentNotice = "pull request ready"
	restepBlocked := haltedStatus(shedengine.StateBlocked, 2).status
	// wireRestepSeams switches the binary re-step on over a driverless child, a changed build and a clean tree.
	wireRestepSeams := func(deps *InnerRunDeps) {
		deps.ChildBuildChanged = func() (string, bool, error) { return "build-1", true, nil }
		deps.ChildDirtyPaths = func() ([]string, error) { return nil, nil }
	}
	tests := []struct {
		name    string
		message string
		status  shedengine.Status
		// covered makes the fake pause record cover the slug on every check, so the awaiting arm's own decision read does not end the Call.
		covered bool
		// recent starts the fake clock seconds after the status file's modification time, so a parked stop's notice has not waited out its report wait.
		recent bool
		// wire installs the seam under test into deps; fail errors on its first three calls.
		wire func(deps *InnerRunDeps, fail func() error)
	}{
		{
			name: "the parent lock record read", message: "parent lock record unreadable",
			status: shedengine.Status{State: shedengine.StateRunning},
			wire: func(deps *InnerRunDeps, fail func() error) {
				lock := failureLockSeams()
				lock.Read = func() (parentlock.Record, bool, error) { return parentlock.Record{}, false, fail() }
				deps.ParentLock = lock
			},
		},
		{
			name: "the agent states read", message: "agent states are unreadable",
			status: shedengine.Status{State: shedengine.StateRunning},
			wire: func(deps *InnerRunDeps, fail func() error) {
				lock := failureLockSeams()
				lock.Read = func() (parentlock.Record, bool, error) { return *loopRecord(), true, nil }
				lock.Agents = func() ([]parentlock.AgentState, error) { return nil, fail() }
				deps.ParentLock = lock
			},
		},
		{
			name: "the parent lock release", message: "could not release the child's parent lock",
			status: blocked,
			wire: func(deps *InnerRunDeps, fail func() error) {
				released := false
				lock := failureLockSeams()
				lock.Read = func() (parentlock.Record, bool, error) { return *handedRecord(), !released, nil }
				lock.Release = func(parentlock.Record) (bool, error) {
					if err := fail(); err != nil {
						return false, err
					}
					released = true
					return true, nil
				}
				deps.ParentLock = lock
			},
		},
		{
			name: "the stale-base decision read", message: "decision unreadable for the stale-base check",
			status: awaitingAt(lockGateRow), covered: true,
			wire: func(deps *InnerRunDeps, fail func() error) {
				deps.ReadDecision = func() (ChildDecision, bool, error) { return ChildDecision{}, false, fail() }
				deps.ParentLock = failureLockSeams()
			},
		},
		{
			name: "the parent-moved read", message: "could not tell whether the parent moved",
			status: awaitingAt(lockGateRow),
			wire: func(deps *InnerRunDeps, fail func() error) {
				lock := failureLockSeams()
				lock.ParentMoved = func() (bool, error) { return false, fail() }
				deps.ParentLock = lock
			},
		},
		{
			name: "the driver liveness read", message: "driver liveness read failed",
			status: shedengine.Status{State: shedengine.StateDone},
			wire: func(deps *InnerRunDeps, fail func() error) {
				deps.DriverAlive = func(context.Context) (bool, error) { return true, fail() }
			},
		},
		{
			name: "the pair directory", message: "pair directory unresolved",
			status: shedengine.Status{State: shedengine.StateRunning},
			wire: func(deps *InnerRunDeps, fail func() error) {
				deps.PairDir = func() (string, error) { return "/pair", fail() }
			},
		},
		{
			name: "the agent activity read", message: "agent activity read failed",
			status: shedengine.Status{State: shedengine.StateRunning},
			wire: func(deps *InnerRunDeps, fail func() error) {
				deps.DriverAlive = func(context.Context) (bool, error) { return true, nil }
				deps.Activity = func(context.Context) ([]AgentActivity, bool, error) { return nil, false, fail() }
			},
		},
		{
			name: "the arbiter marker read", message: "arbiter marker unreadable",
			status: blocked, recent: true,
			wire: func(deps *InnerRunDeps, fail func() error) {
				deps.DriverAlive = func(context.Context) (bool, error) { return true, nil }
				deps.ArbiterInFlight = func(context.Context) (ChildArbiter, bool, error) { return ChildArbiter{}, false, fail() }
			},
		},
		{
			name: "the stop report read", message: "stop report unreadable",
			status: blocked, recent: true,
			wire: func(deps *InnerRunDeps, fail func() error) {
				deps.DriverAlive = func(context.Context) (bool, error) { return true, nil }
				deps.StopReport = func() (string, time.Time, bool, error) { return "", time.Time{}, false, fail() }
			},
		},
		{
			name: "the notice step's decision read", message: "decision unreadable for the notice step",
			status: awaitingNoticed, covered: true,
			wire: func(deps *InnerRunDeps, fail func() error) {
				deps.DriverAlive = func(context.Context) (bool, error) { return true, nil }
				deps.MarkWatched = func(context.Context) (bool, error) { return true, nil }
				deps.StopReport = func() (string, time.Time, bool, error) {
					return "/wt/task/_lyx/stop-report.md", time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC), true, nil
				}
				deps.ReadDecision = func() (ChildDecision, bool, error) { return ChildDecision{}, false, fail() }
			},
		},
		{
			name: "the unreadable orch state", message: "the orch state is unreadable",
			status: shedengine.Status{State: shedengine.StateRunning},
			wire: func(deps *InnerRunDeps, fail func() error) {
				deps.OrchStrandRecorded = func() (bool, error) { return true, fail() }
			},
		},
		{
			name: "the re-step check's driver strand read", message: "re-step check skipped; could not read the child's driver strand",
			status: restepBlocked,
			wire: func(deps *InnerRunDeps, fail func() error) {
				wireRestepSeams(deps)
				// The revive reads the strand before the re-step check on every tick, so the seam fails twice as often to fail the check three times.
				failBoth := failFirstCalls(6)
				deps.DriverStrand = func(context.Context) (ChildDriverStrand, error) { return ChildDriverNone, failBoth() }
			},
		},
		{
			name: "the re-step check's lock read", message: "re-step check skipped; could not read the child's locks",
			status: restepBlocked,
			wire: func(deps *InnerRunDeps, fail func() error) {
				wireRestepSeams(deps)
				deps.ChildRunLockHeld = func() (bool, error) { return false, fail() }
			},
		},
		{
			name: "the re-step check's build read", message: "re-step check skipped; could not read the on-disk build",
			status: restepBlocked,
			wire: func(deps *InnerRunDeps, fail func() error) {
				wireRestepSeams(deps)
				deps.ChildBuildChanged = func() (string, bool, error) { return "build-1", true, fail() }
			},
		},
		{
			name: "the re-step check's dirty-path read", message: "re-step check skipped; could not read the task pair's uncommitted paths",
			status: restepBlocked,
			wire: func(deps *InnerRunDeps, fail func() error) {
				wireRestepSeams(deps)
				deps.ChildDirtyPaths = func() ([]string, error) { return nil, fail() }
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.CaptureVerbose(t)
			logger.SetVerbosity(2)

			clock := &fakeClock{}
			if tt.recent {
				clock.now = time.Date(2026, 1, 1, 0, 0, 10, 0, time.UTC)
			}
			_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{{status: tt.status, found: true}}, clock)
			clock.pauseAtSleep = 8
			deps.NoticeProbe = 0
			deps.Notify = func(context.Context, string) (bool, error) { return true, nil }
			deps.PairDir = func() (string, error) { return "/pair", nil }
			if tt.covered {
				deps.Pause = func() (pauserecord.Coverage, error) {
					return pauserecord.Coverage{Covered: true, Entry: pauserecord.Entry{Scope: pauserecord.ScopeHub}}, nil
				}
			}
			tt.wire(&deps, failFirstCalls(3))

			shedfake.RequireOutcome(t, NewInnerRun("innerrun", "myslug", deps, time.Second, t.TempDir(), testGrace), shedengine.Stuck)

			assertRepeatedFailureEpisode(t, buf.String(), tt.message)
		})
	}

	t.Run("the Go verify-retry records read", func(t *testing.T) {
		buf := logcapture.CaptureVerbose(t)
		logger.SetVerbosity(2)
		h := newRetryHarness(t)
		err := errors.New("seam boom")
		h.goRetryErrs = []error{err, err, err, nil}
		h.goRetrySpent = true

		for range 5 {
			h.call()
		}

		assertRepeatedFailureEpisode(t, buf.String(), "Go verify-retry records unreadable")
	})

	t.Run("a dirty task pair blocking the re-step logs Info for the first check and Debug after", func(t *testing.T) {
		buf := logcapture.CaptureVerbose(t)
		logger.SetVerbosity(2)
		h := newRestepHarness(t)
		h.dirty = []string{"src/a.go"}
		h.probe = time.Second

		h.call(h.producer(), 8)

		const skipped = "not re-stepping the halted child on the new build"
		if info, debug := countLogLines(buf.String(), "INFO", skipped), countLogLines(buf.String(), "DEBUG", skipped); info != 1 || debug != 3 {
			t.Errorf("dirty-pair lines: INFO %d, DEBUG %d; want 1 and 3\n%s", info, debug, buf.String())
		}
	})

	t.Run("a parent lock release logs one Info and no Warn", func(t *testing.T) {
		buf := logcapture.CaptureVerbose(t)
		h := newLockHarness(t, shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Finalize"})
		h.record = handedRecord()

		h.runChecks(2)

		const released = "released the child's parent lock"
		if info, warn := countLogLines(buf.String(), "INFO", released), countLogLines(buf.String(), "WARN", released); info != 1 || warn != 0 {
			t.Errorf("release lines: INFO %d, WARN %d; want 1 and 0\n%s", info, warn, buf.String())
		}
	})

	t.Run("a Publish-retry goto refused busy logs Info for the first attempt and Debug after", func(t *testing.T) {
		buf := logcapture.CaptureVerbose(t)
		logger.SetVerbosity(2)
		h := newRetryHarness(t)
		h.gotoErrs = []error{ErrChildRunBusy, ErrChildRunBusy, nil}
		h.clock.pauseAtSleep = 100
		h.clock.onSleep = func(int) { h.clock.advance(time.Minute) }

		h.call()

		h.wantGotos(3)
		const attempt = "sending the child to the failed Publish's target"
		if info, debug := countLogLines(buf.String(), "INFO", attempt), countLogLines(buf.String(), "DEBUG", attempt); info != 1 || debug != 2 {
			t.Errorf("goto attempt lines: INFO %d, DEBUG %d; want 1 and 2\n%s", info, debug, buf.String())
		}
	})

	t.Run("an awaiting resume refused not parked logs Info for the first attempt and Debug after", func(t *testing.T) {
		buf := logcapture.CaptureVerbose(t)
		logger.SetVerbosity(2)
		clock := &fakeClock{}
		_, _, deps := newInnerRunDeps(t, nil, nil, awaitingStatus(), clock)
		deps.NoticeProbe = 30 * time.Second
		clock.pauseAtSleep = 8
		deps.ReadDecision = func() (ChildDecision, bool, error) {
			return ChildDecision{Kind: DecisionApprove, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}, true, nil
		}
		attempts := 0
		deps.Spawn = func(context.Context) error {
			attempts++
			if attempts <= 2 {
				return fmt.Errorf("%w: exit status 1", ErrChildNotParked)
			}
			return nil
		}

		shedfake.RequireOutcome(t, NewInnerRun("innerrun", "myslug", deps, 10*time.Second, t.TempDir(), testGrace), shedengine.Stuck)

		if attempts != 3 {
			t.Fatalf("Spawn calls = %d; want 3 (two refusals, then landed)", attempts)
		}
		const attempt = "resuming awaiting inner shed run"
		if info, debug := countLogLines(buf.String(), "INFO", attempt), countLogLines(buf.String(), "DEBUG", attempt); info != 1 || debug != 2 {
			t.Errorf("resume attempt lines: INFO %d, DEBUG %d; want 1 and 2\n%s", info, debug, buf.String())
		}
	})
}

// stuck_test.go pins that a stuck verdict's log line takes its level from its cause:
// a failed producer step logs at Warn, and an ordinary wait or move logs at Info and leaves its reason in the reason file.

package battenshed

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// TestStuckReport_LevelFollowsCause drives one producer per cause and asserts the single stuck line carrying the cause's reason has the cause's level.
// An Info cause also leaves its reason in the reason file.
// It captures the process-global logger at Info, so it is not parallel.
func TestStuckReport_LevelFollowsCause(t *testing.T) {
	boom := errors.New("boom")
	running := statusResult{status: shedengine.Status{State: shedengine.StateRunning}, found: true}
	tests := []struct {
		name  string
		level string
		// reason is a fragment of the reason the cause produces.
		reason string
		// fileReason is the fragment the reason file holds once the producer returns; empty means reason, for a cause whose wait a later reason ends.
		fileReason string
		// run drives the cause's producer and returns its scratch directory and name.
		run func(t *testing.T) (scratchDir, producer string)
	}{
		{
			name: "a failed Worktree-Create seam", level: "WARN", reason: "create boom",
			run: func(t *testing.T) (string, string) {
				scratchDir := t.TempDir()
				var released bool
				lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
				producer := NewWorktreeCreate("create", "myslug", func(context.Context) error { return errors.New("create boom") }, lock, scratchDir)
				shedfake.RequireOutcome(t, producer, shedengine.Stuck)
				return scratchDir, "create"
			},
		},
		{
			name: "a Seed-Child board read failure", level: "WARN", reason: "board decode failed",
			run: func(t *testing.T) (string, string) {
				scratchDir := t.TempDir()
				_, deps := newSeedChildDeps("", errors.New("board decode failed"), "claude", nil, nil, nil, nil)
				shedfake.RequireOutcome(t, NewSeedChild("seedchild", "myslug", deps, scratchDir), shedengine.Stuck)
				return scratchDir, "seedchild"
			},
		},
		{
			name: "a failed Land pull", level: "WARN", reason: "pull failed: boom",
			run: func(t *testing.T) (string, string) {
				f := newLandFixture(t)
				f.pullErr = boom
				if outcome, _, err := f.producer().Call(context.Background()); err != nil || outcome != shedengine.Stuck {
					t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
				}
				return f.scratchDir, "Land"
			},
		},
		{
			name: "a failed Worktree-Teardown shutdown", level: "WARN", reason: "session would not end",
			run: func(t *testing.T) (string, string) {
				scratchDir := t.TempDir()
				var released bool
				lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
				rec := &teardownCallRecorder{}
				producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(errors.New("session would not end"), "", nil), lock, scratchDir)
				shedfake.RequireOutcome(t, producer, shedengine.Stuck)
				return scratchDir, "teardown"
			},
		},
		{
			name: "an InnerRun whose status file goes missing mid-wait", level: "WARN", reason: "is gone",
			run: func(t *testing.T) (string, string) {
				scratchDir := t.TempDir()
				clock := &fakeClock{}
				// A running child with no confirmed spawn is spawned and read once more before the wait begins, so the first two reads are the start.
				_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{running, running, {found: false}}, clock)
				deps.NoticeProbe = 0
				clock.pauseAtSleep = 5
				shedfake.RequireOutcome(t, NewInnerRun("innerrun", "myslug", deps, time.Second, scratchDir, testGrace), shedengine.Stuck)
				return scratchDir, "innerrun"
			},
		},
		{
			name: "an InnerRun whose child moves from running to awaiting", level: "INFO", reason: "child running → awaiting",
			run: func(t *testing.T) (string, string) {
				scratchDir := t.TempDir()
				clock := &fakeClock{}
				awaiting := statusResult{status: shedengine.Status{State: shedengine.StateAwaiting}, found: true}
				_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{running, running, awaiting}, clock)
				clock.pauseAtSleep = 5
				clock.onSleep = func(call int) {
					if call == 1 {
						clock.bumpStatus()
					}
				}
				shedfake.RequireOutcome(t, NewInnerRun("innerrun", "myslug", deps, time.Second, scratchDir, testGrace), shedengine.Stuck)
				return scratchDir, "innerrun"
			},
		},
		{
			name: "an InnerRun whose blocked child's wait ends on a pause request", level: "INFO", reason: "pause requested at ",
			run: func(t *testing.T) (string, string) {
				scratchDir := t.TempDir()
				clock := &fakeClock{}
				_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{haltedStatus(shedengine.StateBlocked, 2)}, clock)
				clock.pauseAtSleep = 1
				shedfake.RequireOutcome(t, NewInnerRun("innerrun", "myslug", deps, time.Second, scratchDir, testGrace), shedengine.Stuck)
				return scratchDir, "innerrun"
			},
		},
		{
			name: "a Land paused in its land-lock wait", level: "INFO", reason: "a pause was requested while waiting for",
			run: func(t *testing.T) (string, string) {
				f := newLandFixture(t)
				f.landLockBusy = 100
				f.pauseAfterSleeps = 2
				if outcome, _, err := f.producer().Call(context.Background()); err != nil || outcome != shedengine.Stuck {
					t.Fatalf("Call() = %q, %v; want Stuck, nil", outcome, err)
				}
				return f.scratchDir, "Land"
			},
		},
		{
			name: "an InnerRun whose Publish verify retry is refused busy before a pause", level: "INFO", reason: "waits to send the child to " + retryTarget, fileReason: "pause requested at ",
			run: func(t *testing.T) (string, string) {
				h := newRetryHarness(t)
				h.gotoErrs = []error{ErrChildRunBusy}
				h.clock.pauseAtSleep = 3
				h.call()
				return h.scratchDir, "innerrun"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.CaptureVerbose(t)
			scratchDir, producer := tt.run(t)

			var carrying []string
			for _, line := range strings.Split(logs.String(), "\n") {
				if strings.Contains(line, "battenshed: producer stuck") && strings.Contains(line, tt.reason) {
					carrying = append(carrying, line)
				}
			}
			if len(carrying) != 1 {
				t.Fatalf("stuck lines carrying %q = %d, want 1; log:\n%s", tt.reason, len(carrying), logs.String())
			}
			if !strings.Contains(carrying[0], "level="+tt.level) {
				t.Errorf("stuck line = %s, want level=%s", carrying[0], tt.level)
			}
			if tt.level == "INFO" {
				raw, err := os.ReadFile(StuckReasonFile(scratchDir, producer))
				if err != nil {
					t.Fatalf("reason file: %v", err)
				}
				want := tt.reason
				if tt.fileReason != "" {
					want = tt.fileReason
				}
				if !strings.Contains(string(raw), want) {
					t.Errorf("reason file = %q, want it to hold %q", raw, want)
				}
			}
		})
	}
}

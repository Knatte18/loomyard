// innerrun_test.go covers NewInnerRun's full verdict table over a fake ReadStatus, a fake Now, and
// a fake Sleep that never sleeps -- so the attempt-cap test proves the bound is attempt-counted,
// not wall-clock-timed, in unmeasurable real time.

package lifecycleshed

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// fakeClock is a Now/Sleep pair a test can hold still: Now always returns the same instant
// (advanced only when the test wants to prove elapsed-time reporting), and Sleep records calls
// without ever blocking.
type fakeClock struct {
	now        time.Time
	sleepCalls int
}

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) Sleep(d time.Duration) {
	c.sleepCalls++
}

func newInnerRunDeps(spawnErr error, resolveErr error, statuses []statusResult, clock *fakeClock) (*int, *int, InnerRunDeps) {
	readCalls := 0
	spawnCalls := 0
	deps := InnerRunDeps{
		Spawn: func(ctx context.Context) error {
			spawnCalls++
			return spawnErr
		},
		ResolveStatus: func() (string, string, error) {
			if resolveErr != nil {
				return "", "", resolveErr
			}
			return "/status/path", "/status/lock/path", nil
		},
		ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
			idx := readCalls
			readCalls++
			if idx >= len(statuses) {
				t := statuses[len(statuses)-1]
				return t.status, t.found, t.err
			}
			r := statuses[idx]
			return r.status, r.found, r.err
		},
		Now:   clock.Now,
		Sleep: clock.Sleep,
	}
	return &readCalls, &spawnCalls, deps
}

type statusResult struct {
	status shedengine.Status
	found  bool
	err    error
}

func TestInnerRun_VerdictTable(t *testing.T) {
	tests := []struct {
		name       string
		statuses   []statusResult
		wantDone   bool
		wantStuck  bool
		wantErr    bool
		wantReason string
	}{
		{
			name:     "StateDone",
			statuses: []statusResult{{status: shedengine.Status{State: shedengine.StateDone}, found: true}},
			wantDone: true,
		},
		{
			name:       "StateBlocked",
			statuses:   []statusResult{{status: shedengine.Status{State: shedengine.StateBlocked, Error: "boom", CurrentProducer: "p1"}, found: true}},
			wantStuck:  true,
			wantReason: "blocked",
		},
		{
			name:       "StatePaused",
			statuses:   []statusResult{{status: shedengine.Status{State: shedengine.StatePaused, Error: "paused-err", CurrentProducer: "p2"}, found: true}},
			wantStuck:  true,
			wantReason: "paused",
		},
		{
			name:       "StateFailed",
			statuses:   []statusResult{{status: shedengine.Status{State: shedengine.StateFailed, Error: "failed-err", CurrentProducer: "p3"}, found: true}},
			wantStuck:  true,
			wantReason: "failed",
		},
		{
			name: "StateRunningThenDone",
			statuses: []statusResult{
				{status: shedengine.Status{State: shedengine.StateRunning}, found: true},
				{status: shedengine.Status{State: shedengine.StateDone}, found: true},
			},
			wantDone: true,
		},
		{
			name:      "AbsentStatusAfterSuccessfulSpawn",
			statuses:  []statusResult{{found: false}},
			wantStuck: true,
		},
		{
			name:     "ReadErrorIsReturnedError",
			statuses: []statusResult{{err: errors.New("decode failed")}},
			wantErr:  true,
		},
		{
			name:     "UnrecognizedStateIsReturnedError",
			statuses: []statusResult{{status: shedengine.Status{State: "bogus"}, found: true}},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scratchDir := t.TempDir()
			clock := &fakeClock{now: time.Unix(0, 0)}
			_, _, deps := newInnerRunDeps(nil, nil, tt.statuses, clock)

			producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, 5, scratchDir)
			outcome, _, err := producer.Call(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Call() error = nil; want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if tt.wantDone && outcome != shedengine.Done {
				t.Errorf("Call() outcome = %v; want Done", outcome)
			}
			if tt.wantStuck && outcome != shedengine.Stuck {
				t.Errorf("Call() outcome = %v; want Stuck", outcome)
			}
			if tt.wantReason != "" {
				reason := readStuckFile(t, scratchDir, "innerrun")
				if !strings.Contains(reason, tt.wantReason) {
					t.Errorf("stuck-reason file = %q; want substring %q", reason, tt.wantReason)
				}
			}
		})
	}
}

func TestInnerRun_SpawnFailureIsStuck(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{now: time.Unix(0, 0)}
	spawnErr := errors.New("spawn failed")
	_, spawnCalls, deps := newInnerRunDeps(spawnErr, nil, nil, clock)

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, 5, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %v; want Stuck", outcome)
	}
	if *spawnCalls != 1 {
		t.Errorf("spawn calls = %d; want 1", *spawnCalls)
	}
}

func TestInnerRun_ResolveStatusFailureIsReturnedError(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{now: time.Unix(0, 0)}
	resolveErr := errors.New("resolve failed")
	_, _, deps := newInnerRunDeps(nil, resolveErr, nil, clock)

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, 5, scratchDir)
	_, _, err := producer.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want a returned error")
	}
	if !errors.Is(err, resolveErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, resolveErr)
	}
}

func TestInnerRun_CancelledContext(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{now: time.Unix(0, 0)}
	_, _, deps := newInnerRunDeps(nil, nil, []statusResult{{status: shedengine.Status{State: shedengine.StateDone}, found: true}}, clock)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, 5, scratchDir)
	outcome, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want a non-nil error for a cancelled context")
	}
	if outcome == shedengine.Stuck {
		t.Error("Call() outcome = Stuck; want a cancelled context to never surface as Stuck")
	}
}

// TestInnerRun_StateFailedConsumesZeroPollAttempts asserts StateFailed resolves on its first read,
// calling ReadStatus exactly once and never calling Sleep, proving it does not loop through the
// remaining poll budget the way StateRunning does.
func TestInnerRun_StateFailedConsumesZeroPollAttempts(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{now: time.Unix(0, 0)}
	readCalls, _, deps := newInnerRunDeps(nil, nil, []statusResult{
		{status: shedengine.Status{State: shedengine.StateFailed, Error: "boom"}, found: true},
	}, clock)

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, 5, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %v; want Stuck", outcome)
	}
	if *readCalls != 1 {
		t.Errorf("ReadStatus calls = %d; want exactly 1", *readCalls)
	}
	if clock.sleepCalls != 0 {
		t.Errorf("Sleep calls = %d; want 0", clock.sleepCalls)
	}
}

// TestInnerRun_AttemptCapFiresOnCountNotWallClock proves the poll bound is attempt-counted: with
// the fake clock held still (Now never advances) and Sleep never actually sleeping, exhausting
// pollAttempts while the state stays StateRunning still produces Stuck, in unmeasurable real time.
func TestInnerRun_AttemptCapFiresOnCountNotWallClock(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{now: time.Unix(0, 0)}
	const pollAttempts = 4
	statuses := make([]statusResult, pollAttempts)
	for i := range statuses {
		statuses[i] = statusResult{status: shedengine.Status{State: shedengine.StateRunning}, found: true}
	}
	readCalls, _, deps := newInnerRunDeps(nil, nil, statuses, clock)

	producer := NewInnerRun("innerrun", "myslug", deps, 5*time.Second, pollAttempts, scratchDir)

	start := time.Now()
	outcome, _, err := producer.Call(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %v; want Stuck", outcome)
	}
	if *readCalls != pollAttempts {
		t.Errorf("ReadStatus calls = %d; want exactly %d", *readCalls, pollAttempts)
	}
	if clock.sleepCalls != pollAttempts-1 {
		t.Errorf("Sleep calls = %d; want %d (sleeps happen only between attempts)", clock.sleepCalls, pollAttempts-1)
	}
	if elapsed >= time.Second {
		t.Errorf("Call() took %s of real time; want well under 1s, proving the fake Sleep never actually slept", elapsed)
	}
	reason := readStuckFile(t, scratchDir, "innerrun")
	if !strings.Contains(reason, "4") {
		t.Errorf("stuck-reason file = %q; want it to name the attempt count", reason)
	}
}

// NewInnerRun's nil-seam defaults are exercised implicitly by every production caller; this test
// pins that a nil Now/Sleep resolve to the real stdlib functions rather than panicking.
func TestInnerRun_NilSeamsDefaultToStdlib(t *testing.T) {
	scratchDir := t.TempDir()
	deps := InnerRunDeps{
		Spawn: func(ctx context.Context) error { return nil },
		ResolveStatus: func() (string, string, error) {
			return "/status/path", "/status/lock/path", nil
		},
		ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
			return shedengine.Status{State: shedengine.StateDone}, true, nil
		},
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, 1, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %v; want Done", outcome)
	}
}

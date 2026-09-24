// innerrun_test.go covers NewInnerRun's full re-entrant disposition table over a fake ReadStatus
// and a fake Sleep that never sleeps -- so the still-running case is provably a single
// Stuck with a single sleep, never a bounded poll loop, in unmeasurable real time.

package battenshed

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// fakeClock is a Sleep seam a test can hold still: Sleep records calls without ever blocking.
type fakeClock struct {
	sleepCalls int
}

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) {
	c.sleepCalls++
}

// newInnerRunDeps builds an InnerRunDeps whose ReadStatus returns the next entry of statuses on
// each call, holding on the last entry once exhausted, and whose Spawn returns spawnErr and
// records how many times it was called.
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
			name:     "AlreadyDone",
			statuses: []statusResult{{status: shedengine.Status{State: shedengine.StateDone}, found: true}},
			wantDone: true,
		},
		{
			name:      "StillRunningIsStuck",
			statuses:  []statusResult{{status: shedengine.Status{State: shedengine.StateRunning}, found: true}},
			wantStuck: true,
		},
		{
			name:      "AbsentStatusSpawnsThenRunningIsStuck",
			statuses:  []statusResult{{found: false}, {status: shedengine.Status{State: shedengine.StateRunning}, found: true}},
			wantStuck: true,
		},
		{
			name:       "Blocked",
			statuses:   []statusResult{{status: shedengine.Status{State: shedengine.StateBlocked, Error: "boom", CurrentProducer: "p1"}, found: true}},
			wantErr:    true,
			wantReason: "blocked",
		},
		{
			name:       "Paused",
			statuses:   []statusResult{{status: shedengine.Status{State: shedengine.StatePaused, Error: "paused-err", CurrentProducer: "p2"}, found: true}},
			wantErr:    true,
			wantReason: "paused",
		},
		{
			name:       "Failed",
			statuses:   []statusResult{{status: shedengine.Status{State: shedengine.StateFailed, Error: "failed-err", CurrentProducer: "p3"}, found: true}},
			wantErr:    true,
			wantReason: "failed",
		},
		{
			name:     "AbsentStatusStillAbsentAfterSpawnIsError",
			statuses: []statusResult{{found: false}, {found: false}},
			wantErr:  true,
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
			clock := &fakeClock{}
			_, _, deps := newInnerRunDeps(nil, nil, tt.statuses, clock)

			producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir)
			outcome, _, err := producer.Call(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Call() error = nil; want non-nil")
				}
				if tt.wantReason != "" && !strings.Contains(err.Error(), tt.wantReason) {
					t.Errorf("Call() error = %q; want substring %q", err.Error(), tt.wantReason)
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
		})
	}
}

// TestInnerRun_StuckIsReturnedForRunningAndNoOtherCase is the load-bearing assertion the static
// self-route depends on: every non-running verdict this producer can reach -- Done, or any of the
// three hard-error states -- must never itself be Stuck, since ProducerDef.OnStuck is a static
// per-producer value and once non-empty routes every Stuck from this row back to itself with no
// per-verdict distinction possible.
func TestInnerRun_StuckIsReturnedForRunningAndNoOtherCase(t *testing.T) {
	tests := []struct {
		name   string
		status shedengine.Status
	}{
		{"Done", shedengine.Status{State: shedengine.StateDone}},
		{"Blocked", shedengine.Status{State: shedengine.StateBlocked}},
		{"Paused", shedengine.Status{State: shedengine.StatePaused}},
		{"Failed", shedengine.Status{State: shedengine.StateFailed}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scratchDir := t.TempDir()
			clock := &fakeClock{}
			_, _, deps := newInnerRunDeps(nil, nil, []statusResult{{status: tt.status, found: true}}, clock)

			producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir)
			outcome, _, err := producer.Call(context.Background())
			if outcome == shedengine.Stuck {
				t.Errorf("Call() outcome = Stuck for status %q; want Stuck reserved for the still-running case alone", tt.status.State)
			}
			// A halted child is the one outcome the operator has to act on from inside the task
			// worktree, so the error must say so rather than only name the child's state.
			if tt.status.State != shedengine.StateDone && (err == nil || !strings.Contains(err.Error(), haltedChildRemedy)) {
				t.Errorf("Call() error for status %q = %v; want it to carry the halted-child remedy", tt.status.State, err)
			}
		})
	}
}

func TestInnerRun_SpawnFailureIsReturnedError(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	spawnErr := errors.New("spawn failed")
	_, spawnCalls, deps := newInnerRunDeps(spawnErr, nil, []statusResult{{found: false}}, clock)

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want a returned error")
	}
	if !errors.Is(err, spawnErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, spawnErr)
	}
	if outcome == shedengine.Stuck {
		t.Error("Call() outcome = Stuck; want a spawn failure to hard-error, not Stuck")
	}
	if *spawnCalls != 1 {
		t.Errorf("spawn calls = %d; want 1", *spawnCalls)
	}
}

func TestInnerRun_ResolveStatusFailureIsReturnedError(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	resolveErr := errors.New("resolve failed")
	_, _, deps := newInnerRunDeps(nil, resolveErr, nil, clock)

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir)
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
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(nil, nil, []statusResult{{status: shedengine.Status{State: shedengine.StateDone}, found: true}}, clock)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir)
	outcome, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want a non-nil error for a cancelled context")
	}
	if outcome == shedengine.Stuck {
		t.Error("Call() outcome = Stuck; want a cancelled context to never surface as Stuck")
	}
}

// TestInnerRun_CancelledDuringResolveStatusError, TestInnerRun_CancelledDuringFirstReadStatusError
// and TestInnerRun_CancelledDuringSecondReadStatusError assert the three innerrun.go hard-error
// paths F1 (crucible round sonnet-xhigh-r3) found missing their cancelErr check now carry the
// cancelled-context diagnosis rather than the raw underlying error, when ctx is cancelled by the
// time the failing seam call itself returns.
func TestInnerRun_CancelledDuringResolveStatusError(t *testing.T) {
	scratchDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	resolveErr := errors.New("resolve failed")
	deps := InnerRunDeps{
		Spawn: func(ctx context.Context) error { return nil },
		ResolveStatus: func() (string, string, error) {
			cancel()
			return "", "", resolveErr
		},
		ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
			return shedengine.Status{}, false, nil
		},
		Sleep: (&fakeClock{}).Sleep,
	}

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir)
	_, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want the cancelled-context diagnosis")
	}
	if !strings.Contains(err.Error(), "context cancelled during run") {
		t.Errorf("Call() error = %q; want it to carry the cancelled-context diagnosis, not the raw ResolveStatus error", err.Error())
	}
}

func TestInnerRun_CancelledDuringFirstReadStatusError(t *testing.T) {
	scratchDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	readErr := errors.New("decode failed")
	deps := InnerRunDeps{
		Spawn: func(ctx context.Context) error { return nil },
		ResolveStatus: func() (string, string, error) {
			return "/status/path", "/status/lock/path", nil
		},
		ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
			cancel()
			return shedengine.Status{}, false, readErr
		},
		Sleep: (&fakeClock{}).Sleep,
	}

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir)
	_, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want the cancelled-context diagnosis")
	}
	if !strings.Contains(err.Error(), "context cancelled during run") {
		t.Errorf("Call() error = %q; want it to carry the cancelled-context diagnosis, not the raw ReadStatus error", err.Error())
	}
}

func TestInnerRun_CancelledDuringSecondReadStatusError(t *testing.T) {
	scratchDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	readErr := errors.New("decode failed")
	readCalls := 0
	deps := InnerRunDeps{
		Spawn: func(ctx context.Context) error { return nil },
		ResolveStatus: func() (string, string, error) {
			return "/status/path", "/status/lock/path", nil
		},
		ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
			readCalls++
			if readCalls == 1 {
				// The pre-spawn read: no status file yet, so Call proceeds to Spawn.
				return shedengine.Status{}, false, nil
			}
			// The post-spawn read: this is the one whose own error path is under test.
			cancel()
			return shedengine.Status{}, false, readErr
		},
		Sleep: (&fakeClock{}).Sleep,
	}

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir)
	_, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want the cancelled-context diagnosis")
	}
	if !strings.Contains(err.Error(), "context cancelled during run") {
		t.Errorf("Call() error = %q; want it to carry the cancelled-context diagnosis, not the raw second-read ReadStatus error", err.Error())
	}
}

// TestInnerRun_ReentryAgainstExistingStatusDoesNotRespawn is the second load-bearing assertion:
// once a status file exists, a re-entered Call -- the shape shedengine's on_stuck self-route
// produces -- must read it without spawning again, which is what makes re-entry safe against a
// double spawn.
func TestInnerRun_ReentryAgainstExistingStatusDoesNotRespawn(t *testing.T) {
	scratchDir := t.TempDir()
	if err := os.WriteFile(SpawnConfirmedFile(scratchDir, "innerrun"), []byte("spawned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{}
	_, spawnCalls, deps := newInnerRunDeps(nil, nil, []statusResult{{status: shedengine.Status{State: shedengine.StateRunning}, found: true}}, clock)

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %v; want Stuck", outcome)
	}
	if *spawnCalls != 0 {
		t.Errorf("spawn calls = %d; want 0 against a running status with a confirmed spawn", *spawnCalls)
	}
}

// TestInnerRun_StillRunningSleepsExactlyOnce asserts the still-running arm performs exactly one
// deps.Sleep call and no more -- the single sleep this producer performs, since the wait across
// Call invocations is shedengine's own bounce budget, not a loop inside this producer.
func TestInnerRun_StillRunningSleepsExactlyOnce(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(nil, nil, []statusResult{{status: shedengine.Status{State: shedengine.StateRunning}, found: true}}, clock)

	producer := NewInnerRun("innerrun", "myslug", deps, 5*time.Second, scratchDir)

	start := time.Now()
	outcome, _, err := producer.Call(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %v; want Stuck", outcome)
	}
	if clock.sleepCalls != 1 {
		t.Errorf("Sleep calls = %d; want exactly 1", clock.sleepCalls)
	}
	if elapsed >= time.Second {
		t.Errorf("Call() took %s of real time; want well under 1s, proving the fake Sleep never actually slept", elapsed)
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
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %v; want Done", outcome)
	}
}

// TestWaitOrCancel_ReturnsImmediatelyOnACancelledContext asserts the production sleep value does
// not hold an operator's stop for the whole poll interval.
// It is deadline-based rather than duration-based, so it stays deterministic under -count=5.
func TestWaitOrCancel_ReturnsImmediatelyOnACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		waitOrCancel(ctx, time.Hour)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("waitOrCancel did not return on an already-cancelled context; it waited out its own interval")
	}
}

// TestWaitOrCancel_WaitsOutAShortIntervalWhenNotCancelled asserts the wait is a real wait, not a
// no-op that would satisfy the cancellation test above vacuously.
func TestWaitOrCancel_WaitsOutAShortIntervalWhenNotCancelled(t *testing.T) {
	start := time.Now()
	waitOrCancel(context.Background(), 20*time.Millisecond)
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Errorf("waitOrCancel(background, 20ms) returned after %s; want at least its own interval", elapsed)
	}
}

// TestInnerRun_SpawnsUntilASpawnIsConfirmed pins which child states re-spawn when no spawn has been
// confirmed on this machine. A child bootstrap seeds a running status before it starts its driver,
// so a running status alone is no proof the spawn completed: it is spawned (again), and the
// confirmation is recorded only once Spawn succeeds. A halted or done child is never spawned.
func TestInnerRun_SpawnsUntilASpawnIsConfirmed(t *testing.T) {
	running := statusResult{status: shedengine.Status{State: shedengine.StateRunning}, found: true}
	tests := []struct {
		name          string
		status        statusResult
		confirmed     bool
		spawnErr      error
		wantSpawns    int
		wantConfirmed bool
	}{
		{name: "RunningUnconfirmedSpawns", status: running, wantSpawns: 1, wantConfirmed: true},
		{name: "RunningConfirmedDoesNotSpawn", status: running, confirmed: true, wantSpawns: 0, wantConfirmed: true},
		{name: "RunningUnconfirmedFailedSpawnRecordsNothing", status: running, spawnErr: errors.New("bootstrap exited 1"), wantSpawns: 1, wantConfirmed: false},
		{name: "AbsentStatusClearsAStaleConfirmationBeforeSpawning", status: statusResult{found: false}, confirmed: true, spawnErr: errors.New("bootstrap exited 1"), wantSpawns: 1, wantConfirmed: false},
		{name: "DoneUnconfirmedDoesNotSpawn", status: statusResult{status: shedengine.Status{State: shedengine.StateDone}, found: true}, wantSpawns: 0, wantConfirmed: false},
		{name: "BlockedUnconfirmedDoesNotSpawn", status: statusResult{status: shedengine.Status{State: shedengine.StateBlocked}, found: true}, wantSpawns: 0, wantConfirmed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scratchDir := t.TempDir()
			marker := SpawnConfirmedFile(scratchDir, "innerrun")
			if tt.confirmed {
				if err := os.WriteFile(marker, []byte("spawned\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, spawnCalls, deps := newInnerRunDeps(tt.spawnErr, nil, []statusResult{tt.status}, &fakeClock{})

			_, _, _ = NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir).Call(context.Background())

			if *spawnCalls != tt.wantSpawns {
				t.Errorf("spawn calls = %d; want %d", *spawnCalls, tt.wantSpawns)
			}
			if got := spawnConfirmed(marker); got != tt.wantConfirmed {
				t.Errorf("spawn confirmed = %v; want %v", got, tt.wantConfirmed)
			}
		})
	}
}

// TestInnerRun_FailedSpawnIsRetriedOnTheNextCall walks the live failure shape across two Calls: the
// bootstrap seeds a running status and then fails, and the resumed Call spawns again rather than
// watching a driverless child as running.
func TestInnerRun_FailedSpawnIsRetriedOnTheNextCall(t *testing.T) {
	scratchDir := t.TempDir()
	spawnErr := errors.New("bootstrap exited 1")
	running := statusResult{status: shedengine.Status{State: shedengine.StateRunning}, found: true}
	_, spawnCalls, deps := newInnerRunDeps(nil, nil, []statusResult{{found: false}, running}, &fakeClock{})
	failing := deps
	failing.Spawn = func(ctx context.Context) error {
		*spawnCalls++
		return spawnErr
	}

	if _, _, err := NewInnerRun("innerrun", "myslug", failing, time.Millisecond, scratchDir).Call(context.Background()); !errors.Is(err, spawnErr) {
		t.Fatalf("first Call() error = %v; want it to wrap %v", err, spawnErr)
	}
	outcome, _, err := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir).Call(context.Background())
	if err != nil {
		t.Fatalf("second Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("second Call() outcome = %v; want Stuck (still running)", outcome)
	}
	if *spawnCalls != 2 {
		t.Errorf("spawn calls = %d; want 2 -- the resumed Call must retry the failed spawn", *spawnCalls)
	}
	if !spawnConfirmed(SpawnConfirmedFile(scratchDir, "innerrun")) {
		t.Error("spawn confirmed = false after a successful retry; want true")
	}
}

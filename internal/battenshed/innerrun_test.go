// innerrun_test.go covers the re-entrant disposition table of NewInnerRun, over a fake ReadStatus and a fake clock.
// The fake Sleep never blocks, so a still-running child yields once per pass, and no real time passes.

package battenshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeClock is the Sleep and Now seam of the passes a test drives:
// Sleep moves the clock by the interval and never blocks.
// It also owns the fake child's on-disk status file and the fake batten status's pause flag.
// A test changes them at the end of a pass, so no real time has to pass.
type fakeClock struct {
	sleepCalls int
	now        time.Time
	// t fails a run of passes that never ends.
	t *testing.T
	// statusPath is the fake child's status file, which newInnerRunDeps writes, and statusBumps counts the bumpStatus calls.
	statusPath  string
	statusBumps int
	// pauseAtSleep is the number of Sleep calls from which the fake batten status carries pause_requested; zero means never.
	// newInnerRunDeps sets it to 1, so passes that nothing else ends return at the first pass after a Sleep.
	pauseAtSleep int
	// onSleep runs at every Sleep, between two passes, with the number of Sleep calls so far.
	onSleep func(call int)
	// reasonFile, when set, is the stuck-reason file that pauseRequested reads at the end of every pass.
	// It appends the file's content to reasons, so a test reads the wait reason in force at each pass.
	reasonFile string
	reasons    []string
}

// testGrace is the driver-exit grace every test producer is built with unless it says otherwise.
const testGrace = 10 * time.Minute

// maxFakeSleeps bounds the passes of one test, so passes nothing ends fail instead of hanging.
const maxFakeSleeps = 500

// Now is the fake clock's time, moved by advance and Sleep.
func (c *fakeClock) Now() time.Time {
	if c.now.IsZero() {
		c.now = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	}
	return c.now
}

// advance moves the fake clock forward by d.
func (c *fakeClock) advance(d time.Duration) { c.now = c.Now().Add(d) }

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) {
	c.sleepCalls++
	c.advance(d)
	if c.sleepCalls > maxFakeSleeps {
		if c.t != nil {
			c.t.Fatalf("the run made %d passes without ending", c.sleepCalls)
		}
		panic("the run made too many passes without ending")
	}
	if c.onSleep != nil {
		c.onSleep(c.sleepCalls)
	}
}

// producerFunc adapts a function to a shedengine.ShedProducer.
type producerFunc func(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)

func (f producerFunc) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return f(ctx)
}

// freshPerCall returns a producer whose every Call builds a new producer with newProducer and calls it once.
// Only the memory record in the scratch directory carries state from one Call to the next.
// The same holds between two runs of `lym batten run`.
func freshPerCall(newProducer func() shedengine.ShedProducer) shedengine.ShedProducer {
	return producerFunc(func(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
		return newProducer().Call(ctx)
	})
}

// passDriver is a producer whose Call runs successive passes of the Run-Shed row, the way `lym batten run` does.
// Each pass is one Call of a producer that newProducer builds fresh over the same scratch directory.
// Only the memory record carries state from a pass to the next.
// Between two passes the clock advances by interval.
// Its Call returns the first pass that ends with something other than Yield.
type passDriver struct {
	clock       *fakeClock
	newProducer func() shedengine.ShedProducer
	interval    time.Duration
}

// passes returns a passDriver over newProducer that lets interval pass between two passes.
func (c *fakeClock) passes(newProducer func() shedengine.ShedProducer, interval time.Duration) shedengine.ShedProducer {
	return &passDriver{clock: c, newProducer: newProducer, interval: interval}
}

func (d *passDriver) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	for {
		outcome, pointer, err := d.newProducer().Call(ctx)
		if outcome != shedengine.Yield || err != nil {
			return outcome, pointer, err
		}
		d.clock.Sleep(ctx, d.interval)
	}
}

// watchReason makes the clock record the wait reason at the end of every check.
// It reads the stuck-reason file in scratchDir.
func (c *fakeClock) watchReason(scratchDir string) {
	c.reasonFile = StuckReasonFile(scratchDir, "innerrun")
}

// lastReason is the wait reason in force at the end of the newest check.
func (c *fakeClock) lastReason() string {
	if len(c.reasons) == 0 {
		return ""
	}
	return c.reasons[len(c.reasons)-1]
}

// pauseRequested is the fake batten status's pause_requested.
// When reasonFile is set and readable, it also appends the file's content to reasons.
func (c *fakeClock) pauseRequested() (bool, error) {
	if raw, err := os.ReadFile(c.reasonFile); err == nil {
		c.reasons = append(c.reasons, strings.TrimSuffix(string(raw), "\n"))
	}
	return c.pauseAtSleep > 0 && c.sleepCalls >= c.pauseAtSleep, nil
}

// bumpStatus moves the fake child's status file modification time, which makes the wait decode it.
func (c *fakeClock) bumpStatus() {
	c.t.Helper()
	c.statusBumps++
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(c.statusBumps) * time.Second)
	if err := os.Chtimes(c.statusPath, at, at); err != nil {
		c.t.Fatal(err)
	}
}

// newInnerRunDeps builds an InnerRunDeps whose ReadStatus returns the next entry of statuses on each call.
// Once the entries run out, ReadStatus keeps returning the last one.
// Its Spawn returns spawnErr and counts its calls.
// The fake child keeps a real status file, since the wait stats it,
// and the deps' pause seam ends the wait at its first check unless the clock says otherwise.
func newInnerRunDeps(t *testing.T, spawnErr error, resolveErr error, statuses []statusResult, clock *fakeClock) (*int, *int, InnerRunDeps) {
	t.Helper()
	clock.t = t
	clock.statusPath = filepath.Join(t.TempDir(), "status.json")
	if err := os.WriteFile(clock.statusPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	clock.bumpStatus()
	clock.pauseAtSleep = 1
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
			return clock.statusPath, "/status/lock/path", nil
		},
		PauseRequested: clock.pauseRequested,
		// The wait does its probe work at entry only, unless a test lowers this;
		// a status decode then happens only after bumpStatus.
		NoticeProbe: time.Hour,
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
		Now:          clock.Now,
		ReadDecision: func() (ChildDecision, bool, error) { return ChildDecision{}, false, nil },
		DriverAlive:  func(ctx context.Context) (bool, error) { return false, nil },
	}
	return &readCalls, &spawnCalls, deps
}

type statusResult struct {
	status shedengine.Status
	found  bool
	err    error
}

func TestInnerRun_VerdictTable(t *testing.T) {
	tiergate.Require(t, 0)
	running := statusResult{status: shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Review"}, found: true}
	tests := []struct {
		name     string
		statuses []statusResult
		// changeAtCheck is the check at which the test rewrites the child's status file,
		// so the next entry of statuses is read; zero means never.
		changeAtCheck int
		wantDone      bool
		wantStuck     bool
		wantErr       bool
		wantReason    string
		// wantPath is the prefix of the returned Stuck's Path, which names what ended the wait.
		wantPath string
	}{
		{
			// A done child whose driver is gone must never itself be Stuck.
			// ProducerDef.OnStuck is one static value per producer,
			// so it routes every Stuck from this row back to the row itself, with no per-verdict distinction possible.
			// Every Stuck this row returns is a timed wait.
			// The halted arm's budget-exempt waits are covered in innerrun_halted_test.go.
			name:     "AlreadyDone",
			statuses: []statusResult{{status: shedengine.Status{State: shedengine.StateDone}, found: true}},
			wantDone: true,
		},
		{
			name:      "RunningWaitsUntilPaused",
			statuses:  []statusResult{running},
			wantStuck: true,
			wantPath:  "pause requested at ",
		},
		{
			name:      "AbsentStatusSpawnsThenRunningWaitsUntilPaused",
			statuses:  []statusResult{{found: false}, running},
			wantStuck: true,
			wantPath:  "pause requested at ",
		},
		{
			name:          "BlockedWaitsUntilTheStateChanges",
			statuses:      []statusResult{{status: shedengine.Status{State: shedengine.StateBlocked, Error: "boom", CurrentProducer: "p1"}, found: true}, running},
			changeAtCheck: 1,
			wantStuck:     true,
			wantPath:      "child blocked → running at Plan-Review",
		},
		{
			name:          "PausedWaitsUntilTheStateChanges",
			statuses:      []statusResult{{status: shedengine.Status{State: shedengine.StatePaused, Error: "paused-err", CurrentProducer: "p2"}, found: true}, running},
			changeAtCheck: 1,
			wantStuck:     true,
			wantPath:      "child paused → running at Plan-Review",
		},
		{
			name:          "FailedWaitsUntilTheStateChanges",
			statuses:      []statusResult{{status: shedengine.Status{State: shedengine.StateFailed, Error: "failed-err", CurrentProducer: "p3"}, found: true}, running},
			changeAtCheck: 1,
			wantStuck:     true,
			wantPath:      "child failed → running at Plan-Review",
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
			name:          "DecodeErrorInTheWaitIsReturnedError",
			statuses:      []statusResult{running, {err: errors.New("decode failed")}},
			changeAtCheck: 1,
			wantErr:       true,
			wantReason:    "decode failed",
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
			_, _, deps := newInnerRunDeps(t, nil, nil, tt.statuses, clock)
			if tt.changeAtCheck > 0 {
				clock.onSleep = func(call int) {
					if call == tt.changeAtCheck {
						clock.bumpStatus()
					}
				}
			}

			producer := clock.passes(func() shedengine.ShedProducer {
				return NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)
			}, time.Second)
			outcome, ptr, err := producer.Call(context.Background())

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
			if tt.wantStuck {
				data, readErr := os.ReadFile(filepath.Join(scratchDir, "innerrun"+stuckFileSuffix))
				if readErr != nil {
					t.Fatalf("read stuck file: %v", readErr)
				}
				if ptr.Reason == "" || ptr.Reason+"\n" != string(data) {
					t.Errorf("returned Reason = %q; want the reason file's line %q", ptr.Reason, strings.TrimSuffix(string(data), "\n"))
				}
				if !ptr.BudgetExempt || ptr.Path != ptr.Reason || !strings.HasPrefix(ptr.Path, tt.wantPath) {
					t.Errorf("returned pointer = %+v; want a budget-exempt Stuck whose Path mirrors its Reason and starts %q", ptr, tt.wantPath)
				}
			}
		})
	}
}

func TestInnerRun_SpawnFailureIsReturnedError(t *testing.T) {
	tiergate.Require(t, 0)
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	spawnErr := errors.New("spawn failed")
	_, spawnCalls, deps := newInnerRunDeps(t, spawnErr, nil, []statusResult{{found: false}}, clock)

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)
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
	tiergate.Require(t, 0)
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	resolveErr := errors.New("resolve failed")
	_, _, deps := newInnerRunDeps(t, nil, resolveErr, nil, clock)

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)
	_, _, err := producer.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want a returned error")
	}
	if !errors.Is(err, resolveErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, resolveErr)
	}
}

func TestInnerRun_CancelledContext(t *testing.T) {
	tiergate.Require(t, 0)
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{{status: shedengine.Status{State: shedengine.StateDone}, found: true}}, clock)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)
	outcome, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want a non-nil error for a cancelled context")
	}
	if outcome == shedengine.Stuck {
		t.Error("Call() outcome = Stuck; want a cancelled context to never surface as Stuck")
	}
}

// TestInnerRun_CancelledDuringASeamErrorReportsTheCancellation asserts the three hard-error paths.
// They are the status-path resolve, the pre-spawn read and the post-spawn read.
// Each reports the cancelled context rather than the raw error when ctx is cancelled by the time the seam call returns.
func TestInnerRun_CancelledDuringASeamErrorReportsTheCancellation(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	readErr := errors.New("decode failed")
	tests := []struct {
		name string
		deps func(cancel context.CancelFunc) InnerRunDeps
	}{
		{
			name: "ResolveStatus",
			deps: func(cancel context.CancelFunc) InnerRunDeps {
				return InnerRunDeps{
					Spawn: func(ctx context.Context) error { return nil },
					ResolveStatus: func() (string, string, error) {
						cancel()
						return "", "", errors.New("resolve failed")
					},
					ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
						return shedengine.Status{}, false, nil
					},
				}
			},
		},
		{
			name: "FirstReadStatus",
			deps: func(cancel context.CancelFunc) InnerRunDeps {
				return InnerRunDeps{
					Spawn: func(ctx context.Context) error { return nil },
					ResolveStatus: func() (string, string, error) {
						return "/status/path", "/status/lock/path", nil
					},
					ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
						cancel()
						return shedengine.Status{}, false, readErr
					},
				}
			},
		},
		{
			name: "SecondReadStatus",
			deps: func(cancel context.CancelFunc) InnerRunDeps {
				readCalls := 0
				return InnerRunDeps{
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
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			producer := NewInnerRun("innerrun", "myslug", tt.deps(cancel), time.Millisecond, t.TempDir(), testGrace)

			_, _, err := producer.Call(ctx)
			if err == nil {
				t.Fatal("Call() error = nil; want the cancelled-context diagnosis")
			}
			if !strings.Contains(err.Error(), "context cancelled during run") {
				t.Errorf("Call() error = %q; want it to carry the cancelled-context diagnosis, not the raw seam error", err.Error())
			}
		})
	}
}

// This test pins that a nil Now and a nil Sleep seam resolve to the stdlib functions rather than panicking.
func TestInnerRun_NilSeamsDefaultToStdlib(t *testing.T) {
	tiergate.Require(t, 0)
	scratchDir := t.TempDir()
	statusPath := filepath.Join(t.TempDir(), "status.json")
	if err := os.WriteFile(statusPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := InnerRunDeps{
		Spawn: func(ctx context.Context) error { return nil },
		ResolveStatus: func() (string, string, error) {
			return statusPath, "/status/lock/path", nil
		},
		ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
			return shedengine.Status{State: shedengine.StateDone}, true, nil
		},
		ReadDecision: func() (ChildDecision, bool, error) { return ChildDecision{}, false, nil },
		DriverAlive:  func(ctx context.Context) (bool, error) { return false, nil },
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)
	shedfake.RequireOutcome(t, producer, shedengine.Done)
}

// TestNewInnerRun_FloorsPollInterval pins that a poll interval below one second logs one Warn naming poll_interval_s.
// One second passes through unwarned.
func TestNewInnerRun_FloorsPollInterval(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name     string
		told     time.Duration
		wantWarn bool
	}{
		{"sub-second is floored with one warning", time.Millisecond, true},
		{"one second passes through", time.Second, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.Capture(t)
			_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{{status: shedengine.Status{State: shedengine.StateRunning}, found: true}}, &fakeClock{})

			NewInnerRun("innerrun", "myslug", deps, tt.told, t.TempDir(), testGrace)

			warned := strings.Count(logs.String(), "key=poll_interval_s") == 1 && strings.Contains(logs.String(), "floor_s=1")
			if warned != tt.wantWarn {
				t.Errorf("warned = %v, want %v; log = %q", warned, tt.wantWarn, logs.String())
			}
		})
	}
}

// TestWaitOrCancel_ReturnsImmediatelyOnACancelledContext asserts the production sleep returns at once,
// so an operator's stop is not held for the whole poll interval.
// The test bounds the wait with a deadline, so it stays deterministic under -count=5.
func TestWaitOrCancel_ReturnsImmediatelyOnACancelledContext(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestWaitOrCancel_WaitsOutAShortIntervalWhenNotCancelled asserts the wait is a real wait,
// so a no-op wait cannot pass the cancellation test vacuously.
func TestWaitOrCancel_WaitsOutAShortIntervalWhenNotCancelled(t *testing.T) {
	tiergate.Require(t, 0)
	start := time.Now()
	waitOrCancel(context.Background(), 20*time.Millisecond)
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Errorf("waitOrCancel(background, 20ms) returned after %s; want at least its own interval", elapsed)
	}
}

// pidMarker renders a spawn-confirmation marker holding pid.
func pidMarker(pid int) []byte { return []byte(strconv.Itoa(pid) + "\n") }

// TestInnerRun_SpawnsUntilASpawnIsConfirmed pins which child states re-spawn until this process confirms a spawn.
// A marker from another pid or in the old layout is no confirmation.
// A child bootstrap seeds a running status before it starts its driver,
// so a running status alone does not prove the spawn completed.
// Such a child is spawned again, and the confirmation is recorded only once Spawn succeeds.
// A halted or done child is never spawned.
func TestInnerRun_SpawnsUntilASpawnIsConfirmed(t *testing.T) {
	tiergate.Require(t, 0)
	running := statusResult{status: shedengine.Status{State: shedengine.StateRunning}, found: true}
	tests := []struct {
		name   string
		status statusResult
		// marker is the confirmation marker's content before the Call;
		// nil writes none.
		marker        []byte
		spawnErr      error
		wantSpawns    int
		wantConfirmed bool
	}{
		{name: "RunningUnconfirmedSpawns", status: running, wantSpawns: 1, wantConfirmed: true},
		{name: "RunningConfirmedByThisProcessDoesNotSpawn", status: running, marker: pidMarker(os.Getpid()), wantSpawns: 0, wantConfirmed: true},
		{name: "RunningConfirmedByAnEarlierPidSpawns", status: running, marker: pidMarker(os.Getpid() + 1), wantSpawns: 1, wantConfirmed: true},
		{name: "RunningConfirmedInTheOldLayoutSpawns", status: running, marker: []byte("spawned\n"), wantSpawns: 1, wantConfirmed: true},
		{name: "RunningUnconfirmedFailedSpawnRecordsNothing", status: running, spawnErr: errors.New("bootstrap exited 1"), wantSpawns: 1, wantConfirmed: false},
		{name: "AbsentStatusClearsAStaleConfirmationBeforeSpawning", status: statusResult{found: false}, marker: pidMarker(os.Getpid()), spawnErr: errors.New("bootstrap exited 1"), wantSpawns: 1, wantConfirmed: false},
		{name: "DoneUnconfirmedDoesNotSpawn", status: statusResult{status: shedengine.Status{State: shedengine.StateDone}, found: true}, wantSpawns: 0, wantConfirmed: false},
		{name: "BlockedUnconfirmedDoesNotSpawn", status: statusResult{status: shedengine.Status{State: shedengine.StateBlocked}, found: true}, wantSpawns: 0, wantConfirmed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scratchDir := t.TempDir()
			marker := SpawnConfirmedFile(scratchDir, "innerrun")
			if tt.marker != nil {
				if err := os.WriteFile(marker, tt.marker, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, spawnCalls, deps := newInnerRunDeps(t, tt.spawnErr, nil, []statusResult{tt.status}, &fakeClock{})

			_, _, _ = NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace).Call(context.Background())

			if *spawnCalls != tt.wantSpawns {
				t.Errorf("spawn calls = %d; want %d", *spawnCalls, tt.wantSpawns)
			}
			if got := spawnConfirmed(marker); got != tt.wantConfirmed {
				t.Errorf("spawn confirmed = %v; want %v", got, tt.wantConfirmed)
			}
		})
	}
}

// TestInnerRun_AdoptsARunningChildWithADriver pins the running arm over an unconfirmed spawn:
// a live or retiring driver strand, a held child run lock, or a held child driver lock, means a driver is at work,
// so the child is adopted (the confirmation recorded for this process, nothing spawned);
// no strand or a dead one with both locks free spawns;
// a seam error is a hard error that spawns nothing.
func TestInnerRun_AdoptsARunningChildWithADriver(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	running := statusResult{status: shedengine.Status{State: shedengine.StateRunning}, found: true}
	seamErr := errors.New("reed state unreadable")
	tests := []struct {
		name           string
		strand         ChildDriverStrand
		strandErr      error
		lockHeld       bool
		lockErr        error
		driverLockHeld bool
		wantSpawns     int
		wantAdopt      bool
		wantErr        error
	}{
		{name: "LiveStrandIsAdopted", strand: ChildDriverLive, wantAdopt: true},
		{name: "RetiringStrandIsAdopted", strand: ChildDriverRetiring, wantAdopt: true},
		{name: "HeldRunLockIsAdopted", strand: ChildDriverNone, lockHeld: true, wantAdopt: true},
		{name: "HeldDriverLockIsAdopted", strand: ChildDriverNone, driverLockHeld: true, wantAdopt: true},
		{name: "NoStrandWithAFreeLockSpawns", strand: ChildDriverNone, wantSpawns: 1},
		{name: "DeadStrandWithAFreeLockSpawns", strand: ChildDriverDead, wantSpawns: 1},
		{name: "StrandReadErrorIsAHardError", strandErr: seamErr, wantErr: seamErr},
		{name: "LockReadErrorIsAHardError", lockErr: seamErr, wantErr: seamErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scratchDir := t.TempDir()
			marker := SpawnConfirmedFile(scratchDir, "innerrun")
			if err := os.WriteFile(marker, pidMarker(os.Getpid()+1), 0o644); err != nil {
				t.Fatal(err)
			}
			_, spawnCalls, deps := newInnerRunDeps(t, nil, nil, []statusResult{running}, &fakeClock{})
			deps.DriverStrand = func(context.Context) (ChildDriverStrand, error) { return tt.strand, tt.strandErr }
			deps.ChildRunLockHeld = func() (bool, error) { return tt.lockHeld, tt.lockErr }
			deps.ChildDriverLockHeld = func() (bool, error) { return tt.driverLockHeld, nil }

			_, _, err := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace).Call(context.Background())

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Call() error = %v; want %v", err, tt.wantErr)
			}
			if *spawnCalls != tt.wantSpawns {
				t.Errorf("spawn calls = %d; want %d", *spawnCalls, tt.wantSpawns)
			}
			if got := spawnConfirmed(marker); got != (tt.wantAdopt || tt.wantSpawns > 0) {
				t.Errorf("spawn confirmed = %v after the Call; want %v", got, tt.wantAdopt || tt.wantSpawns > 0)
			}
		})
	}
}

// TestInnerRun_FailedSpawnIsRetriedOnTheNextCall walks the failure shape across two Calls:
// the bootstrap seeds a running status and then fails,
// and the resumed Call spawns again rather than watching a driverless child as running.
func TestInnerRun_FailedSpawnIsRetriedOnTheNextCall(t *testing.T) {
	tiergate.Require(t, 0)
	scratchDir := t.TempDir()
	spawnErr := errors.New("bootstrap exited 1")
	running := statusResult{status: shedengine.Status{State: shedengine.StateRunning}, found: true}
	clock := &fakeClock{}
	_, spawnCalls, deps := newInnerRunDeps(t, nil, nil, []statusResult{{found: false}, running}, clock)
	failing := deps
	failing.Spawn = func(ctx context.Context) error {
		*spawnCalls++
		return spawnErr
	}

	if _, _, err := NewInnerRun("innerrun", "myslug", failing, time.Millisecond, scratchDir, testGrace).Call(context.Background()); !errors.Is(err, spawnErr) {
		t.Fatalf("first Call() error = %v; want it to wrap %v", err, spawnErr)
	}
	shedfake.RequireOutcome(t, clock.passes(func() shedengine.ShedProducer {
		return NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)
	}, time.Second), shedengine.Stuck)
	if *spawnCalls != 2 {
		t.Errorf("spawn calls = %d; want 2 -- the resumed Call must retry the failed spawn", *spawnCalls)
	}
	if !spawnConfirmed(SpawnConfirmedFile(scratchDir, "innerrun")) {
		t.Error("spawn confirmed = false after a successful retry; want true")
	}
}

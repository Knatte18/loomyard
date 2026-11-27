// run_test.go drives the pusher's body over fake seams, a real push ledger, a fake clock and a context the test controls.

package runpusher_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

var (
	errOwed       = errors.New("push owed")
	errRefused    = errors.New("push refused")
	errHandedOver = errors.New("run handed over")
	errGuardStop  = errors.New("guard stop")
	errReadFailed = errors.New("read failed")
)

// fakeStart is the time the fake clock starts at.
var fakeStart = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

// recordsKey is the ledger key of the fake target.
const recordsKey = pushledger.Key("wt-records")

// runFake answers each seam from a queue, and records what Run did.
// A queue that runs out answers its default: false for the reads of work, true for the conditions and a free lock.
type runFake struct {
	ahead       []bool
	conditions  []bool
	gone        []bool
	paused      []bool
	lockResults []bool
	pushErrs    []error
	aheadErr    error
	condErr     error
	lockErr     error
	guardErr    error
	// key is the ledger key of the target, the records key when empty.
	key pushledger.Key
	// onWait runs at every wait before the clock advances, and lets a row cancel the context or mark the entry.
	onWait func()

	hub        string
	now        time.Time
	pushes     int
	guardCalls int
	locksTaken int
	locksFreed int
	waits      []time.Duration
}

// pop returns the head of queue and shortens it, or fallback when it is empty.
func pop(queue *[]bool, fallback bool) bool {
	if len(*queue) == 0 {
		return fallback
	}
	head := (*queue)[0]
	*queue = (*queue)[1:]
	return head
}

// seed writes entry as the target's ledger entry.
func (f *runFake) seed(t *testing.T, entry pushledger.Entry) {
	t.Helper()

	err := pushledger.Update(f.hub, f.targetKey(), f.now, func(pushledger.Entry, bool) (pushledger.Entry, bool, error) {
		return entry, true, nil
	})
	if err != nil {
		t.Fatalf("seed the ledger entry: %v", err)
	}
}

func (f *runFake) targetKey() pushledger.Key {
	if f.key == "" {
		return recordsKey
	}
	return f.key
}

// deps builds the seams over the fake, with the pair's pause and guard seams only when pair is set.
// The timing stays zero, so the pusher runs on its defaults.
func (f *runFake) deps(pair bool) runpusher.Deps {
	deps := runpusher.Deps{
		Lock: func() (func(), bool, error) {
			if f.lockErr != nil {
				return nil, false, f.lockErr
			}
			if !pop(&f.lockResults, true) {
				return nil, false, nil
			}
			f.locksTaken++
			return func() { f.locksFreed++ }, true, nil
		},
		Ahead: func() (bool, error) { return pop(&f.ahead, false), f.aheadErr },
		Conditions: func() (bool, error) {
			return pop(&f.conditions, true), f.condErr
		},
		WorktreeGone: func() (bool, error) { return pop(&f.gone, false), nil },
		Push: func() error {
			f.pushes++
			if len(f.pushErrs) == 0 {
				return nil
			}
			err := f.pushErrs[0]
			f.pushErrs = f.pushErrs[1:]
			return err
		},
		Owed:    func(err error) bool { return errors.Is(err, errOwed) },
		HubPath: f.hub,
		Key:     f.targetKey(),
		Now:     func() time.Time { return f.now },
		Wait: func(ctx context.Context, d time.Duration, _ <-chan struct{}) error {
			f.waits = append(f.waits, d)
			if f.onWait != nil {
				f.onWait()
			}
			f.now = f.now.Add(d)
			return ctx.Err()
		},
	}
	if pair {
		deps.PauseCovers = func() (bool, error) { return pop(&f.paused, false), nil }
		deps.Guard = func() error {
			f.guardCalls++
			return f.guardErr
		}
	}
	return deps
}

// aheadTimes returns a queue answering n reads of work with true.
func aheadTimes(n int) []bool {
	queue := make([]bool, n)
	for i := range queue {
		queue[i] = true
	}
	return queue
}

// TestRun covers the pusher's pass: its lock, its stops, its pushes, the interval between them and the ledger entry.
func TestRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	seconds := func(values ...int) []time.Duration {
		waits := make([]time.Duration, len(values))
		for i, v := range values {
			waits[i] = time.Duration(v) * time.Second
		}
		return waits
	}
	entryKept := func(t *testing.T, f *runFake) {
		t.Helper()
		entry, found, err := pushledger.Read(f.hub, f.targetKey())
		if err != nil || !found || !entry.Owed() {
			t.Errorf("ledger entry = %+v, %v, %v; want it kept as owed", entry, found, err)
		}
	}
	tests := []struct {
		name           string
		pair           bool
		setup          func(t *testing.T, f *runFake, cancel context.CancelFunc)
		wantPushes     int
		wantErr        error
		wantGuardCalls int
		wantLocks      int
		wantWaits      []time.Duration
		// check reads the ledger after the run, and is nil when the row pins nothing there.
		check func(t *testing.T, f *runFake)
	}{
		{
			name: "LevelAfterOnePushRemovesTheEntry", wantPushes: 1, wantLocks: 1,
			setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) { f.ahead = aheadTimes(1) },
			check: func(t *testing.T, f *runFake) {
				if entry, found, err := pushledger.Read(f.hub, recordsKey); err != nil || found {
					t.Errorf("ledger entry = %+v, %v, %v; want none", entry, found, err)
				}
			},
		},
		{
			name: "ATipMovedDuringThePushWaitsTheDefaultInterval", wantPushes: 2, wantLocks: 1, wantWaits: seconds(60),
			setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) { f.ahead = aheadTimes(2) },
		},
		{
			name: "ACommitWhileExitingWithTheLockFreeAgain", wantPushes: 2, wantLocks: 2,
			setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) { f.ahead = []bool{true, false, true, true} },
		},
		{
			name: "ACommitWhileExitingWithTheLockTakenByAnother", wantPushes: 1, wantLocks: 1,
			setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) {
				f.ahead = []bool{true, false, true}
				f.lockResults = []bool{true, false}
			},
		},
		{name: "LockHeldAtStart", setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) { f.lockResults = []bool{false} }},
		{name: "WorktreeGone", setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) { f.gone = []bool{true} }, wantLocks: 1},
		{name: "ConditionsFail", setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) { f.conditions = []bool{false} }, wantLocks: 1},
		{name: "LockTakeFails", setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) { f.lockErr = errReadFailed }, wantErr: errReadFailed},
		{name: "AheadReadFails", setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) { f.aheadErr = errReadFailed }, wantErr: errReadFailed, wantLocks: 1},
		{name: "ConditionsReadFails", setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) { f.condErr = errReadFailed }, wantErr: errReadFailed, wantLocks: 1},
		{
			name: "APauseCoversTheRunBeforeTheGuard", pair: true, wantLocks: 1,
			setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) { f.ahead = aheadTimes(1); f.paused = []bool{true} },
		},
		{
			name: "APauseDuringAnOwedWaitStopsTheNextAttempt", pair: true, wantPushes: 1, wantGuardCalls: 1, wantLocks: 1,
			wantWaits: seconds(120), check: entryKept,
			setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) {
				f.ahead = aheadTimes(2)
				f.paused = []bool{false, true}
				f.pushErrs = []error{errOwed}
			},
		},
		{
			name: "APauseAfterALandedPushWhileStillAhead", pair: true, wantPushes: 1, wantGuardCalls: 1, wantLocks: 1,
			wantWaits: seconds(60),
			setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) {
				f.ahead = aheadTimes(2)
				f.paused = []bool{false, true}
			},
		},
		{
			name: "AGuardStopPushesNothing", pair: true, wantErr: errGuardStop, wantGuardCalls: 1, wantLocks: 1,
			setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) {
				f.ahead = aheadTimes(1)
				f.guardErr = errGuardStop
			},
		},
		{
			name: "OwedFailuresDoubleTheIntervalUpToTheCeiling", wantPushes: 5, wantLocks: 1, wantWaits: seconds(60, 120, 240, 300),
			setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) {
				f.ahead = aheadTimes(5)
				f.pushErrs = []error{nil, errOwed, errOwed, errOwed}
			},
		},
		{
			name: "ALandedPushReturnsTheIntervalToTheBase", wantPushes: 3, wantLocks: 1, wantWaits: seconds(120, 60),
			setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) {
				f.ahead = aheadTimes(3)
				f.pushErrs = []error{errOwed}
			},
		},
		{
			name: "ASecondPusherWaitsOutTheRestOfTheInterval", wantPushes: 1, wantLocks: 1, wantWaits: seconds(40),
			setup: func(t *testing.T, f *runFake, _ context.CancelFunc) {
				f.ahead = aheadTimes(1)
				f.seed(t, pushledger.Entry{FirstOwed: f.now.Add(-time.Minute), LastAttempt: f.now.Add(-20 * time.Second), IntervalS: 60})
			},
		},
		{
			name: "APushNowMarkSkipsOneWait", wantPushes: 2, wantLocks: 1, wantWaits: seconds(60),
			setup: func(t *testing.T, f *runFake, _ context.CancelFunc) {
				f.ahead = aheadTimes(2)
				f.seed(t, pushledger.Entry{FirstOwed: f.now, LastAttempt: f.now, IntervalS: 60, PushNow: true})
			},
		},
		{
			name: "ALevelPassKeepsAHeldEntryAsNotOwed", wantLocks: 1,
			setup: func(t *testing.T, f *runFake, _ context.CancelFunc) {
				f.seed(t, pushledger.Entry{HeldRecords: "abc", FirstOwed: f.now, LastAttempt: f.now, IntervalS: 120})
			},
			check: func(t *testing.T, f *runFake) {
				entry, found, err := pushledger.Read(f.hub, recordsKey)
				if err != nil || !found || entry.HeldRecords != "abc" || entry.Owed() || !entry.LastAttempt.IsZero() || entry.IntervalS != 0 {
					t.Errorf("ledger entry = %+v, %v, %v; want the held records kept with no timing, not owed", entry, found, err)
				}
			},
		},
		{
			name: "ALandingTargetThatPushedNothingWaitsOnItsMark", wantPushes: 2, wantLocks: 1, wantWaits: seconds(300),
			setup: func(t *testing.T, f *runFake, _ context.CancelFunc) {
				f.key = pushledger.LandingKey
				f.ahead = aheadTimes(2)
				landing := pushledger.Landing{Run: "task", Tip: "abc", ParentBranch: "trunk", At: f.now}
				f.seed(t, pushledger.Entry{Landings: []pushledger.Landing{landing}})
				f.onWait = func() {
					f.seed(t, pushledger.Entry{Landings: []pushledger.Landing{landing}, PushNow: true})
				}
			},
		},
		{
			name: "AnAuthRefusalReturnsAfterOnePushWithTheEntryKept", wantPushes: 1, wantErr: errRefused, wantLocks: 1,
			setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) {
				f.ahead = aheadTimes(2)
				f.pushErrs = []error{errRefused}
			},
			check: entryKept,
		},
		{
			name: "AHandedOverPushReturns", wantPushes: 1, wantErr: errHandedOver, wantLocks: 1,
			setup: func(_ *testing.T, f *runFake, _ context.CancelFunc) {
				f.ahead = aheadTimes(1)
				f.pushErrs = []error{errHandedOver}
			},
		},
		{
			name: "ACancelledContextDuringTheWait", wantPushes: 1, wantErr: context.Canceled, wantLocks: 1, wantWaits: seconds(120),
			setup: func(_ *testing.T, f *runFake, cancel context.CancelFunc) {
				f.ahead = aheadTimes(2)
				f.pushErrs = []error{errOwed}
				f.onWait = cancel
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := &runFake{hub: t.TempDir(), now: fakeStart}
			tt.setup(t, f, cancel)
			target := runpusher.Target{Location: &lymcwd.Location{HubPath: f.hub, WorktreeName: "wt"}, Pair: tt.pair}

			err := runpusher.Run(ctx, target, f.deps(tt.pair))

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Run() error = %v; want %v", err, tt.wantErr)
			}
			if f.pushes != tt.wantPushes {
				t.Errorf("pushes = %d; want %d", f.pushes, tt.wantPushes)
			}
			if f.guardCalls != tt.wantGuardCalls {
				t.Errorf("guard calls = %d; want %d", f.guardCalls, tt.wantGuardCalls)
			}
			if f.locksTaken != tt.wantLocks || f.locksFreed != f.locksTaken {
				t.Errorf("locks taken = %d, released = %d; want %d taken and all released", f.locksTaken, f.locksFreed, tt.wantLocks)
			}
			if !reflect.DeepEqual(f.waits, tt.wantWaits) && len(f.waits)+len(tt.wantWaits) > 0 {
				t.Errorf("waits = %v; want %v", f.waits, tt.wantWaits)
			}
			if tt.check != nil {
				tt.check(t, f)
			}
		})
	}
}

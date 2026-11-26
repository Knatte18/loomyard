// run_test.go drives the pusher's body over fake seams, a recording sleep and a context the test controls.

package runpusher_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
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
	// onSleep runs at every sleep, and lets a row cancel the context during a backoff.
	onSleep func()

	pushes     int
	guardCalls int
	locksTaken int
	locksFreed int
	sleeps     []time.Duration
	notices    []string
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

// deps builds the seams over the fake, with the pair's pause and guard seams only when pair is set.
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
		Owed:   func(err error) bool { return errors.Is(err, errOwed) },
		Notify: func(line string) error { f.notices = append(f.notices, line); return nil },
		Sleep: func(d time.Duration) {
			f.sleeps = append(f.sleeps, d)
			if f.onSleep != nil {
				f.onSleep()
			}
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

// repeat returns n copies of err.
func repeat(err error, n int) []error {
	errs := make([]error, n)
	for i := range errs {
		errs[i] = err
	}
	return errs
}

// TestRun covers the pusher's pass: its lock, its stops, its pushes and its owed backoff.
func TestRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	backoff := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, time.Minute, time.Minute}
	tests := []struct {
		name           string
		pair           bool
		setup          func(f *runFake, cancel context.CancelFunc)
		wantPushes     int
		wantErr        error
		wantGuardCalls int
		wantLocks      int
		// wantSleeps is checked only when non-nil.
		wantSleeps  []time.Duration
		wantNotices int
	}{
		{name: "LevelAfterOnePush", setup: func(f *runFake, _ context.CancelFunc) { f.ahead = []bool{true} }, wantPushes: 1, wantLocks: 1},
		{name: "TipMovedDuringThePush", setup: func(f *runFake, _ context.CancelFunc) { f.ahead = []bool{true, true} }, wantPushes: 2, wantLocks: 1},
		{
			name: "ACommitWhileExitingWithTheLockFreeAgain", wantPushes: 2, wantLocks: 2,
			setup: func(f *runFake, _ context.CancelFunc) { f.ahead = []bool{true, false, true, true} },
		},
		{
			name: "ACommitWhileExitingWithTheLockTakenByAnother", wantPushes: 1, wantLocks: 1,
			setup: func(f *runFake, _ context.CancelFunc) {
				f.ahead = []bool{true, false, true}
				f.lockResults = []bool{true, false}
			},
		},
		{name: "LockHeldAtStart", setup: func(f *runFake, _ context.CancelFunc) { f.lockResults = []bool{false} }},
		{name: "WorktreeGone", setup: func(f *runFake, _ context.CancelFunc) { f.gone = []bool{true} }, wantLocks: 1},
		{name: "ConditionsFail", setup: func(f *runFake, _ context.CancelFunc) { f.conditions = []bool{false} }, wantLocks: 1},
		{name: "LockTakeFails", setup: func(f *runFake, _ context.CancelFunc) { f.lockErr = errReadFailed }, wantErr: errReadFailed},
		{name: "AheadReadFails", setup: func(f *runFake, _ context.CancelFunc) { f.aheadErr = errReadFailed }, wantErr: errReadFailed, wantLocks: 1},
		{name: "ConditionsReadFails", setup: func(f *runFake, _ context.CancelFunc) { f.condErr = errReadFailed }, wantErr: errReadFailed, wantLocks: 1},
		{
			name: "APauseCoversTheRunBeforeTheGuard", pair: true, wantLocks: 1,
			setup: func(f *runFake, _ context.CancelFunc) { f.ahead = []bool{true}; f.paused = []bool{true} },
		},
		{
			name: "APauseDuringAnOwedBackoffStopsTheNextAttempt", pair: true, wantPushes: 1, wantGuardCalls: 1, wantLocks: 1,
			setup: func(f *runFake, _ context.CancelFunc) {
				f.ahead = []bool{true}
				f.paused = []bool{false, true}
				f.pushErrs = []error{errOwed}
			},
		},
		{
			name: "APauseAfterALandedPushWhileStillAhead", pair: true, wantPushes: 1, wantGuardCalls: 1, wantLocks: 1,
			setup: func(f *runFake, _ context.CancelFunc) { f.ahead = []bool{true, true}; f.paused = []bool{false, true} },
		},
		{
			name: "AGuardStopPushesNothing", pair: true, wantErr: errGuardStop, wantGuardCalls: 1, wantLocks: 1,
			setup: func(f *runFake, _ context.CancelFunc) { f.ahead = []bool{true}; f.guardErr = errGuardStop },
		},
		{
			name: "OwedFailuresThenALanding", wantPushes: 9, wantLocks: 1, wantNotices: 1, wantSleeps: backoff,
			setup: func(f *runFake, _ context.CancelFunc) { f.ahead = []bool{true}; f.pushErrs = repeat(errOwed, 8) },
		},
		{
			name: "ASecondOwedEpisodeAfterALandingNotifiesAgain", wantPushes: 16, wantLocks: 1, wantNotices: 2,
			setup: func(f *runFake, _ context.CancelFunc) {
				f.ahead = []bool{true, true}
				f.pushErrs = append(append(repeat(errOwed, 7), nil), repeat(errOwed, 7)...)
			},
		},
		{
			name: "ARefusedPushReturns", wantPushes: 1, wantErr: errRefused, wantLocks: 1,
			setup: func(f *runFake, _ context.CancelFunc) { f.ahead = []bool{true}; f.pushErrs = []error{errRefused} },
		},
		{
			name: "AHandedOverPushReturns", wantPushes: 1, wantErr: errHandedOver, wantLocks: 1,
			setup: func(f *runFake, _ context.CancelFunc) { f.ahead = []bool{true}; f.pushErrs = []error{errHandedOver} },
		},
		{
			name: "ACancelledContextDuringTheBackoff", wantPushes: 1, wantErr: context.Canceled, wantLocks: 1,
			setup: func(f *runFake, cancel context.CancelFunc) {
				f.ahead = []bool{true}
				f.pushErrs = repeat(errOwed, 100)
				f.onSleep = cancel
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := &runFake{}
			tt.setup(f, cancel)
			target := runpusher.Target{Location: &lymcwd.Location{HubPath: "hub", WorktreeName: "wt"}, Pair: tt.pair}

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
			if len(f.notices) != tt.wantNotices {
				t.Errorf("notices = %q; want %d", f.notices, tt.wantNotices)
			}
			if tt.wantSleeps != nil && !reflect.DeepEqual(f.sleeps, tt.wantSleeps) {
				t.Errorf("sleeps = %v; want %v", f.sleeps, tt.wantSleeps)
			}
		})
	}
}

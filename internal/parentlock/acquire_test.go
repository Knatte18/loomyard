// acquire_test.go covers the queue wait and the wait record, with a fake Sleep so no test waits out a poll delay.

package parentlock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestAcquire_WaitsForHolderWithDoublingPollThenIsGranted(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	store, processes := newTestStore(t)
	holder := liveOwner(processes, "holder", 1)
	waiter := liveOwner(processes, "waiter", 2)
	mustTryAcquire(t, store, "main", holder)

	var delays []time.Duration
	var observed []Record
	record, err := store.Acquire(context.Background(), "main", waiter, Waiter{
		OnWait: func(holder Record) { observed = append(observed, holder) },
		Sleep: func(_ context.Context, d time.Duration) error {
			delays = append(delays, d)
			if len(delays) == 7 {
				if released, err := store.Release("main", holder); err != nil || !released {
					t.Fatalf("Release(holder) = (%v, %v); want released", released, err)
				}
			}
			return nil
		},
	})

	if err != nil || !record.Owner.SameProcess(waiter) {
		t.Fatalf("Acquire = (%+v, %v); want the waiter's record", record, err)
	}
	wantDelays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
	if !slices.Equal(delays, wantDelays) {
		t.Fatalf("poll delays = %v; want %v", delays, wantDelays)
	}
	if len(observed) != 1 || !observed[0].Owner.SameProcess(holder) {
		t.Fatalf("OnWait calls = %+v; want exactly one naming the holder", observed)
	}
}

func TestAcquire_ReleaseEndsThePollDelayAfterTheFloor(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	store, processes := newTestStore(t)
	holder := liveOwner(processes, "holder", 1)
	waiter := liveOwner(processes, "waiter", 2)
	mustTryAcquire(t, store, "main", holder)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	wokeAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var delays []time.Duration
	var released bool
	record, err := store.Acquire(ctx, "main", waiter, Waiter{
		Now: func() time.Time { return wokeAt },
		Sleep: func(sleepCtx context.Context, d time.Duration) error {
			delays = append(delays, d)
			if released {
				return nil
			}
			released = true
			if ok, err := store.Release("main", holder); err != nil || !ok {
				t.Errorf("Release(holder) = (%v, %v); want released", ok, err)
			}
			<-sleepCtx.Done()
			return sleepCtx.Err()
		},
	})

	if err != nil || !record.Owner.SameProcess(waiter) {
		t.Fatalf("Acquire = (%+v, %v); want the waiter's record", record, err)
	}
	if want := []time.Duration{time.Second, time.Second}; !slices.Equal(delays, want) {
		t.Errorf("sleeps = %v; want %v: the poll delay ended by the release, then the rest of the floor", delays, want)
	}
}

func TestAcquire_FrontWaiterIsGrantedAheadOfAnOlderTicket(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	store, processes := newTestStore(t)
	holder := liveOwner(processes, "holder", 1)
	older := liveOwner(processes, "older", 2)
	returning := liveOwner(processes, "returning", 3)
	mustTryAcquire(t, store, "main", holder)
	if mustTryAcquire(t, store, "main", older).Granted {
		t.Fatal("the older waiter was granted a held lock")
	}

	record, err := store.Acquire(context.Background(), "main", returning, Waiter{
		Front: true,
		Sleep: func(context.Context, time.Duration) error {
			if _, err := store.Release("main", holder); err != nil {
				t.Errorf("Release(holder) error = %v", err)
			}
			return nil
		},
	})

	if err != nil || !record.Owner.SameProcess(returning) {
		t.Fatalf("Acquire = (%+v, %v); want the front waiter's record", record, err)
	}
	tickets, err := store.liveTickets("main", false)
	if err != nil || len(tickets) != 1 || !tickets[0].Owner.SameProcess(older) {
		t.Errorf("tickets = %+v, %v; want only the older waiter's, still queued", tickets, err)
	}
}

func TestTryAcquire_FrontTicketsAreGrantedOldestFirstAheadOfOrdinaryOnes(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	store, processes := newTestStore(t)
	holder := liveOwner(processes, "holder", 1)
	ordinary := liveOwner(processes, "ordinary", 2)
	firstFront := liveOwner(processes, "first-front", 3)
	secondFront := liveOwner(processes, "second-front", 4)
	mustTryAcquire(t, store, "main", holder)
	for _, queued := range []struct {
		owner Owner
		front bool
	}{{ordinary, false}, {firstFront, true}, {secondFront, true}} {
		if attempt, err := store.tryAcquire("main", queued.owner, queued.front); err != nil || attempt.Granted {
			t.Fatalf("tryAcquire(%s) = (%+v, %v); want queued behind the holder", queued.owner.Slug, attempt, err)
		}
	}
	if _, err := store.Release("main", holder); err != nil {
		t.Fatalf("Release(holder) error = %v", err)
	}

	steps := []struct {
		owner       Owner
		front       bool
		wantGranted bool
	}{
		{secondFront, true, false},
		{ordinary, false, false},
		{firstFront, true, true},
		{ordinary, false, false},
		{secondFront, true, true},
		{ordinary, false, true},
	}
	for i, step := range steps {
		attempt, err := store.tryAcquire("main", step.owner, step.front)
		if err != nil || attempt.Granted != step.wantGranted {
			t.Fatalf("step %d: tryAcquire(%s) = (%+v, %v); want granted = %v", i+1, step.owner.Slug, attempt, err, step.wantGranted)
		}
		if attempt.Granted {
			if _, err := store.Release("main", step.owner); err != nil {
				t.Fatalf("step %d: Release(%s) error = %v", i+1, step.owner.Slug, err)
			}
		}
	}
}

func TestPause_ClosedChangesChannelIsReportedAfterTheFullDelay(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	changes := make(chan fswatch.Event)
	close(changes)
	var delays []time.Duration
	waiter := Waiter{Sleep: func(_ context.Context, d time.Duration) error {
		delays = append(delays, d)
		return nil
	}}

	closed, err := waiter.pause(context.Background(), 4*time.Second, time.Time{}, changes)

	if err != nil || !closed {
		t.Fatalf("pause = (%v, %v); want the closed channel reported", closed, err)
	}
	if want := []time.Duration{4 * time.Second}; !slices.Equal(delays, want) {
		t.Errorf("sleeps = %v; want %v: no extra sleep for the rest of the floor", delays, want)
	}
}

func TestAcquire_WatchThatClosesLeavesTheWaitOnItsPollBackoff(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	store, processes := newTestStore(t)
	holder := liveOwner(processes, "holder", 1)
	waiter := liveOwner(processes, "waiter", 2)
	mustTryAcquire(t, store, "main", holder)

	callerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var delays []time.Duration
	// A pause that watches sleeps on a context of its own, and one that does not sleeps on the caller's.
	var sleptOnCallerCtx []bool
	record, err := store.Acquire(callerCtx, "main", waiter, Waiter{
		Watch: func(dir string, names ...string) (*fswatch.Watcher, error) {
			watcher, err := fswatch.Watch(dir, names...)
			if err != nil {
				return nil, err
			}
			if err := watcher.Close(); err != nil {
				return nil, err
			}
			return watcher, nil
		},
		Sleep: func(ctx context.Context, d time.Duration) error {
			delays = append(delays, d)
			sleptOnCallerCtx = append(sleptOnCallerCtx, ctx == callerCtx)
			if len(delays) == 4 {
				if released, err := store.Release("main", holder); err != nil || !released {
					t.Errorf("Release(holder) = (%v, %v); want released", released, err)
				}
			}
			return nil
		},
	})

	if err != nil || !record.Owner.SameProcess(waiter) {
		t.Fatalf("Acquire = (%+v, %v); want the waiter's record", record, err)
	}
	if want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}; !slices.Equal(delays, want) {
		t.Errorf("poll delays = %v; want %v", delays, want)
	}
	if want := []bool{false, true, true, true}; !slices.Equal(sleptOnCallerCtx, want) {
		t.Errorf("sleeps on the caller's context = %v; want %v: the closed watch dropped after the first pause", sleptOnCallerCtx, want)
	}
}

// TestAcquire_WatchThatCannotOpenPollsAndWarnsOnce is not parallel: it captures the process-global log.
func TestAcquire_WatchThatCannotOpenPollsAndWarnsOnce(t *testing.T) {
	tiergate.Require(t, 0)
	logs := logcapture.Capture(t)

	store, processes := newTestStore(t)
	holder := liveOwner(processes, "holder", 1)
	waiter := liveOwner(processes, "waiter", 2)
	mustTryAcquire(t, store, "main", holder)

	var delays []time.Duration
	record, err := store.Acquire(context.Background(), "main", waiter, Waiter{
		Watch: func(string, ...string) (*fswatch.Watcher, error) { return nil, errors.New("no watch available") },
		Sleep: func(_ context.Context, d time.Duration) error {
			delays = append(delays, d)
			if len(delays) == 3 {
				if released, err := store.Release("main", holder); err != nil || !released {
					t.Errorf("Release(holder) = (%v, %v); want released", released, err)
				}
			}
			return nil
		},
	})

	if err != nil || !record.Owner.SameProcess(waiter) {
		t.Fatalf("Acquire = (%+v, %v); want the waiter's record granted on the poll", record, err)
	}
	if want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}; !slices.Equal(delays, want) {
		t.Errorf("poll delays = %v; want %v", delays, want)
	}
	if got := strings.Count(logs.String(), "could not watch the lock record"); got != 1 {
		t.Errorf("watch-open warnings = %d; want 1", got)
	}
}

func TestAcquire_EndsWaitWithTicketRemoved(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		waiter  Waiter
		corrupt bool
		wantErr error
	}{
		{"paused", context.Background(), Waiter{Paused: func() bool { return true }}, false, ErrPaused},
		{"cancelled context", cancelled, Waiter{}, false, context.Canceled},
		{"unreadable record", context.Background(), Waiter{}, true, ErrUnreadable},
	} {
		store, processes := newTestStore(t)
		holder := liveOwner(processes, "holder", 1)
		waiter := liveOwner(processes, "waiter", 2)
		mustTryAcquire(t, store, "main", holder)
		if tc.corrupt {
			if err := os.WriteFile(store.recordPath("main"), []byte("{not json"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		tc.waiter.Sleep = func(context.Context, time.Duration) error {
			t.Errorf("%s: slept instead of ending the wait", tc.name)
			return nil
		}

		if _, err := store.Acquire(tc.ctx, "main", waiter, tc.waiter); !errors.Is(err, tc.wantErr) {
			t.Fatalf("%s: Acquire error = %v; want %v", tc.name, err, tc.wantErr)
		}
		if _, err := os.Stat(store.ticketPath("main", waiter)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: ticket left behind, stat error = %v", tc.name, err)
		}
	}
}

func TestWaitRecord_DeadWriterReadsAbsentAndIsReplaced(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	path := WaitRecordPath(filepath.Join(t.TempDir(), "run"))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, found, err := ReadWait(path, ProcessAlive); err != nil || found {
		t.Fatalf("ReadWait on an absent record = (%v, %v); want not found", found, err)
	}

	dead := Wait{Branch: "main", Holder: "other", PID: 1, StartTime: "t1", Started: time.Unix(1, 0).UTC()}
	if err := WriteWait(path, dead); err != nil {
		t.Fatal(err)
	}
	if _, found, err := ReadWait(path, func(int, string) bool { return false }); err != nil || found {
		t.Fatalf("ReadWait of a dead writer's record = (%v, %v); want not found", found, err)
	}

	own := NewWait("main", "other")
	if err := WriteWait(path, own); err != nil {
		t.Fatal(err)
	}
	got, found, err := ReadWait(path, ProcessAlive)
	if err != nil || !found || got.PID != os.Getpid() || got.Branch != "main" || got.Holder != "other" {
		t.Fatalf("ReadWait after replacement = (%+v, %v, %v); want this process's record", got, found, err)
	}

	for range 2 {
		if err := RemoveWait(path); err != nil {
			t.Fatalf("RemoveWait error = %v; want success whether or not the file exists", err)
		}
	}
}

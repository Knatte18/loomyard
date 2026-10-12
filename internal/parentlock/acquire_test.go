// acquire_test.go covers the queue wait and the wait record, with a fake Sleep so no test waits out a poll delay.

package parentlock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestAcquire_WaitsForHolderWithDoublingPollThenIsGranted(t *testing.T) {
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

func TestAcquire_EndsWaitWithTicketRemoved(t *testing.T) {
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

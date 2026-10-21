// pool_test.go covers the slot pool, its holder records, the inherited slot check, the child environment and the wait records over file locks in a temp directory.
// File locks are not spawns, so the tests stay untagged and run with a shortened poll interval.

package gateslot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

const testPoll = 5 * time.Millisecond

func newPool(t *testing.T, limits func() (Limits, error)) *Pool {
	t.Helper()
	return &Pool{Dir: filepath.Join(t.TempDir(), "gate"), Limits: limits, Poll: testPoll}
}

func fixedLimits(slots int) func() (Limits, error) {
	return func() (Limits, error) { return Limits{Slots: slots, GoParallel: 3}, nil }
}

func shortContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestAcquire_TakesFreeSlotsUpToLimitThenRefusesAnother(t *testing.T) {
	t.Parallel()

	pool := newPool(t, fixedLimits(2))
	var leases []*Lease
	for range 2 {
		lease, err := pool.Acquire(shortContext(t), Holder{Worktree: "wt", Site: "site"})
		if err != nil {
			t.Fatalf("Acquire() error = %v; want a free slot", err)
		}
		leases = append(leases, lease)
	}

	short, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if lease, err := pool.Acquire(short, Holder{Site: "third"}); !errors.Is(err, context.DeadlineExceeded) || lease != nil {
		t.Fatalf("third Acquire() = (%v, %v); want a deadline error and no lease with both slots held", lease, err)
	}

	for _, lease := range leases {
		if err := lease.Release(); err != nil {
			t.Fatal(err)
		}
	}
}

// The logger's output is process-global, so this test does not run in parallel.
func TestAcquire_WaitsForReleaseAndLogsWaitStartAndEnd(t *testing.T) {
	logs := logcapture.CaptureVerbose(t)
	pool := newPool(t, fixedLimits(1))
	first, err := pool.Acquire(shortContext(t), Holder{Site: "first"})
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		lease *Lease
		err   error
	}
	second := make(chan result, 1)
	go func() {
		lease, err := pool.Acquire(shortContext(t), Holder{Site: "second"})
		second <- result{lease, err}
	}()

	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(logs.String(), "gate slot wait start") {
		if time.Now().After(deadline) {
			t.Fatal("no wait start line logged while the only slot was held")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case got := <-second:
		t.Fatalf("second Acquire returned (%v, %v) while the first still held the slot", got.lease, got.err)
	default:
	}

	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	got := <-second
	if got.err != nil {
		t.Fatalf("second Acquire() error = %v; want the released slot", got.err)
	}
	if !strings.Contains(logs.String(), "gate slot wait end") {
		t.Errorf("log = %q; want a wait end line once the slot was acquired", logs.String())
	}
	if err := got.lease.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestAcquire_CancelledWaitHoldsNoSlot(t *testing.T) {
	t.Parallel()

	pool := newPool(t, fixedLimits(1))
	first, err := pool.Acquire(shortContext(t), Holder{Site: "first"})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if lease, err := pool.Acquire(cancelled, Holder{Site: "cancelled"}); !errors.Is(err, context.Canceled) || lease != nil {
		t.Fatalf("Acquire(cancelled) = (%v, %v); want context.Canceled and no lease", lease, err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}

	again, err := pool.Acquire(shortContext(t), Holder{Site: "again"})
	if err != nil {
		t.Fatalf("Acquire() after the cancelled wait = %v; want the slot free", err)
	}
	if err := again.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestAcquire_ReadsLimitsAtEveryAcquire(t *testing.T) {
	t.Parallel()

	slots := 2
	pool := newPool(t, func() (Limits, error) { return Limits{Slots: slots, GoParallel: 1}, nil })
	first, err := pool.Acquire(shortContext(t), Holder{Site: "first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := pool.Acquire(shortContext(t), Holder{Site: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}

	// Lowering the count to one lets a new acquire take the free slot 1 while slot 2's holder, above the count, finishes.
	slots = 1
	third, err := pool.Acquire(shortContext(t), Holder{Site: "third"})
	if err != nil {
		t.Fatalf("Acquire() after lowering the count = %v; want slot 1", err)
	}
	short, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := pool.Acquire(short, Holder{Site: "fourth"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire() with one slot held under a count of one = %v; want a deadline error", err)
	}

	holders, err := pool.Holders()
	if err != nil {
		t.Fatal(err)
	}
	if len(holders) != 2 {
		t.Errorf("Holders() = %v; want the holder of slot 2 above the lowered count to show beside slot 1's", holders)
	}
	for _, lease := range []*Lease{second, third} {
		if err := lease.Release(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHolders_NamesLiveHoldersAndDropsRecordsOfFreeLocks(t *testing.T) {
	t.Parallel()

	pool := newPool(t, fixedLimits(2))
	lease, err := pool.Acquire(shortContext(t), Holder{Worktree: "wt", Site: "webster gate"})
	if err != nil {
		t.Fatal(err)
	}
	staleRecord := filepath.Join(pool.Dir, "slot-2.yaml")
	if err := os.WriteFile(staleRecord, []byte("site: crashed\npid: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	holders, err := pool.Holders()
	if err != nil {
		t.Fatal(err)
	}
	if len(holders) != 1 || holders[0].Worktree != "wt" || holders[0].Site != "webster gate" || holders[0].PID != os.Getpid() || holders[0].Started.IsZero() {
		t.Fatalf("Holders() = %+v; want only the live holder with worktree, site, pid and start time", holders)
	}

	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
	holders, err = pool.Holders()
	if err != nil {
		t.Fatal(err)
	}
	if len(holders) != 0 {
		t.Errorf("Holders() after release = %+v; want none", holders)
	}
}

func TestInherited(t *testing.T) {
	t.Parallel()

	pool := newPool(t, fixedLimits(2))
	held, err := pool.Acquire(shortContext(t), Holder{Site: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = held.Release() })
	free := filepath.Join(pool.Dir, "slot-2.lock")
	if err := os.WriteFile(free, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		slot string
		want bool
	}{
		{"held slot in the pool", filepath.Join(pool.Dir, "slot-1.lock"), true},
		{"free slot in the pool", free, false},
		{"lock held outside the pool", filepath.Join(t.TempDir(), "slot-1.lock"), false},
		{"file in the pool that is no slot lock", filepath.Join(pool.Dir, "slot-1.yaml"), false},
		{"empty value", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := pool.Inherited(tc.slot); got != tc.want {
				t.Errorf("Inherited(%q) = %v; want %v", tc.slot, got, tc.want)
			}
		})
	}
}

// acquireResult is what a goroutine running an acquire reports.
type acquireResult struct {
	lease *Lease
	err   error
}

// acquireSlotsAsync runs AcquireSlots in a goroutine and returns the channel its result arrives on.
func acquireSlotsAsync(ctx context.Context, pool *Pool, h Holder, count int) <-chan acquireResult {
	done := make(chan acquireResult, 1)
	go func() {
		lease, err := pool.AcquireSlots(ctx, h, count)
		done <- acquireResult{lease, err}
	}()
	return done
}

// awaitResult returns the result on done, failing the test when none arrives within the test deadline.
func awaitResult(t *testing.T, done <-chan acquireResult) acquireResult {
	t.Helper()
	select {
	case got := <-done:
		return got
	case <-time.After(10 * time.Second):
		t.Fatal("no acquire result within 10s")
		return acquireResult{}
	}
}

// eventually polls cond until it holds, failing the test at the deadline; the code under test has no signal to wait on.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("never saw %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// holdSlot takes slot number directly, as another holder would, and returns the lease that releases it.
func holdSlot(t *testing.T, pool *Pool, number int) *Lease {
	t.Helper()
	if err := os.MkdirAll(pool.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	slot, acquired, err := pool.tryAcquireSlot(number, Holder{Site: "blocker"})
	if err != nil || !acquired {
		t.Fatalf("tryAcquireSlot(%d) = (%v, %v); want the slot", number, acquired, err)
	}
	return &Lease{slots: []heldSlot{slot}}
}

// holdsSlot reports whether the pool's holder records show slot number held.
func holdsSlot(pool *Pool, number int) bool {
	held, err := lockIsHeld(pool.slotLockPath(number))
	return err == nil && held
}

func TestAcquireSlots(t *testing.T) {
	t.Parallel()

	t.Run("a count of 1 takes one slot as Acquire does", func(t *testing.T) {
		t.Parallel()
		pool := newPool(t, fixedLimits(3))
		lease, err := pool.AcquireSlots(shortContext(t), Holder{Site: "site", Tier: "tier0"}, 1)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = lease.Release() })
		if got := len(lease.slots); got != 1 {
			t.Errorf("AcquireSlots(1) holds %d slots; want 1", got)
		}
		if holders, err := pool.Holders(); err != nil || len(holders) != 1 {
			t.Errorf("Holders() = (%v, %v); want the one holder", holders, err)
		}
	})

	t.Run("two slots on a free pool are slots 1 and 2 and their records name the tier", func(t *testing.T) {
		t.Parallel()
		pool := newPool(t, fixedLimits(3))
		lease, err := pool.AcquireSlots(shortContext(t), Holder{Site: "site", Tier: "tier1"}, 2)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = lease.Release() })
		holders, err := pool.Holders()
		if err != nil || len(holders) != 2 {
			t.Fatalf("Holders() = (%v, %v); want two holders", holders, err)
		}
		for _, h := range holders {
			if h.Tier != "tier1" {
				t.Errorf("holder record Tier = %q; want tier1", h.Tier)
			}
		}
		for number, want := range map[int]bool{1: true, 2: true, 3: false} {
			if got := holdsSlot(pool, number); got != want {
				t.Errorf("slot %d held = %v; want %v", number, got, want)
			}
		}
	})

	t.Run("a count of 0 and a count above the pool each take every slot", func(t *testing.T) {
		t.Parallel()
		for _, count := range []int{0, 5} {
			pool := newPool(t, fixedLimits(2))
			lease, err := pool.AcquireSlots(shortContext(t), Holder{Site: "site"}, count)
			if err != nil {
				t.Fatalf("AcquireSlots(%d) error = %v", count, err)
			}
			if got := len(lease.slots); got != 2 {
				t.Errorf("AcquireSlots(%d) holds %d slots; want every slot, 2", count, got)
			}
			if err := lease.Release(); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("a multi-slot waiter holds slot 1 while slot 2 stays held and completes once it frees", func(t *testing.T) {
		t.Parallel()
		pool := newPool(t, fixedLimits(2))
		blocker := holdSlot(t, pool, 2)
		done := acquireSlotsAsync(shortContext(t), pool, Holder{Site: "waiter", Tier: "tier1"}, 2)

		eventually(t, "the waiter holding slot 1", func() bool { return holdsSlot(pool, 1) })
		select {
		case got := <-done:
			t.Fatalf("AcquireSlots returned (%v, %v) while slot 2 was held", got.lease, got.err)
		default:
		}

		if err := blocker.Release(); err != nil {
			t.Fatal(err)
		}
		got := awaitResult(t, done)
		if got.err != nil {
			t.Fatalf("AcquireSlots error = %v; want both slots once slot 2 freed", got.err)
		}
		t.Cleanup(func() { _ = got.lease.Release() })
		if n := len(got.lease.slots); n != 2 {
			t.Errorf("AcquireSlots holds %d slots; want 2", n)
		}
	})

	t.Run("a single-slot Acquire queues behind a multi-slot waiter holding the drain lock", func(t *testing.T) {
		t.Parallel()
		pool := newPool(t, fixedLimits(3))
		blocker := holdSlot(t, pool, 2)
		done := acquireSlotsAsync(shortContext(t), pool, Holder{Site: "waiter"}, 2)
		eventually(t, "the waiter holding slot 1 and the drain lock", func() bool {
			draining, err := lockIsHeld(pool.drainLockPath())
			return err == nil && draining && holdsSlot(pool, 1)
		})

		// Slot 3 is free, so only the drain lock keeps this acquire from taking it.
		short, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		if lease, err := pool.Acquire(short, Holder{Site: "single"}); !errors.Is(err, context.DeadlineExceeded) || lease != nil {
			t.Fatalf("Acquire() during the drain = (%v, %v); want a deadline error and no lease", lease, err)
		}

		if err := blocker.Release(); err != nil {
			t.Fatal(err)
		}
		waiter := awaitResult(t, done)
		if waiter.err != nil {
			t.Fatalf("AcquireSlots error = %v", waiter.err)
		}
		if err := waiter.lease.Release(); err != nil {
			t.Fatal(err)
		}
		single, err := pool.Acquire(shortContext(t), Holder{Site: "single"})
		if err != nil {
			t.Fatalf("Acquire() after the drain = %v; want a slot", err)
		}
		_ = single.Release()
	})

	t.Run("two concurrent multi-slot acquires both complete", func(t *testing.T) {
		t.Parallel()
		pool := newPool(t, fixedLimits(2))
		// Each waiter releases its lease on completing, so the other can take the slots.
		errs := make(chan error, 2)
		for _, site := range []string{"first", "second"} {
			go func() {
				lease, err := pool.AcquireSlots(shortContext(t), Holder{Site: site}, 2)
				if err == nil {
					err = lease.Release()
				}
				errs <- err
			}()
		}
		for range 2 {
			select {
			case err := <-errs:
				if err != nil {
					t.Fatalf("AcquireSlots error = %v; want both waiters to complete", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("a multi-slot waiter never completed")
			}
		}
	})

	t.Run("a cancelled multi-slot wait releases the slots it held and the drain lock", func(t *testing.T) {
		t.Parallel()
		pool := newPool(t, fixedLimits(2))
		blocker := holdSlot(t, pool, 2)
		t.Cleanup(func() { _ = blocker.Release() })
		ctx, cancel := context.WithCancel(shortContext(t))
		done := acquireSlotsAsync(ctx, pool, Holder{Site: "waiter"}, 2)
		eventually(t, "the waiter holding slot 1", func() bool { return holdsSlot(pool, 1) })

		cancel()
		got := awaitResult(t, done)
		if !errors.Is(got.err, context.Canceled) || got.lease != nil {
			t.Fatalf("AcquireSlots after cancel = (%v, %v); want context.Canceled and no lease", got.lease, got.err)
		}
		if holdsSlot(pool, 1) {
			t.Error("slot 1 is still held after the cancelled wait")
		}
		if draining, err := lockIsHeld(pool.drainLockPath()); err != nil || draining {
			t.Errorf("drain lock held = (%v, %v) after the cancelled wait; want it released", draining, err)
		}
		if !holdsSlot(pool, 2) {
			t.Error("slot 2, held by another holder, was released by the cancelled wait")
		}
	})
}

func TestLeaseEnv(t *testing.T) {
	t.Parallel()

	pool := newPool(t, fixedLimits(1))
	lease, err := pool.Acquire(shortContext(t), Holder{Site: "site"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lease.Release() })
	slotPath := filepath.Join(pool.Dir, "slot-1.lock")

	tests := []struct {
		name string
		base []string
		want []string
	}{
		{"no GOFLAGS", []string{"A=1"}, []string{"A=1", "GOFLAGS=-p=3 -trimpath", "LYX_GATE_SLOT=" + slotPath}},
		{"existing GOFLAGS gets the cap and trimpath appended", []string{"GOFLAGS=-count=1", "A=1"}, []string{"A=1", "GOFLAGS=-count=1 -p=3 -trimpath", "LYX_GATE_SLOT=" + slotPath}},
		{"an older slot variable is replaced", []string{"LYX_GATE_SLOT=old", "A=1"}, []string{"A=1", "GOFLAGS=-p=3 -trimpath", "LYX_GATE_SLOT=" + slotPath}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := slices.Clone(tc.base)
			if got := lease.Env(base); !slices.Equal(got, tc.want) {
				t.Errorf("Env(%v) = %v; want %v", tc.base, got, tc.want)
			}
			if !slices.Equal(base, tc.base) {
				t.Errorf("Env changed its base to %v; want it left as %v", base, tc.base)
			}
		})
	}

	holderTests := []struct {
		name          string
		base          []string
		lease         *Lease
		tier0Parallel int
		want          []string
	}{
		{"tier0 parallel above 0 adds -p and -trimpath", []string{"A=1"}, nil, 3, []string{"A=1", "GOFLAGS=-p=3 -trimpath"}},
		{"tier0 parallel 0 adds -trimpath alone", []string{"A=1"}, nil, 0, []string{"A=1", "GOFLAGS=-trimpath"}},
		{"existing GOFLAGS is kept ahead of the added flags", []string{"GOFLAGS=-count=1", "A=1"}, nil, 2, []string{"A=1", "GOFLAGS=-count=1 -p=2 -trimpath"}},
		{"a lease sets the inheritance variable over an older one", []string{"LYX_GATE_SLOT=old", "A=1"}, lease, 0, []string{"A=1", "GOFLAGS=-trimpath", "LYX_GATE_SLOT=" + slotPath}},
		{"no lease leaves an inherited slot variable alone", []string{"LYX_GATE_SLOT=old", "A=1"}, nil, 0, []string{"LYX_GATE_SLOT=old", "A=1", "GOFLAGS=-trimpath"}},
	}
	for _, tc := range holderTests {
		t.Run("HolderEnv "+tc.name, func(t *testing.T) {
			base := slices.Clone(tc.base)
			if got := HolderEnv(base, tc.lease, tc.tier0Parallel); !slices.Equal(got, tc.want) {
				t.Errorf("HolderEnv(%v, %d) = %v; want %v", tc.base, tc.tier0Parallel, got, tc.want)
			}
			if !slices.Equal(base, tc.base) {
				t.Errorf("HolderEnv changed its base to %v; want it left as %v", base, tc.base)
			}
		})
	}

	t.Run("InheritEnv sets only the inheritance variable", func(t *testing.T) {
		base := []string{"GOFLAGS=-count=1", "LYX_GATE_SLOT=old", "A=1"}
		want := []string{"GOFLAGS=-count=1", "A=1", "LYX_GATE_SLOT=" + slotPath}
		if got := lease.InheritEnv(slices.Clone(base)); !slices.Equal(got, want) {
			t.Errorf("InheritEnv(%v) = %v; want %v", base, got, want)
		}
	})
}

func TestReadWaits_ReturnsLiveWaitersOnly(t *testing.T) {
	t.Parallel()

	dir := WaitDir(t.TempDir())
	if waits, err := ReadWaits(dir); err != nil || len(waits) != 0 {
		t.Fatalf("ReadWaits(missing dir) = (%v, %v); want none", waits, err)
	}

	live := Wait{Site: "lyx gate test", PID: os.Getpid(), Started: time.Now().UTC().Truncate(time.Second)}
	path, err := WriteWait(dir, live)
	if err != nil {
		t.Fatal(err)
	}
	// A pid beyond any pid_max is never alive.
	if err := os.WriteFile(filepath.Join(dir, "wait-dead.yaml"), []byte("site: crashed\npid: 2147483000\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	waits, err := ReadWaits(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(waits) != 1 || waits[0] != live {
		t.Fatalf("ReadWaits() = %+v; want only %+v", waits, live)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if waits, err := ReadWaits(dir); err != nil || len(waits) != 0 {
		t.Errorf("ReadWaits() after the writer removed its record = (%v, %v); want none", waits, err)
	}
}

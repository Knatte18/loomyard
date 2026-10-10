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
		{"no GOFLAGS", []string{"A=1"}, []string{"A=1", "GOFLAGS=-p=3", "LYX_GATE_SLOT=" + slotPath}},
		{"existing GOFLAGS gets the cap appended", []string{"GOFLAGS=-count=1", "A=1"}, []string{"A=1", "GOFLAGS=-count=1 -p=3", "LYX_GATE_SLOT=" + slotPath}},
		{"an older slot variable is replaced", []string{"LYX_GATE_SLOT=old", "A=1"}, []string{"A=1", "GOFLAGS=-p=3", "LYX_GATE_SLOT=" + slotPath}},
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

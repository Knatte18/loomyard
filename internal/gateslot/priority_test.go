// priority_test.go covers the priority order among gate slot waiters:
// the priority a gate call takes, the queue tickets of waiting holders and a lower multi-slot acquire's yield.

package gateslot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestSlotPriority(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name            string
		key             string
		holdsParentLock bool
		want            Priority
	}{
		{"LockHolder_FinalizeKey", "finalize", true, PriorityLockHolder},
		{"LockHolder_CardKey", "card", true, PriorityLockHolder},
		{"LockHolder_NoKey", "", true, PriorityLockHolder},
		{"Implementer_CardKey", "card", false, PriorityImplementer},
		{"Implementer_BatchKey", "batch", false, PriorityImplementer},
		{"Other_FinalizeKey", "finalize", false, PriorityOther},
		{"Other_NoKey", "", false, PriorityOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SlotPriority(tt.key, tt.holdsParentLock); got != tt.want {
				t.Errorf("SlotPriority(%q, %v) = %d; want %d", tt.key, tt.holdsParentLock, got, tt.want)
			}
		})
	}
}

// heldTickets counts the queue tickets whose lock a waiter holds, per priority.
func heldTickets(t *testing.T, pool *Pool) map[Priority]int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(pool.Dir, "queue"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	counts := map[Priority]int{}
	for _, entry := range entries {
		held, err := lockIsHeld(filepath.Join(pool.Dir, "queue", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		rank, _, _ := strings.Cut(entry.Name(), "-")
		number, err := strconv.Atoi(rank)
		if err != nil {
			t.Fatalf("queue file %s names no priority", entry.Name())
		}
		if held {
			counts[Priority(number)]++
		}
	}
	return counts
}

func TestAcquire_HigherPriorityWaiterTakesTheFreedSlotFirst(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	pool := newPool(t, fixedLimits(1))
	blocker := holdSlot(t, pool, 1)
	lower := acquireSlotsAsync(shortContext(t), pool, Holder{Site: "implementer", Priority: PriorityImplementer}, 1)
	higher := acquireSlotsAsync(shortContext(t), pool, Holder{Site: "lock holder", Priority: PriorityLockHolder}, 1)
	eventually(t, "both waiters holding their queue tickets", func() bool {
		counts := heldTickets(t, pool)
		return counts[PriorityImplementer] == 1 && counts[PriorityLockHolder] == 1
	})

	if err := blocker.Release(); err != nil {
		t.Fatal(err)
	}
	won := awaitResult(t, higher)
	if won.err != nil {
		t.Fatalf("lock holder's Acquire error = %v; want the freed slot", won.err)
	}
	select {
	case got := <-lower:
		t.Fatalf("implementer's Acquire returned (%v, %v) while the lock holder held the only slot", got.lease, got.err)
	default:
	}
	if counts := heldTickets(t, pool); counts[PriorityLockHolder] != 0 || counts[PriorityImplementer] != 1 {
		t.Errorf("held tickets after the lock holder's acquire = %v; want only the implementer's", counts)
	}

	if err := won.lease.Release(); err != nil {
		t.Fatal(err)
	}
	next := awaitResult(t, lower)
	if next.err != nil {
		t.Fatalf("implementer's Acquire error = %v; want the slot once the lock holder released it", next.err)
	}
	if err := next.lease.Release(); err != nil {
		t.Fatal(err)
	}
	if counts := heldTickets(t, pool); len(counts) != 0 {
		t.Errorf("held tickets after both acquires = %v; want none", counts)
	}
}

func TestAcquireSlots_LowerDrainHolderYieldsItsSlotsToAHigherWaiter(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	pool := newPool(t, fixedLimits(2))
	blocker := holdSlot(t, pool, 2)
	lower := acquireSlotsAsync(shortContext(t), pool, Holder{Site: "other", Tier: "tier1"}, 2)
	eventually(t, "the lower waiter holding slot 1 and the drain lock", func() bool {
		draining, err := lockIsHeld(pool.drainLockPath())
		return err == nil && draining && holdsSlot(pool, 1)
	})

	higher := acquireSlotsAsync(shortContext(t), pool, Holder{Site: "implementer", Priority: PriorityImplementer}, 1)
	won := awaitResult(t, higher)
	if won.err != nil {
		t.Fatalf("implementer's Acquire error = %v; want slot 1, which the lower waiter yields", won.err)
	}
	if got, want := won.lease.slots[0].lockPath, pool.slotLockPath(1); got != want {
		t.Errorf("implementer's lease holds %s; want %s", got, want)
	}
	select {
	case got := <-lower:
		t.Fatalf("lower AcquireSlots returned (%v, %v) while slots 1 and 2 were held by others", got.lease, got.err)
	default:
	}

	for _, lease := range []*Lease{blocker, won.lease} {
		if err := lease.Release(); err != nil {
			t.Fatal(err)
		}
	}
	got := awaitResult(t, lower)
	if got.err != nil {
		t.Fatalf("lower AcquireSlots error = %v; want both slots once they freed", got.err)
	}
	if n := len(got.lease.slots); n != 2 {
		t.Errorf("lower AcquireSlots holds %d slots; want 2", n)
	}
	if err := got.lease.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestAcquire_UnheldTicketOutranksNobodyAndAStaleOneIsRemoved(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	pool := newPool(t, fixedLimits(1))
	queue := filepath.Join(pool.Dir, "queue")
	if err := os.MkdirAll(queue, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(queue, "2-1-1.lock")
	fresh := filepath.Join(queue, "2-1-2.lock")
	for _, path := range []string{stale, fresh} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * staleTicketAge)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	short, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lease, err := pool.Acquire(short, Holder{Site: "other"})
	if err != nil {
		t.Fatalf("Acquire() error = %v; want the free slot, since no waiter holds either ticket", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stale ticket stat error = %v; want it removed", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh ticket stat error = %v; want it kept, since its waiter may not have locked it yet", err)
	}
}

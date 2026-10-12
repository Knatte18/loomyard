// store_test.go covers the record store, its ticket queue and the compare rules at the package surface, with a fake liveness check and clock.

package parentlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the package's tests under tmuxkit.Main, so their temp files live in the process run directory and go with it.
func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}

// fakeProcesses is a liveness table keyed by pid and start time.
type fakeProcesses struct {
	mu    sync.Mutex
	alive map[string]bool
}

func processKey(pid int, startTime string) string { return fmt.Sprintf("%d/%s", pid, startTime) }

func (f *fakeProcesses) isAlive(pid int, startTime string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alive[processKey(pid, startTime)]
}

func (f *fakeProcesses) set(owner Owner, alive bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alive[processKey(owner.PID, owner.StartTime)] = alive
}

// newTestStore returns a store whose clock advances one second per reading and whose processes are alive only once registered through the returned table.
func newTestStore(t *testing.T) (*Store, *fakeProcesses) {
	t.Helper()
	processes := &fakeProcesses{alive: map[string]bool{}}
	var ticks atomic.Int64
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &Store{
		Dir:   Dir(t.TempDir()),
		Alive: processes.isAlive,
		Now:   func() time.Time { return base.Add(time.Duration(ticks.Add(1)) * time.Second) },
	}
	return store, processes
}

func liveOwner(processes *fakeProcesses, slug string, pid int) Owner {
	owner := Owner{Slug: slug, Pair: "/pairs/" + slug, RunID: "run-" + slug, PID: pid, StartTime: "t1"}
	processes.set(owner, true)
	return owner
}

func mustTryAcquire(t *testing.T, store *Store, branch string, owner Owner) Attempt {
	t.Helper()
	attempt, err := store.TryAcquire(branch, owner)
	if err != nil {
		t.Fatalf("TryAcquire(%q, %s) error = %v", branch, owner.Slug, err)
	}
	return attempt
}

func TestTryAcquire_RacingAcquirersProduceOneGrant(t *testing.T) {
	t.Parallel()

	store, processes := newTestStore(t)
	owners := []Owner{liveOwner(processes, "a", 1), liveOwner(processes, "b", 2)}
	var granted atomic.Int32
	var wg sync.WaitGroup
	for _, owner := range owners {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if mustTryAcquire(t, store, "main", owner).Granted {
				granted.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := granted.Load(); got != 1 {
		t.Fatalf("grants = %d; want exactly 1", got)
	}
}

func TestTryAcquire_GrantsWaitersInTicketOrderSkippingDeadOnes(t *testing.T) {
	t.Parallel()

	store, processes := newTestStore(t)
	holder := liveOwner(processes, "holder", 1)
	first := liveOwner(processes, "first", 2)
	dead := liveOwner(processes, "dead", 3)
	last := liveOwner(processes, "last", 4)

	mustTryAcquire(t, store, "main", holder)
	for _, waiter := range []Owner{first, dead, last} {
		attempt := mustTryAcquire(t, store, "main", waiter)
		if attempt.Granted || !attempt.Holder.Owner.SameProcess(holder) {
			t.Fatalf("waiter %s attempt = %+v; want not granted with holder %s", waiter.Slug, attempt, holder.Slug)
		}
	}
	processes.set(dead, false)

	if released, err := store.Release("main", holder); err != nil || !released {
		t.Fatalf("Release(holder) = (%v, %v); want released", released, err)
	}
	if attempt := mustTryAcquire(t, store, "main", last); attempt.Granted {
		t.Fatal("last waiter granted ahead of the older live ticket")
	}
	if attempt := mustTryAcquire(t, store, "main", first); !attempt.Granted {
		t.Fatal("oldest live waiter not granted on a free record")
	}
	if released, err := store.Release("main", first); err != nil || !released {
		t.Fatalf("Release(first) = (%v, %v); want released", released, err)
	}
	if attempt := mustTryAcquire(t, store, "main", last); !attempt.Granted {
		t.Fatal("last waiter not granted once the dead waiter's ticket is skipped")
	}
	if _, err := os.Stat(store.ticketPath("main", dead)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dead waiter's ticket still on disk, stat error = %v", err)
	}
	if _, err := os.Stat(store.ticketPath("main", last)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("granted waiter's own ticket still on disk, stat error = %v", err)
	}
}

func TestTryAcquire_TakesRecordOfDeadOrRecycledOwner(t *testing.T) {
	t.Parallel()

	store, processes := newTestStore(t)
	crashed := liveOwner(processes, "crashed", 1)
	recycled := liveOwner(processes, "recycled", 2)
	successor := liveOwner(processes, "successor", 3)

	for _, tc := range []struct {
		name   string
		branch string
		owner  Owner
		kill   func()
	}{
		{"dead pid", "one", crashed, func() { processes.set(crashed, false) }},
		{"pid with another start time", "two", recycled, func() {
			processes.set(recycled, false)
			processes.set(Owner{PID: recycled.PID, StartTime: "recycled-start"}, true)
		}},
	} {
		mustTryAcquire(t, store, tc.branch, tc.owner)
		if attempt := mustTryAcquire(t, store, tc.branch, successor); attempt.Granted {
			t.Fatalf("%s: successor granted while the owner lives", tc.name)
		}
		tc.kill()
		if attempt := mustTryAcquire(t, store, tc.branch, successor); !attempt.Granted {
			t.Fatalf("%s: successor not granted after the owner died", tc.name)
		}
		record, found, err := store.Read(tc.branch)
		if err != nil || !found || !record.Owner.SameProcess(successor) {
			t.Fatalf("%s: Read = (%+v, %v, %v); want the successor's record", tc.name, record, found, err)
		}
	}
}

func TestTryAcquire_ConfinesBranchNameToDirAndCreatesIt(t *testing.T) {
	t.Parallel()

	store, processes := newTestStore(t)
	if _, err := os.Stat(store.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Dir exists before the first acquire, stat error = %v", err)
	}

	mustTryAcquire(t, store, "feature/x/../y", liveOwner(processes, "a", 1))

	entries, err := os.ReadDir(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" && filepath.Ext(entry.Name()) != ".lock" {
			t.Errorf("unexpected entry %q in Dir", entry.Name())
		}
	}
	if record, found, err := store.Read("feature/x/../y"); err != nil || !found || record.Branch != "feature/x/../y" {
		t.Fatalf("Read = (%+v, %v, %v); want the record under its own branch name", record, found, err)
	}
}

func TestTryAcquire_ClockJumpDoesNotFreeALiveOwnersRecord(t *testing.T) {
	t.Parallel()

	store, processes := newTestStore(t)
	owner := liveOwner(processes, "a", 1)
	other := liveOwner(processes, "b", 2)
	mustTryAcquire(t, store, "main", owner)

	store.Now = func() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) }
	if attempt := mustTryAcquire(t, store, "main", other); attempt.Granted {
		t.Fatal("another run was granted a record whose owner is alive after a day-long clock jump")
	}
}

func TestTryAcquire_SameRunRewritesOwnerProcess(t *testing.T) {
	t.Parallel()

	store, processes := newTestStore(t)
	batten := liveOwner(processes, "a", 1)
	loop := Owner{Slug: batten.Slug, Pair: batten.Pair, RunID: batten.RunID, PID: 9, StartTime: "t9"}
	processes.set(loop, true)
	mustTryAcquire(t, store, "main", batten)

	if attempt := mustTryAcquire(t, store, "main", loop); !attempt.Granted {
		t.Fatal("a new process of the owning run was not granted its own record")
	}
	record, _, err := store.Read("main")
	if err != nil || !record.Owner.SameProcess(loop) {
		t.Fatalf("Read = (%+v, %v); want the owner process rewritten to the loop", record, err)
	}
}

func TestReleaseAndHandOff_LeaveRecordOfAnotherProcessAlone(t *testing.T) {
	t.Parallel()

	store, processes := newTestStore(t)
	owner := liveOwner(processes, "a", 1)
	sameRunOtherProcess := Owner{Slug: owner.Slug, Pair: owner.Pair, RunID: owner.RunID, PID: 2, StartTime: "t1"}
	otherRun := liveOwner(processes, "b", 3)
	mustTryAcquire(t, store, "main", owner)

	for _, stranger := range []Owner{sameRunOtherProcess, otherRun} {
		if released, err := store.Release("main", stranger); err != nil || released {
			t.Fatalf("Release(%s/%d) = (%v, %v); want the record left alone", stranger.Slug, stranger.PID, released, err)
		}
		if handed, err := store.HandOff("main", stranger, otherRun); err != nil || handed {
			t.Fatalf("HandOff(from %s/%d) = (%v, %v); want the record left alone", stranger.Slug, stranger.PID, handed, err)
		}
	}

	if handed, err := store.HandOff("main", owner, sameRunOtherProcess); err != nil || !handed {
		t.Fatalf("HandOff(owner) = (%v, %v); want handed", handed, err)
	}
	if released, err := store.Release("main", owner); err != nil || released {
		t.Fatalf("Release(old owner) = (%v, %v); want the handed record left alone", released, err)
	}
	if released, err := store.Release("main", sameRunOtherProcess); err != nil || !released {
		t.Fatalf("Release(new owner) = (%v, %v); want released", released, err)
	}
	if _, found, _ := store.Read("main"); found {
		t.Fatal("record still present after its owner released it")
	}
}

func TestStore_UndecodableRecordReturnsErrUnreadableAndChangesNothing(t *testing.T) {
	t.Parallel()

	store, processes := newTestStore(t)
	owner := liveOwner(processes, "a", 1)
	mustTryAcquire(t, store, "main", owner)
	path := store.recordPath("main")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.Read("main"); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("Read error = %v; want ErrUnreadable", err)
	}
	if _, err := store.TryAcquire("main", owner); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("TryAcquire error = %v; want ErrUnreadable", err)
	}
	if _, err := store.Release("main", owner); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("Release error = %v; want ErrUnreadable", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{not json" {
		t.Fatalf("record changed to %q; want it untouched", data)
	}

	entries, err := store.List()
	if err != nil || len(entries) != 1 || entries[0].Record.Branch != "main" || entries[0].Err == "" {
		t.Fatalf("List = (%+v, %v); want one entry for main carrying the error text", entries, err)
	}
}

func TestList_ReportsLivenessAndWaitersOldestFirst(t *testing.T) {
	t.Parallel()

	store, processes := newTestStore(t)
	holder := liveOwner(processes, "holder", 1)
	older := liveOwner(processes, "older", 2)
	newer := liveOwner(processes, "newer", 3)
	gone := liveOwner(processes, "gone", 4)
	mustTryAcquire(t, store, "main", holder)
	for _, waiter := range []Owner{older, gone, newer} {
		mustTryAcquire(t, store, "main", waiter)
	}
	processes.set(gone, false)

	entries, err := store.List()
	if err != nil || len(entries) != 1 {
		t.Fatalf("List = (%+v, %v); want one entry", entries, err)
	}
	entry := entries[0]
	if !entry.Alive || !entry.Record.Owner.SameProcess(holder) {
		t.Fatalf("entry = %+v; want a live record owned by the holder", entry)
	}
	var waiters []string
	for _, ticket := range entry.Waiters {
		waiters = append(waiters, ticket.Owner.Slug)
	}
	if !slices.Equal(waiters, []string{"older", "newer"}) {
		t.Fatalf("waiters = %v; want the live tickets oldest first", waiters)
	}

	processes.set(holder, false)
	if entries, _ = store.List(); entries[0].Alive {
		t.Fatal("entry reports a dead owner as alive")
	}
}

func TestForceRelease_RemovesRecordWhateverItsOwner(t *testing.T) {
	t.Parallel()

	store, processes := newTestStore(t)
	mustTryAcquire(t, store, "alpha", liveOwner(processes, "a", 1))
	mustTryAcquire(t, store, "feature/beta", liveOwner(processes, "b", 2))

	if released, err := store.ForceRelease("missing"); err != nil || released {
		t.Fatalf("ForceRelease(missing) = (%v, %v); want false", released, err)
	}
	if released, err := store.ForceRelease("alpha"); err != nil || !released {
		t.Fatalf("ForceRelease(alpha) = (%v, %v); want released", released, err)
	}

	branches, err := store.ReleaseAll()
	if err != nil || !slices.Equal(branches, []string{"feature/beta"}) {
		t.Fatalf("ReleaseAll = (%v, %v); want [feature/beta]", branches, err)
	}
	if entries, _ := store.List(); len(entries) != 0 {
		t.Fatalf("List after ReleaseAll = %+v; want none", entries)
	}
}

//go:build tier0

// innerrun_respawncap_test.go covers the cap on respawning a running child whose driver keeps dying: the count across passes and the memory record a reader outside the row reads the cap from.

package battenshed

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// respawnHarness drives passes of a running child with no driver at work, one producer per pass over one scratch directory.
type respawnHarness struct {
	t          *testing.T
	clock      *fakeClock
	scratchDir string
	spawns     *int
	deps       InnerRunDeps
	// driverUp is what the fake driver liveness reads, and strand what the fake driver strand reads.
	driverUp bool
	strand   ChildDriverStrand
	notified []string
}

// newRespawnHarness builds a harness whose spawn rewrites the child's status file, as the bootstrap does.
func newRespawnHarness(t *testing.T) *respawnHarness {
	t.Helper()
	h := &respawnHarness{t: t, clock: &fakeClock{}, scratchDir: t.TempDir(), driverUp: true, strand: ChildDriverNone}
	_, spawns, deps := newInnerRunDeps(t, nil, nil, []statusResult{{status: shedengine.Status{State: shedengine.StateRunning}, found: true}}, h.clock)
	h.spawns = spawns
	deps.NoticeProbe = time.Second
	deps.Spawn = func(context.Context) error {
		*spawns++
		h.clock.bumpStatus()
		return nil
	}
	deps.DriverAlive = func(context.Context) (bool, error) { return h.driverUp, nil }
	deps.DriverStrand = func(context.Context) (ChildDriverStrand, error) { return h.strand, nil }
	deps.Notify = func(_ context.Context, line string) (bool, error) {
		h.notified = append(h.notified, line)
		return true, nil
	}
	deps.PairDir = func() (string, error) { return "/wt/task", nil }
	h.deps = deps
	return h
}

// pass runs one pass as a new process: the spawn marker names a foreign pid, so the child reads as not spawned by this process.
func (h *respawnHarness) pass() {
	h.t.Helper()
	if err := os.WriteFile(SpawnConfirmedFile(h.scratchDir, "innerrun"), []byte(strconv.Itoa(os.Getpid()+1)+"\n"), 0o644); err != nil {
		h.t.Fatal(err)
	}
	h.clock.advance(2 * time.Second)
	outcome, _, err := NewInnerRun("innerrun", "myslug", h.deps, time.Second, h.scratchDir, testGrace).Call(context.Background())
	if err != nil || outcome != shedengine.Yield {
		h.t.Fatalf("pass = %q, %v; want a Yield", outcome, err)
	}
}

// TestInnerRun_RespawnCapStopsWithoutProgress asserts a running child with no driver at work is respawned respawnCap times with no status write between.
// It asserts the pass after spawns nothing and sends no notice, RespawnCapReached reads the cap from the memory record, and a live driver strand between two respawns does not reset the count.
// It asserts a rewrite of the status file starts the count again.
func TestInnerRun_RespawnCapStopsWithoutProgress(t *testing.T) {
	t.Parallel()
	t.Run("TheCapStopsTheRespawnAndIsReadFromTheMemoryRecord", func(t *testing.T) {
		t.Parallel()
		h := newRespawnHarness(t)
		for pass := 0; pass < respawnCap; pass++ {
			h.pass()
		}
		if *h.spawns != respawnCap {
			t.Fatalf("after %d passes: spawns = %d; want %d", respawnCap, *h.spawns, respawnCap)
		}

		h.driverUp = false
		h.pass()
		h.pass()

		if *h.spawns != respawnCap {
			t.Errorf("spawns = %d; want none after the cap", *h.spawns)
		}
		if len(h.notified) != 0 {
			t.Errorf("notices = %q; want none: a running child's driver-dead notice is its dobby's", h.notified)
		}
		statusModified := statusModTime(h.clock.statusPath)
		if capped, err := RespawnCapReached(h.scratchDir, "innerrun", statusModified); err != nil || !capped {
			t.Errorf("RespawnCapReached() = %v, %v; want true", capped, err)
		}
		if capped, err := RespawnCapReached(h.scratchDir, "innerrun", statusModified.Add(time.Second)); err != nil || capped {
			t.Errorf("RespawnCapReached() after a status write = %v, %v; want false", capped, err)
		}
	})

	t.Run("ALiveDriverStrandBetweenTwoRespawnsDoesNotResetTheCount", func(t *testing.T) {
		t.Parallel()
		h := newRespawnHarness(t)
		h.pass()
		h.strand = ChildDriverLive
		h.pass()
		h.strand = ChildDriverNone
		h.pass()
		h.pass()
		h.pass()

		if *h.spawns != respawnCap {
			t.Errorf("spawns = %d; want the cap of %d: the adopted pass spawns nothing and does not reset the count", *h.spawns, respawnCap)
		}
	})

	t.Run("ARewriteOfTheStatusFileStartsTheCountAgain", func(t *testing.T) {
		t.Parallel()
		h := newRespawnHarness(t)
		for pass := 0; pass < respawnCap+1; pass++ {
			h.pass()
		}
		h.clock.bumpStatus()
		h.pass()

		if *h.spawns != respawnCap+1 {
			t.Errorf("spawns = %d; want %d: the cap, then one more after the status write", *h.spawns, respawnCap+1)
		}
	})
}

//go:build tier0

// innerrun_respawncap_test.go covers the cap on respawning a running child whose driver keeps dying: the count across passes and the driver-dead notice that names the cap.

package battenshed

import (
	"context"
	"os"
	"strconv"
	"strings"
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
// It asserts the pass after spawns nothing and sends one driver-dead notice naming the cap, and that a live driver strand between two respawns does not reset the count.
// It asserts a rewrite of the status file starts the count again.
func TestInnerRun_RespawnCapStopsWithoutProgress(t *testing.T) {
	t.Parallel()
	capNotice := `driver strand is dead; respawned ` + strconv.Itoa(respawnCap) + ` times without progress; run "lyx loom start" in the task worktree to bring a driver back`

	t.Run("TheCapStopsTheRespawnAndSendsOneNotice", func(t *testing.T) {
		t.Parallel()
		h := newRespawnHarness(t)
		for pass := 0; pass < respawnCap; pass++ {
			h.pass()
		}
		if *h.spawns != respawnCap || len(h.notified) != 0 {
			t.Fatalf("after %d passes: spawns = %d, notices = %q; want %d and none while the driver is up", respawnCap, *h.spawns, h.notified, respawnCap)
		}

		h.driverUp = false
		h.pass()
		h.pass()

		if *h.spawns != respawnCap {
			t.Errorf("spawns = %d; want none after the cap", *h.spawns)
		}
		if len(h.notified) != 1 || !strings.Contains(h.notified[0], capNotice) {
			t.Errorf("notices = %q; want exactly one reading %q", h.notified, capNotice)
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

//go:build tier0

// owned_test.go pins the handling of a run a full pass found owned by another machine: the pause, the single notice, the full pass at the lift and the owner re-read.

package dobbyengine

import (
	"errors"
	"testing"
)

func TestOwnedElsewhere(t *testing.T) {
	t.Parallel()
	owned := PassExit{Code: PassExitOwnedElsewhere}

	t.Run("an owned refusal pauses, releases the lock and notices once across two refusals", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.fake.pauseErr = errors.New("pause refused")
		h.fake.exits = []PassExit{owned, owned}
		for range 3 {
			h.tick(true)
		}
		if h.fake.pauses < 2 || h.fake.releases != h.fake.pauses || len(h.fake.notices) != 1 {
			t.Errorf("pauses, releases, notices = %d, %d, %d; want the handling run for each refusal, with one notice", h.fake.pauses, h.fake.releases, len(h.fake.notices))
		}
		if rs := h.state(); !rs.OwnedElsewhere || !rs.OwnerNoticeSent || rs.FailingPassCount != 0 {
			t.Errorf("state = %+v; want it marked owned elsewhere, noticed, and no failing pass", rs)
		}
	})

	t.Run("a failed pause gets a full pass on the next probe tick with no second notice", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.fake.pauseErr = errors.New("pause refused")
		h.fake.exits = []PassExit{owned}
		h.tick(true)
		h.tick(true)
		if rs := h.state(); !rs.OwnerPauseFailed {
			t.Errorf("OwnerPauseFailed = false after a failed pause; want true")
		}
		spawned := h.fake.spawnsOf("task")
		if len(spawned) != 2 || spawned[1].continuePass {
			t.Errorf("passes = %+v; want the next probe tick to run a full pass", spawned)
		}
		if len(h.fake.notices) != 1 {
			t.Errorf("notices = %d; want the one", len(h.fake.notices))
		}
	})

	t.Run("a covering owner pause stops every pass, a restart's included, until its lift, which runs a full pass whose exit 0 clears the mark", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.fake.owner = "m2"
		h.fake.exits = []PassExit{owned}
		h.tick(true)
		h.tick(true)
		h.d.owesAll = true
		h.tick(true)
		h.tick(true)
		if got := len(h.fake.spawnsOf("task")); got != 1 {
			t.Fatalf("passes under a covering owner pause = %d; want only the one that was refused", got)
		}

		h.fake.runs[0].Covered = false
		h.tick(true)
		spawned := h.fake.spawnsOf("task")
		if len(spawned) != 2 || spawned[1].continuePass {
			t.Fatalf("passes at the lift = %+v; want a full pass", spawned)
		}
		h.tick(false)
		if rs := h.state(); rs.OwnedElsewhere || rs.OwnerNoticeSent || rs.OwnerPauseFailed {
			t.Errorf("state after the full pass exited 0 = %+v; want the mark cleared", rs)
		}
	})

	t.Run("a local Bolt that reads this machine again lifts the owner pause and runs a full pass, and one still reading the other machine lifts nothing", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.fake.owner = "m2"
		h.fake.exits = []PassExit{owned}
		h.tick(true)
		h.tick(true)

		h.tick(true)
		if h.fake.lifts != 0 || len(h.fake.spawnsOf("task")) != 1 {
			t.Fatalf("lifts, passes with the other machine still owning = %d, %d; want 0, 1", h.fake.lifts, len(h.fake.spawnsOf("task")))
		}

		h.fake.owner = "m1"
		h.tick(true)
		spawned := h.fake.spawnsOf("task")
		if h.fake.lifts != 1 || len(spawned) != 2 || spawned[1].continuePass {
			t.Errorf("lifts, passes once this machine owns it = %d, %+v; want 1 lift and a full pass", h.fake.lifts, spawned)
		}
	})

	t.Run("a dobby whose only run is marked owned elsewhere under a covering pause is idle, and that run uncovered counts as in flight", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.fake.owner = "m2"
		h.fake.exits = []PassExit{owned}
		h.tick(true)
		h.tick(true)
		if h.d.inFlight {
			t.Errorf("inFlight = true with the only run marked and covered; want false")
		}

		h.fake.runs[0].Covered = false
		h.d.wasCovered["task"] = false
		h.tick(true)
		if !h.d.inFlight {
			t.Errorf("inFlight = false with the marked run uncovered; want true")
		}
	})
}

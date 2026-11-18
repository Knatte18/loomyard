// owned_test.go pins the handling of a run that a full pass finds owned by another machine.
// It checks the lock release, the owed catch-up, the single notice and the full pass that clears the mark.

package dobbyengine

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestOwnedElsewhere(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	owned := PassExit{Code: PassExitOwnedElsewhere}

	t.Run("an owned refusal pauses nothing, releases the lock, owes the catch-up and notices once across two refusals", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.fake.owner = "m2"
		h.fake.subject = "beam"
		h.d.deps.CatchUp = fakeCatchUpDeps(func() error { return nil })
		h.fake.exits = []PassExit{owned, owned}
		h.tick(true)
		h.tick(true)
		if !h.d.catchUpOwed || h.fake.releases != 1 {
			t.Errorf("catch-up owed, releases = %v, %d after the first refusal; want true, 1", h.d.catchUpOwed, h.fake.releases)
		}
		h.tick(true)
		wantNotice := "task: owned by m2; this machine hands it over; way forward: run lym adopt beam on this machine to take it over, or on the owner's machine to continue it there"
		if h.fake.releases != 2 || len(h.fake.notices) != 1 || h.fake.notices[0] != wantNotice {
			t.Errorf("releases, notices = %d, %q; want 2 releases and the one notice %q", h.fake.releases, h.fake.notices, wantNotice)
		}
		if rs := h.state(); !rs.OwnedElsewhere || !rs.OwnerNoticeSent || rs.FailingPassCount != 0 {
			t.Errorf("state = %+v; want it marked owned elsewhere, noticed, and no failing pass", rs)
		}
	})

	t.Run("the notice names the adopt subject, and the slug when the seam is nil, fails or answers nothing", func(t *testing.T) {
		t.Parallel()
		for _, row := range []struct {
			name       string
			subject    string
			subjectErr error
			nilSeam    bool
			wantAdopt  string
		}{
			{"a beam", "beam", nil, false, "lym adopt beam on this machine"},
			{"root", "root", nil, false, "lym adopt root on this machine"},
			{"a nil seam", "", nil, true, "lym adopt task on this machine"},
			{"a failing seam", "beam", errors.New("board unreadable"), false, "lym adopt task on this machine"},
			{"an empty answer", "", nil, false, "lym adopt task on this machine"},
		} {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				h := newPassHarness(t, runningRun("task"))
				h.fake.exits = []PassExit{owned}
				h.fake.subject, h.fake.subjectErr = row.subject, row.subjectErr
				if row.nilSeam {
					h.d.deps.AdoptSubject = nil
				}
				for range 3 {
					h.tick(true)
				}
				if len(h.fake.notices) != 1 || !strings.Contains(h.fake.notices[0], row.wantAdopt) {
					t.Errorf("notices = %q; want one naming %q", h.fake.notices, row.wantAdopt)
				}
			})
		}
	})

	t.Run("a marked run gets a full pass on the next probe tick, whose exit 0 clears the mark", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.fake.exits = []PassExit{owned}
		h.tick(true)
		h.tick(true)
		spawned := h.fake.spawnsOf("task")
		if len(spawned) != 2 || spawned[1].continuePass {
			t.Fatalf("passes = %+v; want the refused one and then a full pass", spawned)
		}
		h.tick(true)
		if rs := h.state(); rs.OwnedElsewhere || rs.OwnerNoticeSent {
			t.Errorf("state after the full pass exited 0 = %+v; want the mark cleared", rs)
		}
	})

	t.Run("a dobby whose only run is marked owned elsewhere under a covering pause is idle, and that run uncovered counts as in flight", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.fake.exits = []PassExit{owned}
		h.tick(true)
		h.tick(true)
		h.fake.runs[0].Covered = true
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

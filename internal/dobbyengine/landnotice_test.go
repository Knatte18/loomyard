//go:build tier0

// landnotice_test.go pins a beam's dobby's "can land" notice: one per episode over a fake landing reading, kept across a restart and retried after a failed post.

package dobbyengine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// landStep is one scripted answer of the fake CanLand seam.
type landStep struct {
	reading LandReading
	err     error
}

// newLandHarness returns a pass harness over the beam b whose CanLand answers script in order, then its last step again.
func newLandHarness(t *testing.T, script ...landStep) (*passHarness, *int) {
	t.Helper()
	h := newPassHarness(t)
	h.d.geom.Slug = "b"
	reads := 0
	h.d.deps.CanLand = func() (LandReading, error) {
		step := script[min(reads, len(script)-1)]
		reads++
		return step.reading, step.err
	}
	return h, &reads
}

func TestLandNotice(t *testing.T) {
	t.Parallel()
	passing := landStep{reading: LandReading{Passes: true}}
	blocked := landStep{}

	t.Run("a passing reading posts one notice naming the land command and a second tick posts none", func(t *testing.T) {
		t.Parallel()
		h, _ := newLandHarness(t, passing)
		h.tick(false)
		h.tick(false)
		if got := h.fake.notices; len(got) != 1 || !strings.Contains(got[0], "lym beam land b") {
			t.Errorf("notices = %q; want the one notice naming `lym beam land b`", got)
		}
	})

	t.Run("a parent owned elsewhere names its owner and the adopt, not the land command", func(t *testing.T) {
		t.Parallel()
		h, _ := newLandHarness(t, landStep{reading: LandReading{Passes: true, ParentOwner: "m2", ParentSubject: "outer"}})
		h.tick(false)
		got := h.fake.notices
		if len(got) != 1 || !strings.Contains(got[0], "m2") || !strings.Contains(got[0], "lym adopt outer") || strings.Contains(got[0], "lym beam land") {
			t.Errorf("notices = %q; want one naming machine m2 and `lym adopt outer` instead of the land command", got)
		}
	})

	t.Run("a reading that stops passing and passes again posts a second notice", func(t *testing.T) {
		t.Parallel()
		h, _ := newLandHarness(t, passing, blocked, passing)
		h.tick(false)
		h.tick(false)
		if record := h.stateRecord(); record.LandNotice != nil {
			t.Errorf("state LandNotice = %+v after a reading that does not pass; want the episode cleared", record.LandNotice)
		}
		h.tick(false)
		if got := len(h.fake.notices); got != 2 {
			t.Errorf("notices = %d; want a second one after the member opened and closed again", got)
		}
	})

	t.Run("a restart over a record holding the sent episode posts none", func(t *testing.T) {
		t.Parallel()
		h, _ := newLandHarness(t, passing)
		h.tick(false)
		restarted := &daemon{
			geom: h.d.geom, deps: h.d.deps, probe: h.d.probe,
			flights: map[string]flight{}, childSeen: map[string]time.Time{}, wasCovered: map[string]bool{}, liftPending: map[string]bool{},
		}
		if err := restarted.tick(context.Background(), h.clock.now()); err != nil {
			t.Fatalf("tick() = %v; want nil", err)
		}
		if got := len(h.fake.notices); got != 1 {
			t.Errorf("notices = %d; want the restarted dobby to send no second one", got)
		}
	})

	t.Run("the prime's dobby reads nothing and posts none", func(t *testing.T) {
		t.Parallel()
		h, reads := newLandHarness(t, passing)
		h.d.geom.Slug = ""
		h.tick(false)
		if len(h.fake.notices) != 0 || *reads != 0 {
			t.Errorf("notices, reads = %q, %d; want none of either", h.fake.notices, *reads)
		}
	})

	t.Run("a failed post leaves the notice unsent and the next tick retries", func(t *testing.T) {
		t.Parallel()
		h, _ := newLandHarness(t, passing)
		posts := 0
		h.d.deps.Notice = func(_ context.Context, line string) error {
			posts++
			if posts == 1 {
				return errors.New("no orch strand is recorded")
			}
			h.fake.notices = append(h.fake.notices, line)
			return nil
		}
		h.tick(false)
		if record := h.stateRecord(); record.LandNotice != nil || len(h.fake.notices) != 0 {
			t.Fatalf("after a failed post LandNotice, notices = %+v, %q; want the notice unsent", record.LandNotice, h.fake.notices)
		}
		h.tick(false)
		if got := len(h.fake.notices); got != 1 {
			t.Errorf("notices after the retry = %d; want 1", got)
		}
	})

	t.Run("a failed read changes nothing", func(t *testing.T) {
		t.Parallel()
		h, _ := newLandHarness(t, passing, landStep{err: errors.New("the board is unreadable")}, passing)
		h.tick(false)
		h.tick(false)
		if record := h.stateRecord(); record.LandNotice == nil {
			t.Errorf("state LandNotice = nil after a failed read; want the sent episode kept")
		}
		h.tick(false)
		if got := len(h.fake.notices); got != 1 {
			t.Errorf("notices = %d; want the failed read to leave the episode sent and post no second one", got)
		}
	})
}

//go:build tier0

// land_test.go pins a beam's dobby's landing step: one per episode over a fake landing reading, a land request asked for or a notice when it cannot be, kept across a restart and retried after a failed post.

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

// landHarness is a pass harness over the beam b with scripted CanLand answers and a fake Land seam.
type landHarness struct {
	*passHarness
	// reads counts the CanLand calls and lands the Land calls.
	reads, lands int
	// refusal is what the fake Land seam returns.
	refusal error
}

// newLandHarness returns a landHarness whose CanLand answers script in order, then its last step again.
func newLandHarness(t *testing.T, script ...landStep) *landHarness {
	t.Helper()
	h := &landHarness{passHarness: newPassHarness(t)}
	h.d.geom.Slug = "b"
	h.d.deps.CanLand = func() (LandReading, error) {
		step := script[min(h.reads, len(script)-1)]
		h.reads++
		return step.reading, step.err
	}
	h.d.deps.Land = func() error {
		h.lands++
		return h.refusal
	}
	return h
}

func TestLandBeam(t *testing.T) {
	t.Parallel()
	passing := landStep{reading: LandReading{Passes: true}}
	blocked := landStep{}

	t.Run("a passing reading asks for the landing once and posts nothing", func(t *testing.T) {
		t.Parallel()
		h := newLandHarness(t, passing)
		h.tick(false)
		h.tick(false)
		if h.lands != 1 || len(h.fake.notices) != 0 {
			t.Errorf("lands, notices = %d, %q; want one land and no notice", h.lands, h.fake.notices)
		}
	})

	t.Run("a reading that does not pass, as an unflagged beam's, asks for nothing and posts nothing", func(t *testing.T) {
		t.Parallel()
		h := newLandHarness(t, blocked)
		h.tick(false)
		if h.lands != 0 || len(h.fake.notices) != 0 {
			t.Errorf("lands, notices = %d, %q; want neither", h.lands, h.fake.notices)
		}
	})

	t.Run("a parent owned elsewhere posts its owner and the adopt and asks for no landing", func(t *testing.T) {
		t.Parallel()
		h := newLandHarness(t, landStep{reading: LandReading{Passes: true, ParentOwner: "m2", ParentSubject: "outer"}})
		h.tick(false)
		h.tick(false)
		got := h.fake.notices
		if h.lands != 0 || len(got) != 1 || !strings.Contains(got[0], "m2") || !strings.Contains(got[0], "lym adopt outer") {
			t.Errorf("lands, notices = %d, %q; want no land and one notice naming machine m2 and `lym adopt outer`", h.lands, got)
		}
	})

	t.Run("a refused landing posts the refusal once", func(t *testing.T) {
		t.Parallel()
		h := newLandHarness(t, passing)
		h.refusal = errors.New("lym beam land: the pawl run of b is paused at Beam-Await; way forward: run `lym batten status b`")
		h.tick(false)
		h.tick(false)
		got := h.fake.notices
		if len(got) != 1 || !strings.Contains(got[0], "lym batten status b") {
			t.Errorf("notices = %q; want one notice carrying the refusal's way forward", got)
		}
	})

	t.Run("a refusal that waits on the run posts once and the landing is asked again until it succeeds", func(t *testing.T) {
		t.Parallel()
		h := newLandHarness(t, passing)
		h.refusal = errors.New("lym beam land: the pawl run of b is running at Parent-Align")
		h.tick(false)
		h.tick(false)
		if h.lands != 2 || len(h.fake.notices) != 1 {
			t.Fatalf("lands, notices = %d, %q; want the landing asked twice and one notice", h.lands, h.fake.notices)
		}
		h.refusal = nil
		h.tick(false)
		h.tick(false)
		if h.lands != 3 || len(h.fake.notices) != 1 {
			t.Errorf("lands, notices = %d, %q; want one landing after the run waits, no further ask and no second notice", h.lands, h.fake.notices)
		}
		if record := h.stateRecord(); record.Land == nil || record.Land.Refused {
			t.Errorf("state Land = %+v; want the episode closed by the landing", record.Land)
		}
	})

	t.Run("a reading that stops passing and passes again asks for a second landing", func(t *testing.T) {
		t.Parallel()
		h := newLandHarness(t, passing, blocked, passing)
		h.tick(false)
		h.tick(false)
		if record := h.stateRecord(); record.Land != nil {
			t.Errorf("state Land = %+v after a reading that does not pass; want the episode cleared", record.Land)
		}
		h.tick(false)
		if h.lands != 2 {
			t.Errorf("lands = %d; want a second one after the member opened and closed again", h.lands)
		}
	})

	t.Run("a restart over a record holding the episode asks for none", func(t *testing.T) {
		t.Parallel()
		h := newLandHarness(t, passing)
		h.tick(false)
		restarted := &daemon{
			geom: h.d.geom, deps: h.d.deps, probe: h.d.probe,
			flights: map[string]flight{}, childSeen: map[string]time.Time{}, wasCovered: map[string]bool{}, liftPending: map[string]bool{},
		}
		if err := restarted.tick(context.Background(), h.clock.now()); err != nil {
			t.Fatalf("tick() = %v; want nil", err)
		}
		if h.lands != 1 {
			t.Errorf("lands = %d; want the restarted dobby to ask for no second one", h.lands)
		}
	})

	t.Run("the prime's dobby reads nothing and acts on nothing", func(t *testing.T) {
		t.Parallel()
		h := newLandHarness(t, passing)
		h.d.geom.Slug = ""
		h.tick(false)
		if h.reads != 0 || h.lands != 0 || len(h.fake.notices) != 0 {
			t.Errorf("reads, lands, notices = %d, %d, %q; want none of any", h.reads, h.lands, h.fake.notices)
		}
	})

	t.Run("a failed post of a refusal leaves the episode open and the next tick retries", func(t *testing.T) {
		t.Parallel()
		h := newLandHarness(t, passing)
		h.refusal = errors.New("lym beam land: refused")
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
		if record := h.stateRecord(); record.Land != nil || len(h.fake.notices) != 0 {
			t.Fatalf("after a failed post Land, notices = %+v, %q; want the episode open", record.Land, h.fake.notices)
		}
		h.tick(false)
		if got := len(h.fake.notices); got != 1 {
			t.Errorf("notices after the retry = %d; want 1", got)
		}
	})

	t.Run("a failed read changes nothing", func(t *testing.T) {
		t.Parallel()
		h := newLandHarness(t, passing, landStep{err: errors.New("the board is unreadable")}, passing)
		h.tick(false)
		h.tick(false)
		if record := h.stateRecord(); record.Land == nil {
			t.Errorf("state Land = nil after a failed read; want the episode kept")
		}
		h.tick(false)
		if h.lands != 1 {
			t.Errorf("lands = %d; want the failed read to keep the episode and ask for no second landing", h.lands)
		}
	})
}

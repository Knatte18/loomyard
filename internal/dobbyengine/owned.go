// owned.go handles a run a full pass found owned by another machine: the owner pause, the parent lock release, the one notice, and the re-read of the owner that lifts the pause.

package dobbyengine

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// ownedElsewhere handles the owned-elsewhere exit of a pass of slug, which has a status file.
// It keeps the run's full pass owed, records it owned elsewhere, pauses it under the owner, releases the parent lock record naming the run's child, and posts one notice naming `lym adopt` per owner episode.
// A failed owner pause warns once per episode and is recorded as failed; it is never retried blind, since the marker gives the run a full pass on each probe tick while no pause covers it.
// It posts no failing-pass notice.
func (d *daemon) ownedElsewhere(ctx context.Context, slug string, _ time.Time) {
	var noticeSent bool
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		rs := s.Runs[slug]
		rs.FullPassOwed, rs.OwnedElsewhere = true, true
		noticeSent = rs.OwnerNoticeSent
		if s.Runs == nil {
			s.Runs = map[string]RunState{}
		}
		s.Runs[slug] = rs
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the owned-elsewhere mark could not be recorded", "slug", slug, "error", err)
		return
	}

	pauseFailed := false
	if err := d.deps.PauseOwned(ctx, slug); err != nil {
		pauseFailed = true
		d.ownerPauses.Fail(slug, "dobbyengine: pausing a run owned by another machine failed; the next probe tick runs it a full pass", "slug", slug, "error", err)
	} else {
		d.ownerPauses.Recover(slug, "dobbyengine: the owner pause stands again", "slug", slug)
	}
	if err := d.deps.ReleaseParentLock(ctx, slug); err != nil {
		logger.Warn("dobbyengine: releasing the parent lock record of a run owned by another machine failed", "slug", slug, "error", err)
	}

	if !noticeSent {
		noticeSent = d.noticeOwnedElsewhere(ctx, slug)
	}
	err = UpdateState(d.geom.StatePath, func(s *State) error {
		rs := s.Runs[slug]
		rs.OwnerPauseFailed, rs.OwnerNoticeSent = pauseFailed, noticeSent
		s.Runs[slug] = rs
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the owner pause outcome could not be recorded", "slug", slug, "error", err)
	}
}

// noticeOwnedElsewhere posts the owner notice for slug and reports whether it went out.
func (d *daemon) noticeOwnedElsewhere(ctx context.Context, slug string) bool {
	owner, err := d.deps.SlugOwner(slug)
	if err != nil || owner == "" {
		owner = "another machine"
	}
	subject := slug
	if d.deps.AdoptSubject != nil {
		if found, err := d.deps.AdoptSubject(slug); err != nil {
			logger.Warn("dobbyengine: the adopt subject could not be read; the notice names the slug", "slug", slug, "error", err)
		} else if found != "" {
			subject = found
		}
	}
	line := fmt.Sprintf("%s: owned by %s, so it is paused here; way forward: run lym adopt %s on this machine to take it over, or on the owner's machine to continue it there", slug, owner, subject)
	if err := d.deps.Notice(ctx, line); err != nil {
		logger.Warn("dobbyengine: the owned-elsewhere notice did not go out", "slug", slug, "error", err)
		return false
	}
	return true
}

// recheckOwnerPause reads the effective owner of slug from the local Bolt, with no pull, once its owner pause stands.
// When it no longer reads another machine it lifts the dobby's own owner pause entry and owes the run a full pass at once.
// An adopt landing in that window runs on this machine and writes the local Bolt first, so it strands no run on a manual step.
func (d *daemon) recheckOwnerPause(ctx context.Context, slug string) {
	machine, err := d.deps.Machine()
	if err != nil {
		logger.Warn("dobbyengine: this machine's name is unreadable; the owner pause stays", "slug", slug, "error", err)
		return
	}
	owner, err := d.deps.SlugOwner(slug)
	if err != nil {
		logger.Warn("dobbyengine: the run's owner is unreadable; the owner pause stays", "slug", slug, "error", err)
		return
	}
	if owner != "" && owner != machine {
		return
	}
	if err := d.deps.LiftOwnerPause(ctx, slug); err != nil {
		logger.Warn("dobbyengine: lifting the owner pause failed; the next probe tick tries again", "slug", slug, "error", err)
		return
	}
	d.liftPending[slug] = true
	// The lift is handled here, so the view that reads the pause gone next tick registers no second one.
	d.wasCovered[slug] = false
}

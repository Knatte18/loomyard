// owned.go handles a run that a full pass found owned by another machine.
// It covers the owned-elsewhere mark, the parent lock release, the one notice, and the catch-up that hands the run over.

package dobbyengine

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// ownedElsewhere handles the owned-elsewhere exit of a pass of slug, which has a status file.
// It keeps the run's full pass owed and records it owned elsewhere.
// It owes the catch-up from origin, which hands the run over.
// It pauses nothing.
// It releases the parent lock record naming the run's child.
// It posts one notice naming `lym adopt` per owner episode, and no failing-pass notice.
func (d *daemon) ownedElsewhere(ctx context.Context, slug string, _ time.Time) {
	d.oweCatchUp()
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

	if err := d.deps.ReleaseParentLock(ctx, slug); err != nil {
		logger.Warn("dobbyengine: releasing the parent lock record of a run owned by another machine failed", "slug", slug, "error", err)
	}

	if !noticeSent {
		noticeSent = d.noticeOwnedElsewhere(ctx, slug)
	}
	err = UpdateState(d.geom.StatePath, func(s *State) error {
		rs := s.Runs[slug]
		rs.OwnerNoticeSent = noticeSent
		s.Runs[slug] = rs
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the owner notice outcome could not be recorded", "slug", slug, "error", err)
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
	line := fmt.Sprintf("%s: owned by %s; this machine hands it over; way forward: run lym adopt %s on this machine to take it over, or on the owner's machine to continue it there", slug, owner, subject)
	if err := d.deps.Notice(ctx, line); err != nil {
		logger.Warn("dobbyengine: the owned-elsewhere notice did not go out", "slug", slug, "error", err)
		return false
	}
	return true
}

// land.go is a beam's dobby's landing step: once per episode in which the beam is flagged ready and nothing works under it, it asks the beam's pawl run to land, or posts why it cannot.

package dobbyengine

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
)

// LandReading is what one read of a beam's landing state tells the dobby.
// Passes reports whether the beam is flagged ready to merge and the landing's refusal finds nothing working under it.
// ParentOwner and ParentSubject name the machine that owns the beam's parent and what `lym adopt` takes to move it, both empty unless another machine owns it.
type LandReading struct {
	Passes        bool
	ParentOwner   string
	ParentSubject string
}

// landBeam acts once per episode, where an episode runs from a passing reading to the next reading that does not pass.
// It asks for the landing through the Land seam, and posts a notice to the beam's own orch only when it cannot: another machine owns the parent, or Land refused.
// A refusal is posted once and Land is asked again each tick, since it may only wait on the run, until it succeeds or the reading stops passing.
// The prime's dobby and a dobby with no CanLand seam do nothing.
// A failed read or post is logged once per episode and changes nothing, so the next tick tries again.
func (d *daemon) landBeam(ctx context.Context) error {
	if d.deps.CanLand == nil || d.geom.Slug == "" {
		return nil
	}
	reading, err := d.deps.CanLand()
	if err != nil {
		d.landRead.Fail("dobbyengine: the beam's landing state is unreadable; the beam is not landed", "worktree", d.geom.WorktreeName, "error", err)
		return nil
	}
	d.landRead.Recover("dobbyengine: the beam's landing state reads again", "worktree", d.geom.WorktreeName)

	record, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		return err
	}
	if !reading.Passes {
		if record.Land == nil {
			return nil
		}
		return UpdateState(d.geom.StatePath, func(s *State) error {
			s.Land = nil
			return nil
		})
	}
	if record.Land != nil && !record.Land.Refused {
		return nil
	}

	if reading.ParentOwner != "" {
		return d.postLandNotice(ctx, fmt.Sprintf("%s: the beam is flagged ready and nothing works under it, but machine %s owns its parent, so it cannot land here; run `lym adopt %s` to take the parent over", d.geom.Slug, reading.ParentOwner, reading.ParentSubject), false)
	}
	if err := d.deps.Land(); err != nil {
		if record.Land != nil {
			return nil
		}
		return d.postLandNotice(ctx, fmt.Sprintf("%s: the beam is flagged ready and nothing works under it, but its dobby could not ask for the landing: %v", d.geom.Slug, err), true)
	}
	d.logOwn(d.deps.Now(), "dobby: asked the pawl run to land the beam")
	d.recordLand(false)
	return nil
}

// postLandNotice posts line to the beam's own orch and records the episode once it went out, open to a retry of the landing when refused.
func (d *daemon) postLandNotice(ctx context.Context, line string, refused bool) error {
	if err := d.deps.Notice(ctx, line); err != nil {
		d.landPost.Fail("dobbyengine: the landing notice did not go out; the next tick tries again", "worktree", d.geom.WorktreeName, "error", err)
		return nil
	}
	d.landPost.Recover("dobbyengine: the landing notice goes out again", "worktree", d.geom.WorktreeName)
	d.recordLand(refused)
	return nil
}

// recordLand records the episode, as acted on when refused is false, so neither this dobby nor a restarted one acts on it again.
func (d *daemon) recordLand(refused bool) {
	at := d.deps.Now()
	if err := UpdateState(d.geom.StatePath, func(s *State) error {
		s.Land = &LandState{At: at, Refused: refused}
		return nil
	}); err != nil {
		logger.Warn("dobbyengine: the landing step could not be recorded; the next tick acts again", "worktree", d.geom.WorktreeName, "error", err)
	}
}

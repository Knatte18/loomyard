// landnotice.go posts a beam's dobby's one "can land" notice per episode, when nothing works under the beam any more.

package dobbyengine

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
)

// LandReading is what one read of a beam's landing state tells the dobby.
// Passes reports whether the landing's refusal finds nothing working under the beam.
// ParentOwner and ParentSubject name the machine that owns the beam's parent and what `lyx adopt` takes to move it, both empty unless another machine owns it.
type LandReading struct {
	Passes        bool
	ParentOwner   string
	ParentSubject string
}

// landNotice posts the beam's "can land" notice to its own orch once per episode, where an episode runs from a passing reading to the next reading that does not pass.
// The prime's dobby and a dobby with no CanLand seam post none.
// A failed read or post is logged once per episode and changes nothing, so the next tick tries again.
func (d *daemon) landNotice(ctx context.Context) error {
	if d.deps.CanLand == nil || d.geom.Slug == "" {
		return nil
	}
	reading, err := d.deps.CanLand()
	if err != nil {
		d.landRead.Fail("dobbyengine: the beam's landing state is unreadable; no can-land notice is posted", "worktree", d.geom.WorktreeName, "error", err)
		return nil
	}
	d.landRead.Recover("dobbyengine: the beam's landing state reads again", "worktree", d.geom.WorktreeName)

	record, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		return err
	}
	if !reading.Passes {
		if record.LandNotice == nil {
			return nil
		}
		return UpdateState(d.geom.StatePath, func(s *State) error {
			s.LandNotice = nil
			return nil
		})
	}
	if record.LandNotice != nil {
		return nil
	}

	if err := d.deps.Notice(ctx, d.landNoticeText(reading)); err != nil {
		d.landPost.Fail("dobbyengine: the can-land notice did not go out; the next tick tries again", "worktree", d.geom.WorktreeName, "error", err)
		return nil
	}
	d.landPost.Recover("dobbyengine: the can-land notice goes out again", "worktree", d.geom.WorktreeName)
	sentAt := d.deps.Now()
	if err := UpdateState(d.geom.StatePath, func(s *State) error {
		s.LandNotice = &LandNoticeState{SentAt: sentAt}
		return nil
	}); err != nil {
		logger.Warn("dobbyengine: the sent can-land notice could not be recorded; the next tick posts it again", "worktree", d.geom.WorktreeName, "error", err)
	}
	return nil
}

// landNoticeText is the notice of a passing reading: the beam's land command, or the adopt that takes the parent over when another machine owns it.
func (d *daemon) landNoticeText(reading LandReading) string {
	if reading.ParentOwner != "" {
		return fmt.Sprintf("%s: nothing works under the beam any more, but machine %s owns its parent, so it cannot land here; run `lyx adopt %s` to take the parent over", d.geom.Slug, reading.ParentOwner, reading.ParentSubject)
	}
	return fmt.Sprintf("%s: nothing works under the beam any more; run `lyx beam land %s` to land it", d.geom.Slug, d.geom.Slug)
}

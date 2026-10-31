// landnotice.go implements the landing notice: one creel line to the nearest live orch once a run the dobby saw ahead of its Land row lands or ends without landing, as beam.yaml's land_notice and the run's board entry decide.

package dobbyengine

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// landNoticeMaxAttempts is how many ticks try a landing notice that was not posted before it is dropped.
const landNoticeMaxAttempts = 3

// landingCommitWidth is how many leading characters of the landing commit the notice names.
const landingCommitWidth = 12

// LandNoticeState is dobby's memory of one run for its landing notice, kept from the first sight of the run ahead of its Land row until the notice is settled.
type LandNoticeState struct {
	// Requested reports whether the run's board entry carried notify_on_land at the last tick that found the entry on the board.
	Requested bool `json:"requested,omitempty"`
	// Attempts counts the posts of the notice that failed.
	Attempts int `json:"attempts,omitempty"`
}

// landNoticeDue is one run past its Land row whose notice this tick settles.
type landNoticeDue struct {
	run       RunView
	requested bool
	attempts  int
}

// landNotices remembers every listed run ahead of its Land row, with whether its board entry asks for a notice.
// It settles each remembered run now past it: a line goes out when land_notice is all, or per-task with the entry's request, and none under off.
// A run first seen past its Land row is never remembered, so it gets no notice.
// The memory of a run no longer listed is dropped.
// A failed read of the board keeps each remembered request as it was, and a failed read of beam.yaml settles nothing this tick; both are logged once per episode.
func (d *daemon) landNotices(ctx context.Context, now time.Time, runs []RunView) {
	requests, tasksRead := d.landNoticeRequests(runs)
	var due []landNoticeDue
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		due = nil
		listed := make(map[string]bool, len(runs))
		for _, run := range runs {
			listed[run.Slug] = true
			memory, remembered := s.LandNotices[run.Slug]
			if run.PastLand {
				if remembered {
					due = append(due, landNoticeDue{run: run, requested: memory.Requested, attempts: memory.Attempts})
				}
				continue
			}
			if requested, onBoard := requests[run.Slug]; tasksRead && onBoard {
				memory.Requested = requested
			}
			if s.LandNotices == nil {
				s.LandNotices = map[string]LandNoticeState{}
			}
			s.LandNotices[run.Slug] = memory
		}
		for slug := range s.LandNotices {
			if !listed[slug] {
				delete(s.LandNotices, slug)
			}
		}
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the landing-notice memory could not be recorded", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	if len(due) == 0 {
		return
	}
	cfg, err := d.deps.LoadConfig()
	if err != nil {
		d.landNoticeConfig.Fail("dobbyengine: beam.yaml does not load; no landing notice is settled", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	d.landNoticeConfig.Recover("dobbyengine: beam.yaml loads again; landing notices are settled", "worktree", d.geom.WorktreeName)
	for _, notice := range due {
		d.settleLandNotice(ctx, now, cfg.LandNotice, notice)
	}
}

// landNoticeRequests reads, for every listed run ahead of its Land row, whether its board entry carries notify_on_land, keyed by the slugs on the board.
// It reads the board only when such a run is listed, and reports whether the read succeeded.
func (d *daemon) landNoticeRequests(runs []RunView) (map[string]bool, bool) {
	ahead := false
	for _, run := range runs {
		ahead = ahead || !run.PastLand
	}
	if !ahead {
		return nil, false
	}
	tasks, err := d.deps.Tasks()
	if err != nil {
		d.landNoticeBoard.Fail("dobbyengine: the board is unreadable; the landing-notice requests keep their last reading", "worktree", d.geom.WorktreeName, "error", err)
		return nil, false
	}
	d.landNoticeBoard.Recover("dobbyengine: the board reads again for the landing-notice requests", "worktree", d.geom.WorktreeName)
	requests := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		requests[task.Slug] = task.NotifyOnLand
	}
	return requests, true
}

// settleLandNotice posts the notice of one run past its Land row when mode calls for it and drops its memory, or counts a failed post and drops it at the last attempt.
// Once a notice for a run whose entry asked for it went out, the entry's request is cleared through Deps.ClearNotifyOnLand, and a failed clear is only logged.
func (d *daemon) settleLandNotice(ctx context.Context, now time.Time, mode string, notice landNoticeDue) {
	slug := notice.run.Slug
	send := mode == LandNoticeAll || (mode == LandNoticePerTask && notice.requested)
	if send {
		line := landNoticeLine(notice.run)
		if err := d.deps.Notice(ctx, line); err != nil {
			attempts := notice.attempts + 1
			logger.Warn("dobbyengine: the landing notice did not go out", "slug", slug, "attempt", attempts, "error", err)
			if attempts < landNoticeMaxAttempts {
				d.updateLandNotice(slug, func(memory *LandNoticeState) { memory.Attempts = attempts })
				return
			}
			logger.Warn("dobbyengine: the landing notice is dropped after its last attempt", "slug", slug)
		} else {
			d.logLine(now, "land notice "+slug+": "+landNoticeOutcome(notice.run))
			if notice.requested && d.deps.ClearNotifyOnLand != nil {
				if err := d.deps.ClearNotifyOnLand(slug); err != nil {
					logger.Warn("dobbyengine: the board entry's notify_on_land could not be cleared", "slug", slug, "error", err)
				}
			}
		}
	}
	d.updateLandNotice(slug, nil)
}

// updateLandNotice applies mutate to slug's landing-notice memory, or drops the memory when mutate is nil.
func (d *daemon) updateLandNotice(slug string, mutate func(*LandNoticeState)) {
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		memory, remembered := s.LandNotices[slug]
		if !remembered {
			return nil
		}
		if mutate == nil {
			delete(s.LandNotices, slug)
			return nil
		}
		mutate(&memory)
		s.LandNotices[slug] = memory
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the landing-notice memory could not be recorded", "slug", slug, "error", err)
	}
}

// landNoticeLine is the line a run's landing notice posts: the slug, the outcome and, for a landing, the head of its landing commit.
func landNoticeLine(run RunView) string {
	if !run.Landed {
		return fmt.Sprintf("%s %s: %s; lym batten status %s shows the run", runNoticePrefix, run.Slug, landNoticeOutcome(run), run.Slug)
	}
	if run.LandingCommit == "" {
		return fmt.Sprintf("%s %s: landed, commit unknown", runNoticePrefix, run.Slug)
	}
	return fmt.Sprintf("%s %s: landed as %s", runNoticePrefix, run.Slug, run.LandingCommit[:min(len(run.LandingCommit), landingCommitWidth)])
}

// landNoticeOutcome names how a run past its Land row ended.
func landNoticeOutcome(run RunView) string {
	if run.Landed {
		return "landed"
	}
	return "ended without landing"
}

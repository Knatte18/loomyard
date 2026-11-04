// landnotice.go implements the landing notice: one creel line to the nearest live orch once a run the dobby saw ahead of its Land row lands or ends without landing, or once a board entry of this worktree that asks for it is done, as beam.yaml's land_notice and the entry decide.

package dobbyengine

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/logger"
)

// landingCommitWidth is how many leading characters of the landing commit the notice names.
const landingCommitWidth = 12

// LandNoticeState is dobby's memory of one run for its landing notice, kept from the first sight of the run ahead of its Land row, or of its done board entry asking for a notice, until the notice is settled.
type LandNoticeState struct {
	// Requested reports whether the run's board entry carried notify_on_land at the last tick that found the run ahead of its Land row and the entry on the board.
	Requested bool `json:"requested,omitempty"`
	// Line is the notice decided and not yet posted, posted again at each tick until a post succeeds.
	Line string `json:"line,omitempty"`
	// ClearOwed reports that the entry's notify_on_land is cleared once Line is posted, or at once when no line is owed.
	ClearOwed bool `json:"clear_owed,omitempty"`
}

// owed reports whether the memory holds a decided notice, a line to post or a request to clear.
func (s LandNoticeState) owed() bool {
	return s.Line != "" || s.ClearOwed
}

// landNotices remembers every listed run ahead of its Land row, with whether its board entry asks for a notice, and decides the notice of each remembered run now past it.
// While Deps.ClearNotifyOnLand is set, it also takes up each done board entry of this worktree that this machine owns, that carries notify_on_land and holds no memory: a request set after its run passed Land, or a landing the dobby never saw ahead of Land.
// It then posts every owed line and clears every owed request, keeping what fails for the next tick.
// A failed read of the board keeps each remembered request as it was and takes up no done entry, and a failed read of beam.yaml decides nothing this tick; both are logged once per episode.
func (d *daemon) landNotices(ctx context.Context, now time.Time, runs []RunView) {
	tasks, tasksRead := d.landNoticeTasks()
	mode, modeRead := d.landNoticeMode()
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		if s.LandNotices == nil {
			s.LandNotices = map[string]LandNoticeState{}
		}
		requests := make(map[string]bool, len(tasks))
		for _, task := range tasks {
			requests[task.Slug] = task.NotifyOnLand
		}
		rememberRuns(s.LandNotices, runs, requests, tasksRead, mode, modeRead)
		if tasksRead && modeRead && d.deps.ClearNotifyOnLand != nil {
			d.takeDoneEntries(s.LandNotices, runs, tasks, mode)
		}
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the landing-notice memory could not be recorded", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	d.settleLandNotices(ctx, now)
}

// rememberRuns refreshes memory from the runs listed this tick.
// A run ahead of its Land row is remembered with its entry's request, refreshed while the entry is on the board.
// A remembered run past its Land row with no notice decided gets one when mode sends for it, the request read now counting beside the remembered one, and is forgotten otherwise;
// a requested run owes the clear of its entry's field either way, so a landed entry is not kept on the board for a notice that never comes.
// A run first seen past its Land row is never remembered, and the memory of a run no longer listed is dropped unless it owes something.
func rememberRuns(memory map[string]LandNoticeState, runs []RunView, requests map[string]bool, tasksRead bool, mode string, modeRead bool) {
	listed := make(map[string]bool, len(runs))
	for _, run := range runs {
		listed[run.Slug] = true
		note, remembered := memory[run.Slug]
		if !run.PastLand {
			if requested, onBoard := requests[run.Slug]; tasksRead && onBoard {
				note.Requested = requested
			}
			memory[run.Slug] = note
			continue
		}
		if !remembered || note.owed() || !modeRead {
			continue
		}
		requested := note.Requested || (tasksRead && requests[run.Slug])
		note.ClearOwed = requested
		if sendsFor(mode, requested) {
			note.Line = landNoticeLine(run)
		}
		if !note.owed() {
			delete(memory, run.Slug)
			continue
		}
		memory[run.Slug] = note
	}
	for slug, note := range memory {
		if !listed[slug] && !note.owed() {
			delete(memory, slug)
		}
	}
}

// takeDoneEntries adds to memory each done entry of tasks whose parent is this dobby's worktree, that another machine does not own, carries notify_on_land and has no memory of its own.
// It owes the line mode sends, landed with the commit of its run when the run is listed and none recorded otherwise, and the clear of the request under every mode.
// A done entry is read as landed whatever set its status, so the line can name a landing that an entry marked done by hand never made.
func (d *daemon) takeDoneEntries(memory map[string]LandNoticeState, runs []RunView, tasks []TaskView, mode string) {
	for _, task := range tasks {
		if !task.NotifyOnLand || !boardengine.IsDoneStatus(task.Status) || task.Parent != d.geom.WorktreeName || task.OwnedElsewhere {
			continue
		}
		if _, remembered := memory[task.Slug]; remembered {
			continue
		}
		note := LandNoticeState{Requested: true, ClearOwed: true}
		if sendsFor(mode, true) {
			run := RunView{Slug: task.Slug, PastLand: true, Landed: true}
			if i := slices.IndexFunc(runs, func(listed RunView) bool { return listed.Slug == task.Slug }); i >= 0 && runs[i].PastLand {
				run = runs[i]
			}
			note.Line = landNoticeLine(run)
		}
		memory[task.Slug] = note
	}
}

// sendsFor reports whether land_notice mode sends a line for a run whose entry's request reads requested.
func sendsFor(mode string, requested bool) bool {
	return mode == LandNoticeAll || (mode == LandNoticePerTask && requested)
}

// landNoticeTasks reads the board for the landing notices, and reports whether the read succeeded.
func (d *daemon) landNoticeTasks() ([]TaskView, bool) {
	tasks, err := d.deps.Tasks()
	if err != nil {
		d.landNoticeBoard.Fail("dobbyengine: the board is unreadable; the landing-notice requests keep their last reading", "worktree", d.geom.WorktreeName, "error", err)
		return nil, false
	}
	d.landNoticeBoard.Recover("dobbyengine: the board reads again for the landing-notice requests", "worktree", d.geom.WorktreeName)
	return tasks, true
}

// landNoticeMode reads beam.yaml's land_notice, and reports whether the read succeeded.
func (d *daemon) landNoticeMode() (string, bool) {
	cfg, err := d.deps.LoadConfig()
	if err != nil {
		d.landNoticeConfig.Fail("dobbyengine: beam.yaml does not load; no landing notice is decided", "worktree", d.geom.WorktreeName, "error", err)
		return "", false
	}
	d.landNoticeConfig.Recover("dobbyengine: beam.yaml loads again; landing notices are decided", "worktree", d.geom.WorktreeName)
	return cfg.LandNotice, true
}

// settleLandNotices posts each owed line through Deps.Notice and then clears each owed request through Deps.ClearNotifyOnLand, dropping a memory once it owes nothing.
// A failed post keeps the line for the next tick, with no cap on the tries, and is logged once per episode; a failed clear keeps the clear for the next tick and is logged.
func (d *daemon) settleLandNotices(ctx context.Context, now time.Time) {
	record, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		logger.Warn("dobbyengine: the landing-notice memory is unreadable; no notice is settled", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	for _, slug := range slices.Sorted(maps.Keys(record.LandNotices)) {
		note := record.LandNotices[slug]
		if !note.owed() {
			continue
		}
		if note.Line != "" {
			if err := d.deps.Notice(ctx, note.Line); err != nil {
				d.landNoticePosts.Fail(slug, "dobbyengine: the landing notice did not go out; it stays owed for the next tick", "slug", slug, "error", err)
				continue
			}
			d.landNoticePosts.Recover(slug, "dobbyengine: the owed landing notice went out", "slug", slug)
			d.logLine(now, "land notice "+slug+": "+note.Line)
			d.updateLandNotice(slug, func(memory *LandNoticeState) { memory.Line = "" })
		}
		if note.ClearOwed && d.deps.ClearNotifyOnLand != nil {
			if err := d.deps.ClearNotifyOnLand(slug); err != nil {
				logger.Warn("dobbyengine: the board entry's notify_on_land could not be cleared; it is tried again on the next tick", "slug", slug, "error", err)
				continue
			}
		}
		d.updateLandNotice(slug, nil)
	}
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

// landnotice.go implements the landing notice: one creel line to the nearest live orch.
// It goes out once a run seen ahead of its Land row lands or ends without landing.
// It also goes out once a board entry of this worktree that asks for it is done.
// beam.yaml's land_notice and the entry decide whether it does.
// While a beam's pull request is recorded, the beam's members ask for it, flagged or not.
// Their landed line then carries the beam's follow-up.

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

// LandNoticeState is dobby's memory of one run for its landing notice, kept until the notice is settled.
// Dobby first sees the run ahead of its Land row, or sees its done board entry asking for a notice.
type LandNoticeState struct {
	// Requested reports whether the run's board entry asked for notify_on_land at the last tick that saw it ahead of Land.
	Requested bool `json:"requested,omitempty"`
	// Member reports whether the run's entry was a member of the beam at the last tick that saw it ahead of Land.
	// A member's landed line goes out under per-task once the beam's pull request is recorded.
	// It does so whatever the entry's flag read.
	Member bool `json:"member,omitempty"`
	// Line is the notice decided and not yet posted, posted again at each tick until a post succeeds.
	Line string `json:"line,omitempty"`
	// ClearOwed reports that the entry's notify_on_land is cleared once Line is posted, or at once when no line is owed.
	ClearOwed bool `json:"clear_owed,omitempty"`
}

// owed reports whether the memory holds a decided notice, a line to post or a request to clear.
func (s LandNoticeState) owed() bool {
	return s.Line != "" || s.ClearOwed
}

// landNotices remembers each listed run ahead of its Land row, with whether its board entry asks for a notice.
// It decides the notice of each remembered run now past its Land row.
// While Deps.ClearNotifyOnLand is set, it also takes up each done board entry of this worktree that this machine owns.
// Such an entry carries notify_on_land and has no memory of its run.
// That happens when its request was set after the run passed Land, or when the run was never seen ahead of Land.
// It then posts every owed line and clears every owed request, keeping what fails for the next tick.
// While the beam's pull request is recorded, it first flags each open member of the beam.
// A member is flagged only while its run is listed ahead of its Land row.
// A landed line it decides carries the beam's follow-up.
// A member remembered ahead of Land gets that line under per-task even when its entry was never flagged.
// A failed read of the board keeps each remembered request as it was and takes up no done entry.
// A failed read of beam.yaml or of the landing record decides nothing this tick; each is logged once per episode.
func (d *daemon) landNotices(ctx context.Context, now time.Time, runs []RunView) {
	tasks, tasksRead := d.landNoticeTasks()
	mode, modeRead := d.landNoticeMode()
	followUp, followUpRead := d.pullRequestFollowUp()
	if tasksRead && followUp != "" {
		tasks = d.flagMembers(tasks, runs)
	}
	decides := modeRead && followUpRead
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		if s.LandNotices == nil {
			s.LandNotices = map[string]LandNoticeState{}
		}
		entries := make(map[string]boardEntry, len(tasks))
		for _, task := range tasks {
			entries[task.Slug] = boardEntry{requested: task.NotifyOnLand, member: isBeamMember(task, d.geom.Slug)}
		}
		rememberRuns(s.LandNotices, runs, entries, tasksRead, mode, decides, followUp)
		if tasksRead && decides && d.deps.ClearNotifyOnLand != nil {
			d.takeDoneEntries(s.LandNotices, runs, tasks, mode, followUp)
		}
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the landing-notice memory could not be recorded", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	d.settleLandNotices(ctx, now)
}

// boardEntry is what one board entry reads as for the landing notice.
// It says whether the entry asks for one, and whether it is a member of this dobby's beam.
type boardEntry struct {
	requested bool
	member    bool
}

// rememberRuns refreshes memory from the runs listed this tick.
// A run ahead of its Land row is remembered with its entry's request and membership.
// Both are refreshed while the entry is on the board.
// A remembered run past its Land row with no notice decided gets one when mode sends for it.
// That happens only while decides is set.
// A landed line ends with followUp, empty for none.
// While followUp is set, a member of the beam counts as requested, flagged or not.
// The request read now counts beside the remembered one; a run that owes nothing after this is forgotten.
// A run whose entry carries the field owes its clear either way,
// so a landed entry is not kept on the board for a notice that never comes.
// A run first seen past its Land row is never remembered.
// The memory of a run not listed this tick is dropped unless it owes something.
func rememberRuns(memory map[string]LandNoticeState, runs []RunView, entries map[string]boardEntry, tasksRead bool, mode string, decides bool, followUp string) {
	listed := make(map[string]bool, len(runs))
	for _, run := range runs {
		listed[run.Slug] = true
		note, remembered := memory[run.Slug]
		entry, onBoard := entries[run.Slug]
		onBoard = tasksRead && onBoard
		if !run.PastLand {
			if onBoard {
				note.Requested = entry.requested
				note.Member = entry.member
			}
			memory[run.Slug] = note
			continue
		}
		if !remembered || note.owed() || !decides {
			continue
		}
		carriesField := note.Requested || (onBoard && entry.requested)
		requested := carriesField || (followUp != "" && (note.Member || (onBoard && entry.member)))
		note.ClearOwed = carriesField
		if sendsFor(mode, requested) {
			note.Line = landNoticeLine(run, followUp)
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

// takeDoneEntries adds to memory each done entry of tasks whose parent is this dobby's worktree.
// It takes only entries that carry notify_on_land, that no other machine owns, and that have no memory yet.
// The line mode sends reports the run's landing commit when the run is listed past Land, and ends with followUp.
// The clear of the request is owed under every mode.
// A done entry is read as landed whatever set its status,
// so the line can name a landing that an entry marked done by hand never made.
func (d *daemon) takeDoneEntries(memory map[string]LandNoticeState, runs []RunView, tasks []TaskView, mode, followUp string) {
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
			note.Line = landNoticeLine(run, followUp)
		}
		memory[task.Slug] = note
	}
}

// pullRequestFollowUp returns the follow-up a member's landed line carries.
// It is set while the beam's landing record holds a pull request.
// The follow-up asks the orch to add the landing to the change description and to run `lym beam describe <slug>`.
// It is empty while no pull request is recorded, for the prime's dobby, and for a dobby with no PullRequestRecorded seam.
// It also reports whether the read succeeded; a failed read is logged once per episode.
func (d *daemon) pullRequestFollowUp() (string, bool) {
	if d.deps.PullRequestRecorded == nil || d.geom.Slug == "" {
		return "", true
	}
	recorded, err := d.deps.PullRequestRecorded()
	if err != nil {
		d.landNoticePullRequest.Fail("dobbyengine: the beam's landing record is unreadable; no landing notice is decided", "worktree", d.geom.WorktreeName, "error", err)
		return "", false
	}
	d.landNoticePullRequest.Recover("dobbyengine: the beam's landing record reads again; landing notices are decided", "worktree", d.geom.WorktreeName)
	if !recorded {
		return "", true
	}
	return fmt.Sprintf("; add it to the change description, then run `lym beam describe %s`, which updates the pull request", d.geom.Slug), true
}

// isBeamMember reports whether task is a task or project entry whose project is the beam slug.
// The prime's dobby has no beam slug, so it has no members.
func isBeamMember(task TaskView, slug string) bool {
	return slug != "" && task.Project == slug && (task.Kind == boardengine.KindTask || task.Kind == boardengine.KindProject)
}

// flagMembers sets notify_on_land through Deps.SetNotifyOnLand on each open member of this dobby's beam that lacks it.
// An open member is not done and has a listed run ahead of its Land row,
// so a flag cleared by its landed line is not set again while its entry is still open.
// That holds whether or not its run is still listed.
// It returns a copy of tasks with each entry it flagged marked as asking for the notice.
// A failed write is logged once per episode and tried again at the next tick.
func (d *daemon) flagMembers(tasks []TaskView, runs []RunView) []TaskView {
	if d.deps.SetNotifyOnLand == nil {
		return tasks
	}
	flagged := slices.Clone(tasks)
	var failure error
	for i, task := range flagged {
		aheadOfLand := slices.ContainsFunc(runs, func(run RunView) bool { return run.Slug == task.Slug && !run.PastLand })
		if !isBeamMember(task, d.geom.Slug) || task.NotifyOnLand || boardengine.IsDoneStatus(task.Status) || !aheadOfLand {
			continue
		}
		if err := d.deps.SetNotifyOnLand(task.Slug); err != nil {
			failure = err
			continue
		}
		flagged[i].NotifyOnLand = true
	}
	if failure != nil {
		d.landNoticeFlag.Fail("dobbyengine: a beam member's notify_on_land could not be set; it is tried again on the next tick", "worktree", d.geom.WorktreeName, "error", failure)
	} else {
		d.landNoticeFlag.Recover("dobbyengine: the beam members' notify_on_land is set again", "worktree", d.geom.WorktreeName)
	}
	return flagged
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

// settleLandNotices posts each owed line through Deps.Notice.
// It then clears each owed request through Deps.ClearNotifyOnLand, and drops a memory once it owes nothing.
// A failed post keeps the line for the next tick, with no cap on the tries, and is logged once per episode.
// A failed clear keeps the clear for the next tick and is logged.
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

// landNoticeLine is the line a run's landing notice posts.
// It names the slug and the outcome, and for a landing the head of its landing commit followed by followUp.
func landNoticeLine(run RunView, followUp string) string {
	if !run.Landed {
		return fmt.Sprintf("%s %s: %s; lym batten status %s shows the run", runNoticePrefix, run.Slug, landNoticeOutcome(run), run.Slug)
	}
	if run.LandingCommit == "" {
		return fmt.Sprintf("%s %s: landed, commit unknown%s", runNoticePrefix, run.Slug, followUp)
	}
	return fmt.Sprintf("%s %s: landed as %s%s", runNoticePrefix, run.Slug, run.LandingCommit[:min(len(run.LandingCommit), landingCommitWidth)], followUp)
}

// landNoticeOutcome names how a run past its Land row ended.
func landNoticeOutcome(run RunView) string {
	if run.Landed {
		return "landed"
	}
	return "ended without landing"
}

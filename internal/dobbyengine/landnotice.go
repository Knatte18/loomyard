// landnotice.go implements the landing notice: one creel line to the nearest live orch.
// It goes out once a run seen ahead of its Land row lands or ends without landing.
// It goes only when the run's land_notice setting read on, as read ahead of Land or, when that never read, after it.

package dobbyengine

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// landingCommitWidth is how many leading characters of the landing commit the notice names.
const landingCommitWidth = 12

// LandNoticeState is dobby's memory of one run for its landing notice, kept until the notice is settled.
// Dobby first sees the run ahead of its Land row.
type LandNoticeState struct {
	// Requested reports whether land_notice read on for the run at the last tick that read it.
	Requested bool `json:"requested,omitempty"`
	// Read reports whether land_notice was read for the run at least once.
	// A run past Land that was never read is read again at each tick, and its notice stays undecided until a read succeeds.
	Read bool `json:"read,omitempty"`
	// Line is the notice decided and not yet posted, posted again at each tick until a post succeeds.
	Line string `json:"line,omitempty"`
	// ClearOwed is set only by a state record an older binary wrote, once it had posted the run's line.
	// This binary never sets it; a run past Land that holds it owes nothing and is forgotten.
	ClearOwed bool `json:"clear_owed,omitempty"`
}

// owed reports whether the memory holds a decided notice still to post.
func (s LandNoticeState) owed() bool {
	return s.Line != ""
}

// read reports whether land_notice was read for the run.
// This binary sets Read with each reading, and a memory an older binary wrote holds a reading once it holds a request.
func (s LandNoticeState) read() bool {
	return s.Read || s.Requested
}

// landNotices remembers each listed run ahead of its Land row with whether land_notice reads on for it.
// It decides the notice of each remembered run now past its Land row, then posts every owed line.
// What fails to post is kept for the next tick.
// The reading of land_notice comes from Deps.LandNotice.
// A failed read keeps the run's last reading and is logged once per episode.
// A run past Land that was never read is read again at each tick, and its notice waits for a read to succeed.
func (d *daemon) landNotices(ctx context.Context, now time.Time, runs []RunView) {
	requests := d.readLandNoticeRequests(runs, d.unreadLandNoticeRuns(runs))
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		if s.LandNotices == nil {
			s.LandNotices = map[string]LandNoticeState{}
		}
		rememberRuns(s.LandNotices, runs, requests)
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the landing-notice memory could not be recorded", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	d.settleLandNotices(ctx, now)
}

// readLandNoticeRequests reads land_notice through Deps.LandNotice for each run listed ahead of its Land row.
// It reads it too for each run past that row which unread names.
// It returns the readings that succeeded, by slug.
// A nil seam reads every such run as off.
func (d *daemon) readLandNoticeRequests(runs []RunView, unread map[string]bool) map[string]bool {
	requests := make(map[string]bool, len(runs))
	for _, run := range runs {
		if run.PastLand && !unread[run.Slug] {
			continue
		}
		if d.deps.LandNotice == nil {
			requests[run.Slug] = false
			continue
		}
		on, err := d.deps.LandNotice(run.Slug)
		if err != nil {
			d.landNoticeLookup.Fail(run.Slug, "dobbyengine: the run's "+keyName(LandNoticeKey)+" does not read; it keeps its last reading and is read again", "slug", run.Slug, "error", err)
			continue
		}
		d.landNoticeLookup.Recover(run.Slug, "dobbyengine: the run's "+keyName(LandNoticeKey)+" reads again", "slug", run.Slug)
		requests[run.Slug] = on
	}
	return requests
}

// unreadLandNoticeRuns returns the slugs of the listed runs past their Land row that dobby remembers.
// Each has never had its land_notice read, and no older binary had already posted a line for it.
// An unreadable memory names none; settleLandNotices logs it.
func (d *daemon) unreadLandNoticeRuns(runs []RunView) map[string]bool {
	record, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		return nil
	}
	unread := map[string]bool{}
	for _, run := range runs {
		note, remembered := record.LandNotices[run.Slug]
		if run.PastLand && remembered && !note.read() && !note.ClearOwed && !note.owed() {
			unread[run.Slug] = true
		}
	}
	return unread
}

// rememberRuns refreshes memory from the runs listed this tick.
// A run is remembered with the land_notice reading in requests, when it has one.
// A remembered run past its Land row that was read at least once gets its notice decided:
// a line when its remembered reading is on, and none when it is off.
// A remembered run past Land that was never read stays undecided until a read succeeds.
// A run that owes nothing after this is forgotten, and so is one whose line an older binary had already posted.
// A run first seen past its Land row is never remembered.
// The memory of a run not listed this tick is dropped unless it owes something.
func rememberRuns(memory map[string]LandNoticeState, runs []RunView, requests map[string]bool) {
	listed := make(map[string]bool, len(runs))
	for _, run := range runs {
		listed[run.Slug] = true
		note, remembered := memory[run.Slug]
		if requested, read := requests[run.Slug]; read {
			note.Requested, note.Read = requested, true
		}
		if !run.PastLand {
			memory[run.Slug] = note
			continue
		}
		if !remembered || note.owed() {
			continue
		}
		if note.ClearOwed {
			delete(memory, run.Slug)
			continue
		}
		memory[run.Slug] = note
		if !note.read() {
			continue
		}
		if note.Requested {
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

// settleLandNotices posts each owed line through Deps.Notice and drops a memory once it owes nothing.
// A failed post keeps the line for the next tick, with no cap on the tries, and is logged once per episode.
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
		if err := d.deps.Notice(ctx, note.Line); err != nil {
			d.landNoticePosts.Fail(slug, "dobbyengine: the landing notice did not go out; it stays owed for the next tick", "slug", slug, "error", err)
			continue
		}
		d.landNoticePosts.Recover(slug, "dobbyengine: the owed landing notice went out", "slug", slug)
		d.logLine(now, "land notice "+slug+": "+note.Line)
		d.dropLandNotice(slug)
	}
}

// dropLandNotice drops slug's landing-notice memory.
func (d *daemon) dropLandNotice(slug string) {
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		delete(s.LandNotices, slug)
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the landing-notice memory could not be recorded", "slug", slug, "error", err)
	}
}

// landNoticeLine is the line a run's landing notice posts.
// It names the slug and the outcome, and for a landing the head of its landing commit.
// A landing that changed no code path on the parent branch says so in place of naming its commit as the landing.
func landNoticeLine(run RunView) string {
	if !run.Landed {
		return fmt.Sprintf("%s %s: %s; lym batten status %s shows the run", runNoticePrefix, run.Slug, landNoticeOutcome(run), run.Slug)
	}
	if run.LandingCommit == "" {
		return fmt.Sprintf("%s %s: landed, commit unknown", runNoticePrefix, run.Slug)
	}
	commit := run.LandingCommit[:min(len(run.LandingCommit), landingCommitWidth)]
	if run.EmptyLanding {
		return fmt.Sprintf("%s %s: landed with an empty diff: no file of its parent branch changed, which stays at %s", runNoticePrefix, run.Slug, commit)
	}
	return fmt.Sprintf("%s %s: landed as %s", runNoticePrefix, run.Slug, commit)
}

// landNoticeOutcome names how a run past its Land row ended.
func landNoticeOutcome(run RunView) string {
	if run.Landed {
		return "landed"
	}
	return "ended without landing"
}

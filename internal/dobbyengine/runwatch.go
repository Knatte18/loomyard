// runwatch.go implements the run watch.
// It owes the nearest live orch one mail line for each change of a driven run that the run's beam.run_watch asks for.
// It reads every listed run on each scheduling tick, remembers the reading, and owes and delivers the lines.

package dobbyengine

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/runfsm"
)

// runWatchPrefix is the literal every watch line begins with, so the orch can tell it from operator input.
const runWatchPrefix = "[run watch]"

const (
	// watchLineMaxAge is the awake time a line stays owed before it is dropped.
	watchLineMaxAge = time.Hour
	// watchPostMaxAttempts is how many failed posts a line gets before it is dropped.
	watchPostMaxAttempts = 3
)

// RunWatchDeps is the seams of the run watch, told by the hub wiring.
// A nil Level or a nil Post sends no watch line and keeps no memory.
type RunWatchDeps struct {
	// Level reads the beam.run_watch of the run slug: stops, rows or off.
	Level func(slug string) (string, error)
	// Post posts one untagged line about the run slug to the nearest live orch.
	// posted is false with a nil error while no orch strand is recorded.
	Post func(ctx context.Context, slug, line string) (posted bool, err error)
}

// WatchReading is what the run watch reads of one run at one tick.
// It is comparable.
type WatchReading struct {
	// Run is the run's state derived from its tracked records.
	Run string `json:"run,omitempty"`
	// BattenState, BattenRow and BattenError are batten's status state, current producer and reason.
	BattenState string `json:"batten_state,omitempty"`
	BattenRow   string `json:"batten_row,omitempty"`
	BattenError string `json:"batten_error,omitempty"`
	// ChildRead reports whether the child's status was ever read, which the child fields below then describe.
	ChildRead  bool   `json:"child_read,omitempty"`
	ChildState string `json:"child_state,omitempty"`
	ChildRow   string `json:"child_row,omitempty"`
	ChildError string `json:"child_error,omitempty"`
	// WaitDied reports that a driver died while it waited out a network failure.
	WaitDied bool `json:"wait_died,omitempty"`
	// ChildEpisode is the child's history length plus its newest entry's folded repeats.
	ChildEpisode int `json:"child_episode,omitempty"`
	// PairDir is the task worktree's directory, which a reason's paths are rendered relative to.
	PairDir string `json:"pair_dir,omitempty"`
	// Episode tells two stops at the same place apart:
	// the history length plus the newest entry's folded repeats, of batten while it is not running, else of the child.
	Episode int `json:"episode,omitempty"`
	// Since is when the newer of batten's and the child's status was written.
	Since time.Time `json:"since,omitzero"`
}

// compared returns the reading without the parts that make no change of their own:
// its time, its reason texts, the pair directory, and the child's episode, which Episode carries while it counts.
func (r WatchReading) compared() WatchReading {
	r.Since, r.BattenError, r.ChildError, r.PairDir, r.ChildEpisode = time.Time{}, "", "", "", 0
	return r
}

// OwedWatchLine is one watch line still to post.
type OwedWatchLine struct {
	Line   string    `json:"line"`
	OwedAt time.Time `json:"owed_at"`
	// Attempts counts the failed posts, and NextTry is the earliest time of the next.
	Attempts int       `json:"attempts,omitempty"`
	NextTry  time.Time `json:"next_try,omitzero"`
}

// RunWatchState is dobby's memory of one run for the run watch.
type RunWatchState struct {
	// Level is the last beam.run_watch read for the run, empty while none was read.
	Level string `json:"level,omitempty"`
	// Last is the last reading the watch handled, nil while there is none.
	Last *WatchReading `json:"last,omitempty"`
	// LandNotice is the run's beam.land_notice reading copied ahead of its Land row, nil while none was copied.
	LandNotice *bool `json:"land_notice,omitempty"`
	// AheadOfLand reports that the run was ahead of its Land row at the last tick that listed it.
	AheadOfLand bool `json:"ahead_of_land,omitempty"`
	// Owed is the lines still to post, oldest first.
	Owed []OwedWatchLine `json:"owed,omitempty"`
	// GateHolds is the gate holds of the run already noticed.
	GateHolds []string `json:"gate_holds,omitempty"`
	// StatusSeen and ChildSeen are batten's and the child's listed status times at the last child read the watch took.
	// A child written after its listing is not taken.
	StatusSeen time.Time `json:"status_seen,omitzero"`
	ChildSeen  time.Time `json:"child_seen,omitzero"`
}

// runWatch reads every listed run, owes the watch lines its changes call for, and posts the owed ones.
// A run no longer listed keeps its memory while it owes a line or batten still has its status.
// A record that cannot be read or written is one Warn and ends the step; watchRecordFailed then tells the tick.
func (d *daemon) runWatch(ctx context.Context, now time.Time) {
	d.watchRecordFailed = false
	if d.deps.RunWatch.Level == nil || d.deps.RunWatch.Post == nil {
		return
	}
	record, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		d.failWatchRecord(err)
		return
	}
	watches := maps.Clone(record.Watches)
	if watches == nil {
		watches = map[string]RunWatchState{}
	}
	listed := make(map[string]bool, len(d.listedRuns))
	for _, run := range d.listedRuns {
		listed[run.Slug] = true
		watches[run.Slug] = d.watchRun(now, run, watches[run.Slug], record.LandNotices[run.Slug])
	}
	for slug, watch := range watches {
		if listed[slug] {
			continue
		}
		// The landing memory of a run first listed again past Land is gone for good,
		// so the copy would skip a line nothing else sends.
		if _, remembered := record.LandNotices[slug]; watch.AheadOfLand && !remembered {
			watch.LandNotice = nil
			watches[slug] = watch
		}
		if len(watch.Owed) > 0 || d.deps.StatusExists == nil {
			continue
		}
		if exists, err := d.deps.StatusExists(slug); err == nil && !exists {
			delete(watches, slug)
		}
	}
	if !d.recordWatches(watches) {
		return
	}
	if d.postWatchLines(ctx, now, watches, record.Sleeps) {
		d.recordWatches(watches)
	}
}

// failWatchRecord logs a state record the watch could not use and marks the tick.
func (d *daemon) failWatchRecord(err error) {
	d.watchRecordFailed = true
	logger.Warn("dobbyengine: the run watch could not use the state record; the next tick tries again", "worktree", d.geom.WorktreeName, "error", err)
}

// recordWatches writes watches as the record's run watch memory and reports whether it did.
func (d *daemon) recordWatches(watches map[string]RunWatchState) bool {
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		s.Watches = watches
		return nil
	})
	if err != nil {
		d.failWatchRecord(err)
		return false
	}
	return true
}

// watchRun returns run's memory after this tick's reading, with the line it owes, if any.
func (d *daemon) watchRun(now time.Time, run RunView, watch RunWatchState, landNote LandNoticeState) RunWatchState {
	// Copied ahead of Land and before the level is read, since the landing notice drops its memory once it posted.
	if !run.PastLand && landNote.read() {
		requested := landNote.Requested
		watch.LandNotice = &requested
	}
	watch.AheadOfLand = !run.PastLand
	// Past Land the task's worktree, where its own setting lives, is torn down, so a read would answer a parent's.
	if !run.PastLand || watch.Level == "" {
		level, err := d.deps.RunWatch.Level(run.Slug)
		if err != nil {
			d.watchLevelReads.Fail(run.Slug, "dobbyengine: the run's "+keyName(RunWatchKey)+" does not read; it keeps its last level", "slug", run.Slug, "error", err)
		} else {
			d.watchLevelReads.Recover(run.Slug, "dobbyengine: the run's "+keyName(RunWatchKey)+" reads again", "slug", run.Slug)
			watch.Level = level
		}
	}
	// A waiting run has not stopped: its last reading stays, and it owes nothing.
	if watch.Level == "" || run.Waiting {
		return watch
	}
	reading, ok := d.readWatchedRun(now, run, &watch)
	if !ok {
		return watch
	}
	reading.WaitDied = run.WaitDied
	last := watch.Last
	watch.Last = &reading
	if last != nil && reading.compared() == last.compared() {
		return watch
	}
	if watch.Level == RunWatchOff {
		return watch
	}
	if watch.Level == RunWatchStops {
		if !watchStop(reading) {
			return watch
		}
		if last != nil && reading.Run == last.Run && reading.BattenState == last.BattenState && reading.ChildState == last.ChildState &&
			reading.ChildRead == last.ChildRead && reading.Episode == last.Episode && reading.WaitDied == last.WaitDied {
			return watch
		}
	}
	// The landing notice reports this landing.
	if reading.Run == string(runfsm.Landed) && watch.LandNotice != nil && *watch.LandNotice {
		return watch
	}
	told := reading
	if reading.WaitDied {
		// Only the line carries this reason: the remembered reading keeps the child's own.
		told.ChildError = fmt.Sprintf("the driver died waiting at %s on a %s failure", run.WaitProducer, run.WaitClass)
	}
	watch.Owed = append(slices.Clone(watch.Owed), OwedWatchLine{Line: watchLine(run.Slug, told), OwedAt: now})
	return watch
}

// readWatchedRun reads run.
// It reads the child only when watch has no reading yet or a status file was written since the last child read.
// A child that is not read, does not read or has no status keeps the child part of the last reading.
// So does a child written after the run was listed, since the listed state is older than it;
// ok is false when there is no last reading to keep.
// Every other child read that succeeds records the status times in watch.
func (d *daemon) readWatchedRun(now time.Time, run RunView, watch *RunWatchState) (reading WatchReading, ok bool) {
	reading = WatchReading{Run: string(run.Run), BattenState: run.State, BattenRow: run.Producer, BattenError: run.Error}
	if last := watch.Last; last != nil {
		reading.ChildRead, reading.ChildState, reading.ChildRow, reading.ChildError = last.ChildRead, last.ChildState, last.ChildRow, last.ChildError
		reading.ChildEpisode, reading.PairDir = last.ChildEpisode, last.PairDir
	}
	childModified := run.ChildStatusModified
	written := watch.Last == nil || !run.StatusModified.Equal(watch.StatusSeen) || !run.ChildStatusModified.Equal(watch.ChildSeen)
	if read := d.deps.RunNotices.ReadChild; read != nil && written {
		key := run.Slug + "|watch-child"
		child, found, err := read(run.Slug)
		if err != nil {
			d.noticeFailures.Fail(key, "dobbyengine: the child is unreadable; the run watch keeps its last child reading", "slug", run.Slug, "error", err)
		} else {
			d.noticeFailures.Recover(key, "dobbyengine: the child reads again for the run watch", "slug", run.Slug)
			if child.PairDir != "" {
				reading.PairDir = child.PairDir
			}
			// Pairing a newer child with the older listed state would tell one stop twice.
			if found && !child.Modified.Equal(run.ChildStatusModified) {
				if watch.Last == nil {
					return WatchReading{}, false
				}
			} else {
				watch.StatusSeen, watch.ChildSeen = run.StatusModified, run.ChildStatusModified
				if found {
					reading.ChildRead, reading.ChildState, reading.ChildRow, reading.ChildError = true, child.State, child.CurrentProducer, child.Error
					reading.ChildEpisode = child.HistoryLen + child.LastRepeats
				}
			}
		}
	}
	switch {
	case run.State != runStateRunning:
		reading.Episode = run.HistoryLen + run.LastRepeats
	case reading.ChildRead:
		reading.Episode = reading.ChildEpisode
	}
	reading.Since = run.StatusModified
	if childModified.After(reading.Since) {
		reading.Since = childModified
	}
	if reading.Since.IsZero() {
		reading.Since = now
	}
	return reading, true
}

// watchStop reports whether reading shows a run that stopped:
// a derived state of paused, halted, cancelling, cancelled or landed, a batten or child state other than running,
// or a driver that died while it waited.
func watchStop(reading WatchReading) bool {
	if reading.WaitDied {
		return true
	}
	switch runfsm.State(reading.Run) {
	case runfsm.Paused, runfsm.Halted, runfsm.Cancelling, runfsm.Cancelled, runfsm.Landed:
		return true
	}
	return reading.BattenState != runStateRunning || (reading.ChildRead && reading.ChildState != childStateRunning)
}

// watchLine renders the watch line of the run slug:
// its derived state, batten's and the child's state and row, the child's reason or else batten's, and since when.
// Every path under the pair directory is rendered relative to it, and the reason is cut to one line.
func watchLine(slug string, reading WatchReading) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: %s; batten %s at %s", runWatchPrefix, slug, reading.Run, reading.BattenState, reading.BattenRow)
	if reading.ChildRead {
		fmt.Fprintf(&b, "; child %s at %s", reading.ChildState, reading.ChildRow)
	} else {
		b.WriteString("; child none")
	}
	reason := reading.ChildError
	if reason == "" {
		reason = reading.BattenError
	}
	if reason != "" {
		fmt.Fprintf(&b, "; %s", oneLineCut(pairRelative(reason, reading.PairDir), runNoticeErrorMax))
	}
	fmt.Fprintf(&b, "; since %s", reading.Since.UTC().Format(time.RFC3339))
	return b.String()
}

// postWatchLines posts the owed lines of watches, by sorted slug and oldest first.
// It reports whether any memory changed.
// A run's posting ends at the first line that does not go.
// A line owed for watchLineMaxAge of awake time, or whose post failed watchPostMaxAttempts times, is dropped with a Warn.
func (d *daemon) postWatchLines(ctx context.Context, now time.Time, watches map[string]RunWatchState, sleeps []pauserecord.Interval) bool {
	changed := false
	for _, slug := range slices.Sorted(maps.Keys(watches)) {
		watch := watches[slug]
		owed := slices.Clone(watch.Owed)
		for len(owed) > 0 && !owed[0].NextTry.After(now) {
			line := owed[0]
			if pauserecord.AwakeSince(line.OwedAt, now, sleeps) >= watchLineMaxAge {
				logger.Warn("dobbyengine: a watch line owed too long is dropped", "slug", slug, "line", line.Line)
				owed, changed = owed[1:], true
				continue
			}
			posted, err := d.deps.RunWatch.Post(ctx, slug, line.Line)
			if err != nil {
				owed[0].Attempts++
				owed[0].NextTry = now.Add(d.probe)
				changed = true
				d.watchPosts.Fail(slug, "dobbyengine: a watch line did not go out", "slug", slug, "error", err)
				if owed[0].Attempts >= watchPostMaxAttempts {
					logger.Warn("dobbyengine: a watch line is dropped after its failed posts", "slug", slug, "line", line.Line, "attempts", owed[0].Attempts)
					owed = owed[1:]
				}
				break
			}
			if !posted {
				break
			}
			d.watchPosts.Recover(slug, "dobbyengine: the watch lines go out again", "slug", slug)
			d.logLine(now, "run watch "+slug+": "+line.Line)
			owed, changed = owed[1:], true
		}
		watch.Owed = owed
		watches[slug] = watch
	}
	return changed
}

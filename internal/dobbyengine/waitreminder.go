// waitreminder.go implements the wait reminders.
// It keeps the start of each child's wait on the operator in the state record.
// It also posts one reminder line per interval to the hub's Slack program while the operator is away.

package dobbyengine

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// waitCheckInterval is the shortest gap between two checks of the waits.
const waitCheckInterval = time.Minute

// WaitReminderDeps holds the seams the wait reminders read, apart from the child and pause reads.
// The hub wiring supplies them.
// A nil Post, or a nil child read, keeps no wait and sends no reminder.
type WaitReminderDeps struct {
	// Rows are the child rows that wait on the operator alone.
	Rows []string
	// LatestResume reads when the latest resumed pause entry covering slug resumed, zero when the record holds none.
	LatestResume func(slug string) (time.Time, error)
	// Away reports whether the machine's presence says the operator is in Slack.
	Away func() (bool, error)
	// Interval reads the reminder interval.
	Interval func() (time.Duration, error)
	// Post leaves one line for the hub's Slack program.
	Post func(ctx context.Context, line string) error
}

// OperatorWaitState is one child's wait on the operator, kept in the run's entry of the state record so a restarted dobby keeps its start.
// The zero value means the child is in no wait.
type OperatorWaitState struct {
	// Row is the row the child waits at.
	Row string `json:"row,omitempty"`
	// Since is when the wait started.
	Since time.Time `json:"since,omitzero"`
}

// waitReminders checks the waits at most once per waitCheckInterval: it updates each wait whatever the presence, then reminds of the ones due.
// A failure is logged once per episode and never fails the tick.
func (d *daemon) waitReminders(ctx context.Context, now time.Time) {
	if d.deps.WaitReminders.Post == nil || d.deps.RunNotices.ReadChild == nil {
		return
	}
	if !d.lastWaitCheck.IsZero() && now.Sub(d.lastWaitCheck) < waitCheckInterval {
		return
	}
	d.lastWaitCheck = now
	current, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		d.waitFailures.Fail("|state", "dobbyengine: the state record is unreadable; no wait check this minute", "error", err)
		return
	}
	d.waitFailures.Recover("|state", "dobbyengine: the state record reads again for the wait check")
	waits, unread := d.readWaits(now, current.Runs)
	d.recordWaits(current.Runs, waits, unread)
	d.remindOfWaits(ctx, now, waits)
}

// readWaits returns the wait of each run whose batten state is running with no cancel marker, by slug.
// It also returns the slugs whose reading failed, which stand as recorded and get no reminder.
// A run in no wait has no key in the waits.
func (d *daemon) readWaits(now time.Time, recorded map[string]RunState) (waits map[string]OperatorWaitState, unread map[string]bool) {
	waits, unread = map[string]OperatorWaitState{}, map[string]bool{}
	for _, run := range d.runs {
		if run.State != runStateRunning || run.CancelMarker {
			continue
		}
		wait, err := d.readWait(now, run.Slug, recorded[run.Slug].OperatorWait)
		if err != nil {
			d.waitFailures.Fail(run.Slug, "dobbyengine: the wait of the run is unreadable; it stands as recorded", "slug", run.Slug, "error", err)
			unread[run.Slug] = true
			continue
		}
		d.waitFailures.Recover(run.Slug, "dobbyengine: the wait of the run reads again", "slug", run.Slug)
		if wait != (OperatorWaitState{}) {
			waits[run.Slug] = wait
		}
	}
	return waits, unread
}

// readWait returns the wait of the child of slug at now, given the one recorded:
// zero unless the child runs at a reminded row with no pause covering it.
// A wait starts at the first such reading of its row.
// A later resume of a pause covering the child moves its start to that resume,
// so a pause and resume no check saw restarts it too.
func (d *daemon) readWait(now time.Time, slug string, recorded OperatorWaitState) (OperatorWaitState, error) {
	child, found, err := d.deps.RunNotices.ReadChild(slug)
	if err != nil {
		return OperatorWaitState{}, err
	}
	if !found || child.State != childStateRunning || !slices.Contains(d.deps.WaitReminders.Rows, child.CurrentProducer) {
		return OperatorWaitState{}, nil
	}
	pause, err := d.deps.RunNotices.Pause(slug)
	if err != nil {
		return OperatorWaitState{}, err
	}
	if pause.Covered {
		return OperatorWaitState{}, nil
	}
	resumed, err := d.deps.WaitReminders.LatestResume(slug)
	if err != nil {
		return OperatorWaitState{}, err
	}
	wait := recorded
	if wait.Row != child.CurrentProducer {
		wait = OperatorWaitState{Row: child.CurrentProducer, Since: now}
	}
	if resumed.After(wait.Since) {
		wait.Since = resumed
	}
	return wait, nil
}

// recordWaits writes every wait that differs from the record's and clears the wait of each recorded run in none.
// It also forgets the last reminder of every wait that ended.
// The slugs in unread stay as they are.
// A write failure is a Warn: a lost start only delays a reminder.
func (d *daemon) recordWaits(recorded map[string]RunState, waits map[string]OperatorWaitState, unread map[string]bool) {
	changes := map[string]OperatorWaitState{}
	for slug, run := range recorded {
		if !unread[slug] && waits[slug] != run.OperatorWait {
			changes[slug] = waits[slug]
		}
	}
	for slug, wait := range waits {
		if _, known := recorded[slug]; !known {
			changes[slug] = wait
		}
	}
	maps.DeleteFunc(d.lastReminders, func(slug string, _ time.Time) bool {
		_, waiting := waits[slug]
		return !waiting && !unread[slug]
	})
	if len(changes) == 0 {
		return
	}
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		if s.Runs == nil {
			s.Runs = map[string]RunState{}
		}
		for slug, wait := range changes {
			run := s.Runs[slug]
			run.OperatorWait = wait
			s.Runs[slug] = run
		}
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the waits could not be recorded", "error", err)
	}
}

// remindOfWaits, while the operator is away, posts one line for each wait at least an interval old.
// A wait is posted only if it has no reminder or its last reminder is an interval old.
// A presence other than away forgets every last reminder, so a return reminds at once of a wait past the interval.
// A post that fails is logged at Warn and tried at the next check.
func (d *daemon) remindOfWaits(ctx context.Context, now time.Time, waits map[string]OperatorWaitState) {
	seams := d.deps.WaitReminders
	away, err := seams.Away()
	if err != nil {
		d.waitFailures.Fail("|presence", "dobbyengine: the presence is unreadable; no wait reminder", "error", err)
		return
	}
	d.waitFailures.Recover("|presence", "dobbyengine: the presence reads again")
	if !away {
		clear(d.lastReminders)
		return
	}
	if len(waits) == 0 {
		return
	}
	interval, err := seams.Interval()
	if err != nil {
		d.waitFailures.Fail("|interval", "dobbyengine: the reminder interval is unreadable; no wait reminder", "error", err)
		return
	}
	d.waitFailures.Recover("|interval", "dobbyengine: the reminder interval reads again")
	for _, slug := range slices.Sorted(maps.Keys(waits)) {
		wait := waits[slug]
		waited := now.Sub(wait.Since)
		last, reminded := d.lastReminders[slug]
		if waited < interval || (reminded && now.Sub(last) < interval) {
			continue
		}
		line := fmt.Sprintf("%s: waiting at %s for %d minutes", slug, wait.Row, int(waited.Minutes()))
		if err := seams.Post(ctx, line); err != nil {
			logger.Warn("dobbyengine: a wait reminder was not posted; the next check tries again", "slug", slug, "row", wait.Row, "error", err)
			continue
		}
		if d.lastReminders == nil {
			d.lastReminders = map[string]time.Time{}
		}
		d.lastReminders[slug] = now
	}
}

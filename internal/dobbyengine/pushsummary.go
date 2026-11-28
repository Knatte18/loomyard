// pushsummary.go posts the prime's one summary of what stayed owed to origin past the threshold.
// It also posts one all-clear once everything is pushed.

package dobbyengine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/pushledger"
)

// PushSummaryDeps are the reads of the push summary; a nil Ledger posts nothing.
type PushSummaryDeps struct {
	// Ledger lists the hub's push ledger entries.
	Ledger func() (map[pushledger.Key]pushledger.Entry, error)
	// Refused returns the way forward of the refused push standing on the target of key, and whether one stands.
	// A nil Refused reads no target as refused.
	Refused func(key pushledger.Key, entry pushledger.Entry) (wayForward string, refused bool, err error)
	// Waits lists every run of the hub whose driver waits on the network, with Slug, WaitProducer and WaitSince filled.
	// A nil Waits lists none.
	Waits func() ([]RunView, error)
	// BeamCatchUpWaits lists the catch-up from origin each live beam dobby of the hub waits on, keyed by beam slug.
	// A nil BeamCatchUpWaits lists none.
	BeamCatchUpWaits func() (map[string]CatchUpWait, error)
	// Threshold reads how long something stays owed before the summary reports it.
	Threshold func() (time.Duration, error)
}

// pushSummary posts one summary once a push or a network wait has stayed owed for the threshold.
// A catch-up waiting on origin, the dobby's own or a beam dobby's, counts as a network wait.
// It posts one all-clear once nothing is owed after such a summary.
// A failed read or post is logged once per episode and tried again on the next tick.
func (d *daemon) pushSummary(ctx context.Context, now time.Time) {
	if d.deps.PushSummary.Ledger == nil || d.deps.Notice == nil {
		return
	}
	if err := d.summarisePushes(ctx, now); err != nil {
		d.pushSummaryFailures.Fail("dobbyengine: the push summary failed and is tried again", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	d.pushSummaryFailures.Recover("dobbyengine: the push summary succeeds again", "worktree", d.geom.WorktreeName)
}

// summarisePushes reads what is owed, keeps the owed period in the state record and posts the line the period calls for.
func (d *daemon) summarisePushes(ctx context.Context, now time.Time) error {
	seams := d.deps.PushSummary
	entries, err := seams.Ledger()
	if err != nil {
		return err
	}
	var waits []RunView
	if seams.Waits != nil {
		if waits, err = seams.Waits(); err != nil {
			return err
		}
	}
	var beamCatchUps map[string]CatchUpWait
	if seams.BeamCatchUpWaits != nil {
		if beamCatchUps, err = seams.BeamCatchUpWaits(); err != nil {
			return err
		}
	}

	var since time.Time
	earliest := func(at time.Time) {
		if !at.IsZero() && (since.IsZero() || at.Before(since)) {
			since = at
		}
	}
	var owed, held []string
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		entry := entries[key]
		switch {
		case entry.Owed():
			first := owedSince(entry)
			earliest(first)
			line, err := d.owedLine(key, entry, first, now)
			if err != nil {
				return err
			}
			owed = append(owed, line)
		case entry.HeldRecords != "" || len(entry.Dropped) > 0 || len(entry.HandOvers) > 0:
			held = append(held, targetName(key, entry))
		}
	}
	record, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		return err
	}
	var waiting []string
	for _, wait := range waits {
		earliest(wait.WaitSince)
		waiting = append(waiting, fmt.Sprintf("%s at %s for %dm", wait.Slug, wait.WaitProducer, minutesSince(wait.WaitSince, now)))
	}
	addCatchUp := func(whose string, wait CatchUpWait) {
		if wait.Reason != CatchUpWaitOrigin {
			return
		}
		began := wait.waitingSince()
		earliest(began)
		waiting = append(waiting, fmt.Sprintf("%s catch-up from origin for %dm", whose, minutesSince(began, now)))
	}
	if record.CatchUp != nil {
		addCatchUp("the dobby's", *record.CatchUp)
	}
	for _, beam := range slices.Sorted(maps.Keys(beamCatchUps)) {
		addCatchUp(beam+"'s", beamCatchUps[beam])
	}
	period := record.PushPeriod
	if len(owed) == 0 && len(waiting) == 0 {
		if period == nil {
			return nil
		}
		if period.Posted {
			if err := d.deps.Notice(ctx, d.geom.WorktreeName+": every branch is pushed to origin and nothing waits on the network"); err != nil {
				return err
			}
		}
		return d.recordPushPeriod(nil)
	}

	if since.IsZero() {
		since = now
	}
	if period == nil || since.Before(period.Since) {
		period = &PushPeriod{Since: since, Posted: period != nil && period.Posted}
		if err := d.recordPushPeriod(period); err != nil {
			return err
		}
	}
	if period.Posted {
		return nil
	}
	threshold, err := seams.Threshold()
	if err != nil {
		return err
	}
	if now.Sub(period.Since) < threshold {
		return nil
	}
	var parts []string
	if len(owed) > 0 {
		parts = append(parts, "not yet on origin: "+strings.Join(owed, "; "))
	}
	if len(waiting) > 0 {
		parts = append(parts, "waiting on the network: "+strings.Join(waiting, "; "))
	}
	if len(held) > 0 {
		parts = append(parts, "held, until its pair goes or its commit leaves the parent: "+strings.Join(held, "; "))
	}
	parts = append(parts, "lym retries by itself, so only a refused push needs a step")
	if err := d.deps.Notice(ctx, d.geom.WorktreeName+": "+strings.Join(parts, ". ")); err != nil {
		return err
	}
	return d.recordPushPeriod(&PushPeriod{Since: period.Since, Posted: true})
}

// recordPushPeriod writes the owed period into the state record, nil ending it.
func (d *daemon) recordPushPeriod(period *PushPeriod) error {
	return UpdateState(d.geom.StatePath, func(s *State) error {
		s.PushPeriod = period
		return nil
	})
}

// owedLine names one owed target, how long it has been owed and, when its push is refused, the way forward.
func (d *daemon) owedLine(key pushledger.Key, entry pushledger.Entry, first, now time.Time) (string, error) {
	line := fmt.Sprintf("%s, owed %dm", targetName(key, entry), minutesSince(first, now))
	if len(entry.Landings) > 0 && entry.Worktree != "" {
		_, err := os.Stat(entry.Worktree)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return line + ", its worktree " + entry.Worktree + " is gone", nil
		case err != nil:
			return "", err
		}
	}
	if d.deps.PushSummary.Refused == nil {
		return line, nil
	}
	wayForward, refused, err := d.deps.PushSummary.Refused(key, entry)
	if err != nil || !refused {
		return line, err
	}
	return line + ", refused (way forward: " + wayForward + ")", nil
}

// owedSince returns when the entry's push became owed, from its oldest landing when it records no first-owed time.
func owedSince(entry pushledger.Entry) time.Time {
	if !entry.FirstOwed.IsZero() || len(entry.Landings) == 0 {
		return entry.FirstOwed
	}
	return entry.Landings[0].At
}

// targetName names the branches of one push target as the summary shows them.
func targetName(key pushledger.Key, entry pushledger.Entry) string {
	switch {
	case key == pushledger.BoardKey:
		return "the board"
	case key == pushledger.LandingKey:
		return "the prime's code branch"
	case entry.Worktree == "":
		return string(key)
	default:
		return "the branches of " + filepath.Base(entry.Worktree)
	}
}

// minutesSince returns the whole minutes from at to now, zero for an unknown at.
func minutesSince(at, now time.Time) int {
	if at.IsZero() {
		return 0
	}
	return int(now.Sub(at) / time.Minute)
}

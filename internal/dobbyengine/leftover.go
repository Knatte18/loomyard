// leftover.go tears down the pairs left on disk that no run's own teardown takes, and reports the pairs a cancel kept.
// It picks those pairs and starts their teardown through Deps.TearDownLeftovers.
// It reads how the teardown ended, with a wait per pair.

package dobbyengine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/runfsm"
)

// leftoverTeardownCeiling is the longest wait between two teardown attempts of one leftover pair.
const leftoverTeardownCeiling = time.Hour

// errTeardownUnreported is the reason given for a slug the teardown's result left out.
var errTeardownUnreported = errors.New("the teardown ended without reporting on the pair")

// LeftoverKind names why a pair is left over, which decides how the hub wiring tears it down.
type LeftoverKind string

// The kinds of leftover pair.
const (
	// LeftoverEnded is the pair of a run whose board entry ended while batten left the run short of its own teardown.
	LeftoverEnded LeftoverKind = "ended"
	// LeftoverLanded is the pair of a run in the FSM state landed.
	LeftoverLanded LeftoverKind = "landed"
	// LeftoverCancelled is the pair of a run in the FSM state cancelled whose cancel did not keep it.
	// The run's batten record goes with the pair.
	LeftoverCancelled LeftoverKind = "cancelled"
	// LeftoverHandedOver is the local pair of a run this machine handed over and another machine owns.
	// It is removed in local mode, which takes no origin step.
	LeftoverHandedOver LeftoverKind = "handed_over"
	// LeftoverOrphan is a pair that neither a board entry nor a batten record names, idle past the orphan grace.
	LeftoverOrphan LeftoverKind = "orphan"
)

// LeftoverPair is one pair the dobby asks the hub wiring to tear down.
type LeftoverPair struct {
	Slug string
	Kind LeftoverKind
}

// leftoverTeardown is a teardown dobby started and has not read yet: the pairs it covers and its one result.
type leftoverTeardown struct {
	pairs  []LeftoverPair
	result <-chan map[string]error
}

// leftoverRetry holds when a leftover pair's teardown may start next.
// It also holds the wait that set that time and whether the failure is logged.
type leftoverRetry struct {
	next     time.Time
	interval time.Duration
	failing  bool
}

// leftoverKind reports whether run's pair is left over, and its kind.
// No pair is left over while it is absent, a pass of it is in flight, its driver lock is held or a cancel is under way.
// The local pair of a run this machine handed over is left over while another machine owns the run.
// That holds whatever pause or hold covers it, since its removal takes no origin step.
// Every other kind needs no pause, no Bolt hold and no other machine owning the run.
// A landed run's pair is left over, and so is a cancelled run's whose cancel did not keep it.
// So is the pair of a run whose board entry ended while dobby gives it no pass:
// batten's state is neither running, nor paused with its work on its parent, nor done.
// A paused run whose work reached its parent is not, since its Land is unfinished.
func leftoverKind(run RunView, rs RunState, inFlight bool) (LeftoverKind, bool) {
	if !run.PairPresent || inFlight || run.DriverLockHeld || run.CancelMarker {
		return "", false
	}
	if run.HandOverMarked && run.OwnedElsewhere {
		return LeftoverHandedOver, true
	}
	if run.Covered || run.BoltHeld || run.OwnedElsewhere || rs.OwnedElsewhere {
		return "", false
	}
	pausedWithWorkOnParent := run.State == runStatePaused && run.ReachedParent
	switch {
	case run.Run == runfsm.Landed:
		return LeftoverLanded, true
	case run.Run == runfsm.Cancelled && !run.PairKept:
		return LeftoverCancelled, true
	case run.Run == runfsm.Cancelled:
		return "", false
	case run.EntryEnded && !drivable(run) && !pausedWithWorkOnParent && run.State != runStateDone:
		return LeftoverEnded, true
	}
	return "", false
}

// keptPair reports whether run is a cancelled run whose cancel kept its pair, and that pair still stands.
func keptPair(run RunView) bool {
	return run.Run == runfsm.Cancelled && run.PairKept && run.PairPresent
}

// tearDownLeftovers reads the teardown in flight once it ended.
// It then starts one over the leftover pairs whose wait ran out.
// The leftover pairs are those of the listed runs, done runs included, and the due orphan pairs.
// A removed pair logs `teardown <slug>: pair removed`.
// A pair whose teardown failed logs `teardown <slug> failing: <reason>` at the first failure of an episode.
// It then waits one probe interval, and twice its last wait after each further failure, up to leftoverTeardownCeiling.
// A removed pair waits one probe interval too, since the listing of the tick that read the result still shows it.
// A pair no longer left over forgets its wait.
// A teardown in flight keeps the loop out of its idle backoff; a nil Deps.TearDownLeftovers starts none.
func (d *daemon) tearDownLeftovers(ctx context.Context, now time.Time) error {
	if d.deps.TearDownLeftovers == nil {
		return nil
	}
	if d.teardown != nil {
		select {
		case results := <-d.teardown.result:
			d.readLeftoverTeardown(now, d.teardown.pairs, results)
			d.teardown = nil
		default:
			d.busy = true
			return nil
		}
	}
	current, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		return err
	}
	var leftover []LeftoverPair
	for _, run := range d.listedRuns {
		_, inFlight := d.flights[run.Slug]
		if kind, ok := leftoverKind(run, current.Runs[run.Slug], inFlight); ok {
			leftover = append(leftover, LeftoverPair{Slug: run.Slug, Kind: kind})
		}
	}
	for _, slug := range d.dueOrphans(ctx, now) {
		leftover = append(leftover, LeftoverPair{Slug: slug, Kind: LeftoverOrphan})
	}

	if d.leftoverRetries == nil {
		d.leftoverRetries = map[string]leftoverRetry{}
	}
	standing := make(map[string]bool, len(leftover))
	var due []LeftoverPair
	for _, pair := range leftover {
		standing[pair.Slug] = true
		if !now.Before(d.leftoverRetries[pair.Slug].next) {
			due = append(due, pair)
		}
	}
	for slug := range d.leftoverRetries {
		if !standing[slug] {
			delete(d.leftoverRetries, slug)
		}
	}
	if len(due) == 0 {
		return nil
	}
	d.teardown = &leftoverTeardown{pairs: due, result: d.deps.TearDownLeftovers(due)}
	d.busy = true
	return nil
}

// readLeftoverTeardown logs each pair of a teardown that ended by its entry in results and sets the pair's next wait.
// A removed orphan pair leaves the first-sight record.
// A slug results leaves out, as a closed channel's nil map does, failed with errTeardownUnreported.
func (d *daemon) readLeftoverTeardown(now time.Time, pairs []LeftoverPair, results map[string]error) {
	if d.leftoverRetries == nil {
		d.leftoverRetries = map[string]leftoverRetry{}
	}
	for _, pair := range pairs {
		reason, reported := results[pair.Slug]
		if !reported {
			reason = errTeardownUnreported
		}
		if reason == nil {
			d.logLine(now, "teardown "+pair.Slug+": pair removed")
			d.leftoverRetries[pair.Slug] = leftoverRetry{next: now.Add(d.probe)}
			if pair.Kind == LeftoverOrphan {
				d.dropOrphanSight(pair.Slug)
			}
			continue
		}
		retry := d.leftoverRetries[pair.Slug]
		if !retry.failing {
			d.logLine(now, "teardown "+pair.Slug+" failing: "+oneLine(reason.Error()))
		}
		interval := d.probe
		if retry.interval > 0 {
			interval = min(retry.interval*2, leftoverTeardownCeiling)
		}
		d.leftoverRetries[pair.Slug] = leftoverRetry{next: now.Add(interval), interval: interval, failing: true}
	}
}

// markKeptCancels runs once per dobby process.
// It gives each run cancelled with keep before cancel intents existed its finished keep cancel intent.
// Each marked run logs one line.
// A failure is logged once per episode and tried again at the next tick; a nil Deps.MarkKeptCancels marks nothing.
func (d *daemon) markKeptCancels(now time.Time) {
	if d.keptMarked {
		return
	}
	if d.deps.MarkKeptCancels == nil {
		d.keptMarked = true
		return
	}
	slugs, err := d.deps.MarkKeptCancels()
	if err != nil {
		if !d.keptMarkFailing {
			d.keptMarkFailing = true
			d.logLine(now, "kept cancel marking failing: "+oneLine(err.Error()))
		}
		return
	}
	d.keptMarked = true
	d.keptMarkFailing = false
	for _, slug := range slugs {
		d.logLine(now, "run "+slug+": a cancel that kept the pair before the upgrade now records its finished keep cancel intent")
	}
}

// noticeKeptPairs posts one notice per fresh start listing the kept pairs of the listed runs.
// The notice names the verb that removes one.
// It waits for the kept cancels of this process to be marked, so a pre-upgrade kept pair is in the list.
// A start with no kept pair posts nothing.
func (d *daemon) noticeKeptPairs(ctx context.Context) {
	if !d.keptNoticeOwed || !d.keptMarked {
		return
	}
	d.keptNoticeOwed = false
	var items []string
	for _, run := range d.listedRuns {
		if !keptPair(run) {
			continue
		}
		item := run.Slug
		if !run.KeptSince.IsZero() {
			item += " (kept since " + run.KeptSince.UTC().Format(time.RFC3339) + ")"
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return
	}
	d.post(ctx, fmt.Sprintf("%s: kept pairs stand: %s; way forward: `lym fabric remove <slug>` removes each pair no longer needed",
		d.geom.WorktreeName, strings.Join(items, ", ")))
}

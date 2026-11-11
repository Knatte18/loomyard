// ended.go tears down the pair of a run whose board entry ended while batten left the run short of its own teardown.
// It picks those runs, starts their teardown through Deps.TearDownEnded, and reads how it ended, with a wait per run.

package dobbyengine

import (
	"errors"
	"time"
)

// endedTeardownCeiling is the longest wait between two teardown attempts of one stranded run.
const endedTeardownCeiling = time.Hour

// errTeardownUnreported is the reason given for a slug the teardown's result left out.
var errTeardownUnreported = errors.New("the teardown ended without reporting on the run")

// endedTeardown is a teardown dobby started and has not read yet: the slugs it covers and its one result.
type endedTeardown struct {
	slugs  []string
	result <-chan map[string]error
}

// endedRetry is when a stranded run's teardown may start next, the wait that set it, and whether its failure is logged.
type endedRetry struct {
	next     time.Time
	interval time.Duration
	failing  bool
}

// strandedEnded reports whether batten left run short of its own teardown although its board entry ended.
// Its entry is done or gone, its pair is still on disk, and dobby gives it no pass:
// batten's state is neither running, nor paused with its work on its parent, nor done.
// A run with a pass in flight, a held driver lock or a cancel marker is not, since its driver or the cancel ends it.
// Nor is a run a pause covers, or one recorded owned elsewhere, whose pair is not this dobby's to remove.
func strandedEnded(run RunView, rs RunState, inFlight bool) bool {
	return run.EntryEnded && run.PairPresent && !drivable(run) && run.State != runStateDone &&
		!inFlight && !run.DriverLockHeld && !run.CancelMarker && !run.Covered && !rs.OwnedElsewhere
}

// tearDownEnded reads the teardown in flight once it ended, then starts one over the stranded runs whose wait ran out.
// A run whose pair was removed logs `teardown <slug>: pair removed`.
// A run whose teardown failed logs `teardown <slug> failing: <reason>` at the first failure of an episode.
// It then waits one probe interval, and twice its last wait after each further failure, up to endedTeardownCeiling.
// A removed run waits one probe interval too, since the listing of the tick that read the result still shows its pair.
// A run no longer stranded forgets its wait.
// A teardown in flight keeps the loop out of its idle backoff; a nil Deps.TearDownEnded starts none.
func (d *daemon) tearDownEnded(now time.Time) error {
	if d.deps.TearDownEnded == nil {
		return nil
	}
	if d.teardown != nil {
		select {
		case results := <-d.teardown.result:
			d.readEndedTeardown(now, d.teardown.slugs, results)
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
	if d.endedRetries == nil {
		d.endedRetries = map[string]endedRetry{}
	}
	stranded := map[string]bool{}
	var due []string
	for _, run := range d.runs {
		_, inFlight := d.flights[run.Slug]
		if !strandedEnded(run, current.Runs[run.Slug], inFlight) {
			continue
		}
		stranded[run.Slug] = true
		if !now.Before(d.endedRetries[run.Slug].next) {
			due = append(due, run.Slug)
		}
	}
	for slug := range d.endedRetries {
		if !stranded[slug] {
			delete(d.endedRetries, slug)
		}
	}
	if len(due) == 0 {
		return nil
	}
	d.teardown = &endedTeardown{slugs: due, result: d.deps.TearDownEnded(due)}
	d.busy = true
	return nil
}

// readEndedTeardown logs each slug of a teardown that ended by its entry in results and sets the slug's next wait.
// A slug results leaves out, as a closed channel's nil map does, failed with errTeardownUnreported.
func (d *daemon) readEndedTeardown(now time.Time, slugs []string, results map[string]error) {
	if d.endedRetries == nil {
		d.endedRetries = map[string]endedRetry{}
	}
	for _, slug := range slugs {
		reason, reported := results[slug]
		if !reported {
			reason = errTeardownUnreported
		}
		if reason == nil {
			d.logLine(now, "teardown "+slug+": pair removed")
			d.endedRetries[slug] = endedRetry{next: now.Add(d.probe)}
			continue
		}
		retry := d.endedRetries[slug]
		if !retry.failing {
			d.logLine(now, "teardown "+slug+" failing: "+oneLine(reason.Error()))
		}
		interval := d.probe
		if retry.interval > 0 {
			interval = min(retry.interval*2, endedTeardownCeiling)
		}
		d.endedRetries[slug] = endedRetry{next: now.Add(interval), interval: interval, failing: true}
	}
}

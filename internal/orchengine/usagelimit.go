// usagelimit.go holds the orch's creel deliveries while the provider's usage limit stops its session, and wakes it once the limit should have reset.

package orchengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const (
	// usageLimitWakeDelay is how long after a limit's stated reset time the wake line goes, so the limit has lifted when the orch acts on it.
	usageLimitWakeDelay = time.Minute
	// usageLimitRetry is how long the watcher waits for an answer before it types the wake line again, and the first wait when the limit states no reset time.
	usageLimitRetry = 30 * time.Minute
)

// recordUsageLimit records in st the usage-limit reading of the orch's newest turn end, read at now, and reports whether it ended a limit st recorded.
// A limited reading keeps the first sight's UsageLimitAt and sets the reset time and the next wake: one usageLimitWakeDelay past the reset time, at once when that has already passed, and usageLimitRetry from now when the limit states no reset time.
// A reading with no limit clears the record.
func recordUsageLimit(st *State, limit shuttleengine.UsageLimit, now time.Time) (ended bool) {
	if !limit.Limited {
		if st.UsageLimitAt.IsZero() {
			return false
		}
		logger.Info("orch: the orch answered after a usage limit; its deliveries flow again", "strandGUID", st.Strand, "limitSince", st.UsageLimitAt)
		st.UsageLimitAt, st.UsageLimitResetsAt, st.UsageLimitWakeAt = time.Time{}, time.Time{}, time.Time{}
		return true
	}
	if st.UsageLimitAt.IsZero() {
		st.UsageLimitAt = now
		logger.Info("orch: the orch stopped on a usage limit; holding its deliveries until it resets", "strandGUID", st.Strand, "resetsAt", limit.ResetsAt)
	}
	st.UsageLimitResetsAt = limit.ResetsAt
	switch wake := limit.ResetsAt.Add(usageLimitWakeDelay); {
	case limit.ResetsAt.IsZero():
		st.UsageLimitWakeAt = now.Add(usageLimitRetry)
	case wake.After(now):
		st.UsageLimitWakeAt = wake
	default:
		st.UsageLimitWakeAt = now
	}
	return false
}

// tickUsageLimit is the idle tick while a usage limit stands: it renews the creel hold on the orch strand, so no mail reaches a session that cannot act on it, and starts no cycle.
// Once UsageLimitWakeAt has passed it queues the wake line, the watcher's own delivery, which the hold lets through, and moves the next wake usageLimitRetry on.
// A wake line that fails to render is warned about and skipped until the next wake; a creel failure goes through creelFailed and leaves the wake due, so the next tick tries again.
func (w *Watcher) tickUsageLimit(st State, now time.Time) error {
	held, err := w.renewHold(&st)
	if err != nil || !held {
		return err
	}
	if now.Before(st.UsageLimitWakeAt) {
		return nil
	}
	line, err := RenderUsageLimitWake(w.stencilsDir, st.UsageLimitResetsAt)
	if err != nil {
		st.UsageLimitWakeAt = now.Add(usageLimitRetry)
		logger.Warn("orch: usage-limit wake line not rendered; retrying at the next wake", "strandGUID", st.Strand, "nextWakeAt", st.UsageLimitWakeAt, "error", err)
		return w.save(st)
	}
	name, err := w.orchName(st)
	if err != nil {
		return w.creelFailed(&st, err)
	}
	if _, err := w.creel.Inject(name, CreelStep{Text: line}, stepInjectTTL); err != nil {
		return w.creelFailed(&st, err)
	}
	st.UsageLimitWakeAt = now.Add(usageLimitRetry)
	logger.Info("orch: usage-limit wake line queued", "strandGUID", st.Strand, "resetsAt", st.UsageLimitResetsAt, "nextWakeAt", st.UsageLimitWakeAt)
	return w.save(st)
}

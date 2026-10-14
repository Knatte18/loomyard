// stophold.go decides how long the state-changed notice of an arbiter-eligible stop waits for the arbiter to hand on.

package battenshed

import (
	"context"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// failArbiterStop is the seam-failure key of the arbiter-stop reading.
const failArbiterStop = "battenshed: arbiter stop unreadable; holding nothing"

// eligibleStopHold reports whether the state-changed notice of the child's halted stop is held for the arbiter, and when the hold ended otherwise.
// The hold covers only a blocked or awaiting status, so error stops keep the marker hold alone.
// It records arbiterOffAt at the first eligible reading with the arbiter off.
// A seam error warns once per episode through seamFailed, holds nothing and ends at the episode's since; a later good read recovers it.
func (p *innerRunProducer) eligibleStopHold(ctx context.Context, w *childWait, now, parkAt time.Time, parkFound bool) (holding bool, end time.Time) {
	since := p.episode.since
	if w.status.State != shedengine.StateBlocked && w.status.State != shedengine.StateAwaiting {
		return false, since
	}
	reading, err := p.deps.ArbiterStop(ctx, w.status)
	if err != nil {
		p.seamFailed(failArbiterStop, err)
		return false, since
	}
	p.seamRecovered(failArbiterStop)
	if reading.Eligible && !reading.On && p.episode.arbiterOffAt.IsZero() {
		p.episode.arbiterOffAt = now
	}
	return stopHold(reading, since, now, parkAt, parkFound, p.episode.arbiterOffAt)
}

// stopHold decides whether the notice of a stop whose episode began at since is held at now, and the time the hold ended when it is not.
// It decides in this order:
// a stop that is not eligible is not held, ending at since;
// an applied mark ends the hold at the later of its time and since;
// an arbiter that is off ends it at the later of offSeen and since;
// a stop with no spawns left and no attempt at its key never starts it, ending at since;
// a park marker at or after since ends it at the marker's time, unless a hand-over settle record at or after since marks the Go driver's own hand-over;
// an attempt's hand-on at or after since ends it at that time;
// the backstop, the later of since plus the timeout plus noticeReportWait and, with an attempt, its deadline plus noticeReportWait, ends it once now reaches it.
// Every end time comes from a record or the backstop, and none is before since.
func stopHold(reading ChildArbiterStop, since, now, parkAt time.Time, parkFound bool, offSeen time.Time) (holding bool, end time.Time) {
	if !reading.Eligible {
		return false, since
	}
	if !reading.AppliedAt.IsZero() {
		return false, laterOf(reading.AppliedAt, since)
	}
	if !reading.On {
		return false, laterOf(offSeen, since)
	}
	if !reading.SpawnsLeft && !reading.Attempt {
		return false, since
	}
	if parkFound && !parkAt.Before(since) {
		handedOver := !reading.HandOverAt.IsZero() && !reading.HandOverAt.Before(since)
		if !handedOver {
			return false, parkAt
		}
	}
	if reading.Attempt && !reading.HandOnAt.IsZero() && !reading.HandOnAt.Before(since) {
		return false, reading.HandOnAt
	}
	backstop := since.Add(reading.Timeout + noticeReportWait)
	if reading.Attempt {
		backstop = laterOf(backstop, reading.AttemptDeadline.Add(noticeReportWait))
	}
	if !now.Before(backstop) {
		return false, backstop
	}
	return true, time.Time{}
}

// laterOf returns the later of a and b.
func laterOf(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

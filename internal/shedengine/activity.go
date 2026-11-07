// activity.go implements composeActivity, which fills Activity from data Shed already holds on every persist.

package shedengine

import "fmt"

// composeActivity fills Activity mechanically from data Shed already holds: currentProducer, history, st, and errText.
// Now is currentProducer verbatim.
// Last is the empty string when history is empty.
// Otherwise Last is the latest entry, exactly "<producer> → <outcome>".
// When routedTo is non-empty, the latest entry reads exactly "<producer> → bounced to <routedTo>" instead.
// Wait is errText when st is StateBlocked, StateFailed or StateAwaiting, and the empty string for every other state.
// A latest entry that is a reroute composes as exactly "<producer> → rerouted to <target>", whatever routedTo is.
// A latest entry that is a skipped entry composes as exactly "<producer> → skipped", whatever routedTo is.
//
// routedTo is set by the caller that routes a Stuck verdict to another row, or back to the same row.
// It is passed in because the activity line is composed in the same persist that records the bounce.
//
// The Last format is exact because a test asserts it.
func composeActivity(currentProducer string, history []HistoryEntry, st State, errText string, routedTo string) Activity {
	last := ""
	if len(history) > 0 {
		latest := history[len(history)-1]
		switch {
		case latest.Outcome == OutcomeReroute:
			last = fmt.Sprintf("%s → rerouted to %s", latest.Producer, latest.RoutedTo)
		case latest.Outcome == OutcomeSkipped:
			last = fmt.Sprintf("%s → skipped", latest.Producer)
		case routedTo != "":
			last = fmt.Sprintf("%s → bounced to %s", latest.Producer, routedTo)
		default:
			last = fmt.Sprintf("%s → %s", latest.Producer, latest.Outcome)
		}
	}

	wait := ""
	if st == StateBlocked || st == StateFailed || st == StateAwaiting {
		wait = errText
	}

	return Activity{
		Now:  currentProducer,
		Last: last,
		Wait: wait,
	}
}

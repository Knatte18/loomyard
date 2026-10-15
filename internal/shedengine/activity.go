// activity.go implements composeActivity, the single mechanical function that fills Activity from
// data Shed already holds on every persist.

package shedengine

import "fmt"

// composeActivity fills Activity mechanically from data Shed already holds: currentProducer,
// history, st, and errText.
// Now is currentProducer verbatim. Last is the empty string when history is empty, and otherwise
// the most recent entry composed as exactly "<producer> → <outcome>", or, when routedTo is
// non-empty, as exactly "<producer> → bounced to <routedTo>". Wait is errText when st is
// StateBlocked, StateFailed or StateAwaiting, and the empty string for every other state.
// A latest entry that is a reroute entry composes as exactly "<producer> → rerouted to <target>", whatever routedTo is.
//
// routedTo is told by the one call site that routes a Stuck verdict to another row (or back to
// the same row), because the activity line is composed in the same persist that records the bounce.
// The Stuck history entry itself also records the target in its routed_to field, for readers such as the checklist.
//
// The Last format is pinned to an exact string rather than left to judgment because a test
// asserts this field; an unpinned "formatted for a human" cannot be asserted, only approximated.
func composeActivity(currentProducer string, history []HistoryEntry, st State, errText string, routedTo string) Activity {
	last := ""
	if len(history) > 0 {
		latest := history[len(history)-1]
		switch {
		case latest.Outcome == OutcomeReroute:
			last = fmt.Sprintf("%s → rerouted to %s", latest.Producer, latest.RoutedTo)
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

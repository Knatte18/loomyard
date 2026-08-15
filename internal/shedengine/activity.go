// activity.go implements composeActivity, the single mechanical function that fills Activity from
// data Shed already holds on every persist.

package shedengine

import "fmt"

// composeActivity fills Activity mechanically from data Shed already holds: currentProducer,
// history, st, and errText.
// Now is currentProducer verbatim. Last is the empty string when history is empty, and otherwise
// the most recent entry composed as exactly "<producer> → <outcome>". Wait is errText when st is
// StateBlocked or StateFailed, and the empty string for every other state.
//
// The Last format is pinned to an exact string rather than left to judgment because a test
// asserts this field; an unpinned "formatted for a human" cannot be asserted, only approximated.
func composeActivity(currentProducer string, history []HistoryEntry, st State, errText string) Activity {
	last := ""
	if len(history) > 0 {
		latest := history[len(history)-1]
		last = fmt.Sprintf("%s → %s", latest.Producer, latest.Outcome)
	}

	wait := ""
	if st == StateBlocked || st == StateFailed {
		wait = errText
	}

	return Activity{
		Now:  currentProducer,
		Last: last,
		Wait: wait,
	}
}

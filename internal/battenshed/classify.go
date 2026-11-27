// classify.go reads batten's recorded child state changes from its status history.
// It classifies them as batten's notice rules would have at the time, working only on recorded values.

package battenshed

import (
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// runShedRowName is the batten row whose budget-exempt history entries record the child's state changes.
const runShedRowName = "Run-Shed"

// stateChangeSeparator joins the two states of a recorded state change.
const stateChangeSeparator = " → "

// The kinds of Wake and AutomaticStep a classification reports.
// A Wake of any other kind carries the child's new state as its kind.
const (
	// WakeKindReviewEscalation is a halt at a review row whose segment holds an escalation file.
	WakeKindReviewEscalation = "review-escalation"
	// WakeKindPRGate is a halt at the gate row.
	WakeKindPRGate = "pr-gate"
	// WakeKindFailed is a wake for a failed state or a failure episode of a batten row.
	WakeKindFailed = "failed"
	// AutomaticKindVerifyRetry is batten's own retry of a first verify failure.
	AutomaticKindVerifyRetry = "verify-retry"
	// AutomaticKindParentMovedResume is batten's resume of a child awaiting at the gate for a moved parent.
	AutomaticKindParentMovedResume = "parent-moved-resume"
)

// StateChange is one recorded change of the child's state.
// Producer is empty when the recorded text names none; At is zero for a change read straight from text.
type StateChange struct {
	From     shedengine.State
	To       shedengine.State
	Producer string
	At       time.Time
}

// NoticeRules carries the row names and escalation counts that the notice rules classify halts by.
// The caller supplies them, so the classifier hardcodes no row.
type NoticeRules struct {
	// VerifyRows are the rows whose first verify failure batten retries.
	VerifyRows []string
	// GateRow is the row the gate halts at.
	GateRow string
	// EpisodeRows are batten's own rows whose repeated failures form one episode.
	EpisodeRows []string
	// Escalations maps a review row to the number of its segment's rounds that hold an escalation file.
	Escalations map[string]int
}

// BattenRecord holds one run's batten status snapshots and the child's loom history.
type BattenRecord struct {
	// Snapshots is batten's status decoded at each records commit in the run's span that holds it, oldest first.
	Snapshots []shedengine.Status
	// Child is the child's loom history at its archive tag.
	Child []shedengine.HistoryEntry
}

// Wake is one halt that woke the orch.
type Wake struct {
	Kind     string
	Producer string
	At       time.Time
}

// AutomaticStep is one halt batten resolved on its own.
type AutomaticStep struct {
	Kind     string
	Producer string
	At       time.Time
}

// Classification holds the state changes, wakes and automatic steps read from one run's records.
// AccountedChild holds the indexes of Child entries an automatic step accounts for.
type Classification struct {
	Changes        []StateChange
	Wakes          []Wake
	Automatic      []AutomaticStep
	Paused         []StateChange
	AccountedChild []int
}

// ParseStateChange reads the output of a state-change history entry:
// `child <from> → <to>` with an optional ` at <producer>` suffix.
// The bool result is false unless both states are legal.
func ParseStateChange(output string) (StateChange, bool) {
	rest, ok := strings.CutPrefix(output, "child ")
	if !ok {
		return StateChange{}, false
	}
	from, rest, ok := strings.Cut(rest, stateChangeSeparator)
	if !ok {
		return StateChange{}, false
	}
	to, producer, _ := strings.Cut(rest, " at ")
	if !isLegalState(shedengine.State(from)) || !isLegalState(shedengine.State(to)) {
		return StateChange{}, false
	}
	return StateChange{From: shedengine.State(from), To: shedengine.State(to), Producer: producer}, true
}

// isLegalState reports whether state is one of the legal child states.
func isLegalState(state shedengine.State) bool {
	switch state {
	case shedengine.StateRunning, shedengine.StatePaused, shedengine.StateDone, shedengine.StateBlocked, shedengine.StateFailed, shedengine.StateAwaiting:
		return true
	}
	return false
}

// ClassifyRecords sorts the child's state changes into wakes, automatic steps and pauses.
// The changes come from the last snapshot's history; failure episodes come from the whole snapshot sequence.
// A change into paused is not a wake.
// A verify retry or a moved-parent resume is an automatic step, not a wake.
// Every other halt is one wake.
// A failure episode of an episode row is one wake, however many failures it spans.
// With no snapshots it returns an empty Classification.
func ClassifyRecords(rec BattenRecord, rules NoticeRules) Classification {
	var result Classification
	if len(rec.Snapshots) == 0 {
		return result
	}
	for _, entry := range rec.Snapshots[len(rec.Snapshots)-1].History {
		if entry.Producer != runShedRowName || !entry.BudgetExempt {
			continue
		}
		if change, ok := ParseStateChange(entry.Output); ok {
			change.At = parseEntryTime(entry.At)
			result.Changes = append(result.Changes, change)
		}
	}
	classifyChanges(&result, rec.Child, rules)
	classifyEpisodes(&result, rec.Snapshots, rules.EpisodeRows)
	return result
}

// classifyChanges sorts result.Changes into pauses, automatic steps and wakes.
func classifyChanges(result *Classification, child []shedengine.HistoryEntry, rules NoticeRules) {
	changes := result.Changes
	consumed := make([]bool, len(changes))
	blockedSeen := map[string]bool{}
	escalated := map[string]int{}

	for i, change := range changes {
		switch change.To {
		case shedengine.StatePaused:
			if !consumed[i] {
				result.Paused = append(result.Paused, change)
			}
		case shedengine.StateAwaiting, shedengine.StateBlocked, shedengine.StateFailed:
			row := change.Producer
			if change.To == shedengine.StateBlocked && slices.Contains(rules.VerifyRows, row) {
				first := !blockedSeen[row]
				blockedSeen[row] = true
				if first {
					next := nextHaltIndex(changes, i)
					if index, ok := firstEntryAfter(child, change.At, boundaryAt(changes, next), shedengine.OutcomeGoto, ""); ok {
						result.Automatic = append(result.Automatic, AutomaticStep{Kind: AutomaticKindVerifyRetry, Producer: row, At: change.At})
						result.AccountedChild = append(result.AccountedChild, index)
						for j := i + 1; j < next; j++ {
							consumed[j] = true
						}
						continue
					}
				}
			}
			if change.To == shedengine.StateAwaiting && row == rules.GateRow {
				if index, ok := firstEntryAfter(child, change.At, time.Time{}, "", rules.GateRow); ok && child[index].Outcome == shedengine.OutcomeReroute {
					result.Automatic = append(result.Automatic, AutomaticStep{Kind: AutomaticKindParentMovedResume, Producer: row, At: change.At})
					result.AccountedChild = append(result.AccountedChild, index)
					continue
				}
			}
			kind := string(change.To)
			switch {
			case rules.Escalations[row] > escalated[row]:
				escalated[row]++
				kind = WakeKindReviewEscalation
			case row == rules.GateRow:
				kind = WakeKindPRGate
			}
			result.Wakes = append(result.Wakes, Wake{Kind: kind, Producer: row, At: change.At})
		}
	}
}

// classifyEpisodes adds one wake per failure episode of each episode row.
// A snapshot in state failed at the row opens an episode when none is open.
// The first later snapshot whose history holds more done entries for the row closes it.
func classifyEpisodes(result *Classification, snapshots []shedengine.Status, rows []string) {
	for _, row := range rows {
		open := false
		var openingDone int
		for _, snapshot := range snapshots {
			if open && countDone(snapshot.History, row) > openingDone {
				open = false
			}
			if open || snapshot.State != shedengine.StateFailed || snapshot.CurrentProducer != row {
				continue
			}
			open = true
			openingDone = countDone(snapshot.History, row)
			var at time.Time
			if n := len(snapshot.History); n > 0 {
				at = parseEntryTime(snapshot.History[n-1].At)
			}
			result.Wakes = append(result.Wakes, Wake{Kind: WakeKindFailed, Producer: row, At: at})
		}
	}
}

// countDone counts the done entries of row in history.
func countDone(history []shedengine.HistoryEntry, row string) int {
	count := 0
	for _, entry := range history {
		if entry.Producer == row && entry.Outcome == shedengine.Done {
			count++
		}
	}
	return count
}

// nextHaltIndex returns the index of the first change after index into awaiting, blocked or failed.
// It returns len(changes) when none follows.
func nextHaltIndex(changes []StateChange, index int) int {
	for j := index + 1; j < len(changes); j++ {
		switch changes[j].To {
		case shedengine.StateAwaiting, shedengine.StateBlocked, shedengine.StateFailed:
			return j
		}
	}
	return len(changes)
}

// boundaryAt returns the time of changes[index], or the zero time when index is past the end.
func boundaryAt(changes []StateChange, index int) time.Time {
	if index >= len(changes) {
		return time.Time{}
	}
	return changes[index].At
}

// firstEntryAfter returns the index of the first entry of child dated after after and, when before is non-zero, before it.
// A non-empty outcome or producer narrows the match to that outcome or row.
func firstEntryAfter(child []shedengine.HistoryEntry, after, before time.Time, outcome shedengine.Outcome, producer string) (int, bool) {
	for index, entry := range child {
		if outcome != "" && entry.Outcome != outcome {
			continue
		}
		if producer != "" && entry.Producer != producer {
			continue
		}
		at := parseEntryTime(entry.At)
		if !at.After(after) || (!before.IsZero() && !at.Before(before)) {
			continue
		}
		return index, true
	}
	return 0, false
}

// parseEntryTime reads a history entry's RFC 3339 time, and returns the zero time for text that is not one.
func parseEntryTime(text string) time.Time {
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}
	}
	return at
}

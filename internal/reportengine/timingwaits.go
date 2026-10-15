// timingwaits.go times what a run spent waiting on someone else and the arbiter and alignment attempts that served those waits.
// A wait is an interval of one kind, netted so overlapping waits and machine sleep count once; an attempt is one spawn with the time it ran.

package reportengine

import (
	"fmt"
	"path"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/alignment"
	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// The kinds of wait a run's time is counted under, listed in the order an overlap is credited to them.
const (
	// WaitParentReview is a parent-review round, from its request to its verdict.
	WaitParentReview = "parent_review"
	// WaitPRGate is a child awaiting at the PR gate.
	WaitPRGate = "pr_gate"
	// WaitEscalation is a child or a batten row awaiting at any other row.
	WaitEscalation = "escalation"
	// WaitHalt is a child blocked or failed, or a batten row blocked.
	WaitHalt = "halt"
)

// The kinds of attempt a run's spawns are counted under.
const (
	attemptKindArbiter               = "arbiter"
	attemptKindAlignment             = "alignment"
	attemptKindParentReviewAlignment = "parent_review_alignment"
)

// timingWaitKinds lists every wait kind in crediting order.
var timingWaitKinds = []string{WaitParentReview, WaitPRGate, WaitEscalation, WaitHalt}

// waitInterval is a stretch of a run's time spent waiting, of one kind.
type waitInterval struct {
	kind string
	Interval
}

// AttemptFigure is the count of one kind of attempt and the summed time of those whose time is known, nil when none is.
type AttemptFigure struct {
	Count   int    `json:"count"`
	Seconds *int64 `json:"seconds"`
}

// TimingAttempts holds one figure per kind of attempt: the arbiter's, the PR-Gate alignment agent's and the parent-review rounds' alignment agent's.
// An attempt's time lies inside the wait it serves and is reported beside the waits, never added to them.
type TimingAttempts struct {
	Arbiter               AttemptFigure `json:"arbiter"`
	Alignment             AttemptFigure `json:"alignment"`
	ParentReviewAlignment AttemptFigure `json:"parent_review_alignment"`

	// parts are the attempts behind the figures, kept so buckets can clip each to their window.
	parts []attemptPart
}

// attemptPart is one attempt: its kind, spawn time and, when known, the interval it ran and that interval less machine sleep.
type attemptPart struct {
	kind      string
	spawnedAt time.Time
	known     bool
	gross     Interval
	net       []Interval
}

// latestBattenHistory returns the history of the run's batten record, nil without one.
func latestBattenHistory(row RunRow) []shedengine.HistoryEntry {
	if n := len(row.battenSnapshots); n > 0 {
		return row.battenSnapshots[n-1].History
	}
	return nil
}

// battenRowHalts returns the indexes of the history entries at which a batten row left batten awaiting or blocked.
// Such an entry is at one of batten's own rows, never Run-Shed, and is an awaiting outcome or a stuck outcome no OnStuck edge bounced.
// A stuck entry written before routed_to existed reads as a halt even when an edge bounced it.
func battenRowHalts(history []shedengine.HistoryEntry) []int {
	var halts []int
	for i, entry := range history {
		if entry.Producer == battenrecipe.NameRunShed {
			continue
		}
		if entry.Outcome == shedengine.Awaiting || (entry.Outcome == shedengine.Stuck && entry.RoutedTo == "") {
			halts = append(halts, i)
		}
	}
	return halts
}

// stampAfter returns the stamp of the first row of either history stamped after from, and end when no row is.
func stampAfter(rows []timedRow, from, end time.Time) time.Time {
	for _, candidate := range rows {
		if candidate.stamped && candidate.stamp.After(from) {
			return candidate.stamp
		}
	}
	return end
}

// runWaits returns the run's waits: its parent-review rounds, its child's changes into awaiting, blocked or failed, and its batten-row halts.
// A change's or halt's wait runs from its entry to the next row of either history stamped after it, and a wait with no such row and no verdict runs to end.
func runWaits(row RunRow, rows []timedRow, end time.Time) []waitInterval {
	var waits []waitInterval
	for _, round := range row.parentRounds {
		if round.Request == nil || round.Request.OpenedAt.IsZero() {
			continue
		}
		to := end
		if round.Verdict != nil && !round.Verdict.RecordedAt.IsZero() {
			to = round.Verdict.RecordedAt.UTC().Truncate(time.Second)
		}
		waits = append(waits, waitInterval{kind: WaitParentReview, Interval: Interval{From: round.Request.OpenedAt.UTC().Truncate(time.Second), To: to}})
	}

	for _, change := range row.changes {
		var kind string
		switch change.To {
		case shedengine.StateAwaiting:
			kind = WaitEscalation
			if change.Producer == loomshed.NamePRGate {
				kind = WaitPRGate
			}
		case shedengine.StateBlocked, shedengine.StateFailed:
			kind = WaitHalt
		default:
			continue
		}
		if change.At.IsZero() {
			continue
		}
		at := change.At.Truncate(time.Second)
		waits = append(waits, waitInterval{kind: kind, Interval: Interval{From: at, To: stampAfter(rows, at, end)}})
	}

	history := latestBattenHistory(row)
	for _, index := range battenRowHalts(history) {
		from, ok := entryStamp(history[index])
		if !ok {
			continue
		}
		kind := WaitHalt
		if history[index].Outcome == shedengine.Awaiting {
			kind = WaitEscalation
		}
		waits = append(waits, waitInterval{kind: kind, Interval: Interval{From: from, To: stampAfter(rows, from, end)}})
	}
	return waits
}

// clipWaits returns the parts of waits inside span.
func clipWaits(waits []waitInterval, span Interval) []waitInterval {
	var clipped []waitInterval
	for _, wait := range waits {
		for _, piece := range clipIntervals([]Interval{wait.Interval}, span) {
			clipped = append(clipped, waitInterval{kind: wait.kind, Interval: piece})
		}
	}
	return clipped
}

// netWaits returns each wait kind's time as disjoint intervals, every kind present.
// Machine sleep comes out first, then each kind's union less every earlier kind's, so overlapping waits count once under the kind listed first and a sleep inside a wait counts as sleep alone.
func netWaits(waits []waitInterval, sleeps []Interval) map[string][]Interval {
	net := make(map[string][]Interval, len(timingWaitKinds))
	var earlier []Interval
	for _, kind := range timingWaitKinds {
		var gross []Interval
		for _, wait := range waits {
			if wait.kind == kind {
				gross = append(gross, wait.Interval)
			}
		}
		kept := withoutIntervals(withoutIntervals(unionIntervals(gross), sleeps), earlier)
		net[kind] = kept
		earlier = append(earlier, kept...)
	}
	return net
}

// spawn is one attempt as the records name it: its kind, spawn time and the file its time is reported against.
type spawn struct {
	kind      string
	spawnedAt time.Time
	label     string
	file      string
}

// runAttempts returns the figures of the run's arbiter attempts, its PR-Gate alignment attempts and its parent-review rounds' alignment records, each one spawn.
// An attempt's time runs, net of sleep, from its spawn to the first row of either history stamped after it, or to the span's close in a run in flight.
// In a landed run an attempt spawned outside span, or with no row after it, has its time absent and adds a finding naming its file; its spawn still counts.
func runAttempts(row RunRow, rows []timedRow, span Interval, sleeps []Interval) (TimingAttempts, []Finding) {
	arbiterDir := ""
	if rels, err := shedrun.TreeRunRels(row.Slug); err == nil {
		arbiterDir = rels.Arbiter
	}

	var spawns []spawn
	for _, record := range row.arbiterRecords {
		spawns = append(spawns, spawn{kind: attemptKindArbiter, spawnedAt: record.Attempt.SpawnedAt, label: fmt.Sprintf("arbiter attempt %d", record.Attempt.Number), file: arbiterDir})
	}
	for _, attempt := range row.alignmentAttempts {
		if attempt.Point != alignment.PointPRGate {
			continue
		}
		spawns = append(spawns, spawn{kind: attemptKindAlignment, spawnedAt: attempt.SpawnedAt, label: fmt.Sprintf("alignment attempt %d", attempt.Number), file: attempt.Dir})
	}
	parentDir := filepath.ToSlash(loomengine.LoomParentReviewDirRel())
	for _, round := range row.parentRounds {
		if round.Alignment == nil {
			continue
		}
		spawns = append(spawns, spawn{kind: attemptKindParentReviewAlignment, spawnedAt: round.Alignment.SpawnedAt, label: fmt.Sprintf("parent-review round %d alignment", round.Number), file: path.Join(parentDir, fmt.Sprintf("round-%d", round.Number))})
	}

	var findings []Finding
	var attempts TimingAttempts
	totals := map[string]time.Duration{}
	known := map[string]bool{}
	counts := map[string]int{}
	for _, one := range spawns {
		counts[one.kind]++
		one.spawnedAt = one.spawnedAt.Truncate(time.Second)
		part := attemptPart{kind: one.kind, spawnedAt: one.spawnedAt.UTC()}
		to := span.To
		absent := ""
		switch {
		case one.spawnedAt.Before(span.From) || one.spawnedAt.After(span.To):
			absent = "it was spawned outside the run's span"
		case !row.inFlight:
			if next := stampAfter(rows, one.spawnedAt, time.Time{}); next.IsZero() {
				absent = "no row of either history is stamped after it"
			} else {
				to = next
			}
		default:
			to = stampAfter(rows, one.spawnedAt, span.To)
		}
		if absent != "" {
			findings = append(findings, Finding{Tag: row.Tag, File: one.file, Error: fmt.Sprintf("%s spawned %s has no time: %s", one.label, one.spawnedAt.UTC().Format(time.RFC3339), absent)})
		} else {
			part.known = true
			part.gross = Interval{From: part.spawnedAt, To: to}
			part.net = withoutIntervals([]Interval{part.gross}, sleeps)
			totals[one.kind] += lengthOf(part.net)
			known[one.kind] = true
		}
		attempts.parts = append(attempts.parts, part)
	}

	figures := map[string]*AttemptFigure{attemptKindArbiter: &attempts.Arbiter, attemptKindAlignment: &attempts.Alignment, attemptKindParentReviewAlignment: &attempts.ParentReviewAlignment}
	for kind, figure := range figures {
		figure.Count = counts[kind]
		if known[kind] {
			seconds := int64(totals[kind] / time.Second)
			figure.Seconds = &seconds
		}
	}
	return attempts, findings
}

// attemptsInWindow returns the figures of parts for one bucket window: each spawn counted when its spawned_at falls in window, and each known time clipped to window.
// A figure's seconds are nil when no attempt of its kind with a known time overlaps window.
func attemptsInWindow(parts []attemptPart, window Interval) TimingAttempts {
	var attempts TimingAttempts
	figures := map[string]*AttemptFigure{attemptKindArbiter: &attempts.Arbiter, attemptKindAlignment: &attempts.Alignment, attemptKindParentReviewAlignment: &attempts.ParentReviewAlignment}
	totals := map[string]time.Duration{}
	known := map[string]bool{}
	for _, part := range parts {
		if !part.spawnedAt.Before(window.From) && part.spawnedAt.Before(window.To) {
			figures[part.kind].Count++
		}
		if part.known && overlaps(part.gross, window) {
			known[part.kind] = true
			totals[part.kind] += lengthOf(clipIntervals(part.net, window))
		}
	}
	for kind, figure := range figures {
		if known[kind] {
			seconds := int64(totals[kind] / time.Second)
			figure.Seconds = &seconds
		}
	}
	return attempts
}

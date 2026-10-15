// timingspans.go times one run: it merges the loom and batten histories into stamped rows, opens and closes the run's span, counts each row interval under a phase, and takes the machine's sleep out of them.
// The interval arithmetic on sets of Intervals lives here too.

package reportengine

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// exactLoomPhases maps the loom rows that no row family names to their phase.
var exactLoomPhases = map[string]string{
	loomshed.NamePreflight:       PhaseSetup,
	loomshed.NameLoomPreflight:   PhaseSetup,
	loomshed.NameBatchifier:      PhasePlan,
	loomshed.NameDescribe:        PhaseWebster,
	loomshed.NamePublish:         PhasePublishFinalize,
	loomshed.NamePRAlign:         PhasePublishFinalize,
	loomshed.NamePRGate:          PhasePublishFinalize,
	loomshed.NamePRRework:        PhasePublishFinalize,
	loomshed.NameFinalize:        PhasePublishFinalize,
	loomshed.NameFrictionReflect: PhasePublishFinalize,
}

// loomPhaseFamilies maps the first dash-separated segment of a loom row name to the phase every row of that family counts under.
var loomPhaseFamilies = map[string]string{
	rowFamily(loomshed.NameDiscussionWrite): PhaseDiscussion,
	rowFamily(loomshed.NamePlanWrite):       PhasePlan,
	rowFamily(loomshed.NameWebster):         PhaseWebster,
	rowFamily(loomshed.NameMergeIn):         PhaseMerge,
	rowFamily(loomshed.NameDarnWrite):       PhasePublishFinalize,
}

// rowFamily returns the text of a row name before its first dash, the whole name when it has none.
func rowFamily(name string) string {
	family, _, _ := strings.Cut(name, "-")
	return family
}

// loomRowPhase returns the phase of a loom row by name and whether the name is in the phase table.
func loomRowPhase(producer string) (string, bool) {
	if phase, ok := exactLoomPhases[producer]; ok {
		return phase, true
	}
	phase, ok := loomPhaseFamilies[rowFamily(producer)]
	return phase, ok
}

// timedRow is one history entry of a run, from the loom history or from the batten record.
// position is the entry's index in its own history, and stamp is its last_at when set, else its at; stamped is false when neither is a usable time.
type timedRow struct {
	batten   bool
	position int
	producer string
	outcome  shedengine.Outcome
	output   string
	stamp    time.Time
	stamped  bool
}

// phasedInterval is a stretch of a run's time counted under one phase.
type phasedInterval struct {
	phase string
	Interval
}

// entryStamp returns the UTC time a history entry is stamped with and whether it has one.
func entryStamp(entry shedengine.HistoryEntry) (time.Time, bool) {
	text := entry.At
	if entry.LastAt != "" {
		text = entry.LastAt
	}
	stamp, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, false
	}
	return stamp.UTC(), true
}

// statusFileOf returns the tree-relative status file of slug, empty when the slug is no legal run id.
func statusFileOf(slug string) string {
	rels, err := shedrun.TreeRunRels(slug)
	if err != nil {
		return ""
	}
	return rels.Status
}

// mergeRows merges the loom history and the batten record's history in stamp order, each history keeping its own order.
// A row with no usable stamp is placed at its history's head and gets a finding naming the row and the file.
func mergeRows(row RunRow) ([]timedRow, []Finding) {
	var findings []Finding
	file := statusFileOf(row.Slug)
	build := func(history []shedengine.HistoryEntry, batten bool) []timedRow {
		label := "loom"
		if batten {
			label = "batten"
		}
		rows := make([]timedRow, len(history))
		for i, entry := range history {
			stamp, ok := entryStamp(entry)
			rows[i] = timedRow{batten: batten, position: i, producer: entry.Producer, outcome: entry.Outcome, output: entry.Output, stamp: stamp, stamped: ok}
			if !ok {
				findings = append(findings, Finding{Tag: row.Tag, File: file, Error: fmt.Sprintf("%s history row %d (%s) has no stamp: its time is empty or not RFC 3339", label, i+1, entry.Producer)})
			}
		}
		return rows
	}

	loom, batten := build(row.childHistory, false), build(latestBattenHistory(row), true)

	merged := make([]timedRow, 0, len(loom)+len(batten))
	for len(loom) > 0 || len(batten) > 0 {
		var takeBatten bool
		switch {
		case len(loom) == 0:
			takeBatten = true
		case len(batten) == 0 || !loom[0].stamped:
			takeBatten = false
		case !batten[0].stamped:
			takeBatten = true
		default:
			takeBatten = batten[0].stamp.Before(loom[0].stamp)
		}
		if takeBatten {
			merged, batten = append(merged, batten[0]), batten[1:]
		} else {
			merged, loom = append(merged, loom[0]), loom[1:]
		}
	}
	return merged, findings
}

// phaseOf returns the phase the row at index counts under, and false when it is a loom row the phase table does not name, which counts under PhaseOther.
// A batten row counts by position: before batten's first Run-Shed entry under PhaseSetup, after it under PhaseLand.
// A Run-Shed entry records the child's state changes and counts under the phase of the producer its change names, else the phase of the latest loom row before it, else PhaseSetup.
func phaseOf(rows []timedRow, index int) (string, bool) {
	row := rows[index]
	if !row.batten {
		if phase, ok := loomRowPhase(row.producer); ok {
			return phase, true
		}
		return PhaseOther, false
	}

	if row.producer == battenrecipe.NameRunShed {
		if change, ok := battenshed.ParseStateChange(row.output); ok && change.Producer != "" {
			if phase, known := loomRowPhase(change.Producer); known {
				return phase, true
			}
		}
		for i := index - 1; i >= 0; i-- {
			if rows[i].batten {
				continue
			}
			if phase, known := loomRowPhase(rows[i].producer); known {
				return phase, true
			}
		}
		return PhaseSetup, true
	}

	for _, candidate := range rows {
		if candidate.batten && candidate.producer == battenrecipe.NameRunShed {
			if row.position > candidate.position {
				return PhaseLand, true
			}
			break
		}
	}
	return PhaseSetup, true
}

// spanOpening returns where a run's span opens: the first entry of the batten record, else the seed's started_at, else the first stamped loom row.
// The last case adds a finding saying so, and found is false when no row carries a stamp at all.
func spanOpening(row RunRow, rows []timedRow) (opening time.Time, found bool, finding *Finding) {
	if n := len(row.battenSnapshots); n > 0 {
		if history := row.battenSnapshots[n-1].History; len(history) > 0 {
			if stamp, ok := entryStamp(history[0]); ok {
				return stamp, true, nil
			}
		}
	}
	if stamp, err := time.Parse(time.RFC3339, row.startedAt); err == nil {
		return stamp.UTC(), true, nil
	}
	for _, candidate := range rows {
		if !candidate.batten && candidate.stamped {
			return candidate.stamp, true, &Finding{Tag: row.Tag, File: statusFileOf(row.Slug), Error: "neither the batten record nor the seed carries a stamp: the span opens at the first stamped loom row"}
		}
	}
	return time.Time{}, false, nil
}

// timeRun times one run: its span, the sleep inside it and the span split over the phases.
// The span opens as spanOpening says and closes at the last stamp of either history, or at snapshot for a run in flight, whose trailing interval counts under the phase of its loom status's current producer.
// Each row's interval runs from the latest stamp so far to the row's own stamp; a stamp earlier than the latest, or none, gives a zero interval and a finding.
// A run no stamp opens a span for comes back as the zero TimingRun with a finding.
// Every boundary is cut to a whole second, as the row stamps are, so the truncated figures add up to the truncated span.
func timeRun(row RunRow, opts TimingOptions, snapshot time.Time) (TimingRun, []Finding) {
	snapshot = snapshot.Truncate(time.Second)
	rows, findings := mergeRows(row)
	file := statusFileOf(row.Slug)
	opening, found, openFinding := spanOpening(row, rows)
	if !found {
		return TimingRun{}, append(findings, Finding{Tag: row.Tag, File: file, Error: "no row of the run carries a stamp, so its span has no opening: left out"})
	}
	if openFinding != nil {
		findings = append(findings, *openFinding)
	}

	held := map[string]bool{}
	var intervals []phasedInterval
	latest := opening
	for i, timed := range rows {
		phase, known := phaseOf(rows, i)
		held[phase] = true
		if !known {
			findings = append(findings, Finding{Tag: row.Tag, File: file, Error: fmt.Sprintf("row %s names no phase: counted under %s", timed.producer, PhaseOther)})
		}
		if !timed.stamped {
			continue
		}
		if timed.stamp.Before(latest) {
			findings = append(findings, Finding{Tag: row.Tag, File: file, Error: fmt.Sprintf("row %s is stamped %s, before the latest stamp %s: its interval is zero", timed.producer, timed.stamp.Format(time.RFC3339), latest.Format(time.RFC3339))})
			continue
		}
		intervals = append(intervals, phasedInterval{phase: phase, Interval: Interval{From: latest, To: timed.stamp}})
		latest = timed.stamp
	}

	if row.inFlight && snapshot.After(latest) {
		phase, known := loomRowPhase(row.currentProducer)
		if !known {
			phase = PhaseOther
			if row.currentProducer != "" {
				findings = append(findings, Finding{Tag: row.Tag, File: file, Error: fmt.Sprintf("current producer %s names no phase: counted under %s", row.currentProducer, PhaseOther)})
			}
		}
		held[phase] = true
		intervals = append(intervals, phasedInterval{phase: phase, Interval: Interval{From: latest, To: snapshot}})
		latest = snapshot
	}

	span := Interval{From: opening, To: latest}
	wholeSleeps := make([]Interval, len(opts.Sleeps))
	for i, sleep := range opts.Sleeps {
		wholeSleeps[i] = Interval{From: sleep.From.Truncate(time.Second), To: sleep.To.Truncate(time.Second)}
	}
	sleeps := clipIntervals(unionIntervals(wholeSleeps), span)
	var parts []phasedInterval
	phaseTime := map[string]time.Duration{}
	for _, interval := range intervals {
		for _, piece := range withoutIntervals([]Interval{interval.Interval}, sleeps) {
			parts = append(parts, phasedInterval{phase: interval.phase, Interval: piece})
			phaseTime[interval.phase] += piece.length()
		}
	}

	phases := map[string]*int64{}
	for _, phase := range timingPhases {
		if held[phase] {
			seconds := int64(phaseTime[phase] / time.Second)
			phases[phase] = &seconds
		} else {
			phases[phase] = nil
		}
	}

	netByKind := netWaits(clipWaits(runWaits(row, rows, span.To), span), sleeps)
	waitSeconds := map[string]int64{}
	var waitUnion []Interval
	for _, kind := range timingWaitKinds {
		waitSeconds[kind] = int64(lengthOf(netByKind[kind]) / time.Second)
		waitUnion = append(waitUnion, netByKind[kind]...)
	}
	attempts, attemptFindings := runAttempts(row, rows, span, sleeps)
	findings = append(findings, attemptFindings...)
	stops, stopFindings := runStops(row, rows)
	findings = append(findings, stopFindings...)

	state := RunStateLanded
	if row.inFlight {
		state = RunStateInFlight
	}
	run := TimingRun{
		Slug:         row.Slug,
		Tag:          row.Tag,
		State:        state,
		Start:        span.From,
		End:          span.To,
		SpanSeconds:  int64(span.length() / time.Second),
		SleepSeconds: int64(lengthOf(sleeps) / time.Second),
		Phases:       phases,
		Waits:        waitSeconds,
		Attempts:     attempts,
		Stops:        stops,
		committed:    row.Committed,
		span:         span,
		sleeps:       sleeps,
		parts:        parts,
		waitParts:    netByKind,
		waitUnion:    waitUnion,
	}
	run.ActiveSeconds = run.SpanSeconds - run.SleepSeconds - int64(lengthOf(waitUnion)/time.Second)
	return run, findings
}

// length is the interval's duration, zero for an empty or reversed one.
func (i Interval) length() time.Duration {
	if !i.To.After(i.From) {
		return 0
	}
	return i.To.Sub(i.From)
}

// lengthOf is the summed duration of the intervals, which the caller keeps disjoint.
func lengthOf(set []Interval) time.Duration {
	var total time.Duration
	for _, interval := range set {
		total += interval.length()
	}
	return total
}

// overlaps reports whether a and b share time; an empty a counts when it lies inside b.
func overlaps(a, b Interval) bool {
	if !a.To.After(a.From) {
		return !a.From.Before(b.From) && a.From.Before(b.To)
	}
	return a.From.Before(b.To) && a.To.After(b.From)
}

// unionIntervals returns the non-empty intervals of set merged where they overlap or touch, oldest first.
func unionIntervals(set []Interval) []Interval {
	sorted := make([]Interval, 0, len(set))
	for _, interval := range set {
		if interval.To.After(interval.From) {
			sorted = append(sorted, interval)
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].From.Before(sorted[j].From) })

	var merged []Interval
	for _, interval := range sorted {
		if n := len(merged); n > 0 && !interval.From.After(merged[n-1].To) {
			if interval.To.After(merged[n-1].To) {
				merged[n-1].To = interval.To
			}
			continue
		}
		merged = append(merged, interval)
	}
	return merged
}

// withoutIntervals returns the parts of set that cut does not cover.
func withoutIntervals(set, cut []Interval) []Interval {
	pieces := set
	for _, hole := range unionIntervals(cut) {
		var next []Interval
		for _, piece := range pieces {
			if !hole.From.Before(piece.To) || !hole.To.After(piece.From) {
				next = append(next, piece)
				continue
			}
			if hole.From.After(piece.From) {
				next = append(next, Interval{From: piece.From, To: hole.From})
			}
			if hole.To.Before(piece.To) {
				next = append(next, Interval{From: hole.To, To: piece.To})
			}
		}
		pieces = next
	}
	var kept []Interval
	for _, piece := range pieces {
		if piece.To.After(piece.From) {
			kept = append(kept, piece)
		}
	}
	return kept
}

// clipIntervals returns the parts of set inside window.
func clipIntervals(set []Interval, window Interval) []Interval {
	var clipped []Interval
	for _, interval := range set {
		from, to := interval.From, interval.To
		if from.Before(window.From) {
			from = window.From
		}
		if to.After(window.To) {
			to = window.To
		}
		if to.After(from) {
			clipped = append(clipped, Interval{From: from, To: to})
		}
	}
	return clipped
}

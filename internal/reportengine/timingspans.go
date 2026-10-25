// timingspans.go times one run: it merges the loom and batten histories into stamped rows, opens and closes the run's span, counts each row interval under a phase, and takes the machine's sleep out of them.
// The interval arithmetic on sets of Intervals lives here too.

package reportengine

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// phaseTable maps the rows of one recipe's run to their phase: exact names a row by its whole name, and families names every row whose first dash-separated segment it holds.
type phaseTable struct {
	exact    map[string]string
	families map[string]string
}

// retiredBatchifierRow is the loom row that batched the plan before the implementer took batching over; a past run's history still carries it.
const retiredBatchifierRow = "Batchifier"

// The loom rows the recipes no longer hold, written as literals so a past run's history and status still read.
const (
	retiredPublishRow             = "Publish"
	retiredPRAlignRow             = "PR-Align"
	retiredPRGateRow              = "PR-Gate"
	retiredPRReworkRow            = "PR-Rework"
	retiredMergeInRow             = "Merge-In"
	retiredMergeBouncerRow        = "Merge-Bouncer"
	retiredMergeBurlerReviewRow   = "Merge-Burler-Review"
	retiredMergeBurlerFixRow      = "Merge-Burler-Fix"
	retiredWebsterBouncerRow      = "Webster-Bouncer"
	retiredWebsterBurlerReviewRow = "Webster-Burler-Review"
	retiredWebsterBurlerFixRow    = "Webster-Burler-Fix"
)

// The darn perch rows the darn recipe no longer holds, now that it runs loom's Code-Review rows; a past darn run's history still carries them.
const (
	retiredDarnBouncerRow      = "Darn-Bouncer"
	retiredDarnBurlerReviewRow = "Darn-Burler-Review"
	retiredDarnBurlerFixRow    = "Darn-Burler-Fix"
)

// retiredRowNames maps each retired loom row to the phase a past run's time on it counts under.
var retiredRowNames = map[string]string{
	retiredPublishRow:             PhasePublishFinalize,
	retiredPRAlignRow:             PhasePublishFinalize,
	retiredPRGateRow:              PhasePublishFinalize,
	retiredPRReworkRow:            PhasePublishFinalize,
	retiredMergeInRow:             PhaseMerge,
	retiredMergeBouncerRow:        PhaseMerge,
	retiredMergeBurlerReviewRow:   PhaseMerge,
	retiredMergeBurlerFixRow:      PhaseMerge,
	retiredWebsterBouncerRow:      PhaseWebster,
	retiredWebsterBurlerReviewRow: PhaseWebster,
	retiredWebsterBurlerFixRow:    PhaseWebster,
}

// recipePhaseTables holds the phase table of every recipe a report selects.
var recipePhaseTables = map[string]phaseTable{
	shedrun.RecipeLoom: {
		exact: loomPhases(),
		families: map[string]string{
			rowFamily(loomshed.NameDiscussionWrite): PhaseDiscussion,
			rowFamily(loomshed.NamePlanWrite):       PhasePlan,
			rowFamily(loomshed.NameWebster):         PhaseWebster,
			rowFamily(loomshed.NameCodeBouncer):     PhaseWebster,
			rowFamily(loomshed.NameParentAlign):     PhaseMerge,
			rowFamily(retiredMergeInRow):            PhaseMerge,
		},
	},
	shedrun.RecipeDarn: {exact: darnPhases()},
}

// loomPhases maps every row of a loom run that a phase table names whole, the retired rows of past runs included, to its phase.
func loomPhases() map[string]string {
	phases := map[string]string{
		loomshed.NamePreflight:       PhaseSetup,
		loomshed.NameLoomPreflight:   PhaseSetup,
		retiredBatchifierRow:         PhaseWebster,
		loomshed.NameDescribe:        PhaseWebster,
		loomshed.NamePRReview:        PhasePublishFinalize,
		loomshed.NameFinalize:        PhasePublishFinalize,
		loomshed.NameFrictionReflect: PhasePublishFinalize,
	}
	maps.Copy(phases, retiredRowNames)
	return phases
}

// darnPhases maps every row of a darn run, and each retired row name a darn status may still carry, to its phase.
func darnPhases() map[string]string {
	phases := map[string]string{
		loomshed.NamePreflight:        PhaseSetup,
		loomshed.NameDarnWrite:        PhaseWrite,
		loomshed.NameDescribe:         PhaseWrite,
		loomshed.NameCodeBouncer:      PhaseReview,
		loomshed.NameCodeBurlerReview: PhaseReview,
		loomshed.NameCodeBurlerFix:    PhaseReview,
		retiredMergeInRow:             PhaseMerge,
		loomshed.NamePRReview:         PhasePublishFinalize,
		loomshed.NameFinalize:         PhasePublishFinalize,
		loomshed.NameFrictionReflect:  PhasePublishFinalize,
		retiredPublishRow:             PhasePublishFinalize,
		retiredPRAlignRow:             PhasePublishFinalize,
		retiredPRGateRow:              PhasePublishFinalize,
		retiredDarnBouncerRow:         PhaseReview,
		retiredDarnBurlerReviewRow:    PhaseReview,
		retiredDarnBurlerFixRow:       PhaseReview,
	}
	for retired, successor := range loomrecipe.DarnRetiredRows() {
		phases[retired] = phases[successor]
	}
	return phases
}

// recipePhases returns, in report order, the phases a run of recipe can hold: those its phase table names, and setup, land, paused and other, which every run can.
func recipePhases(recipe string) []string {
	named := map[string]bool{PhaseSetup: true, PhaseLand: true, PhasePaused: true, PhaseOther: true}
	table := recipePhaseTables[recipe]
	for _, phase := range table.exact {
		named[phase] = true
	}
	for _, phase := range table.families {
		named[phase] = true
	}
	var phases []string
	for _, phase := range timingPhases {
		if named[phase] {
			phases = append(phases, phase)
		}
	}
	return phases
}

// rowFamily returns the text of a row name before its first dash, the whole name when it has none.
func rowFamily(name string) string {
	family, _, _ := strings.Cut(name, "-")
	return family
}

// childRowPhase returns the phase of a row of a recipe's run by name and whether the recipe's phase table names it.
func childRowPhase(recipe, producer string) (string, bool) {
	table := recipePhaseTables[recipe]
	if phase, ok := table.exact[producer]; ok {
		return phase, true
	}
	phase, ok := table.families[rowFamily(producer)]
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
// A skipped entry is left out: the row it passed through was never called, so its time counts under the row before it.
func mergeRows(row RunRow) ([]timedRow, []Finding) {
	var findings []Finding
	file := statusFileOf(row.Slug)
	build := func(history []shedengine.HistoryEntry, batten bool) []timedRow {
		label := "loom"
		if batten {
			label = "batten"
		}
		rows := make([]timedRow, 0, len(history))
		for i, entry := range history {
			if entry.Outcome == shedengine.OutcomeSkipped {
				continue
			}
			stamp, ok := entryStamp(entry)
			rows = append(rows, timedRow{batten: batten, position: i, producer: entry.Producer, outcome: entry.Outcome, output: entry.Output, stamp: stamp, stamped: ok})
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

// phaseOf returns the phase the row at index of a run of recipe counts under, and false when it is a child row the recipe's phase table does not name, which counts under PhaseOther.
// A batten row counts by position: before batten's first Run-Shed entry under PhaseSetup, after it under PhaseLand.
// A Run-Shed entry records the child's state changes: a change from paused closes the pause and counts under PhasePaused,
// and any other counts under the phase of the producer its change names, else the phase of the latest child row before it, else PhaseSetup.
func phaseOf(recipe string, rows []timedRow, index int) (string, bool) {
	row := rows[index]
	if !row.batten {
		if phase, ok := childRowPhase(recipe, row.producer); ok {
			return phase, true
		}
		return PhaseOther, false
	}

	if row.producer == battenrecipe.NameRunShed {
		change, ok := battenshed.ParseStateChange(row.output)
		if ok && change.From == shedengine.StatePaused {
			return PhasePaused, true
		}
		if ok && change.Producer != "" {
			if phase, known := childRowPhase(recipe, change.Producer); known {
				return phase, true
			}
		}
		for i := index - 1; i >= 0; i-- {
			if rows[i].batten {
				continue
			}
			if phase, known := childRowPhase(recipe, rows[i].producer); known {
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

// spanOpening returns where a run's span opens: the first entry of the batten record, else the seed's started_at, else the first stamped child row.
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
			return candidate.stamp, true, &Finding{Tag: row.Tag, File: statusFileOf(row.Slug), Error: "neither the batten record nor the seed carries a stamp: the span opens at the first stamped child row"}
		}
	}
	return time.Time{}, false, nil
}

// timeRun times one run: its span, the sleep inside it and the span split over the phases.
// The span opens as spanOpening says and closes at the last stamp of either history, or at snapshot for a run in flight, whose trailing interval counts under the phase of its loom status's current producer, or under PhasePaused while its child stands paused.
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
		phase, known := phaseOf(row.Recipe, rows, i)
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
		phase, known := childRowPhase(row.Recipe, row.currentProducer)
		if standsPaused(rows) {
			phase, known = PhasePaused, true
		}
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
	var inactive []Interval
	for _, kind := range timingWaitKinds {
		waitSeconds[kind] = int64(lengthOf(netByKind[kind]) / time.Second)
		inactive = append(inactive, netByKind[kind]...)
	}
	for _, part := range parts {
		if part.phase == PhasePaused {
			inactive = append(inactive, part.Interval)
		}
	}
	inactive = unionIntervals(inactive)
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
		Recipe:       row.Recipe,
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
		inactive:     inactive,
	}
	run.ActiveSeconds = run.SpanSeconds - run.SleepSeconds - int64(lengthOf(inactive)/time.Second)
	return run, findings
}

// standsPaused reports whether the last of rows is batten's record of the child's change into paused, so the child has not resumed since.
func standsPaused(rows []timedRow) bool {
	if len(rows) == 0 {
		return false
	}
	last := rows[len(rows)-1]
	if !last.batten || last.producer != battenrecipe.NameRunShed {
		return false
	}
	change, ok := battenshed.ParseStateChange(last.output)
	return ok && change.To == shedengine.StatePaused
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

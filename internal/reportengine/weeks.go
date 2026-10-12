// weeks.go aggregates a report's runs, partial runs and landings per ISO week, and counts the runs lacking each source.
// A week sums a figure over the runs holding its source and states how many lack it; a figure no run holds, and a mean or share nothing carries, is nil and never 0.

package reportengine

import (
	"sort"
)

// sourceNames lists every source a run can hold, in report order.
var sourceNames = []string{SourceBatten, SourceParentReview, SourceAlignment, SourceLandReadyTasks, SourceHaltNotes, SourceArbiter, SourceDriveReports, SourceDecisions}

// meanWakeSources are the wake sources a run must hold to enter a week's wake mean.
// land-ready-tasks is absent for every run, so it takes no part in the test.
var meanWakeSources = []string{SourceBatten, SourceParentReview, SourceAlignment}

// ClassCount counts one stop or failure class over a week: its stops, and the number of the week's runs it occurred in.
type ClassCount struct {
	Class string `json:"class"`
	Stops int    `json:"stops"`
	Runs  int    `json:"runs"`
}

// FreshCounts counts the landings of one kind by their fresh-code verdict.
type FreshCounts struct {
	Fresh      int `json:"fresh"`
	Aged       int `json:"aged"`
	NoPreImage int `json:"no_pre_image"`
}

// share is fresh over fresh plus aged, nil when neither is counted.
func (c FreshCounts) share() *float64 {
	return ratio(c.Fresh, c.Fresh+c.Aged)
}

// SegmentWeek is the review rounds of one segment over a week: the mean over the runs holding it, how many do and do not, and the count per outcome.
type SegmentWeek struct {
	Mean     *float64       `json:"mean"`
	Runs     int            `json:"runs"`
	Lacking  int            `json:"lacking"`
	Outcomes map[string]int `json:"outcomes"`
}

// Week is the figures of one ISO week.
// Lacking counts the week's runs lacking each source; WakeRuns is how many runs the wake mean covers.
type Week struct {
	Week    string         `json:"week"`
	Runs    int            `json:"runs"`
	Partial int            `json:"partial"`
	Lacking map[string]int `json:"lacking"`

	WakesPerRun *float64                  `json:"wakes_per_run"`
	WakeRuns    int                       `json:"wake_runs"`
	Wakes       map[string]map[string]int `json:"wakes"`
	Automatic   map[string]int            `json:"automatic"`
	Paused      *int                      `json:"paused"`

	StopClasses    []ClassCount `json:"stop_classes"`
	FailureClasses []ClassCount `json:"failure_classes"`

	Landings      map[string]int `json:"landings"`
	Hand          int            `json:"hand"`
	Fix           FreshCounts    `json:"fix"`
	Unmapped      FreshCounts    `json:"unmapped"`
	FixShare      *float64       `json:"fix_share"`
	UnmappedShare *float64       `json:"unmapped_share"`

	Rounds       map[string]*SegmentWeek `json:"rounds"`
	Arbiter      []ArbiterOutcome        `json:"arbiter"`
	LLMSteps     map[string]int          `json:"llm_steps"`
	DriveReports *int                    `json:"drive_reports"`
	Decisions    map[string]int          `json:"decisions"`
	Alignment    []AlignmentOutcome      `json:"alignment"`
}

// ratio returns numerator over denominator, nil when the denominator is 0.
func ratio(numerator, denominator int) *float64 {
	if denominator == 0 {
		return nil
	}
	value := float64(numerator) / float64(denominator)
	return &value
}

// absentCounts counts, per source, the runs that lack it.
func absentCounts(runs []RunRow) map[string]int {
	absent := map[string]int{}
	for _, name := range sourceNames {
		absent[name] = 0
		for _, run := range runs {
			if !run.Sources[name] {
				absent[name]++
			}
		}
	}
	return absent
}

// computeWeeks buckets runs and partial runs by their week and landings by their commit's week, and returns one Week per week holding any, ascending.
func computeWeeks(report Report) []Week {
	runs := map[string][]RunRow{}
	labels := map[string]bool{}
	for _, run := range report.Runs {
		runs[run.Week] = append(runs[run.Week], run)
		labels[run.Week] = true
	}
	partial := map[string]int{}
	for _, p := range report.Partial {
		partial[p.Week]++
		labels[p.Week] = true
	}
	landings := map[string][]Landing{}
	for _, landing := range report.Landings {
		landings[landing.Week] = append(landings[landing.Week], landing)
		labels[landing.Week] = true
	}

	var ordered []string
	for label := range labels {
		ordered = append(ordered, label)
	}
	sort.Strings(ordered)

	weeks := make([]Week, 0, len(ordered))
	for _, label := range ordered {
		week := Week{Week: label, Runs: len(runs[label]), Partial: partial[label], Lacking: absentCounts(runs[label])}
		week.fillWakes(runs[label])
		week.fillStops(runs[label])
		week.fillLandings(landings[label])
		week.fillRounds(runs[label])
		week.fillRepairs(runs[label])
		weeks = append(weeks, week)
	}
	return weeks
}

// fillWakes sums the wakes, automatic steps and paused changes, and takes the mean over the runs holding every wake source.
func (w *Week) fillWakes(runs []RunRow) {
	var total, counted int
	for _, run := range runs {
		holdsAll := true
		runTotal := 0
		for _, source := range meanWakeSources {
			if run.Wakes[source] == nil {
				holdsAll = false
				continue
			}
			runTotal += run.Wakes[source].Total
		}
		if holdsAll {
			counted++
			total += runTotal
		}

		for source, wakes := range run.Wakes {
			if wakes == nil {
				continue
			}
			if w.Wakes == nil {
				w.Wakes = map[string]map[string]int{}
			}
			if w.Wakes[source] == nil {
				w.Wakes[source] = map[string]int{}
			}
			for kind, count := range wakes.Kinds {
				w.Wakes[source][kind] += count
			}
		}
		if run.Automatic != nil {
			if w.Automatic == nil {
				w.Automatic = map[string]int{}
			}
			for kind, count := range run.Automatic {
				w.Automatic[kind] += count
			}
		}
		if run.Paused != nil {
			paused := *run.Paused
			if w.Paused != nil {
				paused += *w.Paused
			}
			w.Paused = &paused
		}
	}
	w.WakeRuns = counted
	w.WakesPerRun = ratio(total, counted)
}

// fillStops counts the stop and failure classes over the runs holding the halt-note source.
func (w *Week) fillStops(runs []RunRow) {
	stops := map[string]*ClassCount{}
	failures := map[string]*ClassCount{}
	held := false
	count := func(into map[string]*ClassCount, class string, n int) {
		if into[class] == nil {
			into[class] = &ClassCount{Class: class}
		}
		into[class].Stops += n
	}
	for _, run := range runs {
		if run.Stops == nil {
			continue
		}
		held = true
		seenStops := map[string]bool{}
		for _, stop := range run.Stops {
			count(stops, stop.Class, 1)
			seenStops[stop.Class] = true
		}
		for class := range seenStops {
			stops[class].Runs++
		}
		for class, n := range run.Failures {
			count(failures, class, n)
			failures[class].Runs++
		}
	}
	if held {
		w.StopClasses, w.FailureClasses = sortedClasses(stops), sortedClasses(failures)
	}
}

// sortedClasses lists counts by stops descending, then class.
func sortedClasses(counts map[string]*ClassCount) []ClassCount {
	list := make([]ClassCount, 0, len(counts))
	for _, count := range counts {
		list = append(list, *count)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Stops != list[j].Stops {
			return list[i].Stops > list[j].Stops
		}
		return list[i].Class < list[j].Class
	})
	return list
}

// fillLandings counts the week's landings per kind and hand landings, and rates the fix and unmapped landings' fresh-code verdicts.
func (w *Week) fillLandings(landings []Landing) {
	w.Landings = map[string]int{LandingKindFix: 0, LandingKindFeature: 0, LandingKindOther: 0, LandingKindUnmapped: 0}
	for _, landing := range landings {
		w.Landings[landing.Kind]++
		if landing.Hand {
			w.Hand++
		}
		var into *FreshCounts
		switch landing.Kind {
		case LandingKindFix:
			into = &w.Fix
		case LandingKindUnmapped:
			into = &w.Unmapped
		default:
			continue
		}
		switch landing.Fresh {
		case FreshnessFresh:
			into.Fresh++
		case FreshnessAged:
			into.Aged++
		case FreshnessNoPreImage:
			into.NoPreImage++
		}
	}
	w.FixShare, w.UnmappedShare = w.Fix.share(), w.Unmapped.share()
}

// fillRounds takes the mean rounds and outcome counts of every segment some run of the week holds.
func (w *Week) fillRounds(runs []RunRow) {
	names := map[string]bool{}
	for _, run := range runs {
		for name := range run.Segments {
			names[name] = true
		}
	}
	if len(names) == 0 {
		return
	}
	w.Rounds = map[string]*SegmentWeek{}
	for name := range names {
		segment := &SegmentWeek{Outcomes: map[string]int{}}
		total := 0
		for _, run := range runs {
			held := run.Segments[name]
			if held == nil {
				segment.Lacking++
				continue
			}
			segment.Runs++
			total += held.Rounds
			if held.Outcome != "" {
				segment.Outcomes[held.Outcome]++
			}
		}
		segment.Mean = ratio(total, segment.Runs)
		w.Rounds[name] = segment
	}
}

// fillRepairs sums the arbiter outcomes, LLM steps, drive reports, decisions and alignment outcomes over the runs holding each.
func (w *Week) fillRepairs(runs []RunRow) {
	arbiter := map[string]*ArbiterOutcome{}
	type alignmentKey struct{ point, verdict string }
	alignment := map[alignmentKey]*AlignmentOutcome{}
	var heldArbiter, heldAlignment bool
	for _, run := range runs {
		if run.Arbiter != nil {
			heldArbiter = true
			for _, outcome := range run.Arbiter {
				if arbiter[outcome.Verdict] == nil {
					arbiter[outcome.Verdict] = &ArbiterOutcome{Verdict: outcome.Verdict}
				}
				arbiter[outcome.Verdict].Count += outcome.Count
				arbiter[outcome.Verdict].Applied += outcome.Applied
			}
		}
		if run.Alignment != nil {
			heldAlignment = true
			for _, outcome := range run.Alignment {
				key := alignmentKey{outcome.Point, outcome.Verdict}
				if alignment[key] == nil {
					alignment[key] = &AlignmentOutcome{Point: outcome.Point, Verdict: outcome.Verdict}
				}
				alignment[key].Count += outcome.Count
				alignment[key].Applied += outcome.Applied
			}
		}
		for source, steps := range run.LLMSteps {
			if w.LLMSteps == nil {
				w.LLMSteps = map[string]int{}
			}
			w.LLMSteps[source] += steps
		}
		if run.DriveReports != nil {
			reports := *run.DriveReports
			if w.DriveReports != nil {
				reports += *w.DriveReports
			}
			w.DriveReports = &reports
		}
		if run.Decisions != nil {
			if w.Decisions == nil {
				w.Decisions = map[string]int{}
			}
			for author, count := range run.Decisions {
				w.Decisions[author] += count
			}
		}
	}

	if heldArbiter {
		w.Arbiter = make([]ArbiterOutcome, 0, len(arbiter))
		for _, outcome := range arbiter {
			w.Arbiter = append(w.Arbiter, *outcome)
		}
		sort.Slice(w.Arbiter, func(i, j int) bool { return w.Arbiter[i].Verdict < w.Arbiter[j].Verdict })
	}
	if heldAlignment {
		w.Alignment = make([]AlignmentOutcome, 0, len(alignment))
		for _, outcome := range alignment {
			w.Alignment = append(w.Alignment, *outcome)
		}
		sort.Slice(w.Alignment, func(i, j int) bool {
			a, b := w.Alignment[i], w.Alignment[j]
			if a.Point != b.Point {
				return a.Point < b.Point
			}
			return a.Verdict < b.Verdict
		})
	}
}

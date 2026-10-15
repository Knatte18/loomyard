// timing.go declares the timing report's options and result types, ComputeTiming, which assembles it from one snapshot of sources, and the selection of the runs in flight it covers beside the landed ones.

package reportengine

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// The bucket widths of a timing report.
const (
	BucketDay  = "day"
	BucketWeek = "week"
)

// The phases a run's time is counted under.
const (
	PhaseSetup           = "setup"
	PhaseDiscussion      = "discussion"
	PhasePlan            = "plan"
	PhaseWebster         = "webster"
	PhaseMerge           = "merge"
	PhasePublishFinalize = "publish-finalize"
	PhaseLand            = "land"
	PhaseOther           = "other"
)

// The states of a timed run.
const (
	// RunStateLanded is a run with an archive tag, selected as a full run.
	RunStateLanded = "landed"
	// RunStateInFlight is a run of a live pair, still on its records branch.
	RunStateInFlight = "in-flight"
)

// timingPhases lists every phase in report order.
var timingPhases = []string{PhaseSetup, PhaseDiscussion, PhasePlan, PhaseWebster, PhaseMerge, PhasePublishFinalize, PhaseLand, PhaseOther}

// Interval is a span of time from From to To.
type Interval struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// TimingOptions shapes a timing report; everything in it is told, since the engine runs no process and reads no clock.
type TimingOptions struct {
	// Since is the UTC date the report's window opens at.
	Since time.Time

	// By is the bucket width, BucketDay or BucketWeek.
	By string

	// Live lists the slugs whose pair the caller found live.
	Live []string

	// Sleeps are the intervals the machine slept, and SleepNote says where they came from or why there are none.
	Sleeps    []Interval
	SleepNote string
}

// TimingSelection records what a timing report was computed over.
type TimingSelection struct {
	RecordsBranch string    `json:"records_branch"`
	RecordsTip    string    `json:"records_tip"`
	Since         time.Time `json:"since"`
	By            string    `json:"by"`
	SnapshotAt    time.Time `json:"snapshot_at"`
}

// TimingRun is the time of one run: its span from first to last stamp, the machine sleep inside it, and the span split over the phases.
// Every duration is whole seconds, and a phase the run holds no row of is nil, never zero.
type TimingRun struct {
	Slug          string            `json:"slug"`
	Tag           string            `json:"tag"`
	State         string            `json:"state"`
	Start         time.Time         `json:"start"`
	End           time.Time         `json:"end"`
	SpanSeconds   int64             `json:"span_seconds"`
	SleepSeconds  int64             `json:"sleep_seconds"`
	ActiveSeconds int64             `json:"active_seconds"`
	Phases        map[string]*int64 `json:"phases"`

	// Waits maps every wait kind to its net seconds, and Attempts holds the arbiter and alignment attempts that served the waits.
	// Active, the waits and sleep add up to the span.
	Waits    map[string]int64 `json:"waits"`
	Attempts TimingAttempts   `json:"attempts"`

	// committed is the archive tag's committer time, the zero time for a run in flight.
	committed time.Time

	// span is the run's span, sleeps the machine sleep clipped to it, and parts the row intervals less that sleep, each under its phase.
	span   Interval
	sleeps []Interval
	parts  []phasedInterval

	// waitParts are the net wait intervals per kind, and waitUnion the union of every kind's.
	waitParts map[string][]Interval
	waitUnion []Interval
}

// PhaseMedian is the median seconds of one phase over the landed runs holding it.
type PhaseMedian struct {
	Seconds *int64 `json:"seconds"`
	Runs    int    `json:"runs"`
}

// TimingBucket is the time of one day or ISO week of the window, summed over the runs overlapping it.
// Two parallel runs add two hours to an hour, and a phase no run in the bucket holds is absent from Phases.
// Waits holds every wait kind, and Attempts counts each spawn in the bucket its spawned_at falls in with its time clipped to the bucket.
type TimingBucket struct {
	Label         string           `json:"label"`
	Start         time.Time        `json:"start"`
	Runs          int              `json:"runs"`
	Phases        map[string]int64 `json:"phases"`
	Waits         map[string]int64 `json:"waits"`
	Attempts      TimingAttempts   `json:"attempts"`
	SleepSeconds  int64            `json:"sleep_seconds"`
	ActiveSeconds int64            `json:"active_seconds"`
}

// TimingReport is the result of ComputeTiming.
type TimingReport struct {
	Selection TimingSelection         `json:"selection"`
	Runs      []TimingRun             `json:"runs"`
	Phases    map[string]*PhaseMedian `json:"phases"`
	Buckets   []TimingBucket          `json:"buckets"`
	SleepNote string                  `json:"sleep_note"`
	Findings  []Finding               `json:"findings"`
}

// ComputeTiming times the runs of src: the landed runs convergence selects and the runs in flight on live pairs' records branches, from opts.Since to src.SnapshotAt.
// A record that does not decode is a Finding and never an error; an error is a failed read of the repositories themselves or an unknown bucket width.
func ComputeTiming(src Sources, opts TimingOptions) (TimingReport, error) {
	if opts.By != BucketDay && opts.By != BucketWeek {
		return TimingReport{}, fmt.Errorf("reportengine: unknown bucket width %q; way forward: use %q or %q", opts.By, BucketDay, BucketWeek)
	}

	since := opts.Since.UTC()
	window := Interval{From: since, To: src.SnapshotAt}
	report := TimingReport{
		Selection: TimingSelection{RecordsBranch: src.RecordsBranch, RecordsTip: src.RecordsTip, Since: since, By: opts.By, SnapshotAt: src.SnapshotAt},
		Runs:      []TimingRun{},
		Buckets:   []TimingBucket{},
		SleepNote: opts.SleepNote,
		Findings:  []Finding{},
	}
	if window.From.After(window.To) {
		report.Phases = phaseMedians(nil, window)
		return report, nil
	}

	var scratch Report
	if err := selectRuns(src, Options{Since: since}, &scratch); err != nil {
		return TimingReport{}, err
	}
	report.Findings = append(report.Findings, scratch.Findings...)
	inFlight, err := selectInFlight(src, opts, &report.Findings)
	if err != nil {
		return TimingReport{}, err
	}

	for _, row := range append(scratch.Runs, inFlight...) {
		run, findings := timeRun(row, opts, src.SnapshotAt)
		if run.Start.IsZero() {
			report.Findings = append(report.Findings, findings...)
			continue
		}
		if run.End.Before(window.From) || run.Start.After(window.To) {
			continue
		}
		report.Findings = append(report.Findings, findings...)
		report.Runs = append(report.Runs, run)
	}
	sort.SliceStable(report.Runs, func(i, j int) bool {
		if !report.Runs[i].Start.Equal(report.Runs[j].Start) {
			return report.Runs[i].Start.Before(report.Runs[j].Start)
		}
		return report.Runs[i].Tag < report.Runs[j].Tag
	})

	report.Buckets = bucketRuns(report.Runs, bucketWindows(opts.By, window))
	report.Phases = phaseMedians(report.Runs, window)
	return report, nil
}

// selectInFlight returns the runs of live pairs, read from the records branches in src.Branches.
// Only a pair records branch counts: a branch other than src.RecordsBranch whose name is the one fabricengine.RecordsBranchName forms from a slug.
// A branch whose tip is the commit of one of its slug's archive tags is landed and skipped; a branch whose slug opts.Live names is in flight; any other is abandoned, left out with a finding.
// A pair not seeded yet, or seeded to another recipe, is left out silently, and a record that does not decode is a finding.
func selectInFlight(src Sources, opts TimingOptions, findings *[]Finding) ([]RunRow, error) {
	live := map[string]bool{}
	for _, slug := range opts.Live {
		live[slug] = true
	}
	taggedTips := map[string]map[string]bool{}
	newestTag := map[string]time.Time{}
	for _, tag := range src.Tags {
		slug, named := fabricengine.ArchiveTagSlug(tag.Name)
		if !named {
			continue
		}
		if taggedTips[slug] == nil {
			taggedTips[slug] = map[string]bool{}
		}
		taggedTips[slug][tag.Commit] = true
		if tag.Committed.After(newestTag[slug]) {
			newestTag[slug] = tag.Committed
		}
	}

	var batten *battenIndex
	var rows []RunRow
	for _, branch := range src.Branches {
		if branch.Name == src.RecordsBranch {
			continue
		}
		slug, parsed := pairSlug(branch.Name)
		if !parsed || taggedTips[slug][branch.Commit] {
			continue
		}
		if !live[slug] {
			*findings = append(*findings, Finding{Tag: branch.Name, Error: "the branch has no archive tag at its tip and no live pair: left out as abandoned"})
			continue
		}
		rels, err := shedrun.TreeRunRels(slug)
		if err != nil {
			*findings = append(*findings, Finding{Tag: branch.Name, Error: err.Error()})
			continue
		}

		seedBytes, err := src.Records.FileAtRevision(branch.Commit, rels.Seed)
		if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reportengine: read %s at %s: %w", rels.Seed, branch.Name, err)
		}
		seed, err := shedrun.ParseSeed(seedBytes)
		if err != nil {
			*findings = append(*findings, Finding{Tag: branch.Name, File: rels.Seed, Error: err.Error()})
			continue
		}
		if seed.Recipe != shedrun.RecipeLoom {
			continue
		}
		statusBytes, err := src.Records.FileAtRevision(branch.Commit, rels.Status)
		if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reportengine: read %s at %s: %w", rels.Status, branch.Name, err)
		}
		status, err := shedengine.ParseStatusTolerant(statusBytes)
		if err != nil {
			*findings = append(*findings, Finding{Tag: branch.Name, File: rels.Status, Error: err.Error()})
			continue
		}

		if batten == nil {
			if batten, err = newBattenIndex(src); err != nil {
				return nil, err
			}
		}
		row := RunRow{
			Tag:             branch.Name,
			Slug:            slug,
			Commit:          branch.Commit,
			Committed:       branch.Committed.UTC(),
			Week:            isoWeekLabel(branch.Committed),
			Sources:         map[string]bool{},
			childHistory:    status.History,
			startedAt:       seed.StartedAt,
			inFlight:        true,
			currentProducer: status.CurrentProducer,
		}
		var earlier *time.Time
		if newest, tagged := newestTag[slug]; tagged {
			earlier = &newest
		}
		snapshots, battenFindings, err := batten.snapshotsOfRun(slug, branch.Name, earlier, src.SnapshotAt)
		if err != nil {
			return nil, err
		}
		*findings = append(*findings, battenFindings...)
		row.battenSnapshots = snapshots
		row.Sources[SourceBatten] = len(snapshots) > 0

		tip := gitrepo.TagRef{Name: branch.Name, Commit: branch.Commit, Committed: branch.Committed}
		wakeFindings, err := row.readWakes(src.Records, tip)
		if err != nil {
			return nil, err
		}
		stopFindings, err := row.readStops(src.Records, tip)
		if err != nil {
			return nil, err
		}
		roundFindings, err := row.readRounds(src.Records, tip)
		if err != nil {
			return nil, err
		}
		*findings = append(*findings, wakeFindings...)
		*findings = append(*findings, stopFindings...)
		*findings = append(*findings, roundFindings...)
		rows = append(rows, row)
	}
	return rows, nil
}

// pairSlug returns the slug whose pair records branch is named branch, and false when branch is no such name or names an empty slug.
// The records branch name suffix is read off RecordsBranchName, the one place that forms it.
func pairSlug(branch string) (string, bool) {
	slug, ok := strings.CutSuffix(branch, fabricengine.RecordsBranchName(""))
	return slug, ok && slug != ""
}

// bucketWindow is one day or ISO week of the report's window, clipped to it.
type bucketWindow struct {
	label string
	Interval
}

// bucketWindows cuts window into UTC days labelled 2026-10-15, or ISO weeks labelled as isoWeekLabel does, with Monday first.
// The first bucket opens at the window's start and the last closes at its end, so no bucket reaches outside the window.
func bucketWindows(by string, window Interval) []bucketWindow {
	var windows []bucketWindow
	cursor := window.From
	for cursor.Before(window.To) {
		day := time.Date(cursor.Year(), cursor.Month(), cursor.Day(), 0, 0, 0, 0, time.UTC)
		var label string
		var next time.Time
		if by == BucketWeek {
			weekStart := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
			label, next = isoWeekLabel(day), weekStart.AddDate(0, 0, 7)
		} else {
			label, next = day.Format("2006-01-02"), day.AddDate(0, 0, 1)
		}
		end := next
		if end.After(window.To) {
			end = window.To
		}
		windows = append(windows, bucketWindow{label: label, Interval: Interval{From: cursor, To: end}})
		cursor = next
	}
	return windows
}

// bucketRuns sums the runs over windows: each row interval and sleep is clipped to every bucket it overlaps, so an interval across midnight splits over both days.
func bucketRuns(runs []TimingRun, windows []bucketWindow) []TimingBucket {
	buckets := make([]TimingBucket, 0, len(windows))
	for _, w := range windows {
		bucket := TimingBucket{Label: w.label, Start: w.From, Phases: map[string]int64{}, Waits: map[string]int64{}}
		phaseTime := map[string]time.Duration{}
		waitTime := map[string]time.Duration{}
		var attemptParts []attemptPart
		var sleep, active time.Duration
		for _, run := range runs {
			attemptParts = append(attemptParts, run.Attempts.parts...)
			if !overlaps(run.span, w.Interval) {
				continue
			}
			bucket.Runs++
			sleepIn := lengthOf(clipIntervals(run.sleeps, w.Interval))
			sleep += sleepIn
			waitIn := lengthOf(clipIntervals(run.waitUnion, w.Interval))
			active += lengthOf(clipIntervals([]Interval{run.span}, w.Interval)) - sleepIn - waitIn
			for kind, parts := range run.waitParts {
				waitTime[kind] += lengthOf(clipIntervals(parts, w.Interval))
			}
			for phase, seconds := range run.Phases {
				if seconds != nil {
					phaseTime[phase] += 0
				}
			}
			for _, part := range run.parts {
				phaseTime[part.phase] += lengthOf(clipIntervals([]Interval{part.Interval}, w.Interval))
			}
		}
		for phase, held := range phaseTime {
			bucket.Phases[phase] = int64(held / time.Second)
		}
		for _, kind := range timingWaitKinds {
			bucket.Waits[kind] = int64(waitTime[kind] / time.Second)
		}
		bucket.Attempts = attemptsInWindow(attemptParts, w.Interval)
		bucket.SleepSeconds = int64(sleep / time.Second)
		bucket.ActiveSeconds = int64(active / time.Second)
		buckets = append(buckets, bucket)
	}
	return buckets
}

// phaseMedians returns every phase's median over the landed runs holding it whose archive tag is committed in window.
// A run in flight never enters a median, and a phase no landed run holds has nil seconds.
func phaseMedians(runs []TimingRun, window Interval) map[string]*PhaseMedian {
	medians := map[string]*PhaseMedian{}
	for _, phase := range timingPhases {
		var values []int64
		for _, run := range runs {
			if run.State != RunStateLanded || run.committed.Before(window.From) || run.committed.After(window.To) {
				continue
			}
			if seconds := run.Phases[phase]; seconds != nil {
				values = append(values, *seconds)
			}
		}
		medians[phase] = &PhaseMedian{Seconds: medianSeconds(values), Runs: len(values)}
	}
	return medians
}

// medianSeconds returns the median of values, nil for none.
// The median of an even count is the mean of the two middle values, rounded down to a whole second.
func medianSeconds(values []int64) *int64 {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	middle := len(sorted) / 2
	median := sorted[middle]
	if len(sorted)%2 == 0 {
		median = (sorted[middle-1] + sorted[middle]) / 2
	}
	return &median
}

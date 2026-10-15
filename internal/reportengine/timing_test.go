// timing_test.go tests ComputeTiming over a fake records repository: which runs it covers, each run's span and phases, machine sleep, the day and week buckets and the phase medians.

package reportengine

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// secondsAfter returns the time seconds after base.
func secondsAfter(base time.Time, seconds int) time.Time {
	return base.Add(time.Duration(seconds) * time.Second)
}

// entryAt is a done history entry of producer stamped when.
func entryAt(producer string, when time.Time) shedengine.HistoryEntry {
	return shedengine.HistoryEntry{Producer: producer, Outcome: shedengine.Done, At: when.Format(time.RFC3339)}
}

// changeAt is the Run-Shed entry batten records for the child's state change output, stamped when.
func changeAt(output string, when time.Time) shedengine.HistoryEntry {
	entry := entryAt(battenrecipe.NameRunShed, when)
	entry.Output = output
	entry.BudgetExempt = true
	return entry
}

// seedStartedAt is the text of a loom seed started at start.
func seedStartedAt(start time.Time) string {
	return fmt.Sprintf(`{"recipe":"loom","driver":"go","started_at":%q}`, start.Format(time.RFC3339))
}

// landedRun adds the archive tag of a full loom run of slug at hour holding the loom history and seed text, with each batten status committed in the hours before it.
// The loom history must hold a Finalize entry for the tag to be a full run.
func landedRun(w *world, slug, sha string, hour int, seed string, loom []shedengine.HistoryEntry, batten ...shedengine.Status) {
	for i, status := range batten {
		w.battenRecord(fmt.Sprintf("%s-b%d", sha, i), hour-len(batten)+i, slug, status)
	}
	w.tag(slug, sha, at(hour), seed, statusJSON(w.t, shedengine.Status{State: shedengine.StateDone, History: loom}))
}

// battenRecordOf is a batten status whose history is exactly entries, the first being the worktree-create row.
func battenRecordOf(entries ...shedengine.HistoryEntry) shedengine.Status {
	return shedengine.Status{State: shedengine.StateDone, History: entries}
}

// seconds is the pointer a TimingRun.Phases value holds.
func seconds(n int64) *int64 { return &n }

// runOf returns the run of slug in report.
func runOf(t *testing.T, report TimingReport, slug string) TimingRun {
	t.Helper()
	for _, run := range report.Runs {
		if run.Slug == slug {
			return run
		}
	}
	t.Fatalf("report holds no run of %q; runs = %+v", slug, report.Runs)
	return TimingRun{}
}

// wantPhases asserts the run's phases hold exactly want seconds, every phase not in want being nil.
func wantPhases(t *testing.T, run TimingRun, want map[string]int64) {
	t.Helper()
	for _, phase := range timingPhases {
		got := run.Phases[phase]
		expected, held := want[phase]
		switch {
		case held && (got == nil || *got != expected):
			t.Errorf("run %s phase %s = %v; want %d", run.Slug, phase, describe(got), expected)
		case !held && got != nil:
			t.Errorf("run %s phase %s = %d; want nil", run.Slug, phase, *got)
		}
	}
}

func describe(n *int64) string {
	if n == nil {
		return "nil"
	}
	return fmt.Sprint(*n)
}

// bucketOf returns the bucket labelled label.
func bucketOf(t *testing.T, report TimingReport, label string) TimingBucket {
	t.Helper()
	for _, bucket := range report.Buckets {
		if bucket.Label == label {
			return bucket
		}
	}
	t.Fatalf("report holds no bucket %q; buckets = %+v", label, report.Buckets)
	return TimingBucket{}
}

// wantFindings asserts the report holds exactly len(want) findings, the i-th naming tag and containing the i-th text.
func wantFindings(t *testing.T, report TimingReport, want ...[2]string) {
	t.Helper()
	if len(report.Findings) != len(want) {
		t.Fatalf("findings = %+v; want %d", report.Findings, len(want))
	}
	for i, finding := range report.Findings {
		if finding.Tag != want[i][0] || !strings.Contains(finding.Error, want[i][1]) {
			t.Errorf("finding %d = %+v; want tag %q and text containing %q", i, finding, want[i][0], want[i][1])
		}
	}
}

// loomRowPhases lists every loom row with the phase it counts under, in the order the rows run.
var loomRowPhases = []struct{ row, phase string }{
	{loomshed.NamePreflight, PhaseSetup},
	{loomshed.NameLoomPreflight, PhaseSetup},
	{loomshed.NameDiscussionWrite, PhaseDiscussion},
	{loomshed.NameDiscussionBouncer, PhaseDiscussion},
	{loomshed.NamePlanWrite, PhasePlan},
	{loomshed.NamePlanBouncer, PhasePlan},
	{loomshed.NameBatchifier, PhasePlan},
	{loomshed.NameWebster, PhaseWebster},
	{loomshed.NameWebsterBouncer, PhaseWebster},
	{loomshed.NameDescribe, PhaseWebster},
	{loomshed.NameMergeIn, PhaseMerge},
	{loomshed.NameMergeBouncer, PhaseMerge},
	{loomshed.NamePublish, PhasePublishFinalize},
	{loomshed.NamePRAlign, PhasePublishFinalize},
	{loomshed.NamePRGate, PhasePublishFinalize},
	{loomshed.NamePRRework, PhasePublishFinalize},
	{loomshed.NameDarnWrite, PhasePublishFinalize},
	{loomshed.NameDarnBouncer, PhasePublishFinalize},
	{loomshed.NameFinalize, PhasePublishFinalize},
	{loomshed.NameFrictionReflect, PhasePublishFinalize},
	{loomshed.NameDiscussionBurlerReview, PhaseDiscussion},
	{loomshed.NameDiscussionBurlerFix, PhaseDiscussion},
	{loomshed.NamePlanBurlerReview, PhasePlan},
	{loomshed.NamePlanBurlerFix, PhasePlan},
	{loomshed.NameWebsterBurlerReview, PhaseWebster},
	{loomshed.NameWebsterBurlerFix, PhaseWebster},
	{loomshed.NameMergeBurlerReview, PhaseMerge},
	{loomshed.NameMergeBurlerFix, PhaseMerge},
	{loomshed.NameDarnBurlerReview, PhasePublishFinalize},
	{loomshed.NameDarnBurlerFix, PhasePublishFinalize},
}

func TestComputeTiming(t *testing.T) {
	t.Parallel()

	// run is the run every row below starts from: at(1) is its opening, and each row's stamps are offsets from it.
	run := at(1)
	tests := []struct {
		name string
		// since defaults to at(0), and by to BucketDay.
		since  time.Time
		by     string
		live   []string
		sleeps []Interval
		setup  func(w *world)
		check  func(t *testing.T, got TimingReport)
	}{
		{
			name: "a landed run with both histories sums its phases and sleep to its span, a batten row between two loom rows splitting their gap",
			sleeps: []Interval{
				{From: secondsAfter(run, 720), To: secondsAfter(run, 960)},
			},
			setup: func(w *world) {
				loom := []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 300)),
					entryAt(loomshed.NameDiscussionWrite, secondsAfter(run, 600)),
					entryAt(loomshed.NamePlanWrite, secondsAfter(run, 1200)),
					entryAt(loomshed.NameWebster, secondsAfter(run, 1800)),
					entryAt(loomshed.NameMergeIn, secondsAfter(run, 2040)),
					entryAt(loomshed.NamePublish, secondsAfter(run, 2160)),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 2280)),
				}
				batten := battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, run),
					entryAt(battenrecipe.NameSeedChild, secondsAfter(run, 120)),
					changeAt("child running → blocked at Plan-Write", secondsAfter(run, 1500)),
					changeAt("child running → done at Finalize", secondsAfter(run, 2400)),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 2700)),
					entryAt(battenrecipe.NameWorktreeTeardown, secondsAfter(run, 2760)),
				)
				landedRun(w, "both", "t1", 3, seedText("loom"), loom, batten)
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "both")
				if got1.State != RunStateLanded || got1.Tag != tagName("both", "t1") || !got1.Start.Equal(run) || !got1.End.Equal(secondsAfter(run, 2760)) {
					t.Errorf("run = %s %s %v-%v; want landed under its archive tag over the batten record's span", got1.State, got1.Tag, got1.Start, got1.End)
				}
				// The Plan-Write row's gap to Webster is split by the Run-Shed row at 1500, which counts under the producer its change names.
				wantPhases(t, got1, map[string]int64{
					PhaseSetup: 300, PhaseDiscussion: 300, PhasePlan: 660, PhaseWebster: 300,
					PhaseMerge: 240, PhasePublishFinalize: 360, PhaseLand: 360,
				})
				if got1.SpanSeconds != 2760 || got1.SleepSeconds != 240 || got1.ActiveSeconds != 2520 {
					t.Errorf("span, sleep, active = %d, %d, %d; want 2760, 240, 2520", got1.SpanSeconds, got1.SleepSeconds, got1.ActiveSeconds)
				}
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none", got.Findings)
				}
			},
		},
		{
			name: "a folded entry's last_at closes its interval, and a run without the batten source opens at the seed's started_at with land nil",
			setup: func(w *world) {
				folded := entryAt(loomshed.NamePlanWrite, secondsAfter(run, 1200))
				folded.Repeats = 2
				folded.LastAt = secondsAfter(run, 1800).Format(time.RFC3339)
				landedRun(w, "folded", "t1", 3, seedStartedAt(run), []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 600)),
					folded,
					entryAt(loomshed.NameFinalize, secondsAfter(run, 2100)),
				})
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "folded")
				wantPhases(t, got1, map[string]int64{PhaseSetup: 600, PhasePlan: 1200, PhasePublishFinalize: 300})
				if got1.SpanSeconds != 2100 || !got1.Start.Equal(run) {
					t.Errorf("span, start = %d, %v; want 2100 from the seed's started_at", got1.SpanSeconds, got1.Start)
				}
			},
		},
		{
			name: "an out-of-order stamp gives a zero interval and a finding, and the next interval starts from the latest stamp",
			setup: func(w *world) {
				landedRun(w, "disorder", "t1", 3, seedStartedAt(run), []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 600)),
					entryAt(loomshed.NamePlanWrite, secondsAfter(run, 1800)),
					entryAt(loomshed.NameWebster, secondsAfter(run, 1200)),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 2400)),
				})
			},
			check: func(t *testing.T, got TimingReport) {
				wantPhases(t, runOf(t, got, "disorder"), map[string]int64{PhaseSetup: 600, PhasePlan: 1200, PhaseWebster: 0, PhasePublishFinalize: 600})
				wantFindings(t, got, [2]string{tagName("disorder", "t1"), "row Webster is stamped"})
			},
		},
		{
			name: "an unknown row counts under other with a finding",
			setup: func(w *world) {
				landedRun(w, "mystery", "t1", 3, seedStartedAt(run), []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 600)),
					entryAt("Mystery-Row", secondsAfter(run, 1200)),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 1800)),
				})
			},
			check: func(t *testing.T, got TimingReport) {
				wantPhases(t, runOf(t, got, "mystery"), map[string]int64{PhaseSetup: 600, PhaseOther: 600, PhasePublishFinalize: 600})
				wantFindings(t, got, [2]string{tagName("mystery", "t1"), "Mystery-Row names no phase"})
			},
		},
		{
			name: "a run with neither a batten record nor a seed stamp opens at its first loom row with a finding",
			setup: func(w *world) {
				landedRun(w, "unstamped", "t1", 3, seedText("loom"), []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 600)),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 1200)),
				})
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "unstamped")
				wantPhases(t, got1, map[string]int64{PhaseSetup: 0, PhasePublishFinalize: 600})
				if !got1.Start.Equal(secondsAfter(run, 600)) {
					t.Errorf("start = %v; want the first loom row's stamp", got1.Start)
				}
				wantFindings(t, got, [2]string{tagName("unstamped", "t1"), "neither the batten record nor the seed carries a stamp"})
			},
		},
		{
			name: "a row with an empty at gives a zero interval and a finding while the span opens and closes at the stamped rows",
			setup: func(w *world) {
				blank := entryAt(loomshed.NamePlanWrite, run)
				blank.At = ""
				landedRun(w, "blank", "t1", 3, seedStartedAt(run), []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 300)),
					blank,
					entryAt(loomshed.NameFinalize, secondsAfter(run, 1500)),
				})
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "blank")
				wantPhases(t, got1, map[string]int64{PhaseSetup: 300, PhasePlan: 0, PhasePublishFinalize: 1200})
				if !got1.Start.Equal(run) || !got1.End.Equal(secondsAfter(run, 1500)) {
					t.Errorf("span = %v-%v; want the stamped rows' %v-%v", got1.Start, got1.End, run, secondsAfter(run, 1500))
				}
				wantFindings(t, got, [2]string{tagName("blank", "t1"), "loom history row 2 (Plan-Write) has no stamp"})
			},
		},
		{
			name: "a history holding one row of every table entry counts each under its phase, batten rows before, between and after its Run-Shed entries included",
			setup: func(w *world) {
				loom := make([]shedengine.HistoryEntry, len(loomRowPhases))
				for k, row := range loomRowPhases {
					loom[k] = entryAt(row.row, secondsAfter(run, 600+60*k))
				}
				batten := battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, run),
					entryAt(battenrecipe.NameSeedChild, secondsAfter(run, 60)),
					// Names no producer and follows no loom row.
					changeAt("child running → paused", secondsAfter(run, 300)),
					// Lies between two Run-Shed entries.
					entryAt(battenrecipe.NameOpenWindow, secondsAfter(run, 360)),
					// Names a producer.
					changeAt("child running → blocked at Plan-Write", secondsAfter(run, 1455)),
					// Names no producer and follows a loom row.
					changeAt("child blocked → running", secondsAfter(run, 2010)),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 2400)),
					entryAt(battenrecipe.NameWorktreeTeardown, secondsAfter(run, 2460)),
				)
				landedRun(w, "every", "t1", 3, seedText("loom"), loom, batten)
			},
			check: func(t *testing.T, got TimingReport) {
				wantPhases(t, runOf(t, got, "every"), map[string]int64{
					PhaseSetup: 600, PhaseDiscussion: 240, PhasePlan: 345, PhaseWebster: 270,
					PhaseMerge: 240, PhasePublishFinalize: 585, PhaseLand: 180,
				})
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none, since every row names a phase", got.Findings)
				}
			},
		},
		{
			name: "an interval across UTC midnight splits over two day buckets",
			setup: func(w *world) {
				start := at(23)
				landedRun(w, "midnight", "t1", 30, seedStartedAt(start), []shedengine.HistoryEntry{
					entryAt(loomshed.NamePlanWrite, at(25)),
					entryAt(loomshed.NameFinalize, secondsAfter(at(25), 1800)),
				})
			},
			check: func(t *testing.T, got TimingReport) {
				first, second := bucketOf(t, got, "2026-01-05"), bucketOf(t, got, "2026-01-06")
				if first.Runs != 1 || first.Phases[PhasePlan] != 3600 || first.ActiveSeconds != 3600 {
					t.Errorf("bucket 2026-01-05 = %+v; want one run, plan 3600 and active 3600", first)
				}
				if second.Runs != 1 || second.Phases[PhasePlan] != 3600 || second.Phases[PhasePublishFinalize] != 1800 || second.ActiveSeconds != 5400 {
					t.Errorf("bucket 2026-01-06 = %+v; want one run, plan 3600, publish-finalize 1800 and active 5400", second)
				}
				if last := got.Buckets[len(got.Buckets)-1]; last.Label != "2026-01-13" || !last.Start.Equal(at(192)) {
					t.Errorf("last bucket = %+v; want 2026-01-13 opening at its midnight", last)
				}
			},
		},
		{
			name: "the same run under week buckets lands in its ISO week",
			by:   BucketWeek,
			setup: func(w *world) {
				landedRun(w, "midnight", "t1", 30, seedStartedAt(at(23)), []shedengine.HistoryEntry{
					entryAt(loomshed.NamePlanWrite, at(25)),
					entryAt(loomshed.NameFinalize, secondsAfter(at(25), 1800)),
				})
			},
			check: func(t *testing.T, got TimingReport) {
				if len(got.Buckets) != 2 || got.Buckets[0].Label != "2026-W02" || got.Buckets[1].Label != "2026-W03" {
					t.Fatalf("buckets = %+v; want 2026-W02 and 2026-W03", got.Buckets)
				}
				week := got.Buckets[0]
				if week.Runs != 1 || week.Phases[PhasePlan] != 7200 || week.Phases[PhasePublishFinalize] != 1800 || got.Buckets[1].Runs != 0 {
					t.Errorf("buckets = %+v; want the run wholly in 2026-W02", got.Buckets)
				}
			},
		},
		{
			name:  "a run starting before since keeps its span and its bucket list starts at since",
			since: at(24),
			setup: func(w *world) {
				landedRun(w, "midnight", "t1", 30, seedStartedAt(at(23)), []shedengine.HistoryEntry{
					entryAt(loomshed.NamePlanWrite, at(25)),
					entryAt(loomshed.NameFinalize, secondsAfter(at(25), 1800)),
				})
			},
			check: func(t *testing.T, got TimingReport) {
				if got.Buckets[0].Label != "2026-01-06" || !got.Buckets[0].Start.Equal(at(24)) || got.Buckets[0].Phases[PhasePlan] != 3600 {
					t.Errorf("first bucket = %+v; want 2026-01-06 opening at since with plan 3600", got.Buckets[0])
				}
				if got1 := runOf(t, got, "midnight"); !got1.Start.Equal(at(23)) {
					t.Errorf("start = %v; want the run's own opening before since", got1.Start)
				}
			},
		},
		{
			name:  "a since after the snapshot gives no run and no bucket",
			since: secondsAfter(snapshotAt, 86400),
			setup: func(w *world) {
				landedRun(w, "early", "t1", 3, seedStartedAt(run), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, secondsAfter(run, 600))})
			},
			check: func(t *testing.T, got TimingReport) {
				if len(got.Runs) != 0 || len(got.Buckets) != 0 || got.Phases[PhasePublishFinalize].Seconds != nil {
					t.Errorf("report = %+v; want no run, no bucket and nil medians", got)
				}
			},
		},
		{
			name: "two overlapping runs add their time in the bucket they share",
			setup: func(w *world) {
				landedRun(w, "first", "t1", 14, seedStartedAt(at(10)), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, at(11))})
				landedRun(w, "second", "t2", 14, seedStartedAt(secondsAfter(at(10), 1800)), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, secondsAfter(at(11), 1800))})
			},
			check: func(t *testing.T, got TimingReport) {
				bucket := bucketOf(t, got, "2026-01-05")
				if bucket.Runs != 2 || bucket.Phases[PhasePublishFinalize] != 7200 || bucket.ActiveSeconds != 7200 {
					t.Errorf("bucket = %+v; want two runs adding to publish-finalize 7200 and active 7200", bucket)
				}
			},
		},
		{
			name: "a partial run enters neither the run table nor any bucket",
			setup: func(w *world) {
				w.tag("partial", "t1", at(14), seedText("loom"), statusText(w.t, loomshed.NamePlanWrite))
			},
			check: func(t *testing.T, got TimingReport) {
				if len(got.Runs) != 0 {
					t.Errorf("runs = %+v; want none", got.Runs)
				}
				for _, bucket := range got.Buckets {
					if bucket.Runs != 0 {
						t.Errorf("bucket = %+v; want no run", bucket)
					}
				}
			},
		},
		{
			name: "two landed runs' phase median is the rounded-down mean of their two values",
			setup: func(w *world) {
				landedRun(w, "short", "t1", 14, seedStartedAt(at(10)), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, secondsAfter(at(10), 300))})
				landedRun(w, "long", "t2", 14, seedStartedAt(at(11)), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, secondsAfter(at(11), 301))})
			},
			check: func(t *testing.T, got TimingReport) {
				median := got.Phases[PhasePublishFinalize]
				if median.Runs != 2 || median.Seconds == nil || *median.Seconds != 300 {
					t.Errorf("publish-finalize median = %+v; want 300 over two runs", median)
				}
				if got.Phases[PhaseLand].Seconds != nil || got.Phases[PhaseLand].Runs != 0 {
					t.Errorf("land median = %+v; want nil over no run", got.Phases[PhaseLand])
				}
			},
		},
		{
			name: "a run in flight runs to the snapshot under its current producer's phase and enters no median",
			live: []string{"flight"},
			setup: func(w *world) {
				landedRun(w, "done", "t1", 14, seedStartedAt(at(10)), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, secondsAfter(at(10), 300))})
				start := at(199)
				status := shedengine.Status{
					State:           shedengine.StateRunning,
					CurrentProducer: loomshed.NamePlanWrite,
					History: []shedengine.HistoryEntry{
						entryAt(loomshed.NamePreflight, secondsAfter(start, 600)),
						entryAt(loomshed.NameDiscussionWrite, secondsAfter(start, 1800)),
					},
				}
				w.pairBranch("flight", "pf1", secondsAfter(start, 1900), seedStartedAt(start), statusJSON(w.t, status), nil)
			},
			check: func(t *testing.T, got TimingReport) {
				flight := runOf(t, got, "flight")
				if flight.State != RunStateInFlight || flight.Tag != fabricengine.RecordsBranchName("flight") || !flight.End.Equal(snapshotAt) {
					t.Errorf("run = %s %s ending %v; want in-flight under its branch name and ending at the snapshot", flight.State, flight.Tag, flight.End)
				}
				wantPhases(t, flight, map[string]int64{PhaseSetup: 600, PhaseDiscussion: 1200, PhasePlan: 1800})
				if median := got.Phases[PhasePublishFinalize]; median.Runs != 1 || *median.Seconds != 300 {
					t.Errorf("publish-finalize median = %+v; want the landed run's 300 alone", median)
				}
				if median := got.Phases[PhasePlan]; median.Runs != 0 || median.Seconds != nil {
					t.Errorf("plan median = %+v; want none, since a run in flight enters no median", median)
				}
				if bucket := bucketOf(t, got, "2026-01-13"); bucket.Runs != 1 || bucket.Phases[PhasePlan] != 1800 {
					t.Errorf("bucket 2026-01-13 = %+v; want the run in flight with plan 1800", bucket)
				}
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none", got.Findings)
				}
			},
		},
		{
			name: "a pair branch at its slug's archive tag commit is the landed run and not a second one",
			live: []string{"landed"},
			setup: func(w *world) {
				landedRun(w, "landed", "t1", 14, seedStartedAt(at(10)), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, secondsAfter(at(10), 300))})
				w.branches = append(w.branches, gitrepo.BranchRef{Name: fabricengine.RecordsBranchName("landed"), Commit: "t1", Committed: at(14)})
			},
			check: func(t *testing.T, got TimingReport) {
				if len(got.Runs) != 1 || got.Runs[0].State != RunStateLanded || len(got.Findings) != 0 {
					t.Errorf("runs, findings = %+v, %+v; want the one landed run and no finding", got.Runs, got.Findings)
				}
			},
		},
		{
			name: "the records branch itself and a branch no pair name parses are left out with no finding",
			live: []string{"feature"},
			setup: func(w *world) {
				w.branches = append(w.branches, gitrepo.BranchRef{Name: "feature", Commit: "unread", Committed: at(14)})
			},
			check: func(t *testing.T, got TimingReport) {
				if len(got.Runs) != 0 || len(got.Findings) != 0 {
					t.Errorf("runs, findings = %+v, %+v; want none", got.Runs, got.Findings)
				}
			},
		},
		{
			name: "a live pair not seeded yet, or seeded to another recipe, is left out with no finding",
			live: []string{"unseeded", "statusless", "other"},
			setup: func(w *world) {
				status := statusJSON(w.t, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: loomshed.NamePreflight})
				w.pairBranch("unseeded", "p1", at(198), "", status, nil)
				w.pairBranch("statusless", "p2", at(198), seedStartedAt(at(198)), "", nil)
				w.pairBranch("other", "p3", at(198), `{"recipe":"darn","driver":"go"}`, status, nil)
			},
			check: func(t *testing.T, got TimingReport) {
				if len(got.Runs) != 0 || len(got.Findings) != 0 {
					t.Errorf("runs, findings = %+v, %+v; want none", got.Runs, got.Findings)
				}
			},
		},
		{
			name: "a live pair whose status does not decode is left out with a finding",
			live: []string{"broken"},
			setup: func(w *world) {
				w.pairBranch("broken", "p1", at(198), seedStartedAt(at(198)), "{bad", nil)
			},
			check: func(t *testing.T, got TimingReport) {
				if len(got.Runs) != 0 {
					t.Errorf("runs = %+v; want none", got.Runs)
				}
				wantFindings(t, got, [2]string{fabricengine.RecordsBranchName("broken"), ""})
			},
		},
		{
			name: "a branch neither tagged nor live is left out with a finding",
			setup: func(w *world) {
				status := statusJSON(w.t, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: loomshed.NamePreflight})
				w.pairBranch("abandoned", "p1", at(198), seedStartedAt(at(198)), status, nil)
			},
			check: func(t *testing.T, got TimingReport) {
				if len(got.Runs) != 0 {
					t.Errorf("runs = %+v; want none", got.Runs)
				}
				wantFindings(t, got, [2]string{fabricengine.RecordsBranchName("abandoned"), "no archive tag at its tip and no live pair"})
			},
		},
		{
			name: "a live slug reused after a tag reads only the batten snapshots after that tag",
			live: []string{"reuse"},
			setup: func(w *world) {
				landedRun(w, "reuse", "t1", 150, seedText("loom"),
					[]shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, at(149))},
					battenRecordOf(entryAt(battenrecipe.NameWorktreeCreate, at(148))))
				// Teardown removes the earlier run's batten record, so the next one is a fresh seed.
				w.battenDeleted("reuse-del", 160, "reuse")
				w.battenRecord("reuse-b", 195, "reuse", battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, at(198)),
					entryAt(battenrecipe.NameSeedChild, secondsAfter(at(198), 120)),
				))
				status := shedengine.Status{
					State:           shedengine.StateRunning,
					CurrentProducer: loomshed.NameDiscussionWrite,
					History:         []shedengine.HistoryEntry{entryAt(loomshed.NamePreflight, secondsAfter(at(198), 600))},
				}
				w.pairBranch("reuse", "pf1", at(199), seedText("loom"), statusJSON(w.t, status), nil)
			},
			check: func(t *testing.T, got TimingReport) {
				var flight TimingRun
				for _, candidate := range got.Runs {
					if candidate.State == RunStateInFlight {
						flight = candidate
					}
				}
				if !flight.Start.Equal(at(198)) {
					t.Fatalf("in-flight run start = %v; want the batten record after the tag, opening at %v", flight.Start, at(198))
				}
				wantPhases(t, flight, map[string]int64{PhaseSetup: 600, PhaseDiscussion: 6600})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := newWorld(t)
			tt.setup(w)
			opts := TimingOptions{Since: tt.since, By: tt.by, Live: tt.live, Sleeps: tt.sleeps, SleepNote: "from the test"}
			if opts.Since.IsZero() {
				opts.Since = at(0)
			}
			if opts.By == "" {
				opts.By = BucketDay
			}

			got, err := ComputeTiming(w.sources(), opts)
			if err != nil {
				t.Fatalf("ComputeTiming error = %v", err)
			}
			if got.SleepNote != "from the test" || got.Selection.By != opts.By || !got.Selection.SnapshotAt.Equal(snapshotAt) {
				t.Errorf("selection, sleep note = %+v, %q; want the options and snapshot told through", got.Selection, got.SleepNote)
			}
			if got.Runs == nil || got.Buckets == nil || got.Findings == nil || got.Phases == nil {
				t.Errorf("report = %+v; want every slice non-nil", got)
			}
			tt.check(t, got)
		})
	}
}

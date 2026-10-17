// timing_test.go tests ComputeTiming over a fake records repository: which runs it covers, each run's span and phases, machine sleep, the day and week buckets and the phase medians.

package reportengine

import (
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/haltnote"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
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
	landedRunWith(w, slug, sha, hour, seed, loom, nil, batten...)
}

// landedRunWith is landedRun with extra record files, keyed by tree-relative path, in the tagged tree.
func landedRunWith(w *world, slug, sha string, hour int, seed string, loom []shedengine.HistoryEntry, extra map[string]string, batten ...shedengine.Status) {
	for i, status := range batten {
		w.battenRecord(fmt.Sprintf("%s-b%d", sha, i), hour-len(batten)+i, slug, status)
	}
	w.tagWith(slug, sha, at(hour), seed, statusJSON(w.t, shedengine.Status{State: shedengine.StateDone, History: loom}), extra)
}

// parentRoundTimed returns the files of parent-review round number with a request opened at opened (none when zero), a verdict recorded at verdict (none when zero) and an alignment record spawned at spawned (none when zero).
func parentRoundTimed(number int, opened, verdict, spawned time.Time) map[string]string {
	dir := path.Join(filepath.ToSlash(loomengine.LoomParentReviewDirRel()), fmt.Sprintf("round-%d", number))
	files := map[string]string{path.Join(dir, "delivery.json"): `{}`}
	if !opened.IsZero() {
		files[path.Join(dir, "request.json")] = fmt.Sprintf(`{"round":%d,"state":"open","opened_at":%q}`, number, opened.Format(time.RFC3339))
	}
	if !verdict.IsZero() {
		files[path.Join(dir, "verdict.json")] = fmt.Sprintf(`{"kind":"approve","recorded_at":%q}`, verdict.Format(time.RFC3339))
	}
	if !spawned.IsZero() {
		files[path.Join(dir, "alignment.json")] = fmt.Sprintf(`{"state":"applied","spawned_at":%q}`, spawned.Format(time.RFC3339))
	}
	return files
}

// arbiterSpawnedFiles returns the files of arbiter attempt number of slug, spawned at spawned.
func arbiterSpawnedFiles(t *testing.T, slug string, number int, spawned time.Time) map[string]string {
	return arbiterAttemptFiles(t, slug, number, loomengine.ArbiterAttempt{SpawnedAt: spawned}, "", false)
}

// alignmentSpawnedFiles returns the files of attempt number of slug's alignment agent at point, spawned at spawned and recording applied.
func alignmentSpawnedFiles(t *testing.T, slug, point string, number int, spawned time.Time, applied string) map[string]string {
	t.Helper()

	dir := path.Join(recordPaths(t, slug).Alignment, point, fmt.Sprintf("attempt-%d", number))
	return map[string]string{
		path.Join(dir, "attempt.json"): fmt.Sprintf(`{"number":%d,"point":%q,"token":"tok","spawned_at":%q,"applied":%q}`, number, point, spawned.Format(time.RFC3339), applied),
	}
}

// stuckAt is a stuck history entry of producer stamped when.
func stuckAt(producer string, when time.Time) shedengine.HistoryEntry {
	entry := entryAt(producer, when)
	entry.Outcome = shedengine.Stuck
	return entry
}

// gotoAt is a goto history entry of producer stamped when, written by the caller labelled by.
func gotoAt(producer string, when time.Time, by string) shedengine.HistoryEntry {
	entry := entryAt(producer, when)
	entry.Outcome = shedengine.OutcomeGoto
	entry.By = by
	return entry
}

// settleRecord is a settle record at the stop key (producer, length, state) by settler taking action.
func settleRecord(producer string, length int, state, settler, action string) shedrun.SettleRecord {
	return shedrun.SettleRecord{
		StopKey: shedrun.StopKey{CurrentProducer: producer, HistoryLength: length, State: state},
		Settler: settler,
		Action:  action,
		Outcome: "ok",
		At:      at(1),
	}
}

// settleFile returns the text of a settle-records file holding records.
func settleFile(t *testing.T, records ...shedrun.SettleRecord) string {
	t.Helper()

	var lines []string
	for _, record := range records {
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal settle record: %v", err)
		}
		lines = append(lines, string(data))
	}
	return strings.Join(lines, "\n") + "\n"
}

// blockedNote is the files of the halt note file name for a blocked halt at producer.
func blockedNote(name, producer string) map[string]string {
	return haltNoteFiles("", name, haltnote.Note{Producer: producer, State: "blocked", Anomaly: "watchdog", Reason: "stalled"})
}

// stopRun adds a landed run of slug whose loom history is loom and batten history is batten, with the loom file's settle records and batten's, and extra files in the tagged tree.
func stopRun(w *world, slug string, loom, batten []shedengine.HistoryEntry, loomRecords, battenRecords []shedrun.SettleRecord, extra map[string]string) {
	files := mergeFiles(extra)
	if len(loomRecords) > 0 {
		files[recordPaths(w.t, slug).SettleRecords] = settleFile(w.t, loomRecords...)
	}
	w.battenSettles("bs-"+slug, 2, slug, battenRecordOf(batten...), battenRecords...)
	landedRunWith(w, slug, "t1", 3, seedText("loom"), loom, files)
}

// stopsOfKind returns the run's stops of kind.
func stopsOfKind(run TimingRun, kind string) []TimingStop {
	var stops []TimingStop
	for _, stop := range run.Stops {
		if stop.Kind == kind {
			stops = append(stops, stop)
		}
	}
	return stops
}

// wantSettlers asserts the run's stops of kind carry exactly the settlers in order.
func wantSettlers(t *testing.T, run TimingRun, kind string, want ...string) {
	t.Helper()
	var got []string
	for _, stop := range stopsOfKind(run, kind) {
		got = append(got, stop.Settler)
	}
	if !slices.Equal(got, want) {
		t.Errorf("run %s %s settlers = %v; want %v", run.Slug, kind, got, want)
	}
}

// wantWaits asserts the run's waits hold exactly want seconds, every kind not in want being zero.
func wantWaits(t *testing.T, run TimingRun, want map[string]int64) {
	t.Helper()
	for _, kind := range timingWaitKinds {
		got, held := run.Waits[kind]
		if !held || got != want[kind] {
			t.Errorf("run %s wait %s = %d (held %t); want %d", run.Slug, kind, got, held, want[kind])
		}
	}
	waits := int64(0)
	for _, seconds := range run.Waits {
		waits += seconds
	}
	if run.ActiveSeconds+waits+run.SleepSeconds != run.SpanSeconds {
		t.Errorf("run %s active %d + waits %d + sleep %d != span %d", run.Slug, run.ActiveSeconds, waits, run.SleepSeconds, run.SpanSeconds)
	}
}

// wantFigure asserts an attempt figure's count and seconds, nil seconds when wantSeconds is negative.
func wantFigure(t *testing.T, name string, got AttemptFigure, count int, wantSeconds int64) {
	t.Helper()
	switch {
	case got.Count != count:
		t.Errorf("%s count = %d; want %d", name, got.Count, count)
	case wantSeconds < 0 && got.Seconds != nil:
		t.Errorf("%s seconds = %d; want nil", name, *got.Seconds)
	case wantSeconds >= 0 && (got.Seconds == nil || *got.Seconds != wantSeconds):
		t.Errorf("%s seconds = %v; want %d", name, describe(got.Seconds), wantSeconds)
	}
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

	// convergenceHalts is the halt count convergence reports over the one row that sets it in its setup and reads it in its check.
	var convergenceHalts int
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
				// The blocked change at 1500 waits until the Webster row at 1800.
				if got1.SpanSeconds != 2760 || got1.SleepSeconds != 240 || got1.ActiveSeconds != 2220 {
					t.Errorf("span, sleep, active = %d, %d, %d; want 2760, 240, 2220", got1.SpanSeconds, got1.SleepSeconds, got1.ActiveSeconds)
				}
				wantWaits(t, got1, map[string]int64{WaitHalt: 300})
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none", got.Findings)
				}
			},
		},
		{
			name: "a sleep with fractional seconds is cut to whole seconds, so the phases plus sleep equal the span",
			sleeps: []Interval{
				{From: secondsAfter(run, 720).Add(400 * time.Millisecond), To: secondsAfter(run, 960).Add(700 * time.Millisecond)},
			},
			setup: func(w *world) {
				loom := []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 300)),
					entryAt(loomshed.NameDiscussionWrite, secondsAfter(run, 1200)),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 1500)),
				}
				landedRun(w, "fraction", "t1", 3, seedStartedAt(run), loom)
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "fraction")
				var phases int64
				for _, seconds := range got1.Phases {
					if seconds != nil {
						phases += *seconds
					}
				}
				if phases+got1.SleepSeconds != got1.SpanSeconds {
					t.Errorf("phases %d + sleep %d != span %d", phases, got1.SleepSeconds, got1.SpanSeconds)
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
		{
			name: "a parent-review round overlapping a halt counts once as parent_review and the halt keeps only the rest, active plus waits plus sleep summing to the span",
			setup: func(w *world) {
				batten := battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, run),
					changeAt("child running → blocked at "+loomshed.NamePlanWrite, secondsAfter(run, 600)),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 2500)),
				)
				loom := []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 300)),
					entryAt(loomshed.NamePlanWrite, secondsAfter(run, 1200)),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 2400)),
				}
				landedRunWith(w, "overlap", "t1", 3, seedText("loom"), loom, parentRoundTimed(1, secondsAfter(run, 300), secondsAfter(run, 900), time.Time{}), batten)
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "overlap")
				wantWaits(t, got1, map[string]int64{WaitParentReview: 600, WaitHalt: 300})
				if got1.SpanSeconds != 2500 || got1.ActiveSeconds != 1600 {
					t.Errorf("span, active = %d, %d; want 2500, 1600", got1.SpanSeconds, got1.ActiveSeconds)
				}
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none", got.Findings)
				}
			},
		},
		{
			name: "a pause before Plan-Write counts under paused and not plan, splits over the day buckets it crosses and leaves active with an overlapping wait taken out once",
			setup: func(w *world) {
				start := at(23)
				batten := battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, start),
					changeAt("child running → paused at "+loomshed.NamePlanWrite, secondsAfter(start, 1260)),
					changeAt("child paused → running at "+loomshed.NamePlanWrite, secondsAfter(start, 7260)),
					changeAt("child running → done at "+loomshed.NameFinalize, secondsAfter(start, 8400)),
					entryAt(battenrecipe.NameLand, secondsAfter(start, 8500)),
				)
				loom := []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(start, 600)),
					entryAt(loomshed.NameDiscussionWrite, secondsAfter(start, 1200)),
					entryAt(loomshed.NamePlanWrite, secondsAfter(start, 8000)),
					entryAt(loomshed.NameFinalize, secondsAfter(start, 8300)),
				}
				round := parentRoundTimed(1, secondsAfter(start, 7000), secondsAfter(start, 7500), time.Time{})
				landedRunWith(w, "paused", "t1", 30, seedText("loom"), loom, round, batten)
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "paused")
				wantPhases(t, got1, map[string]int64{
					PhaseSetup: 600, PhaseDiscussion: 600, PhasePlan: 800, PhasePaused: 6000,
					PhasePublishFinalize: 400, PhaseLand: 100,
				})
				if review := got1.Waits[WaitParentReview]; review != 500 {
					t.Errorf("parent_review wait = %d; want 500", review)
				}
				// The paused 1260-7260 and the round 7000-7500 overlap by 260 seconds, which active loses once.
				if got1.SpanSeconds != 8500 || got1.ActiveSeconds != 2260 {
					t.Errorf("span, active = %d, %d; want 8500, 2260", got1.SpanSeconds, got1.ActiveSeconds)
				}
				first, second := bucketOf(t, got, "2026-01-05"), bucketOf(t, got, "2026-01-06")
				if first.Phases[PhasePaused] != 2340 || first.ActiveSeconds != 1260 {
					t.Errorf("bucket 2026-01-05 = %+v; want paused 2340 and active 1260", first)
				}
				if second.Phases[PhasePaused] != 3660 || second.ActiveSeconds != 1000 {
					t.Errorf("bucket 2026-01-06 = %+v; want paused 3660 and active 1000", second)
				}
				if median := got.Phases[PhasePaused]; median.Runs != 1 || median.Seconds == nil || *median.Seconds != 6000 {
					t.Errorf("paused median = %+v; want 6000 over one run", median)
				}
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none", got.Findings)
				}
			},
		},
		{
			name: "a run in flight whose child stands paused runs to the snapshot under paused",
			live: []string{"flight"},
			setup: func(w *world) {
				start := at(198)
				w.battenRecord("fl-b", 195, "flight", battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, start),
					changeAt("child running → paused at "+loomshed.NamePlanWrite, secondsAfter(start, 1800)),
				))
				status := shedengine.Status{
					State:           shedengine.StatePaused,
					CurrentProducer: loomshed.NamePlanWrite,
					History: []shedengine.HistoryEntry{
						entryAt(loomshed.NamePreflight, secondsAfter(start, 600)),
						entryAt(loomshed.NameDiscussionWrite, secondsAfter(start, 1500)),
					},
				}
				w.pairBranch("flight", "pf1", at(199), seedStartedAt(start), statusJSON(w.t, status), nil)
			},
			check: func(t *testing.T, got TimingReport) {
				flight := runOf(t, got, "flight")
				wantPhases(t, flight, map[string]int64{PhaseSetup: 600, PhaseDiscussion: 900, PhasePlan: 300, PhasePaused: 5400})
				if flight.SpanSeconds != 7200 || flight.ActiveSeconds != 1800 {
					t.Errorf("span, active = %d, %d; want 7200, 1800", flight.SpanSeconds, flight.ActiveSeconds)
				}
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none", got.Findings)
				}
			},
		},
		{
			name: "a change into awaiting at PR-Gate is pr_gate, at another row escalation, and into blocked or failed halt, each running to the next row after it",
			setup: func(w *world) {
				batten := battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, run),
					changeAt("child running → awaiting at "+loomshed.NamePRGate, secondsAfter(run, 100)),
					changeAt("child awaiting → running", secondsAfter(run, 200)),
					changeAt("child running → awaiting at "+loomshed.NamePlanBouncer, secondsAfter(run, 300)),
					changeAt("child awaiting → running", secondsAfter(run, 400)),
					changeAt("child running → blocked at "+loomshed.NamePlanWrite, secondsAfter(run, 500)),
					changeAt("child blocked → running", secondsAfter(run, 600)),
					changeAt("child running → failed at "+loomshed.NameWebster, secondsAfter(run, 700)),
					changeAt("child failed → running", secondsAfter(run, 800)),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 900)),
				)
				landedRun(w, "kinds", "t1", 3, seedText("loom"), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, secondsAfter(run, 850))}, batten)
			},
			check: func(t *testing.T, got TimingReport) {
				wantWaits(t, runOf(t, got, "kinds"), map[string]int64{WaitPRGate: 100, WaitEscalation: 100, WaitHalt: 200})
			},
		},
		{
			name: "a batten row's stuck entry with no routed_to is a halt and an awaiting one an escalation, while Run-Shed entries and a bounced stuck entry add no wait",
			setup: func(w *world) {
				selfBounce := changeAt("", secondsAfter(run, 100))
				selfBounce.Outcome = shedengine.Stuck
				bounced := entryAt(battenrecipe.NameLand, secondsAfter(run, 200))
				bounced.Outcome = shedengine.Stuck
				bounced.RoutedTo = battenrecipe.NameWorktreeCreate
				halted := entryAt(battenrecipe.NameLand, secondsAfter(run, 300))
				halted.Outcome = shedengine.Stuck
				awaiting := entryAt(battenrecipe.NameLand, secondsAfter(run, 400))
				awaiting.Outcome = shedengine.Awaiting
				batten := battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, run),
					selfBounce,
					changeAt("child running → paused", secondsAfter(run, 150)),
					bounced,
					halted,
					awaiting,
					entryAt(battenrecipe.NameLand, secondsAfter(run, 500)),
					entryAt(battenrecipe.NameWorktreeTeardown, secondsAfter(run, 600)),
				)
				landedRun(w, "rowhalts", "t1", 3, seedText("loom"), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, secondsAfter(run, 50))}, batten)
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "rowhalts")
				wantWaits(t, got1, map[string]int64{WaitHalt: 100, WaitEscalation: 100})
				if got1.SpanSeconds != 600 || got1.ActiveSeconds != 400 {
					t.Errorf("span, active = %d, %d; want 600, 400", got1.SpanSeconds, got1.ActiveSeconds)
				}
			},
		},
		{
			name: "a sleep inside a halt counts as sleep and not as wait",
			sleeps: []Interval{
				{From: secondsAfter(run, 150), To: secondsAfter(run, 250)},
			},
			setup: func(w *world) {
				batten := battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, run),
					changeAt("child running → blocked at "+loomshed.NamePlanWrite, secondsAfter(run, 100)),
					changeAt("child blocked → running", secondsAfter(run, 300)),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 400)),
				)
				landedRun(w, "sleepy", "t1", 3, seedText("loom"), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, secondsAfter(run, 350))}, batten)
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "sleepy")
				wantWaits(t, got1, map[string]int64{WaitHalt: 100})
				if got1.SleepSeconds != 100 || got1.ActiveSeconds != 200 {
					t.Errorf("sleep, active = %d, %d; want 100, 200", got1.SleepSeconds, got1.ActiveSeconds)
				}
			},
		},
		{
			name: "a parent-review round with no verdict waits to the span's close, and a round with no request adds no wait",
			setup: func(w *world) {
				batten := battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, run),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 600)),
				)
				extra := mergeFiles(
					parentRoundTimed(1, secondsAfter(run, 100), time.Time{}, time.Time{}),
					parentRoundTimed(2, time.Time{}, time.Time{}, time.Time{}),
				)
				landedRunWith(w, "openround", "t1", 3, seedText("loom"), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, secondsAfter(run, 500))}, extra, batten)
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "openround")
				wantWaits(t, got1, map[string]int64{WaitParentReview: 500})
				if got1.ActiveSeconds != 100 || len(got.Findings) != 0 {
					t.Errorf("active, findings = %d, %+v; want 100 and none", got1.ActiveSeconds, got.Findings)
				}
			},
		},
		{
			name: "a run in flight runs its open halt wait and open arbiter attempt to the snapshot with no finding",
			live: []string{"flight"},
			setup: func(w *world) {
				start := at(198)
				w.battenRecord("fl-b", 195, "flight", battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, start),
					changeAt("child running → blocked at "+loomshed.NamePlanWrite, secondsAfter(start, 600)),
				))
				status := shedengine.Status{
					State:           shedengine.StateBlocked,
					CurrentProducer: loomshed.NamePlanWrite,
					History:         []shedengine.HistoryEntry{entryAt(loomshed.NamePreflight, secondsAfter(start, 300))},
				}
				w.pairBranch("flight", "pf1", at(199), seedStartedAt(start), statusJSON(w.t, status), arbiterSpawnedFiles(w.t, "flight", 1, secondsAfter(start, 900)))
			},
			check: func(t *testing.T, got TimingReport) {
				flight := runOf(t, got, "flight")
				wantWaits(t, flight, map[string]int64{WaitHalt: 6600})
				wantFigure(t, "arbiter", flight.Attempts.Arbiter, 1, 6300)
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none", got.Findings)
				}
			},
		},
		{
			name: "a landed run's arbiter attempt spawned before the span has its time absent, a finding naming its directory and a count of one",
			setup: func(w *world) {
				batten := battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, run),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 1000)),
				)
				landedRunWith(w, "early", "t1", 3, seedText("loom"), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, secondsAfter(run, 900))},
					arbiterSpawnedFiles(w.t, "early", 1, at(0)), batten)
			},
			check: func(t *testing.T, got TimingReport) {
				wantFigure(t, "arbiter", runOf(t, got, "early").Attempts.Arbiter, 1, -1)
				wantFindings(t, got, [2]string{tagName("early", "t1"), "arbiter attempt 1 spawned"})
				if got.Findings[0].File != recordPaths(t, "early").Arbiter {
					t.Errorf("finding file = %q; want the arbiter directory %q", got.Findings[0].File, recordPaths(t, "early").Arbiter)
				}
			},
		},
		{
			name: "a landed run's arbiter attempts spawned at and after its last row have their time absent, a finding each and a count of two",
			setup: func(w *world) {
				batten := battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, run),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 1000)),
				)
				extra := mergeFiles(
					arbiterSpawnedFiles(w.t, "late", 1, secondsAfter(run, 1000)),
					arbiterSpawnedFiles(w.t, "late", 2, secondsAfter(run, 5000)),
				)
				landedRunWith(w, "late", "t1", 3, seedText("loom"), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, secondsAfter(run, 900))}, extra, batten)
			},
			check: func(t *testing.T, got TimingReport) {
				wantFigure(t, "arbiter", runOf(t, got, "late").Attempts.Arbiter, 2, -1)
				wantFindings(t, got,
					[2]string{tagName("late", "t1"), "no row of either history is stamped after it"},
					[2]string{tagName("late", "t1"), "spawned outside the run's span"})
			},
		},
		{
			name: "a PR-Gate alignment attempt and a parent-review alignment record each count with their time to the next row, and a parent-review point's attempt adds no spawn",
			setup: func(w *world) {
				batten := battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, run),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 600)),
				)
				loom := []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 100)),
					entryAt(loomshed.NamePlanWrite, secondsAfter(run, 200)),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 500)),
				}
				extra := mergeFiles(
					alignmentSpawnedFiles(w.t, "aligned", "pr-gate", 1, secondsAfter(run, 250), ""),
					alignmentSpawnedFiles(w.t, "aligned", "parent-review", 1, secondsAfter(run, 120), ""),
					parentRoundTimed(1, secondsAfter(run, 150), secondsAfter(run, 300), secondsAfter(run, 160)),
				)
				landedRunWith(w, "aligned", "t1", 3, seedText("loom"), loom, extra, batten)
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "aligned")
				wantFigure(t, "alignment", got1.Attempts.Alignment, 1, 250)
				wantFigure(t, "parent review alignment", got1.Attempts.ParentReviewAlignment, 1, 40)
				wantFigure(t, "arbiter", got1.Attempts.Arbiter, 0, -1)
				wantWaits(t, got1, map[string]int64{WaitParentReview: 150})
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none", got.Findings)
				}
			},
		},
		{
			name: "a halt wait and an arbiter attempt spanning UTC midnight clip across both day buckets, the spawn counting only in the bucket of its spawned_at",
			setup: func(w *world) {
				batten := battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, at(22)),
					changeAt("child running → blocked at "+loomshed.NamePlanWrite, at(23)),
					entryAt(battenrecipe.NameLand, at(26)),
				)
				landedRunWith(w, "midnightwait", "t1", 30, seedText("loom"),
					[]shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, secondsAfter(at(25), 1800))},
					arbiterSpawnedFiles(w.t, "midnightwait", 1, secondsAfter(at(23), 1800)), batten)
			},
			check: func(t *testing.T, got TimingReport) {
				first, second := bucketOf(t, got, "2026-01-05"), bucketOf(t, got, "2026-01-06")
				if first.Waits[WaitHalt] != 3600 || second.Waits[WaitHalt] != 5400 {
					t.Errorf("halt waits = %d, %d; want 3600 and 5400 across the two days", first.Waits[WaitHalt], second.Waits[WaitHalt])
				}
				if first.ActiveSeconds != 3600 || second.ActiveSeconds != 1800 {
					t.Errorf("active = %d, %d; want 3600 and 1800", first.ActiveSeconds, second.ActiveSeconds)
				}
				wantFigure(t, "first day arbiter", first.Attempts.Arbiter, 1, 1800)
				wantFigure(t, "second day arbiter", second.Attempts.Arbiter, 0, 5400)
				wantFigure(t, "first day alignment", first.Attempts.Alignment, 0, -1)
			},
		},
		{
			name: "a bounce reads judge, and a run without the halt-note source has no stop kind while its bounce and a batten-row halt a batten goto by orch moved on still count",
			setup: func(w *world) {
				bounced := stuckAt(loomshed.NamePlanBouncer, secondsAfter(run, 200))
				bounced.RoutedTo = loomshed.NamePlanWrite
				halted := entryAt(battenrecipe.NameLand, secondsAfter(run, 400))
				halted.Outcome = shedengine.Stuck
				stopRun(w, "bounce", []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 100)),
					bounced,
					entryAt(loomshed.NamePlanWrite, secondsAfter(run, 300)),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 500)),
				}, []shedengine.HistoryEntry{
					entryAt(battenrecipe.NameWorktreeCreate, run),
					halted,
					gotoAt(battenrecipe.NameLand, secondsAfter(run, 450), "orch"),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 600)),
				}, nil, nil, nil)
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "bounce")
				wantSettlers(t, got1, StopKindStop)
				wantSettlers(t, got1, StopKindBounce, SettledByJudge)
				wantSettlers(t, got1, StopKindBattenHalt, "orch")
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none", got.Findings)
				}
			},
		},
		{
			name: "every settler class of a child stop is reached: a go-rule restep or verify-retry, a tackler restep, an applied arbiter attempt, a verify-retry automatic step, and the by of the entry that moved the stop on",
			setup: func(w *world) {
				loom := []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 100)),
					stuckAt(loomshed.NamePlanWrite, secondsAfter(run, 990)),
					entryAt(loomshed.NamePlanWrite, secondsAfter(run, 1100)),
					stuckAt(loomshed.NameWebster, secondsAfter(run, 1990)),
					entryAt(loomshed.NameWebster, secondsAfter(run, 2100)),
					stuckAt(loomshed.NameMergeIn, secondsAfter(run, 2990)),
					entryAt(loomshed.NameMergeIn, secondsAfter(run, 3200)),
					stuckAt(loomshed.NamePublish, secondsAfter(run, 3990)),
					gotoAt(loomshed.NamePublish, secondsAfter(run, 4100), "orch"),
					stuckAt(loomshed.NameDarnWrite, secondsAfter(run, 4990)),
					gotoAt(loomshed.NameDarnWrite, secondsAfter(run, 5100), "orch"),
					stuckAt(loomshed.NameBatchifier, secondsAfter(run, 5990)),
					gotoAt(loomshed.NameBatchifier, secondsAfter(run, 6100), shedengine.CallerShed),
					stuckAt(loomshed.NameDiscussionWrite, secondsAfter(run, 6990)),
					entryAt(loomshed.NameDiscussionWrite, secondsAfter(run, 7100)),
					stuckAt(loomshed.NameDescribe, secondsAfter(run, 7990)),
					entryAt(loomshed.NameDescribe, secondsAfter(run, 8100)),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 8500)),
				}
				halted := []string{loomshed.NamePlanWrite, loomshed.NameWebster, loomshed.NameMergeIn, loomshed.NamePublish, loomshed.NameDarnWrite, loomshed.NameBatchifier, loomshed.NameDiscussionWrite, loomshed.NameDescribe}
				batten := []shedengine.HistoryEntry{entryAt(battenrecipe.NameWorktreeCreate, run)}
				extra := arbiterAttemptFiles(w.t, "settlers", 1, loomengine.ArbiterAttempt{
					StopKey:   shedrun.StopKey{CurrentProducer: loomshed.NameMergeIn, State: "blocked"},
					SpawnedAt: secondsAfter(run, 3100),
				}, "", true)
				for k, producer := range halted {
					batten = append(batten, changeAt("child running → blocked at "+producer, secondsAfter(run, 1000*(k+1))))
					name := "loom-halt.md"
					if k > 0 {
						name = fmt.Sprintf("loom-halt-%d.md", k+1)
					}
					extra = mergeFiles(extra, blockedNote(name, producer))
				}
				batten = append(batten, entryAt(battenrecipe.NameLand, secondsAfter(run, 8600)))
				stopRun(w, "settlers", loom, batten, []shedrun.SettleRecord{
					settleRecord(loomshed.NamePlanWrite, 2, "blocked", shedrun.SettlerGoRule, shedrun.SettleRestep),
					settleRecord(loomshed.NameWebster, 4, "blocked", shedrun.SettlerTackler, shedrun.SettleRestep),
					settleRecord(loomshed.NameDarnWrite, 10, "blocked", shedrun.SettlerGoRule, shedrun.SettleMoved),
					settleRecord(loomshed.NameDescribe, 16, "blocked", shedrun.SettlerGoRule, shedrun.SettleVerifyRetry),
				}, nil, extra)
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "settlers")
				wantSettlers(t, got1, StopKindStop, SettledByShed, SettledByTackler, SettledByArbiter, SettledByShed, "orch", SettledByShed, SettledByUnknown, SettledByShed)
				bucket := bucketOf(t, got, "2026-01-05")
				want := map[string]int{SettledByShed: 4, SettledByTackler: 1, SettledByArbiter: 1, "orch": 1, SettledByUnknown: 1}
				if !maps.Equal(bucket.Settlers[StopKindStop], want) {
					t.Errorf("bucket settlers = %v; want %v", bucket.Settlers[StopKindStop], want)
				}
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none", got.Findings)
				}
			},
		},
		{
			name: "an applied PR-Gate alignment attempt and an applied parent-review alignment record spawned after a stop read alignment",
			setup: func(w *world) {
				batten := []shedengine.HistoryEntry{
					entryAt(battenrecipe.NameWorktreeCreate, run),
					changeAt("child running → awaiting at "+loomshed.NamePRGate, secondsAfter(run, 1000)),
					changeAt("child running → blocked at "+loomshed.NamePlanWrite, secondsAfter(run, 2000)),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 3100)),
				}
				extra := mergeFiles(
					blockedNote("loom-halt.md", loomshed.NamePlanWrite),
					alignmentSpawnedFiles(w.t, "alignstops", "pr-gate", 1, secondsAfter(run, 1100), "approve"),
					parentRoundTimed(1, secondsAfter(run, 1500), secondsAfter(run, 2200), secondsAfter(run, 2100)),
				)
				stopRun(w, "alignstops", []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 100)),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 3000)),
				}, batten, nil, nil, extra)
			},
			check: func(t *testing.T, got TimingReport) {
				wantSettlers(t, runOf(t, got, "alignstops"), StopKindStop, SettledByAlignment, SettledByAlignment)
			},
		},
		{
			name: "a record that settles nothing by itself leaves the stop to the entry that moved it on, here a goto by operator",
			setup: func(w *world) {
				producers := []string{loomshed.NamePlanWrite, loomshed.NameWebster, loomshed.NameDescribe, loomshed.NameDarnWrite, loomshed.NameBatchifier, loomshed.NameDiscussionWrite}
				records := []shedrun.SettleRecord{
					settleRecord(producers[0], 2, "blocked", shedrun.SettlerGoRule, shedrun.SettleBusyExit),
					settleRecord(producers[1], 4, "blocked", shedrun.SettlerGoRule, shedrun.SettleRunningExit),
					settleRecord(producers[2], 6, "blocked", shedrun.SettlerGoRule, shedrun.SettleHandOver),
					settleRecord(producers[3], 8, "blocked", shedrun.SettlerTackler, shedrun.SettleSpawn),
					settleRecord(producers[4], 10, "blocked", shedrun.SettlerParent, shedrun.SettleHandBack),
					settleRecord(producers[5], 12, "blocked", shedrun.SettlerArbiter, shedrun.SettleMoved),
				}
				loom := []shedengine.HistoryEntry{entryAt(loomshed.NamePreflight, secondsAfter(run, 50))}
				batten := []shedengine.HistoryEntry{entryAt(battenrecipe.NameWorktreeCreate, run)}
				var extra map[string]string
				for k, producer := range producers {
					stamp := 1000 * (k + 1)
					loom = append(loom, stuckAt(producer, secondsAfter(run, stamp-10)), gotoAt(producer, secondsAfter(run, stamp+100), "operator"))
					batten = append(batten, changeAt("child running → blocked at "+producer, secondsAfter(run, stamp)))
					name := "loom-halt.md"
					if k > 0 {
						name = fmt.Sprintf("loom-halt-%d.md", k+1)
					}
					extra = mergeFiles(extra, blockedNote(name, producer))
				}
				loom = append(loom, entryAt(loomshed.NameFinalize, secondsAfter(run, 7500)))
				batten = append(batten, entryAt(battenrecipe.NameLand, secondsAfter(run, 7600)))
				stopRun(w, "nothing", loom, batten, records, nil, extra)
			},
			check: func(t *testing.T, got TimingReport) {
				wantSettlers(t, runOf(t, got, "nothing"), StopKindStop, "operator", "operator", "operator", "operator", "operator", "operator")
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none", got.Findings)
				}
			},
		},
		{
			name: "a note-only stop has no time and is counted in its run's closing bucket, taking its settler from a record naming its producer and state, else from its later entry's by, and a change with no producer reads its later entry's by",
			setup: func(w *world) {
				stopRun(w, "noteonly", []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 100)),
					stuckAt(loomshed.NamePlanWrite, secondsAfter(run, 300)),
					gotoAt(loomshed.NamePlanWrite, secondsAfter(run, 400), "orch"),
					stuckAt(loomshed.NamePublish, secondsAfter(run, 500)),
					gotoAt(loomshed.NamePublish, secondsAfter(run, 600), "operator"),
					gotoAt(loomshed.NameDescribe, secondsAfter(run, 660), "orch"),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 680)),
				}, []shedengine.HistoryEntry{
					entryAt(battenrecipe.NameWorktreeCreate, run),
					changeAt("child running → blocked", secondsAfter(run, 650)),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 700)),
				}, []shedrun.SettleRecord{
					settleRecord(loomshed.NamePlanWrite, 2, "blocked", shedrun.SettlerGoRule, shedrun.SettleRestep),
				}, nil, mergeFiles(
					blockedNote("loom-halt.md", loomshed.NamePlanWrite),
					blockedNote("loom-halt-2.md", loomshed.NamePublish),
				))
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "noteonly")
				wantSettlers(t, got1, StopKindStop, "orch", SettledByShed, "operator")
				stops := stopsOfKind(got1, StopKindStop)
				if stops[0].At == nil || stops[1].At != nil || stops[2].At != nil {
					t.Errorf("stop times = %v, %v, %v; want only the change stop timed", stops[0].At, stops[1].At, stops[2].At)
				}
				if bucket := bucketOf(t, got, "2026-01-05"); bucket.Settlers[StopKindStop][SettledByShed] != 1 || bucket.Settlers[StopKindStop]["operator"] != 1 || bucket.Settlers[StopKindStop]["orch"] != 1 {
					t.Errorf("bucket settlers = %v; want each stop in the bucket of the run's close", bucket.Settlers)
				}
			},
		},
		{
			name: "a halt restepped into a second halt before batten observed it reads the second record's settler, and the earlier record folds into it with no finding",
			setup: func(w *world) {
				stopRun(w, "restepped", []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 100)),
					stuckAt(loomshed.NameWebster, secondsAfter(run, 200)),
					stuckAt(loomshed.NameWebster, secondsAfter(run, 300)),
					entryAt(loomshed.NameWebster, secondsAfter(run, 400)),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 500)),
				}, []shedengine.HistoryEntry{
					entryAt(battenrecipe.NameWorktreeCreate, run),
					changeAt("child running → blocked at "+loomshed.NameWebster, secondsAfter(run, 350)),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 600)),
				}, []shedrun.SettleRecord{
					settleRecord(loomshed.NameWebster, 2, "blocked", shedrun.SettlerGoRule, shedrun.SettleRestep),
					settleRecord(loomshed.NameWebster, 3, "blocked", shedrun.SettlerTackler, shedrun.SettleRestep),
				}, nil, blockedNote("loom-halt.md", loomshed.NameWebster))
			},
			check: func(t *testing.T, got TimingReport) {
				wantSettlers(t, runOf(t, got, "restepped"), StopKindStop, SettledByTackler)
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none", got.Findings)
				}
			},
		},
		{
			name: "a batten-file record names only a batten-row halt, counted under batten-halt and not among the stop kind, whose count equals convergence's halt count",
			setup: func(w *world) {
				halted := entryAt(battenrecipe.NameLand, secondsAfter(run, 300))
				halted.Outcome = shedengine.Stuck
				stopRun(w, "battenfile", []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 100)),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 200)),
				}, []shedengine.HistoryEntry{
					entryAt(battenrecipe.NameWorktreeCreate, run),
					changeAt("child running → blocked at "+loomshed.NamePlanWrite, secondsAfter(run, 150)),
					halted,
					entryAt(battenrecipe.NameLand, secondsAfter(run, 400)),
				}, nil, []shedrun.SettleRecord{
					settleRecord(battenrecipe.NameLand, 3, "blocked", shedrun.SettlerGoRule, shedrun.SettleRestep),
				}, blockedNote("loom-halt.md", loomshed.NamePlanWrite))
				convergence, err := Compute(w.sources(), Options{Since: at(0)})
				if err != nil {
					w.t.Fatalf("Compute error = %v", err)
				}
				convergenceHalts = len(convergence.Runs[0].Stops)
			},
			check: func(t *testing.T, got TimingReport) {
				got1 := runOf(t, got, "battenfile")
				wantSettlers(t, got1, StopKindBattenHalt, SettledByShed)
				if count := len(stopsOfKind(got1, StopKindStop)); count != convergenceHalts || count != 1 {
					t.Errorf("stop kind count = %d; want convergence's %d, which is 1", count, convergenceHalts)
				}
			},
		},
		{
			name: "a loom-file record matching no stop, an undecodable settle line and a batten-file record matching no batten-row halt are findings",
			setup: func(w *world) {
				files := mergeFiles(blockedNote("loom-halt.md", loomshed.NamePlanWrite))
				files[recordPaths(w.t, "unmatched").SettleRecords] = settleFile(w.t, settleRecord(loomshed.NameDescribe, 3, "blocked", shedrun.SettlerGoRule, shedrun.SettleRestep)) + "{bad\n"
				w.battenSettles("bs-unmatched", 2, "unmatched", battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, run),
					changeAt("child running → blocked at "+loomshed.NamePlanWrite, secondsAfter(run, 150)),
					entryAt(battenrecipe.NameLand, secondsAfter(run, 400)),
				), settleRecord(battenrecipe.NameLand, 9, "blocked", shedrun.SettlerGoRule, shedrun.SettleRestep))
				landedRunWith(w, "unmatched", "t1", 3, seedText("loom"), []shedengine.HistoryEntry{
					entryAt(loomshed.NamePreflight, secondsAfter(run, 100)),
					entryAt(loomshed.NameFinalize, secondsAfter(run, 200)),
				}, files)
			},
			check: func(t *testing.T, got TimingReport) {
				var texts []string
				for _, finding := range got.Findings {
					texts = append(texts, finding.Error)
				}
				for _, want := range []string{"line 2 does not decode", "Describe blocked, history length 3, matches no stop", "in batten's copy: settle record at Land blocked, history length 9, matches no stop"} {
					if !slices.ContainsFunc(texts, func(text string) bool { return strings.Contains(text, want) }) {
						t.Errorf("findings = %v; want one containing %q", texts, want)
					}
				}
				if len(got.Findings) != 3 {
					t.Errorf("findings = %+v; want exactly three", got.Findings)
				}
			},
		},
		{
			name: "a stop of a run in flight with no later entry reads open with no finding",
			live: []string{"openstop"},
			setup: func(w *world) {
				start := at(198)
				w.battenRecord("os-b", 195, "openstop", battenRecordOf(
					entryAt(battenrecipe.NameWorktreeCreate, start),
					changeAt("child running → blocked at "+loomshed.NamePlanWrite, secondsAfter(start, 600)),
				))
				status := shedengine.Status{
					State:           shedengine.StateBlocked,
					CurrentProducer: loomshed.NamePlanWrite,
					History: []shedengine.HistoryEntry{
						entryAt(loomshed.NamePreflight, secondsAfter(start, 300)),
						stuckAt(loomshed.NamePlanWrite, secondsAfter(start, 590)),
					},
				}
				w.pairBranch("openstop", "pf1", at(199), seedStartedAt(start), statusJSON(w.t, status), blockedNote("loom-halt.md", loomshed.NamePlanWrite))
			},
			check: func(t *testing.T, got TimingReport) {
				wantSettlers(t, runOf(t, got, "openstop"), StopKindStop, SettledByOpen)
				if len(got.Findings) != 0 {
					t.Errorf("findings = %+v; want none", got.Findings)
				}
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

// markdownSection returns the text of the section under heading, up to the next section heading or the end.
func markdownSection(t *testing.T, document, heading string) string {
	t.Helper()

	_, rest, found := strings.Cut(document, heading+"\n")
	if !found {
		t.Fatalf("document holds no heading %q:\n%s", heading, document)
	}
	section, _, _ := strings.Cut(rest, "\n## ")
	return section
}

// markdownCell returns the cell of column header in the first row of section whose first cell is first.
func markdownCell(t *testing.T, section, header, first string) string {
	t.Helper()

	split := func(line string) []string {
		return strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "| "), " |"), " | ")
	}
	lines := strings.Split(strings.TrimSpace(section), "\n")
	column := slices.Index(split(lines[0]), header)
	if column < 0 {
		t.Fatalf("section holds no column %q:\n%s", header, section)
	}
	for _, line := range lines[2:] {
		if cells := split(line); cells[0] == first {
			return cells[column]
		}
	}
	t.Fatalf("section holds no row starting %q:\n%s", first, section)
	return ""
}

func TestRenderTimingMarkdown(t *testing.T) {
	t.Parallel()

	landTime := seconds(1800)
	report := TimingReport{
		Selection: TimingSelection{RecordsBranch: "records", RecordsTip: "tip1", Since: at(0), By: BucketDay, SnapshotAt: at(200)},
		Runs: []TimingRun{
			{
				Slug: "short", State: RunStateLanded, Start: at(1), SpanSeconds: 3725, ActiveSeconds: 3000, SleepSeconds: 90,
				Waits:    map[string]int64{WaitParentReview: 0, WaitPRGate: 600, WaitEscalation: 0, WaitHalt: 35},
				Phases:   map[string]*int64{PhaseSetup: seconds(45), PhaseLand: nil, PhasePublishFinalize: landTime},
				Attempts: TimingAttempts{Arbiter: AttemptFigure{Count: 2, Seconds: seconds(250)}},
			},
			{Slug: "long", State: RunStateInFlight, Start: at(2), SpanSeconds: 183600, Phases: map[string]*int64{}},
		},
		Phases:    map[string]*PhaseMedian{PhaseSetup: {Seconds: seconds(45), Runs: 1}, PhaseLand: {Runs: 0}},
		Buckets:   []TimingBucket{{Label: "2026-01-05", Runs: 2, SleepSeconds: 90, Settlers: map[string]map[string]int{StopKindStop: {"orch": 2, SettledByShed: 1}}}},
		SleepNote: "from the test",
		Findings:  []Finding{{Tag: "t", Error: "e"}},
	}

	got := RenderTimingMarkdown(report)

	last := -1
	for _, heading := range []string{HeadingTimingRuns, HeadingTimingPhases, HeadingTimingBuckets, HeadingTimingSettlers} {
		index := strings.Index(got, heading+"\n")
		if index <= last {
			t.Errorf("heading %q at %d, after %d; want the four headings in order:\n%s", heading, index, last, got)
		}
		last = index
	}
	if !strings.Contains(got, "- runs: 2, in flight: 1, findings: 1\n") || !strings.Contains(got, "- sleep: from the test\n") {
		t.Errorf("selection lacks the counts or the sleep note:\n%s", got)
	}

	runs := markdownSection(t, got, HeadingTimingRuns)
	for _, tt := range []struct{ run, column, want string }{
		{"short", "Span", "1h02m05s"},
		{"long", "Span", "2d03h"},
		{"short", PhaseLand, "no data"},
		{"short", PhasePublishFinalize, "30m00s"},
		{"short", "Sleep", "1m30s"},
		{"short", WaitPRGate, "10m00s"},
		{"short", "Arbiter attempts", "2 (4m10s)"},
		{"short", "Alignment attempts", "0 (no data)"},
		{"long", WaitHalt, "no data"},
	} {
		if cell := markdownCell(t, runs, tt.column, tt.run); cell != tt.want {
			t.Errorf("run %s column %s = %q; want %q", tt.run, tt.column, cell, tt.want)
		}
	}
	if cell := markdownCell(t, markdownSection(t, got, HeadingTimingPhases), "Median", PhaseLand); cell != "no data" {
		t.Errorf("land median = %q; want no data", cell)
	}
	if cell := markdownCell(t, markdownSection(t, got, HeadingTimingBuckets), "Sleep", "2026-01-05"); cell != "1m30s" {
		t.Errorf("bucket sleep = %q; want 1m30s", cell)
	}
	settlers := markdownSection(t, got, HeadingTimingSettlers)
	if !strings.Contains(settlers, "| 2026-01-05 | stop | orch | 2 |\n") || !strings.Contains(settlers, "| 2026-01-05 | stop | shed | 1 |\n") {
		t.Errorf("stops by settler = %q; want a row per settler", settlers)
	}

	empty := RenderTimingMarkdown(TimingReport{})
	for _, heading := range []string{HeadingTimingRuns, HeadingTimingPhases, HeadingTimingBuckets, HeadingTimingSettlers} {
		if section := markdownSection(t, empty, heading); strings.TrimSpace(section) != "no data" {
			t.Errorf("empty report section %q = %q; want no data", heading, section)
		}
	}
}

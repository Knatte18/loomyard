// cost_test.go tests ComputeCost over a fake records repository:
// the tags that enter a cell, a run's dollars and hours from its usage files, and the cell means and fits.

package reportengine

import (
	"fmt"
	"math"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// costTolerance is how far a computed figure may sit from its expected value.
const costTolerance = 1e-9

// usageFiles returns the files of a run's usage directory, keyed by tree-relative path.
// It holds one session file per entry of sessions, keyed by session id.
// The run and landed-size records are included unless their text is empty.
func usageFiles(sessions map[string]string, run, landed string) map[string]string {
	dir := filepath.ToSlash(shedrun.UsageDirRel())
	files := map[string]string{}
	for id, text := range sessions {
		files[path.Join(dir, "session-"+id+".json")] = text
	}
	if run != "" {
		files[path.Join(dir, "run.json")] = run
	}
	if landed != "" {
		files[path.Join(dir, "landed.json")] = landed
	}
	return files
}

// sessionText is the text of a session record costing cost, a JSON number or null, over the span started to ended;
// a zero time leaves its stamp out.
func sessionText(cost string, started, ended time.Time) string {
	text := fmt.Sprintf(`{"session_id":"s","row":"r","role":"x","cost_usd":%s`, cost)
	if !started.IsZero() {
		text += fmt.Sprintf(`,"started":%q`, started.Format(time.RFC3339))
	}
	if !ended.IsZero() {
		text += fmt.Sprintf(`,"ended":%q`, ended.Format(time.RFC3339))
	}
	return text + "}"
}

// runText is the text of a run record.
func runText(recipe, profile string, storedRecord bool) string {
	return fmt.Sprintf(`{"recipe":%q,"profile":%q,"stored_record":%t}`, recipe, profile, storedRecord)
}

// landedText is the text of a landed-size record of added and deleted lines.
func landedText(added, deleted int) string {
	return fmt.Sprintf(`{"lines_added":%d,"lines_deleted":%d,"files_changed":1}`, added, deleted)
}

// tapestryRun adds a landed tapestry run of slug, which the timing report times over hours from startHour.
// It holds one session costing usd and a landed diff of lines changed lines.
func tapestryRun(w *world, slug string, startHour int, hours, usd float64, lines int, profile string) {
	start := at(startHour)
	finalized := start.Add(time.Duration(hours * float64(time.Hour)))
	extra := usageFiles(
		map[string]string{"a": sessionText(fmt.Sprint(usd), time.Time{}, time.Time{})},
		runText("tapestry", profile, false),
		landedText(lines-40, 40),
	)
	landedRunWith(w, slug, "sha-"+slug, startHour+5, seedStartedAt(start), []shedengine.HistoryEntry{entryAt(loomshed.NameFinalize, finalized)}, extra)
}

// motifRun adds a done motif run of slug at hour holding the usage extra files, with no landed-size record.
func motifRun(w *world, slug string, hour int, extra map[string]string) {
	status := statusJSON(w.t, shedengine.Status{State: shedengine.StateDone, History: historyOf("Discussion-Write")})
	w.tagWith(slug, "sha-"+slug, at(hour), seedText("motif"), status, extra)
}

// motifUsage is the usage files of a motif run, with one session costing usd.
// The session spans from hour 30 to hour 30 plus hours.
func motifUsage(usd string, hours int, storedRecord bool) map[string]string {
	sessions := map[string]string{"a": sessionText(usd, at(30), at(30).Add(time.Duration(hours)*time.Hour))}
	return usageFiles(sessions, runText("motif", "p", storedRecord), "")
}

// figure returns a pointer to v.
func figure(v float64) *float64 { return &v }

// sameFigure reports whether two optional figures are both absent or both within costTolerance.
func sameFigure(got, want *float64) bool {
	if got == nil || want == nil {
		return got == want
	}
	return math.Abs(*got-*want) < costTolerance
}

// sameFit reports whether two optional fits are both absent or agree in every figure.
func sameFit(got, want *CostFit) bool {
	if got == nil || want == nil {
		return got == want
	}
	return got.Runs == want.Runs &&
		math.Abs(got.Fixed-want.Fixed) < costTolerance &&
		math.Abs(got.Per100Lines-want.Per100Lines) < costTolerance &&
		math.Abs(got.Correlation-want.Correlation) < costTolerance
}

// TestComputeCost covers the cell grouping, the landed test per recipe and the dollar and hour sources.
// It also covers the fits and the minimum.
func TestComputeCost(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tag := func(slug string) string { return fabricengine.ArchiveTagPrefix + slug + "/sha-" + slug }
	tests := []struct {
		name         string
		since        time.Time
		minRuns      int
		build        func(w *world)
		want         []CostCell
		wantFindings []string
	}{
		{
			name:    "SizedRunsGiveTheFixedAndPer100LineFit",
			minRuns: 3,
			build: func(w *world) {
				tapestryRun(w, "a1", 10, 1.5, 15, 100, "p")
				tapestryRun(w, "a2", 20, 2.5, 25, 200, "p")
				tapestryRun(w, "a3", 30, 3.5, 35, 300, "p")
			},
			want: []CostCell{{
				Recipe: "tapestry", Profile: "p", Runs: 3,
				MeanUSD: figure(25), MeanHours: figure(2.5),
				USDFit:   &CostFit{Fixed: 5, Per100Lines: 10, Correlation: 1, Runs: 3},
				HoursFit: &CostFit{Fixed: 0.5, Per100Lines: 1, Correlation: 1, Runs: 3},
			}},
		},
		{
			name:    "MotifRunsWithNoLandedRecordGiveMeansFromSessionSpans",
			minRuns: 3,
			build: func(w *world) {
				for i, slug := range []string{"m1", "m2", "m3"} {
					sessions := map[string]string{
						"a": sessionText("1.0", at(30), at(31)),
						"b": sessionText("0.5", at(30).Add(30*time.Minute), at(32)),
					}
					motifRun(w, slug, 40+i, usageFiles(sessions, runText("motif", "p", false), ""))
				}
			},
			want: []CostCell{{Recipe: "motif", Profile: "p", Runs: 3, MeanUSD: figure(1.5), MeanHours: figure(2)}},
		},
		{
			name:    "SizedRunsOfOneSizeGiveMeansAndNoFit",
			minRuns: 3,
			build: func(w *world) {
				tapestryRun(w, "c1", 10, 1, 10, 100, "p")
				tapestryRun(w, "c2", 20, 1, 20, 100, "p")
				tapestryRun(w, "c3", 30, 1, 30, 100, "p")
			},
			want: []CostCell{{Recipe: "tapestry", Profile: "p", Runs: 3, MeanUSD: figure(20), MeanHours: figure(1)}},
		},
		{
			name:    "ACellUnderTheMinimumReadsTooFewRuns",
			minRuns: 3,
			build: func(w *world) {
				motifRun(w, "d1", 40, motifUsage("1.0", 1, false))
				motifRun(w, "d2", 41, motifUsage("2.0", 1, false))
			},
			want: []CostCell{{Recipe: "motif", Profile: "p", Runs: 2, Note: CostNoteTooFewRuns}},
		},
		{
			name:    "AnUnknownCostSessionLeavesHoursAlone",
			minRuns: 1,
			build: func(w *world) {
				motifRun(w, "e1", 40, motifUsage("null", 1, false))
			},
			want: []CostCell{{Recipe: "motif", Profile: "p", Runs: 1, MeanHours: figure(1)}},
		},
		{
			name:    "ARunWithNoRunRecordIsSkippedAndAFinding",
			minRuns: 1,
			build: func(w *world) {
				motifRun(w, "f1", 40, usageFiles(map[string]string{"a": sessionText("1.0", at(30), at(31))}, "", ""))
			},
			want:         []CostCell{},
			wantFindings: []string{tag("f1")},
		},
		{
			name:    "ACancelledRunWithNoLandedRecordIsLeftOut",
			minRuns: 1,
			build: func(w *world) {
				status := statusJSON(w.t, shedengine.Status{State: shedengine.StateFailed, History: historyOf("Plan-Write")})
				extra := usageFiles(map[string]string{"a": sessionText("1.0", at(30), at(31))}, runText("tapestry", "p", false), "")
				w.tagWith("g1", "sha-g1", at(40), seedText("tapestry"), status, extra)
			},
			want: []CostCell{},
		},
		{
			name:    "TheStoredRecordFlagSplitsCells",
			minRuns: 1,
			build: func(w *world) {
				motifRun(w, "h1", 40, motifUsage("1.0", 1, true))
				motifRun(w, "h2", 41, motifUsage("2.0", 1, false))
			},
			want: []CostCell{
				{Recipe: "motif", Profile: "p", StoredRecord: false, Runs: 1, MeanUSD: figure(2), MeanHours: figure(1)},
				{Recipe: "motif", Profile: "p", StoredRecord: true, Runs: 1, MeanUSD: figure(1), MeanHours: figure(1)},
			},
		},
		{
			name:    "AReviewRoundUsageRecordNeverEntersTheSum",
			minRuns: 1,
			build: func(w *world) {
				roundRecord := map[string]string{path.Join(filepath.ToSlash(loomengine.LoomReviewsDirRel()), "Code-Review", "usage.json"): `{"cost_usd":999}`}
				motifRun(w, "i1", 40, mergeFiles(motifUsage("1.5", 1, false), roundRecord))
			},
			want: []CostCell{{Recipe: "motif", Profile: "p", Runs: 1, MeanUSD: figure(1.5), MeanHours: figure(1)}},
		},
		{
			name:    "ATagBeforeTheWindowIsLeftOut",
			since:   at(50),
			minRuns: 1,
			build: func(w *world) {
				motifRun(w, "j1", 40, motifUsage("1.0", 1, false))
				motifRun(w, "j2", 60, motifUsage("2.0", 1, false))
			},
			want: []CostCell{{Recipe: "motif", Profile: "p", Runs: 1, MeanUSD: figure(2), MeanHours: figure(1)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			tt.build(w)
			since := tt.since
			if since.IsZero() {
				since = at(0)
			}

			got, err := ComputeCost(w.sources(), CostOptions{Since: since, MinRuns: tt.minRuns})

			if err != nil {
				t.Fatalf("ComputeCost() error = %v; want nil", err)
			}
			if len(got.Cells) != len(tt.want) {
				t.Fatalf("ComputeCost() cells = %+v; want %d cells", got.Cells, len(tt.want))
			}
			for i, want := range tt.want {
				cell := got.Cells[i]
				if cell.Recipe != want.Recipe || cell.Profile != want.Profile || cell.StoredRecord != want.StoredRecord || cell.Runs != want.Runs || cell.Note != want.Note {
					t.Errorf("cell %d = %+v; want recipe %q, profile %q, stored %v, runs %d, note %q", i, cell, want.Recipe, want.Profile, want.StoredRecord, want.Runs, want.Note)
				}
				if !sameFigure(cell.MeanUSD, want.MeanUSD) || !sameFigure(cell.MeanHours, want.MeanHours) {
					t.Errorf("cell %d means = usd %v, hours %v; want usd %v, hours %v", i, deref(cell.MeanUSD), deref(cell.MeanHours), deref(want.MeanUSD), deref(want.MeanHours))
				}
				if !sameFit(cell.USDFit, want.USDFit) || !sameFit(cell.HoursFit, want.HoursFit) {
					t.Errorf("cell %d fits = usd %+v, hours %+v; want usd %+v, hours %+v", i, cell.USDFit, cell.HoursFit, want.USDFit, want.HoursFit)
				}
			}
			var gotFindings []string
			for _, finding := range got.Findings {
				gotFindings = append(gotFindings, finding.Tag)
			}
			if fmt.Sprint(gotFindings) != fmt.Sprint(tt.wantFindings) {
				t.Errorf("ComputeCost() finding tags = %v; want %v", gotFindings, tt.wantFindings)
			}
		})
	}
}

// deref spells an optional figure for a failure message, nil as "none".
func deref(figure *float64) string {
	if figure == nil {
		return "none"
	}
	return fmt.Sprint(*figure)
}

// calibrate_test.go drives the calibration over an in-memory plan history and base tree and a fixture batcher.yaml: the row arithmetic, the skip reasons and the fit summary.
// Tier-1 (no git, no spawn).

package main

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// fakeHistory is an in-memory PlanHistory: commits by subject and file contents by "<sha>:<path>".
type fakeHistory struct {
	commits map[string][]gitrepo.SubjectCommit
	files   map[string]string
}

func (f fakeHistory) CommitsWithSubject(subject string) ([]gitrepo.SubjectCommit, error) {
	return f.commits[subject], nil
}

func (f fakeHistory) FileAtRevision(rev, relPath string) ([]byte, error) {
	data, ok := f.files[rev+":"+relPath]
	if !ok {
		return nil, gitrepo.ErrPathNotAtRevision
	}
	return []byte(data), nil
}

// fakeBase is an in-memory BaseTrees: file contents by "<sha>:<path>" for each known sha.
type fakeBase struct {
	shas  map[string]bool
	files map[string]string
}

func (f fakeBase) SHAExists(sha string) bool { return f.shas[sha] }

func (f fakeBase) FileAtRevision(rev, relPath string) ([]byte, error) {
	data, ok := f.files[rev+":"+relPath]
	if !ok {
		return nil, gitrepo.ErrPathNotAtRevision
	}
	return []byte(data), nil
}

func (f fakeBase) FilesInDirAtRevision(rev, dir string) ([]string, error) {
	var names []string
	for key := range f.files {
		prefix := rev + ":" + dir + "/"
		if rest, ok := strings.CutPrefix(key, prefix); ok && !strings.Contains(rest, "/") {
			names = append(names, rest)
		}
	}
	return names, nil
}

// planWith renders a plan whose cards each edit one file, keyed by card number.
func planWith(targets map[int]string) map[string]string {
	p := plankit.Plan{Approved: true, Language: "go"}
	for number := 1; number <= len(targets); number++ {
		p.Cards = append(p.Cards, plankit.Card{
			Number: number,
			Slug:   fmt.Sprintf("c%d", number),
			Groups: []plankit.Group{{Label: "Edit", Targets: []string{targets[number]}}},
		})
	}
	files := map[string]string{}
	for name, data := range plankit.Render(p) {
		files[name] = string(data)
	}
	return files
}

// lines is content of n lines.
func lines(n int) string { return strings.Repeat("x\n", n) }

// The base files of a based run hold these many lines each.
const (
	claudeLines   = 3
	patternLines  = 4
	templateLines = 5
)

// withBaseFiles adds the files a run's Merriam base is computed from to the base tree at sha:
// CLAUDE.md, PATTERN.md and the Master template.
func withBaseFiles(files map[string]string, sha string) {
	files[sha+":CLAUDE.md"] = lines(claudeLines)
	files[sha+":PATTERN.md"] = lines(patternLines)
	files[sha+":"+masterTemplatePath] = lines(templateLines)
}

// knownBaseContext is the context the seeded profiles (a context_per_line of 1) price for a based run whose plan renders overview:
// the lines of the overview and the three base files, plus the fixed system-prompt context.
func knownBaseContext(overview string) float64 {
	base := websterengine.MerriamBaseOf(overview, lines(claudeLines), lines(patternLines), lines(templateLines))
	return float64(base.Lines) + base.Fixed
}

// basedRunFixture returns a plan history and base tree in which run slug has a plan commit before the forks of the tests, a base commit sha-of-slug holding the base files, and the base context those give.
func basedRunFixture(slug string, planTargets map[int]string, forksStarted time.Time) (fakeHistory, fakeBase, float64) {
	plan := planWith(planTargets)
	history := fakeHistory{
		commits: map[string][]gitrepo.SubjectCommit{planCommitSubjectPrefix + slug: {{SHA: slug + "plan", Committed: forksStarted.Add(-time.Hour)}}},
		files:   map[string]string{},
	}
	for name, data := range plan {
		history.files[slug+"plan:_lyx/plan/"+name] = data
	}
	base := fakeBase{shas: map[string]bool{"base" + slug: true}, files: map[string]string{}}
	withBaseFiles(base.files, "base"+slug)
	return history, base, knownBaseContext(plan["00-overview.md"])
}

// seedProfile writes a batcher.yaml whose "fit" profile estimates one card's peak as orientation + the lines of its file, with every other coefficient 0;
// its "grow" profile is "fit" with a batch_growth of 5.
func seedProfile(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(configengine.ConfigDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	const config = `profiles:
  fit:
    weights:
      orientation: 10
      batch_growth: 0
      fork_messages: 0
      message_context: 0
      write_per_card_line: 0
      target_messages: 1
      test_file_messages: 0
      uses_messages: 0
      context_per_line: 1
      package_context: 0
  grow:
    weights:
      orientation: 10
      batch_growth: 5
      fork_messages: 0
      message_context: 0
      write_per_card_line: 0
      target_messages: 1
      test_file_messages: 0
      uses_messages: 0
      context_per_line: 1
      package_context: 0
`
	if err := os.WriteFile(configengine.ConfigFile(dir, "batcher"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fork returns a fork in the session "" that ran cards, whose first message held start tokens of context and whose largest held peak.
func fork(started time.Time, start, peak int, cards ...string) ForkTally {
	return forkIn("", started, start, peak, cards...)
}

// forkIn is fork under the webster session transcript named session.
func forkIn(session string, started time.Time, start, peak int, cards ...string) ForkTally {
	return ForkTally{Cards: cards, Started: started, Session: session, Usage: Usage{Input: peak}, StartContext: start, PeakContext: peak}
}

// TestCalibrate asserts the per-fork peak rows with their positions and growth ratios, the skip reasons of runs, forks and cards, the start rows of runs left out for their plan or base, and the printed fit lines.
func TestCalibrate(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	seedProfile(t, configDir)

	firstFork := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	planDir := "_lyx/plan/"
	history := fakeHistory{commits: map[string][]gitrepo.SubjectCommit{}, files: map[string]string{}}
	addPlan := func(slug, sha string, committed time.Time, files map[string]string) {
		subject := planCommitSubjectPrefix + slug
		history.commits[subject] = append(history.commits[subject], gitrepo.SubjectCommit{SHA: sha, Committed: committed})
		for name, data := range files {
			history.files[sha+":"+planDir+name] = data
		}
	}
	// The newest commit of alpha is after its first fork and holds no plan files, so picking it fails the run.
	addPlan("alpha", "alphalate", firstFork.Add(time.Hour), nil)
	addPlan("alpha", "alphaplan", firstFork.Add(-time.Hour), planWith(map[int]string{
		1: "internal/a/a.go", 2: "internal/b/b.go", 3: "internal/c/c.go", 4: "internal/d/d.go", 5: "internal/e/e.go",
	}))
	addPlan("beta", "betaplan", firstFork.Add(-time.Hour), planWith(map[int]string{1: "internal/a/a.go"}))
	addPlan("nobase", "nobaseplan", firstFork.Add(-time.Hour), planWith(map[int]string{1: "internal/a/a.go"}))
	addPlan("lostbase", "lostbaseplan", firstFork.Add(-time.Hour), planWith(map[int]string{1: "internal/a/a.go"}))
	addPlan("badplan", "badplanplan", firstFork.Add(-time.Hour), map[string]string{"00-overview.md": "not a plan"})
	addPlan("afterfork", "afterforkplan", firstFork.Add(time.Hour), planWith(map[int]string{1: "internal/a/a.go"}))

	base := fakeBase{
		shas: map[string]bool{"basea": true, "baseb": true, "basebad": true, "basenoplan": true},
		files: map[string]string{
			"basea:internal/a/a.go": lines(100),
			"basea:internal/b/b.go": lines(50),
			"basea:internal/c/c.go": lines(30),
			"basea:internal/d/d.go": lines(20),
			"baseb:internal/a/a.go": lines(40),
		},
	}

	// Every fork starts at 10 tokens, the profile's orientation, so the start rows lie on a flat line.
	runs := []RunTally{
		{Slug: "alpha", BaseSHA: "basea", Forks: []ForkTally{
			fork(firstFork, 10, 220, "01-c1"),
			fork(firstFork.Add(time.Minute), 10, 40, "02-c2"),
			fork(firstFork.Add(2*time.Minute), 10, 90, "02-c2"),
			fork(firstFork.Add(3*time.Minute), 10, 120, "03-c3", "04-c4"),
		}},
		{Slug: "beta", BaseSHA: "baseb", Forks: []ForkTally{
			fork(firstFork, 10, 50, "01-c1"),
			fork(firstFork.Add(time.Minute), 10, 70, "09-ghost"),
		}},
		{Slug: "nofork", BaseSHA: "basea"},
		{Slug: "afterfork", BaseSHA: "basea", Forks: []ForkTally{fork(firstFork, 10, 1, "01-c1")}},
		{Slug: "nobase", Forks: []ForkTally{fork(firstFork, 10, 1, "01-c1")}},
		{Slug: "lostbase", BaseSHA: "gone", Forks: []ForkTally{fork(firstFork, 10, 1, "01-c1")}},
		{Slug: "badplan", BaseSHA: "basebad", Forks: []ForkTally{fork(firstFork, 10, 1, "01-c1")}},
	}

	got, err := Calibrate(runs, "fit", configDir, history, base)
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}

	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	wantRows := []CalibrationRow{
		{Run: "alpha", Card: "01-c1", Position: 1, Estimate: 110, Measured: 220, Ratio: 2, EstimatedGrowth: 100, MeasuredGrowth: 210, GrowthRatio: 2.1},
		{Run: "alpha", Card: "02-c2", Position: 2, Estimate: 60, Measured: 40, Ratio: 40.0 / 60, EstimatedGrowth: 50, MeasuredGrowth: 30, GrowthRatio: 0.6},
		{Run: "alpha", Card: "02-c2", Position: 3, Estimate: 60, Measured: 90, Ratio: 1.5, EstimatedGrowth: 50, MeasuredGrowth: 80, GrowthRatio: 1.6},
		// A two-card fork's estimate is the peak of both cards together.
		{Run: "alpha", Card: "03-c3, 04-c4", Position: 4, Estimate: 60, Measured: 120, Ratio: 2, EstimatedGrowth: 50, MeasuredGrowth: 110, GrowthRatio: 2.2},
		{Run: "beta", Card: "01-c1", Position: 1, Estimate: 50, Measured: 50, Ratio: 1, EstimatedGrowth: 40, MeasuredGrowth: 40, GrowthRatio: 1},
	}
	if len(got.Rows) != len(wantRows) {
		t.Fatalf("rows = %+v; want %+v", got.Rows, wantRows)
	}
	for i, want := range wantRows {
		row := got.Rows[i]
		if row.Run != want.Run || row.Card != want.Card || row.Position != want.Position || !row.HasGrowthRatio ||
			!near(row.Estimate, want.Estimate) || !near(row.Measured, want.Measured) || !near(row.Ratio, want.Ratio) ||
			!near(row.EstimatedGrowth, want.EstimatedGrowth) || !near(row.MeasuredGrowth, want.MeasuredGrowth) || !near(row.GrowthRatio, want.GrowthRatio) {
			t.Errorf("row %d = %+v; want %+v", i, row, want)
		}
	}

	wantSkips := map[string]string{
		"alpha 05-c5":   "no fork names it",
		"beta 09-ghost": "names a card the plan lacks",
		"nofork":        "no webster fork",
		"afterfork":     "no plan commit before the first fork",
		"nobase":        "no base: no begin-batch result in its webster sessions",
		"lostbase":      "base gone is not in the repository",
		"badplan":       "plan at badplanplan does not parse",
	}
	gotSkips := map[string]string{}
	for _, skip := range got.Skips {
		subject := skip.Run
		if skip.Card != "" {
			subject += " " + skip.Card
		}
		gotSkips[subject] = skip.Reason
	}
	if len(gotSkips) != len(wantSkips) {
		t.Errorf("skips = %v; want subjects %v", gotSkips, wantSkips)
	}
	for subject, reason := range wantSkips {
		if !strings.HasPrefix(gotSkips[subject], reason) {
			t.Errorf("skip of %q = %q; want reason starting %q", subject, gotSkips[subject], reason)
		}
	}

	// Runs left out for their plan or base still list their forks' start rows, every card-naming fork of every run;
	// none of these runs holds a Master template at its base commit, so no row carries a base and none enters the fit.
	if len(got.Starts) != 10 || got.StartFit.NotFitted != "fewer than two distinct positions" || got.StartFit.Forks != 0 {
		t.Errorf("starts = %d rows, fit = %+v; want 10 rows and no fit over zero forks", len(got.Starts), got.StartFit)
	}
	for _, row := range got.Starts {
		if row.HasBase {
			t.Errorf("start row %+v has a base; want none, the base trees hold no Master template", row)
		}
	}

	alpha, alphaGrowth := FitOf(got.Rows[:4]), GrowthFitOf(got.Rows[:4])
	if alpha.Forks != 4 || !near(alpha.Median, 1.75) || alphaGrowth.Forks != 4 || !near(alphaGrowth.Median, 1.85) {
		t.Errorf("alpha fits = %+v, %+v; want 4 forks, medians 1.75 and 1.85", alpha, alphaGrowth)
	}
	overall, overallGrowth := FitOf(got.Rows), GrowthFitOf(got.Rows)
	if overall.Forks != 5 || !near(overall.Median, 1.5) || overallGrowth.Forks != 5 || !near(overallGrowth.Median, 1.6) {
		t.Errorf("overall fits = %+v, %+v; want 5 forks, medians 1.5 and 1.6", overall, overallGrowth)
	}
	if spread, ok := overall.Spread(); !ok || !near(spread, 2) {
		t.Errorf("overall spread = %v, %v; want 2", spread, ok)
	}

	var out bytes.Buffer
	got.WriteMarkdown(&out)
	for _, want := range []string{
		"## Calibration (fit)",
		"| alpha |  | 1 | 10 | n/a (no Master template at base basea) | 10 |",
		"Start fit: not fitted, fewer than two distinct positions (0 forks).",
		"| alpha | 03-c3, 04-c4 | 4 | 60 | 120 | 2.000 | 50 | 110 | 2.200 |",
		"- alpha 05-c5: no fork names it",
		"- alpha: 4 forks, median ratio 1.750, spread 1.548; growth: 4 forks, median ratio 1.850, spread 1.574",
		"- overall: 5 forks, median ratio 1.500, spread 2.000; growth: 5 forks, median ratio 1.600, spread 2.100",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("markdown lacks %q:\n%s", want, out.String())
		}
	}

	t.Run("peak rows are priced at each fork's own position", func(t *testing.T) {
		t.Parallel()
		// The grow profile is fit with a batch_growth of 5, so alpha's forks at positions 1 to 4 estimate 110, 65, 70 and 75, each with an estimated growth of the same 100, 50, 50 and 50 once its own start is taken off.
		grown, err := Calibrate(runs[:1], "grow", configDir, history, base)
		if err != nil {
			t.Fatalf("Calibrate: %v", err)
		}
		wantEstimates, wantGrowths := []float64{110, 65, 70, 75}, []float64{100, 50, 50, 50}
		if len(grown.Rows) != len(wantEstimates) {
			t.Fatalf("rows = %+v; want %d", grown.Rows, len(wantEstimates))
		}
		for i, row := range grown.Rows {
			if !near(row.Estimate, wantEstimates[i]) || !near(row.EstimatedGrowth, wantGrowths[i]) {
				t.Errorf("row %d at position %d = estimate %v, estimated growth %v; want %v and %v", i, row.Position, row.Estimate, row.EstimatedGrowth, wantEstimates[i], wantGrowths[i])
			}
		}
	})

	t.Run("a run with a computed base prices its start and peak from it", func(t *testing.T) {
		t.Parallel()
		basedHistory, basedBase, baseContext := basedRunFixture("delta", map[int]string{1: "internal/a/a.go"}, firstFork)
		basedBase.files["basedelta:internal/a/a.go"] = lines(100)
		basedRuns := []RunTally{{Slug: "delta", BaseSHA: "basedelta", Forks: []ForkTally{fork(firstFork, int(baseContext)+10, int(baseContext)+220, "01-c1")}}}

		based, err := Calibrate(basedRuns, "fit", configDir, basedHistory, basedBase)
		if err != nil {
			t.Fatalf("Calibrate: %v", err)
		}
		if len(based.Starts) != 1 || !based.Starts[0].HasBase || !near(based.Starts[0].Base, baseContext) || !near(based.Starts[0].Estimate, baseContext+10) {
			t.Errorf("starts = %+v; want one row with base %v and estimated start %v", based.Starts, baseContext, baseContext+10)
		}
		// The peak adds the base to the card's 100 lines and the orientation of 10, and the in-fork growth is unchanged: the same base comes off the estimated start.
		if len(based.Rows) != 1 || !near(based.Rows[0].Estimate, baseContext+110) || !near(based.Rows[0].EstimatedGrowth, 100) || !near(based.Rows[0].MeasuredGrowth, 210) {
			t.Errorf("rows = %+v; want one row with estimate %v, estimated growth 100 and measured growth 210", based.Rows, baseContext+110)
		}
	})

	t.Run("a fork with no estimated growth has no growth ratio and stays out of the growth fit", func(t *testing.T) {
		t.Parallel()
		// Card 1 edits an empty file, so the fit profile estimates its peak at the start alone.
		flatHistory := fakeHistory{
			commits: map[string][]gitrepo.SubjectCommit{planCommitSubjectPrefix + "gamma": {{SHA: "gammaplan", Committed: firstFork.Add(-time.Hour)}}},
			files:   map[string]string{},
		}
		for name, data := range planWith(map[int]string{1: "internal/a/a.go", 2: "internal/b/b.go"}) {
			flatHistory.files["gammaplan:"+planDir+name] = data
		}
		flatBase := fakeBase{
			shas:  map[string]bool{"basegamma": true},
			files: map[string]string{"basegamma:internal/a/a.go": "", "basegamma:internal/b/b.go": lines(50)},
		}
		flatRuns := []RunTally{{Slug: "gamma", BaseSHA: "basegamma", Forks: []ForkTally{
			fork(firstFork, 10, 30, "01-c1"),
			fork(firstFork.Add(time.Minute), 10, 90, "02-c2"),
		}}}
		flat, err := Calibrate(flatRuns, "fit", configDir, flatHistory, flatBase)
		if err != nil {
			t.Fatalf("Calibrate: %v", err)
		}
		if len(flat.Rows) != 2 || flat.Rows[0].HasGrowthRatio || !flat.Rows[1].HasGrowthRatio || !near(flat.Rows[1].GrowthRatio, 1.6) {
			t.Fatalf("rows = %+v; want the empty-file fork without a growth ratio and the other with 1.6", flat.Rows)
		}
		if growth := GrowthFitOf(flat.Rows); growth.Forks != 1 || !near(growth.Median, 1.6) {
			t.Errorf("growth fit = %+v; want only the fork with a growth ratio", growth)
		}
		var out bytes.Buffer
		flat.WriteMarkdown(&out)
		for _, want := range []string{
			"| gamma | 01-c1 | 1 | 10 | 30 | 3.000 | 0 | 20 | n/a |",
			"- gamma: 2 forks, median ratio 2.250, spread 1.400; growth: 1 forks, median ratio 1.600, spread 1.000",
		} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("markdown lacks %q:\n%s", want, out.String())
			}
		}
	})
}

// TestCalibrateStartFit asserts the start rows' positions restart in each webster session, and the least-squares fit of the start coefficients over the measured start minus the run's computed base: exact on starts laid on a line above the base, not fitted from a single position, marked unusable when the growth comes out negative, and leaving a run without a readable base listed with its reason and outside the fit.
func TestCalibrateStartFit(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	seedProfile(t, configDir)
	at := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	minute := func(n int) time.Time { return at.Add(time.Duration(n) * time.Minute) }
	history, base, baseContext := basedRunFixture("r", map[int]string{1: "internal/a/a.go"}, at)
	// The orientation is the seeded start minus this known base.
	b := int(baseContext)

	tests := []struct {
		name  string
		forks []ForkTally
		// orphan holds the forks of a second run with no plan commit, so no base.
		orphan []ForkTally
		// wantPositions are the rows' positions in order, the based run's then the orphan's.
		wantPositions   []int
		wantForks       int
		wantNotFitted   string
		wantOrientation float64
		wantGrowth      float64
		wantUnusable    bool
		wantMarkdownIn  []string
	}{
		{
			name: "positions restart in a second session and starts on a line above the base fit exactly",
			forks: []ForkTally{
				forkIn("s1.jsonl", minute(0), b+40, 100, "01-x"),
				forkIn("s1.jsonl", minute(1), b+47, 100, "02-x"),
				forkIn("s1.jsonl", minute(2), b+54, 100, "03-x"),
				forkIn("s2.jsonl", minute(3), b+40, 100, "04-x"),
				forkIn("s2.jsonl", minute(4), b+47, 100, "05-x"),
				// A fork naming no cards is no position.
				forkIn("s2.jsonl", minute(5), b+90, 100),
			},
			wantPositions:   []int{1, 2, 3, 1, 2},
			wantForks:       5,
			wantOrientation: 40,
			wantGrowth:      7,
			wantMarkdownIn:  []string{fmt.Sprintf("| r | s2.jsonl | 2 | %d | %d | %d |", b+47, b, b+15)},
		},
		{
			name: "one distinct position is not fitted",
			forks: []ForkTally{
				forkIn("s1.jsonl", minute(0), b+40, 100, "01-x"),
				forkIn("s2.jsonl", minute(1), b+44, 100, "02-x"),
			},
			wantPositions:  []int{1, 1},
			wantForks:      2,
			wantNotFitted:  "fewer than two distinct positions",
			wantMarkdownIn: []string{"Start fit: not fitted, fewer than two distinct positions (2 forks)."},
		},
		{
			name: "starts falling with position give a negative growth marked unusable",
			forks: []ForkTally{
				forkIn("s1.jsonl", minute(0), b+100, 100, "01-x"),
				forkIn("s1.jsonl", minute(1), b+60, 100, "02-x"),
			},
			wantPositions:   []int{1, 2},
			wantForks:       2,
			wantOrientation: 100,
			wantGrowth:      -40,
			wantUnusable:    true,
			wantMarkdownIn:  []string{"unusable: a negative coefficient"},
		},
		{
			name: "a run without a readable base stays listed with its reason and outside the fit",
			forks: []ForkTally{
				forkIn("s1.jsonl", minute(0), b+40, 100, "01-x"),
				forkIn("s1.jsonl", minute(1), b+47, 100, "02-x"),
			},
			orphan:          []ForkTally{forkIn("s1.jsonl", minute(0), 9999, 100, "01-x"), forkIn("s1.jsonl", minute(1), 1, 100, "02-x")},
			wantPositions:   []int{1, 2, 1, 2},
			wantForks:       2,
			wantOrientation: 40,
			wantGrowth:      7,
			wantMarkdownIn: []string{
				"| orphan | s1.jsonl | 1 | 9999 | n/a (no plan commit before the first fork) | 10 |",
				"Start fit over 2 forks: orientation 40 (profile 10), batch_growth 7 (profile 5)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runs := []RunTally{{Slug: "r", BaseSHA: "baser", Forks: tt.forks}}
			if tt.orphan != nil {
				runs = append(runs, RunTally{Slug: "orphan", Forks: tt.orphan})
			}
			got, err := Calibrate(runs, "grow", configDir, history, base)
			if err != nil {
				t.Fatalf("Calibrate: %v", err)
			}
			var positions []int
			for _, row := range got.Starts {
				positions = append(positions, row.Position)
				want := float64(10 + 5*(row.Position-1))
				if row.HasBase {
					want += baseContext
				}
				if row.Estimate != want {
					t.Errorf("start row of %s at position %d estimate = %v; want the profile's %v", row.Run, row.Position, row.Estimate, want)
				}
			}
			if !slices.Equal(positions, tt.wantPositions) {
				t.Errorf("start row positions = %v; want %v", positions, tt.wantPositions)
			}
			fit := got.StartFit
			if fit.NotFitted != tt.wantNotFitted || fit.Unusable() != tt.wantUnusable || fit.Forks != tt.wantForks {
				t.Fatalf("fit = %+v; want %d forks, not fitted %q, unusable %v", fit, tt.wantForks, tt.wantNotFitted, tt.wantUnusable)
			}
			if tt.wantNotFitted == "" && (math.Abs(fit.Orientation-tt.wantOrientation) > 1e-9 || math.Abs(fit.BatchGrowth-tt.wantGrowth) > 1e-9) {
				t.Errorf("fit = orientation %v, batch_growth %v; want %v, %v", fit.Orientation, fit.BatchGrowth, tt.wantOrientation, tt.wantGrowth)
			}
			var out bytes.Buffer
			got.WriteMarkdown(&out)
			for _, want := range tt.wantMarkdownIn {
				if !strings.Contains(out.String(), want) {
					t.Errorf("markdown lacks %q:\n%s", want, out.String())
				}
			}
		})
	}
}

func TestCalibrateFailsOnAnUnusableProfile(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	seedProfile(t, configDir)
	_, err := Calibrate(nil, "absent", configDir, fakeHistory{}, fakeBase{})
	if err == nil || !strings.Contains(err.Error(), "batcher.yaml") {
		t.Errorf("Calibrate with an absent profile error = %v; want one naming batcher.yaml", err)
	}
}

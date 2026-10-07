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

// seedProfile writes a batcher.yaml whose "fit" profile estimates one card's peak as master_base + the lines of its file, with every other coefficient 0;
// its "grow" profile is "fit" with a batch_growth of 5.
func seedProfile(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(configengine.ConfigDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	const config = `profiles:
  fit:
    weights:
      master_base: 10
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
      master_base: 10
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

	// Every fork starts at 10 tokens, the profile's master_base, so the start rows lie on a flat line.
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

	// Runs left out for their plan or base still contribute their forks' start rows: every card-naming fork of every run.
	if len(got.Starts) != 10 || got.StartFit.NotFitted != "" || !near(got.StartFit.MasterBase, 10) || !near(got.StartFit.BatchGrowth, 0) {
		t.Errorf("starts = %d rows, fit = %+v; want 10 rows fitted to master_base 10, batch_growth 0", len(got.Starts), got.StartFit)
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
		"| alpha |  | 1 | 10 | 10 |",
		"Start fit over 10 forks: master_base 10 (profile 10), batch_growth 0 (profile 0), residual spread 1.000.",
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
}

// TestCalibrateStartFit asserts the start rows' positions restart in each webster session, and the least-squares fit of the start coefficients: exact on starts laid on a line, not fitted from a single position, and marked unusable when the growth comes out negative.
func TestCalibrateStartFit(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	seedProfile(t, configDir)
	at := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	minute := func(n int) time.Time { return at.Add(time.Duration(n) * time.Minute) }

	tests := []struct {
		name  string
		forks []ForkTally
		// wantPositions are the rows' positions in order.
		wantPositions  []int
		wantNotFitted  string
		wantBase       float64
		wantGrowth     float64
		wantUnusable   bool
		wantMarkdownIn string
	}{
		{
			name: "positions restart in a second session and starts on a line fit exactly",
			forks: []ForkTally{
				forkIn("s1.jsonl", minute(0), 40, 100, "01-x"),
				forkIn("s1.jsonl", minute(1), 47, 100, "02-x"),
				forkIn("s1.jsonl", minute(2), 54, 100, "03-x"),
				forkIn("s2.jsonl", minute(3), 40, 100, "04-x"),
				forkIn("s2.jsonl", minute(4), 47, 100, "05-x"),
				// A fork naming no cards is no position.
				forkIn("s2.jsonl", minute(5), 90, 100),
			},
			wantPositions:  []int{1, 2, 3, 1, 2},
			wantBase:       40,
			wantGrowth:     7,
			wantMarkdownIn: "| r | s2.jsonl | 2 | 47 | 15 |",
		},
		{
			name: "one distinct position is not fitted",
			forks: []ForkTally{
				forkIn("s1.jsonl", minute(0), 40, 100, "01-x"),
				forkIn("s2.jsonl", minute(1), 44, 100, "02-x"),
			},
			wantPositions:  []int{1, 1},
			wantNotFitted:  "fewer than two distinct positions",
			wantMarkdownIn: "Start fit: not fitted, fewer than two distinct positions (2 forks).",
		},
		{
			name: "starts falling with position give a negative growth marked unusable",
			forks: []ForkTally{
				forkIn("s1.jsonl", minute(0), 100, 100, "01-x"),
				forkIn("s1.jsonl", minute(1), 60, 100, "02-x"),
			},
			wantPositions:  []int{1, 2},
			wantBase:       100,
			wantGrowth:     -40,
			wantUnusable:   true,
			wantMarkdownIn: "unusable: a negative coefficient",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Calibrate([]RunTally{{Slug: "r", Forks: tt.forks}}, "grow", configDir, fakeHistory{}, fakeBase{})
			if err != nil {
				t.Fatalf("Calibrate: %v", err)
			}
			var positions []int
			for _, row := range got.Starts {
				positions = append(positions, row.Position)
				if want := float64(10 + 5*(row.Position-1)); row.Estimate != want {
					t.Errorf("start row at position %d estimate = %v; want the profile's %v", row.Position, row.Estimate, want)
				}
			}
			if !slices.Equal(positions, tt.wantPositions) {
				t.Errorf("start row positions = %v; want %v", positions, tt.wantPositions)
			}
			fit := got.StartFit
			if fit.NotFitted != tt.wantNotFitted || fit.Unusable() != tt.wantUnusable {
				t.Fatalf("fit = %+v; want not fitted %q, unusable %v", fit, tt.wantNotFitted, tt.wantUnusable)
			}
			if tt.wantNotFitted == "" && (math.Abs(fit.MasterBase-tt.wantBase) > 1e-9 || math.Abs(fit.BatchGrowth-tt.wantGrowth) > 1e-9) {
				t.Errorf("fit = master_base %v, batch_growth %v; want %v, %v", fit.MasterBase, fit.BatchGrowth, tt.wantBase, tt.wantGrowth)
			}
			var out bytes.Buffer
			got.WriteMarkdown(&out)
			if !strings.Contains(out.String(), tt.wantMarkdownIn) {
				t.Errorf("markdown lacks %q:\n%s", tt.wantMarkdownIn, out.String())
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

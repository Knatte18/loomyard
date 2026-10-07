// calibrate_test.go drives the calibration over an in-memory plan history and base tree and a
// fixture batcher.yaml: the row arithmetic, the skip reasons and the fit summary.
// Tier-1 (no git, no spawn).

package main

import (
	"bytes"
	"fmt"
	"math"
	"os"
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

// seedProfile writes a batcher.yaml whose "fit" profile prices one card as target_messages x
// (startup_context + the lines of its file), with every other coefficient 0.
func seedProfile(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(configengine.ConfigDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	const config = `profiles:
  fit:
    weights:
      startup_context: 10
      fork_messages: 0
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

func fork(started time.Time, input int, cards ...string) ForkTally {
	return ForkTally{Cards: cards, Started: started, Usage: Usage{Input: input}}
}

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
			"baseb:internal/a/a.go": lines(40),
		},
	}

	runs := []RunTally{
		{Slug: "alpha", BaseSHA: "basea", Forks: []ForkTally{
			fork(firstFork, 220, "01-c1"),
			fork(firstFork.Add(time.Minute), 40, "02-c2"),
			fork(firstFork.Add(2*time.Minute), 50, "02-c2"),
			fork(firstFork.Add(3*time.Minute), 999, "03-c3", "04-c4"),
		}},
		{Slug: "beta", BaseSHA: "baseb", Forks: []ForkTally{fork(firstFork, 50, "01-c1")}},
		{Slug: "nofork", BaseSHA: "basea"},
		{Slug: "afterfork", BaseSHA: "basea", Forks: []ForkTally{fork(firstFork, 1, "01-c1")}},
		{Slug: "nobase", Forks: []ForkTally{fork(firstFork, 1, "01-c1")}},
		{Slug: "lostbase", BaseSHA: "gone", Forks: []ForkTally{fork(firstFork, 1, "01-c1")}},
		{Slug: "badplan", BaseSHA: "basebad", Forks: []ForkTally{fork(firstFork, 1, "01-c1")}},
	}

	got, err := Calibrate(runs, "fit", configDir, history, base)
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}

	wantRows := []CalibrationRow{
		{Run: "alpha", Card: "01-c1", Estimate: 110, Measured: 220, Ratio: 2},
		{Run: "alpha", Card: "02-c2", Estimate: 60, Measured: 90, Ratio: 1.5},
		{Run: "beta", Card: "01-c1", Estimate: 50, Measured: 50, Ratio: 1},
	}
	if len(got.Rows) != len(wantRows) {
		t.Fatalf("rows = %+v; want %+v", got.Rows, wantRows)
	}
	for i, want := range wantRows {
		if got.Rows[i] != want {
			t.Errorf("row %d = %+v; want %+v", i, got.Rows[i], want)
		}
	}

	wantSkips := map[string]string{
		"alpha 03-c3": multiCardSkipReason,
		"alpha 04-c4": multiCardSkipReason,
		"alpha 05-c5": "no fork names it",
		"nofork":      "no webster fork",
		"afterfork":   "no plan commit before the first fork",
		"nobase":      "no base: no begin-batch result in its webster sessions",
		"lostbase":    "base gone is not in the repository",
		"badplan":     "plan at badplanplan does not parse",
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

	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	alpha := FitOf(got.Rows[:2])
	if alpha.Cards != 2 || !near(alpha.Median, 1.75) {
		t.Errorf("alpha fit = %+v; want 2 cards, median 1.75", alpha)
	}
	if spread, ok := alpha.Spread(); !ok || !near(spread, 1.875/1.625) {
		t.Errorf("alpha spread = %v, %v; want %v", spread, ok, 1.875/1.625)
	}
	overall := FitOf(got.Rows)
	if overall.Cards != 3 || !near(overall.Median, 1.5) {
		t.Errorf("overall fit = %+v; want 3 cards, median 1.5", overall)
	}
	if spread, ok := overall.Spread(); !ok || !near(spread, 1.75/1.25) {
		t.Errorf("overall spread = %v, %v; want %v", spread, ok, 1.75/1.25)
	}

	var out bytes.Buffer
	got.WriteMarkdown(&out)
	for _, want := range []string{
		"## Calibration (fit)",
		"| alpha | 01-c1 | 110 | 220 | 2.000 |",
		"- alpha 05-c5: no fork names it",
		"- alpha: 2 cards, median ratio 1.750, spread 1.154",
		"- overall: 3 cards, median ratio 1.500, spread 1.400",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("markdown lacks %q:\n%s", want, out.String())
		}
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

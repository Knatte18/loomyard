// calibrate.go sets the batcher's per-card estimate beside the measured weight of the Webster
// fork that ran the card, for past runs: the plan comes from the history repository, the tree the
// estimate reads is the run's base commit in the code repository, and the profile's weights come
// from batcher.yaml.
// Read-only: it calls batcher.SegmentCost, the same function the live batchifier calls.

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// planCommitSubjectPrefix starts the subject of the commit that records a run's plan artifacts.
const planCommitSubjectPrefix = "loom: plan artifacts for "

// multiCardSkipReason is the skip reason of a card a fork ran together with other cards.
const multiCardSkipReason = "ran in a multi-card fork"

// PlanHistory reads the repository holding the runs' plan commits.
type PlanHistory interface {
	CommitsWithSubject(subject string) ([]gitrepo.SubjectCommit, error)
	FileAtRevision(rev, relPath string) ([]byte, error)
}

// BaseTrees reads the code repository's trees, where each run's base commit lives.
type BaseTrees interface {
	SHAExists(sha string) bool
	FileAtRevision(rev, relPath string) ([]byte, error)
	FilesInDirAtRevision(rev, dir string) ([]string, error)
}

// CalibrationRow is one card's estimate beside its measured fork weight.
type CalibrationRow struct {
	Run, Card string
	Estimate  float64
	Measured  float64
	// Ratio is Measured over Estimate.
	Ratio float64
}

// CalibrationSkip is a run, or one card of a run when Card is set, left out with its reason.
type CalibrationSkip struct {
	Run, Card, Reason string
}

// Calibration is the estimator run on past plans beside the measured fork weights.
type Calibration struct {
	Profile string
	Rows    []CalibrationRow
	Skips   []CalibrationSkip
}

// Fit summarizes the ratios of a set of rows.
type Fit struct {
	Cards  int
	Median float64
	// P25 and P75 are the 25th and 75th percentile of the ratios.
	P25, P75 float64
}

// Spread is the 75th percentile of the ratios over the 25th, and false when the 25th is 0.
func (f Fit) Spread() (float64, bool) {
	if f.P25 == 0 {
		return 0, false
	}
	return f.P75 / f.P25, true
}

// FitOf summarizes the ratios of rows; no rows is a zero Fit.
func FitOf(rows []CalibrationRow) Fit {
	ratios := make([]float64, 0, len(rows))
	for _, row := range rows {
		ratios = append(ratios, row.Ratio)
	}
	sort.Float64s(ratios)
	if len(ratios) == 0 {
		return Fit{}
	}
	return Fit{
		Cards:  len(ratios),
		Median: percentile(ratios, 0.5),
		P25:    percentile(ratios, 0.25),
		P75:    percentile(ratios, 0.75),
	}
}

// percentile interpolates linearly between the closest ranks of the sorted, non-empty values.
func percentile(sorted []float64, p float64) float64 {
	rank := p * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	return sorted[lower] + (rank-float64(lower))*(sorted[upper]-sorted[lower])
}

// Calibrate runs the estimator over every run's plan, with the weights of the profile named in
// batcher.yaml under configDir.
// A run or card that cannot be reconstructed is a skip, never an error; an error is a failed read
// of the history or the base tree, or an unusable profile.
func Calibrate(runs []RunTally, profile, configDir string, history PlanHistory, base BaseTrees) (Calibration, error) {
	weights, err := batcher.ProfileWeights(configDir, profile)
	if err != nil {
		return Calibration{}, err
	}
	calibration := Calibration{Profile: profile}
	for _, run := range runs {
		if err := calibration.addRun(run, weights, history, base); err != nil {
			return Calibration{}, err
		}
	}
	return calibration, nil
}

func (c *Calibration) addRun(run RunTally, weights batcher.Weights, history PlanHistory, base BaseTrees) error {
	skipRun := func(reason string) {
		c.Skips = append(c.Skips, CalibrationSkip{Run: run.Slug, Reason: reason})
	}

	firstFork := earliestFork(run.Forks)
	if firstFork.IsZero() {
		skipRun("no webster fork")
		return nil
	}

	commits, err := history.CommitsWithSubject(planCommitSubjectPrefix + run.Slug)
	if err != nil {
		return fmt.Errorf("find plan commits of %s: %w", run.Slug, err)
	}
	planCommit := ""
	for _, commit := range commits {
		if commit.Committed.Before(firstFork) {
			planCommit = commit.SHA
			break
		}
	}
	if planCommit == "" {
		skipRun("no plan commit before the first fork")
		return nil
	}

	plan, err := planparser.ParsePlanFrom(planparser.PlanDirRel(), func(name string) ([]byte, error) {
		data, err := history.FileAtRevision(planCommit, path.Join(planparser.PlanDirRel(), name))
		if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
			return nil, fmt.Errorf("%s at %s: %w", name, planCommit, fs.ErrNotExist)
		}
		return data, err
	})
	if err != nil {
		skipRun(fmt.Sprintf("plan at %s does not parse: %v", planCommit, err))
		return nil
	}

	switch {
	case run.BaseSHA == "":
		skipRun("no base: no begin-batch result in its webster sessions")
		return nil
	case !base.SHAExists(run.BaseSHA):
		skipRun(fmt.Sprintf("base %s is not in the repository", run.BaseSHA))
		return nil
	}
	sizes := treeSizes{trees: base, rev: run.BaseSHA}

	for _, card := range plan.Cards {
		id := fmt.Sprintf("%02d-%s", card.Number, card.Slug)
		skipCard := func(reason string) {
			c.Skips = append(c.Skips, CalibrationSkip{Run: run.Slug, Card: id, Reason: reason})
		}

		var measured float64
		named, multi := false, false
		for _, fork := range run.Forks {
			if !slices.Contains(fork.Cards, id) {
				continue
			}
			named = true
			multi = multi || len(fork.Cards) > 1
			measured += fork.Usage.Weight()
		}
		switch {
		case !named:
			skipCard("no fork names it")
			continue
		case multi:
			skipCard(multiCardSkipReason)
			continue
		}

		estimate, err := batcher.SegmentCost(plan, []planparser.Card{card}, sizes, weights)
		if err != nil {
			return fmt.Errorf("estimate %s of %s: %w", id, run.Slug, err)
		}
		if estimate == 0 {
			skipCard("estimate is 0")
			continue
		}
		c.Rows = append(c.Rows, CalibrationRow{Run: run.Slug, Card: id, Estimate: estimate, Measured: measured, Ratio: measured / estimate})
	}
	return nil
}

// earliestFork is the smallest start time of any fork, zero when no fork has one.
func earliestFork(forks []ForkTally) time.Time {
	var earliest time.Time
	for _, fork := range forks {
		if !fork.Started.IsZero() && (earliest.IsZero() || fork.Started.Before(earliest)) {
			earliest = fork.Started
		}
	}
	return earliest
}

// treeSizes is a batcher.SizeSource over the tree of one commit.
type treeSizes struct {
	trees BaseTrees
	rev   string
}

func (t treeSizes) Lines(relPath string) (int, bool, error) {
	data, err := t.trees.FileAtRevision(t.rev, relPath)
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	lines := strings.Count(string(data), "\n")
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lines++
	}
	return lines, true, nil
}

func (t treeSizes) TestFiles(dir string) ([]string, error) {
	names, err := t.trees.FilesInDirAtRevision(t.rev, dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			files = append(files, path.Join(dir, name))
		}
	}
	return files, nil
}

// WriteMarkdown writes the calibration section: one row per card, the skipped runs and cards each
// with its reason, and the fit per run and overall.
func (c Calibration) WriteMarkdown(w io.Writer) {
	fmt.Fprintf(w, "## Calibration (%s)\n\n", c.Profile)
	fmt.Fprintln(w, "| run | card | estimate | measured | ratio |")
	fmt.Fprintln(w, "|---|---|---|---|---|")
	for _, row := range c.Rows {
		fmt.Fprintf(w, "| %s | %s | %.0f | %.0f | %.3f |\n", row.Run, row.Card, row.Estimate, row.Measured, row.Ratio)
	}
	fmt.Fprintln(w)

	if len(c.Skips) > 0 {
		fmt.Fprintln(w, "Skipped:")
		fmt.Fprintln(w)
		for _, skip := range c.Skips {
			subject := skip.Run
			if skip.Card != "" {
				subject += " " + skip.Card
			}
			fmt.Fprintf(w, "- %s: %s\n", subject, skip.Reason)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "Fit:")
	fmt.Fprintln(w)
	var order []string
	byRun := map[string][]CalibrationRow{}
	for _, row := range c.Rows {
		if _, seen := byRun[row.Run]; !seen {
			order = append(order, row.Run)
		}
		byRun[row.Run] = append(byRun[row.Run], row)
	}
	for _, run := range order {
		writeFit(w, run, FitOf(byRun[run]))
	}
	writeFit(w, "overall", FitOf(c.Rows))
	fmt.Fprintln(w)
}

func writeFit(w io.Writer, label string, fit Fit) {
	if fit.Cards == 0 {
		fmt.Fprintf(w, "- %s: 0 cards\n", label)
		return
	}
	spread := "n/a"
	if s, ok := fit.Spread(); ok {
		spread = fmt.Sprintf("%.3f", s)
	}
	fmt.Fprintf(w, "- %s: %d cards, median ratio %.3f, spread %s\n", label, fit.Cards, fit.Median, spread)
}

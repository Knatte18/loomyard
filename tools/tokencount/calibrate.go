// calibrate.go sets the batcher's estimates beside what past Webster forks measured: each fork's start context against the start the profile estimates for its position and the fit of the two start coefficients, and each fork's peak context against the estimate of the cards it ran.
// The plan comes from the history repository, the tree the estimate reads is the run's base commit in the code repository, and the profile's weights come from batcher.yaml.
// Read-only: it calls batcher.PeakContext, the same estimate the live batchifier bounds its batches by.

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// planCommitSubjectPrefix starts the subject of the commit that records a run's plan artifacts.
const planCommitSubjectPrefix = "loom: plan artifacts for "

// websterRecordSubjectPrefix starts the subject of the commit that records a run's webster state.
const websterRecordSubjectPrefix = "loom: webster run record for "

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

// StartRow is one card-naming fork's measured start context beside the start the profile estimates for its position.
type StartRow struct {
	Run string
	// Session is the file name of the webster session the fork ran under.
	Session string
	// Position is one plus the card-naming forks before it in the same session, by start time.
	Position int
	// Measured is the fork's StartContext.
	Measured float64
	// Estimate is the profile's Orientation + (Position-1) x BatchGrowth.
	Estimate float64
}

// StartFit is the least-squares fit of the start coefficients over every StartRow.
type StartFit struct {
	// Forks is the number of rows fitted over.
	Forks int
	// NotFitted is why no fit exists, empty when the fit was made.
	NotFitted string
	// MasterBase and BatchGrowth are the fitted coefficients, meaningful when NotFitted is empty.
	MasterBase, BatchGrowth float64
	// Residual summarizes each fork's measured start over its fitted start, over the forks whose fitted start is positive.
	Residual Fit
}

// Unusable reports whether a fit was made with a negative coefficient, which the batcher.yaml loader refuses.
func (f StartFit) Unusable() bool {
	return f.NotFitted == "" && (f.MasterBase < 0 || f.BatchGrowth < 0)
}

// CalibrationRow is one fork's peak-context estimate beside its measured peak context.
type CalibrationRow struct {
	Run string
	// Card holds the fork's card ids, comma-separated.
	Card string
	// Position is the fork's position in its session.
	Position int
	// Estimate is batcher.PeakContext of the fork's cards at Position.
	Estimate float64
	Measured float64
	// Ratio is Measured over Estimate.
	Ratio float64
	// MeasuredGrowth is Measured minus the fork's measured start, and EstimatedGrowth is Estimate minus the profile's start estimate for Position.
	MeasuredGrowth, EstimatedGrowth float64
	// GrowthRatio is MeasuredGrowth over EstimatedGrowth, and HasGrowthRatio is false when EstimatedGrowth is not positive.
	GrowthRatio    float64
	HasGrowthRatio bool
}

// CalibrationSkip is a run, or the cards of one fork or one card of a run when Card is set, left out with its reason.
type CalibrationSkip struct {
	Run, Card, Reason string
}

// Calibration is the estimator run on past plans beside the measured forks.
type Calibration struct {
	Profile string
	// Weights are the profile's coefficients.
	Weights  batcher.Weights
	Starts   []StartRow
	StartFit StartFit
	Rows     []CalibrationRow
	Skips    []CalibrationSkip
}

// Fit summarizes a set of ratios.
type Fit struct {
	Forks  int
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

// FitOf summarizes the peak ratios of rows;
// no rows is a zero Fit.
func FitOf(rows []CalibrationRow) Fit {
	ratios := make([]float64, 0, len(rows))
	for _, row := range rows {
		ratios = append(ratios, row.Ratio)
	}
	return fitOfRatios(ratios)
}

// GrowthFitOf summarizes the in-fork growth ratios of the rows that have one.
func GrowthFitOf(rows []CalibrationRow) Fit {
	var ratios []float64
	for _, row := range rows {
		if row.HasGrowthRatio {
			ratios = append(ratios, row.GrowthRatio)
		}
	}
	return fitOfRatios(ratios)
}

// fitOfRatios summarizes ratios, which it may reorder;
// none is a zero Fit.
func fitOfRatios(ratios []float64) Fit {
	sort.Float64s(ratios)
	if len(ratios) == 0 {
		return Fit{}
	}
	return Fit{
		Forks:  len(ratios),
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

// Calibrate runs the estimator over every run's forks, with the weights of the profile named in batcher.yaml under configDir.
// A run or fork that cannot be reconstructed is a skip, never an error;
// an error is a failed read of the history or the base tree, or an unusable profile.
func Calibrate(runs []RunTally, profile, configDir string, history PlanHistory, base BaseTrees) (Calibration, error) {
	weights, err := batcher.ProfileWeights(configDir, profile)
	if err != nil {
		return Calibration{}, err
	}
	calibration := Calibration{Profile: profile, Weights: weights}
	for _, run := range runs {
		calibration.addStarts(run)
	}
	calibration.StartFit = fitStart(calibration.Starts)
	for _, run := range runs {
		if err := calibration.addRun(run, history, base); err != nil {
			return Calibration{}, err
		}
	}
	return calibration, nil
}

// startEstimate is the start context the weights estimate for a fork at the 1-based position.
func startEstimate(w batcher.Weights, position int) float64 {
	return w.Orientation + float64(position-1)*w.BatchGrowth
}

// forkPositions returns each fork's 1-based position in its Merriam session: one plus the card-naming forks before it in that session, by start time.
// A fork that names no cards has position 0.
// A resumed session is a session of its own, so its positions restart at 1.
func forkPositions(forks []ForkTally) []int {
	order := make([]int, len(forks))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return forks[order[a]].Started.Before(forks[order[b]].Started) })
	next := map[string]int{}
	positions := make([]int, len(forks))
	for _, i := range order {
		if len(forks[i].Cards) == 0 {
			continue
		}
		next[forks[i].Session]++
		positions[i] = next[forks[i].Session]
	}
	return positions
}

// addStarts adds a row per card-naming fork of run, which needs only the run's transcripts.
func (c *Calibration) addStarts(run RunTally) {
	for i, position := range forkPositions(run.Forks) {
		if position == 0 {
			continue
		}
		fork := run.Forks[i]
		c.Starts = append(c.Starts, StartRow{Run: run.Slug, Session: fork.Session, Position: position, Measured: float64(fork.StartContext), Estimate: startEstimate(c.Weights, position)})
	}
}

// fitStart fits MasterBase and BatchGrowth by least squares of the measured start over position - 1.
func fitStart(rows []StartRow) StartFit {
	fit := StartFit{Forks: len(rows)}
	positions := map[int]bool{}
	for _, row := range rows {
		positions[row.Position] = true
	}
	if len(positions) < 2 {
		fit.NotFitted = "fewer than two distinct positions"
		return fit
	}
	var n, sumX, sumY, sumXX, sumXY float64
	for _, row := range rows {
		x := float64(row.Position - 1)
		n++
		sumX += x
		sumY += row.Measured
		sumXX += x * x
		sumXY += x * row.Measured
	}
	fit.BatchGrowth = (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)
	fit.MasterBase = (sumY - fit.BatchGrowth*sumX) / n
	var ratios []float64
	for _, row := range rows {
		if fitted := fit.MasterBase + fit.BatchGrowth*float64(row.Position-1); fitted > 0 {
			ratios = append(ratios, row.Measured/fitted)
		}
	}
	fit.Residual = fitOfRatios(ratios)
	return fit
}

// readRunPlan reads the plan a run executed: the newest plan commit made before the run's first fork, parsed over history.
// A run whose plan cannot be reconstructed returns a nil plan and the reason;
// an error is a failed read of the history.
func readRunPlan(run RunTally, history PlanHistory) (*planparser.Plan, string, error) {
	firstFork := earliestFork(run.Forks)
	if firstFork.IsZero() {
		return nil, "no webster fork", nil
	}

	commits, err := history.CommitsWithSubject(planCommitSubjectPrefix + run.Slug)
	if err != nil {
		return nil, "", fmt.Errorf("find plan commits of %s: %w", run.Slug, err)
	}
	planCommit := ""
	for _, commit := range commits {
		if commit.Committed.Before(firstFork) {
			planCommit = commit.SHA
			break
		}
	}
	if planCommit == "" {
		return nil, "no plan commit before the first fork", nil
	}

	plan, err := planparser.ParsePlanFrom(planparser.PlanDirRel(), func(name string) ([]byte, error) {
		data, err := history.FileAtRevision(planCommit, path.Join(planparser.PlanDirRel(), name))
		if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
			return nil, fmt.Errorf("%s at %s: %w", name, planCommit, fs.ErrNotExist)
		}
		return data, err
	})
	if err != nil {
		return nil, fmt.Sprintf("plan at %s does not parse: %v", planCommit, err), nil
	}
	return plan, "", nil
}

// addRun adds a row per card-naming fork of run whose cards are all cards of its plan, and the skips of what cannot be reconstructed.
func (c *Calibration) addRun(run RunTally, history PlanHistory, base BaseTrees) error {
	skip := func(card, reason string) {
		c.Skips = append(c.Skips, CalibrationSkip{Run: run.Slug, Card: card, Reason: reason})
	}

	plan, reason, err := readRunPlan(run, history)
	if err != nil {
		return err
	}
	if plan == nil {
		skip("", reason)
		return nil
	}

	switch {
	case run.BaseSHA == "":
		skip("", "no base: no begin-batch result in its webster sessions")
		return nil
	case !base.SHAExists(run.BaseSHA):
		skip("", fmt.Sprintf("base %s is not in the repository", run.BaseSHA))
		return nil
	}
	sizes := treeSizes{trees: base, rev: run.BaseSHA}

	cardsByID := map[string]planparser.Card{}
	for _, card := range plan.Cards {
		cardsByID[fmt.Sprintf("%02d-%s", card.Number, card.Slug)] = card
	}
	named := map[string]bool{}
	for i, position := range forkPositions(run.Forks) {
		if position == 0 {
			continue
		}
		fork := run.Forks[i]
		subject := strings.Join(fork.Cards, ", ")
		var cards []planparser.Card
		missing := ""
		for _, id := range fork.Cards {
			card, ok := cardsByID[id]
			if !ok {
				missing = id
				break
			}
			named[id] = true
			cards = append(cards, card)
		}
		if missing != "" {
			skip(subject, "names a card the plan lacks: "+missing)
			continue
		}

		estimate, err := batcher.PeakContext(plan, cards, sizes, c.Weights, batcher.StartBase{}, position)
		if err != nil {
			return fmt.Errorf("estimate %s of %s: %w", subject, run.Slug, err)
		}
		if estimate == 0 {
			skip(subject, "estimate is 0")
			continue
		}
		measured := float64(fork.PeakContext)
		row := CalibrationRow{
			Run: run.Slug, Card: subject, Position: position, Estimate: estimate, Measured: measured, Ratio: measured / estimate,
			MeasuredGrowth:  measured - float64(fork.StartContext),
			EstimatedGrowth: estimate - startEstimate(c.Weights, position),
		}
		if row.EstimatedGrowth > 0 {
			row.GrowthRatio, row.HasGrowthRatio = row.MeasuredGrowth/row.EstimatedGrowth, true
		}
		c.Rows = append(c.Rows, row)
	}
	for _, card := range plan.Cards {
		if id := fmt.Sprintf("%02d-%s", card.Number, card.Slug); !named[id] {
			skip(id, "no fork names it")
		}
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

// WriteMarkdown writes the calibration section: the start-context table and its fit, the peak table with one row per fork, the skipped runs, forks and cards each with its reason, and the peak and growth fits per run and overall.
func (c Calibration) WriteMarkdown(w io.Writer) {
	fmt.Fprintf(w, "## Calibration (%s)\n\n", c.Profile)
	fmt.Fprintln(w, "Start context:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| run | session | position | measured start | estimated start |")
	fmt.Fprintln(w, "|---|---|---|---|---|")
	for _, row := range c.Starts {
		fmt.Fprintf(w, "| %s | %s | %d | %.0f | %.0f |\n", row.Run, row.Session, row.Position, row.Measured, row.Estimate)
	}
	fmt.Fprintln(w)
	c.StartFit.write(w, c.Weights)

	fmt.Fprintln(w, "Peak context:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| run | cards | position | estimate | measured | ratio | estimated growth | measured growth | growth ratio |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|")
	for _, row := range c.Rows {
		growthRatio := "n/a"
		if row.HasGrowthRatio {
			growthRatio = fmt.Sprintf("%.3f", row.GrowthRatio)
		}
		fmt.Fprintf(w, "| %s | %s | %d | %.0f | %.0f | %.3f | %.0f | %.0f | %s |\n", row.Run, row.Card, row.Position, row.Estimate, row.Measured, row.Ratio, row.EstimatedGrowth, row.MeasuredGrowth, growthRatio)
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
		writeFit(w, run, byRun[run])
	}
	writeFit(w, "overall", c.Rows)
	fmt.Fprintln(w)
}

// write prints the fitted start coefficients beside the profile's, or why there is no fit.
func (f StartFit) write(w io.Writer, profile batcher.Weights) {
	if f.NotFitted != "" {
		fmt.Fprintf(w, "Start fit: not fitted, %s (%d forks).\n\n", f.NotFitted, f.Forks)
		return
	}
	fmt.Fprintf(w, "Start fit over %d forks: master_base %.0f (profile %.0f), batch_growth %.0f (profile %.0f), residual spread %s", f.Forks, f.MasterBase, profile.Orientation, f.BatchGrowth, profile.BatchGrowth, spreadText(f.Residual))
	if f.Unusable() {
		fmt.Fprint(w, "; unusable: a negative coefficient, which batcher.yaml refuses")
	}
	fmt.Fprint(w, ".\n\n")
}

// spreadText renders a fit's spread, n/a when it has none.
func spreadText(fit Fit) string {
	if s, ok := fit.Spread(); ok {
		return fmt.Sprintf("%.3f", s)
	}
	return "n/a"
}

func writeFit(w io.Writer, label string, rows []CalibrationRow) {
	peak, growth := FitOf(rows), GrowthFitOf(rows)
	if peak.Forks == 0 {
		fmt.Fprintf(w, "- %s: 0 forks\n", label)
		return
	}
	fmt.Fprintf(w, "- %s: %d forks, median ratio %.3f, spread %s; growth: %d forks, median ratio %.3f, spread %s\n",
		label, peak.Forks, peak.Median, spreadText(peak), growth.Forks, growth.Median, spreadText(growth))
}

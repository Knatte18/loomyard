// calibrate.go sets the batcher's estimates beside what past Webster forks measured: each fork's start context against the start the profile estimates for its position and the fit of the two start coefficients, and each fork's peak context against the estimate of the cards it ran.
// The plan comes from the history repository, the tree the estimate reads is the run's base commit in the code repository, and the profile's weights come from batcher.yaml.
// Each run's Merriam base is reconstructed from those trees with websterengine.MerriamBaseOf, so the start fit and the peak estimates price what the live batchifier prices.
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
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
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

// masterTemplatePath is the repository's copy of the Master template, which Merriam loads at start.
const masterTemplatePath = "contracts/stencils/webster/webster-template-master.md"

// directiveStencilPath is the repository's copy of the orchestrator PATTERN directive stencil, which the Master template inlines with PATTERN.md.
const directiveStencilPath = "contracts/stencils/pattern/pattern-directive-orchestrator.md"

// StartRow is one card-naming fork's measured start context beside the start the profile estimates for its position.
type StartRow struct {
	Run string
	// Session is the file name of the webster session the fork ran under.
	Session string
	// Position is one plus the card-naming forks before it in the same session, by start time.
	Position int
	// Measured is the fork's StartContext.
	Measured float64
	// Base is the context of the run's computed Merriam base under the profile's context_per_line, meaningful when HasBase.
	Base    float64
	HasBase bool
	// NoBase is why the run has no computed base, empty when HasBase.
	NoBase string
	// Estimate is Base + the profile's Orientation + (Position-1) x BatchGrowth, with a Base of 0 for a run without one.
	Estimate float64
}

// StartFit is the least-squares fit of the start coefficients over every StartRow that carries a base.
type StartFit struct {
	// Forks is the number of rows fitted over.
	Forks int
	// NotFitted is why no fit exists, empty when the fit was made.
	NotFitted string
	// Orientation and BatchGrowth are the fitted coefficients, meaningful when NotFitted is empty.
	Orientation, BatchGrowth float64
	// Residual summarizes each fork's measured start over its fitted start, over the forks whose fitted start is positive.
	Residual Fit
}

// Unusable reports whether a fit was made with a negative coefficient, which the batcher.yaml loader refuses.
func (f StartFit) Unusable() bool {
	return f.NotFitted == "" && (f.Orientation < 0 || f.BatchGrowth < 0)
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

// FixedRow is one run's measured Merriam start beside the lines of the texts MerriamBase counts.
type FixedRow struct {
	Run string
	// Session is the file name of the Merriam session measured.
	Session string
	// Measured is the session's StartContext.
	Measured float64
	// Lines is the summed line count of the run's merriamTexts.
	Lines int
	// Fixed is Measured minus Lines at the profile's context_per_line.
	Fixed float64
}

// FixedContext is the Merriam fixed-context section: what the system prompt, tools and launch-time skills weigh, derived per run from its measured start.
type FixedContext struct {
	Rows  []FixedRow
	Skips []CalibrationSkip
	// Fit summarizes the rows' Fixed: its median and the 25th and 75th percentiles.
	Fit Fit
	// Current is the fixed context the live Merriam base uses.
	Current float64
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
	Fixed    FixedContext

	// bases is each run's computed Merriam base by run slug.
	bases map[string]runStartBase
}

// runStartBase is the Merriam base reconstructed for one run.
type runStartBase struct {
	base batcher.StartBase
	// reason is why no base could be reconstructed, empty when one was.
	reason string
}

func (b runStartBase) found() bool { return b.reason == "" }

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
	calibration := Calibration{Profile: profile, Weights: weights, bases: map[string]runStartBase{}}
	for _, run := range runs {
		if err := calibration.addStarts(run, history, base); err != nil {
			return Calibration{}, err
		}
		if err := calibration.addFixed(run, base); err != nil {
			return Calibration{}, err
		}
	}
	calibration.Fixed.summarize()
	calibration.StartFit = fitStart(calibration.Starts)
	for _, run := range runs {
		if err := calibration.addRun(run, history, base); err != nil {
			return Calibration{}, err
		}
	}
	return calibration, nil
}

// baseContext is the context of the start base under w: its lines at ContextPerLine plus its fixed part.
func baseContext(w batcher.Weights, base batcher.StartBase) float64 {
	return float64(base.Lines)*w.ContextPerLine + base.Fixed
}

// startEstimate is the start context the weights estimate for a fork at the 1-based position: the start base, the orientation and the growth of the batches before it.
func startEstimate(w batcher.Weights, base batcher.StartBase, position int) float64 {
	return baseContext(w, base) + w.Orientation + float64(position-1)*w.BatchGrowth
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

// addStarts adds a row per card-naming fork of run, carrying the run's computed Merriam base or the reason it has none.
// A run whose base cannot be reconstructed is a row without a base, never an error;
// an error is a failed read of the history or the base tree.
func (c *Calibration) addStarts(run RunTally, history PlanHistory, base BaseTrees) error {
	runBase, err := readRunStartBase(run, history, base)
	if err != nil {
		return err
	}
	c.bases[run.Slug] = runBase
	for i, position := range forkPositions(run.Forks) {
		if position == 0 {
			continue
		}
		fork := run.Forks[i]
		row := StartRow{
			Run: run.Slug, Session: fork.Session, Position: position, Measured: float64(fork.StartContext),
			NoBase: runBase.reason, HasBase: runBase.found(), Estimate: startEstimate(c.Weights, runBase.base, position),
		}
		if row.HasBase {
			row.Base = baseContext(c.Weights, runBase.base)
		}
		c.Starts = append(c.Starts, row)
	}
	return nil
}

// fileAtBase reads name at run's base commit; found is false when the path is absent there.
func fileAtBase(trees BaseTrees, run RunTally, name string) (data []byte, found bool, err error) {
	data, err = trees.FileAtRevision(run.BaseSHA, name)
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s of %s at %s: %w", name, run.Slug, run.BaseSHA, err)
	}
	return data, true, nil
}

// merriamTexts reconstructs, at run's base commit, the texts websterengine.MerriamBase counts but the plan's 00-overview.md:
// CLAUDE.md, the Master template and the orchestrator PATTERN directive rendered from the directive stencil and PATTERN.md.
// A CLAUDE.md or PATTERN.md absent at the base commit adds nothing, as it does for a live run.
// CLAUDE.local.md, which MerriamBase also counts, is untracked and cannot be reconstructed; the derivation assumes the run's worktree had none, as a lyx-created worktree has none.
// A run without a base, whose base commit is not in the repository, or whose Master template or directive stencil is absent there has a reason instead of texts;
// a failed read or render is an error.
func merriamTexts(run RunTally, trees BaseTrees) ([]string, string, error) {
	switch {
	case run.BaseSHA == "":
		return nil, "no base: no begin-batch result in its webster sessions", nil
	case !trees.SHAExists(run.BaseSHA):
		return nil, fmt.Sprintf("base %s is not in the repository", run.BaseSHA), nil
	}

	var texts []string
	claude, found, err := fileAtBase(trees, run, "CLAUDE.md")
	if err != nil {
		return nil, "", err
	}
	if found {
		texts = append(texts, string(claude))
	}
	template, found, err := fileAtBase(trees, run, masterTemplatePath)
	if err != nil {
		return nil, "", err
	}
	if !found {
		return nil, fmt.Sprintf("no Master template at base %s", run.BaseSHA), nil
	}
	texts = append(texts, string(template))

	patternText, found, err := fileAtBase(trees, run, "PATTERN.md")
	if err != nil {
		return nil, "", err
	}
	if found {
		stencil, found, err := fileAtBase(trees, run, directiveStencilPath)
		if err != nil {
			return nil, "", err
		}
		if !found {
			return nil, fmt.Sprintf("no orchestrator directive stencil at base %s", run.BaseSHA), nil
		}
		directive, err := pattern.FillDirective(directiveStencilPath, string(stencil), string(patternText))
		if err != nil {
			return nil, "", fmt.Errorf("render the orchestrator directive of %s at %s: %w", run.Slug, run.BaseSHA, err)
		}
		texts = append(texts, directive)
	}
	return texts, "", nil
}

// readRunStartBase reconstructs the Merriam base of run: websterengine.MerriamBaseOf over merriamTexts and the plan's 00-overview.md.
// A run whose plan, base commit, Master template or directive stencil cannot be found has a reason instead of a base.
func readRunStartBase(run RunTally, history PlanHistory, trees BaseTrees) (runStartBase, error) {
	plan, reason, err := readRunPlan(run, history)
	if err != nil {
		return runStartBase{}, err
	}
	if plan == nil {
		return runStartBase{reason: reason}, nil
	}
	texts, reason, err := merriamTexts(run, trees)
	if err != nil {
		return runStartBase{}, err
	}
	if reason != "" {
		return runStartBase{reason: reason}, nil
	}
	return runStartBase{base: websterengine.MerriamBaseOf(append(texts, plan.OverviewText)...)}, nil
}

// addFixed adds the Merriam fixed-context row of run's first Merriam session, by start time, and a skip for each session it cannot measure.
// A session with no measured start, and any session of a run whose merriamTexts gives a reason, is a skip with that reason;
// every later session of the run starts with prior conversation or a compaction summary in context, and is a skip for that.
func (c *Calibration) addFixed(run RunTally, trees BaseTrees) error {
	texts, reason, err := merriamTexts(run, trees)
	if err != nil {
		return err
	}
	lines := websterengine.MerriamBaseOf(texts...).Lines
	for i, merriam := range run.Merriams {
		skip := func(reason string) {
			c.Fixed.Skips = append(c.Fixed.Skips, CalibrationSkip{Run: run.Slug, Card: merriam.Session, Reason: reason})
		}
		switch {
		case i > 0:
			skip("starts with prior conversation or a compaction summary in context")
		case merriam.NoStart != "":
			skip(merriam.NoStart)
		case reason != "":
			skip(reason)
		default:
			measured := float64(merriam.StartContext)
			c.Fixed.Rows = append(c.Fixed.Rows, FixedRow{
				Run: run.Slug, Session: merriam.Session, Measured: measured, Lines: lines,
				Fixed: measured - float64(lines)*c.Weights.ContextPerLine,
			})
		}
	}
	return nil
}

// summarize sets the median and spread of the rows' Fixed and the current constant.
func (f *FixedContext) summarize() {
	fixed := make([]float64, 0, len(f.Rows))
	for _, row := range f.Rows {
		fixed = append(fixed, row.Fixed)
	}
	f.Fit = fitOfRatios(fixed)
	f.Current = websterengine.MerriamBaseOf().Fixed
}

// fitStart fits Orientation and BatchGrowth by least squares of the measured start minus the run's base over position - 1, over the rows that carry a base.
func fitStart(rows []StartRow) StartFit {
	var fitted []StartRow
	for _, row := range rows {
		if row.HasBase {
			fitted = append(fitted, row)
		}
	}
	fit := StartFit{Forks: len(fitted)}
	positions := map[int]bool{}
	for _, row := range fitted {
		positions[row.Position] = true
	}
	if len(positions) < 2 {
		fit.NotFitted = "fewer than two distinct positions"
		return fit
	}
	var n, sumX, sumY, sumXX, sumXY float64
	for _, row := range fitted {
		x, y := float64(row.Position-1), row.Measured-row.Base
		n++
		sumX += x
		sumY += y
		sumXX += x * x
		sumXY += x * y
	}
	fit.BatchGrowth = (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)
	fit.Orientation = (sumY - fit.BatchGrowth*sumX) / n
	var ratios []float64
	for _, row := range fitted {
		if start := row.Base + fit.Orientation + fit.BatchGrowth*float64(row.Position-1); start > 0 {
			ratios = append(ratios, row.Measured/start)
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
	// The peak is priced from the run's computed Merriam base, so the peak table and the start table agree;
	// a run whose base could not be reconstructed is priced without one.
	var startBase batcher.StartBase
	if runBase := c.bases[run.Slug]; runBase.found() {
		startBase = runBase.base
	}

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

		estimate, err := batcher.PeakContext(plan, cards, sizes, c.Weights, startBase, position)
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
			EstimatedGrowth: estimate - startEstimate(c.Weights, startBase, position),
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
	fmt.Fprintln(w, "| run | session | position | measured start | base | estimated start |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	for _, row := range c.Starts {
		base := fmt.Sprintf("%.0f", row.Base)
		if !row.HasBase {
			base = "n/a (" + row.NoBase + ")"
		}
		fmt.Fprintf(w, "| %s | %s | %d | %.0f | %s | %.0f |\n", row.Run, row.Session, row.Position, row.Measured, base, row.Estimate)
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

	c.Fixed.write(w)
}

// write prints the Merriam fixed-context table, its median and spread beside the current constant, and the sessions left out with their reasons.
func (f FixedContext) write(w io.Writer) {
	fmt.Fprintln(w, "Merriam fixed context:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| run | session | measured start | lines | fixed |")
	fmt.Fprintln(w, "|---|---|---|---|---|")
	for _, row := range f.Rows {
		fmt.Fprintf(w, "| %s | %s | %.0f | %d | %.0f |\n", row.Run, row.Session, row.Measured, row.Lines, row.Fixed)
	}
	fmt.Fprintln(w)
	if f.Fit.Forks == 0 {
		fmt.Fprintf(w, "Fixed: no run measured; current constant %.0f.\n\n", f.Current)
	} else {
		fmt.Fprintf(w, "Fixed over %d runs: median %.0f, 25th percentile %.0f, 75th percentile %.0f; current constant %.0f.\n\n", f.Fit.Forks, f.Fit.Median, f.Fit.P25, f.Fit.P75, f.Current)
	}
	if len(f.Skips) > 0 {
		fmt.Fprintln(w, "Skipped:")
		fmt.Fprintln(w)
		for _, skip := range f.Skips {
			fmt.Fprintf(w, "- %s %s: %s\n", skip.Run, skip.Card, skip.Reason)
		}
		fmt.Fprintln(w)
	}
}

// write prints the fitted start coefficients beside the profile's, or why there is no fit.
func (f StartFit) write(w io.Writer, profile batcher.Weights) {
	if f.NotFitted != "" {
		fmt.Fprintf(w, "Start fit: not fitted, %s (%d forks).\n\n", f.NotFitted, f.Forks)
		return
	}
	fmt.Fprintf(w, "Start fit over %d forks: orientation %.0f (profile %.0f), batch_growth %.0f (profile %.0f), residual spread %s", f.Forks, f.Orientation, profile.Orientation, f.BatchGrowth, profile.BatchGrowth, spreadText(f.Residual))
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

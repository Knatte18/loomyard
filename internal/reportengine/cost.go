// cost.go declares the cost report's options and result types and ComputeCost, which prices the landed runs of one snapshot from the usage records each run keeps in its tree.

package reportengine

import (
	"errors"
	"fmt"
	"math"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// CostNoteTooFewRuns is the note of a cell holding fewer runs than CostOptions.MinRuns, which then carries no figure.
const CostNoteTooFewRuns = "too few runs"

// secondsPerHour converts a run's seconds into the hours the report states.
const secondsPerHour = 3600

// CostOptions shapes a cost report; everything in it is told, since the engine runs no process and reads no clock.
type CostOptions struct {
	// Since is the UTC date the window opens at: a run enters when its archive tag is committed on or after it.
	Since time.Time

	// MinRuns is the number of runs a cell needs before it shows its figures.
	MinRuns int
}

// CostSelection records what a cost report was computed over.
type CostSelection struct {
	RecordsBranch string    `json:"records_branch"`
	RecordsTip    string    `json:"records_tip"`
	Since         time.Time `json:"since"`
	MinRuns       int       `json:"min_runs"`
	SnapshotAt    time.Time `json:"snapshot_at"`
}

// CostFit is the least-squares line of a cost over the size of the diff a run landed: a fixed cost plus a cost per 100 changed lines.
// Runs is the number of sized runs the line was fitted over.
// Correlation is the Pearson correlation of the cost with the size, 0 when the cost does not vary.
type CostFit struct {
	Fixed       float64 `json:"fixed"`
	Per100Lines float64 `json:"per_100_lines"`
	Correlation float64 `json:"correlation"`
	Runs        int     `json:"runs"`
}

// CostCell is the cost of the landed runs of one recipe, profile and stored-record flag.
// Runs counts every run of the cell, whether or not it carries a dollar or an hour figure.
// A cell under CostOptions.MinRuns carries Note and no figure.
// MeanUSD and MeanHours are over the runs holding that figure and nil when none does, and a fit is nil unless two sized runs of distinct sizes hold the figure.
type CostCell struct {
	Recipe       string   `json:"recipe"`
	Profile      string   `json:"profile"`
	StoredRecord bool     `json:"stored_record"`
	Runs         int      `json:"runs"`
	Note         string   `json:"note,omitempty"`
	MeanUSD      *float64 `json:"mean_usd"`
	MeanHours    *float64 `json:"mean_hours"`
	USDFit       *CostFit `json:"usd_fit"`
	HoursFit     *CostFit `json:"hours_fit"`
}

// CostReport is the result of ComputeCost.
type CostReport struct {
	Selection CostSelection `json:"selection"`
	Cells     []CostCell    `json:"cells"`
	Findings  []Finding     `json:"findings"`
}

// costCellKey is what groups runs into a cell.
type costCellKey struct {
	recipe       string
	profile      string
	storedRecord bool
}

// costRun is one landed run's figures.
// usd and hours are nil when the run has no such figure, and sized says the run's landed diff size is known, size being its changed lines per 100.
type costRun struct {
	usd   *float64
	hours *float64
	sized bool
	size  float64
}

// ComputeCost prices the landed runs of src whose archive tag is committed on or after opts.Since, whatever their recipe.
// A run's figures come from the usage files at its tag's commit, parsed by shedrun.ParseRunUsage and never from a review round's usage record.
// A run with no run record, or with a usage file that does not decode, is skipped and listed as a finding.
// A run is landed when its usage holds a landed-size record, or, for a recipe with no Finalize row, when its status at the tag is done; any other run is left out.
// A run's dollars are the sum of its sessions' costs and are absent when it has no session or any session has an unknown cost.
// A run's hours are its active time from ComputeTiming for a run the timing report times, else the union of its sessions' spans.
// An error is a failed read of the repositories themselves.
func ComputeCost(src Sources, opts CostOptions) (CostReport, error) {
	since := opts.Since.UTC()
	report := CostReport{
		Selection: CostSelection{RecordsBranch: src.RecordsBranch, RecordsTip: src.RecordsTip, Since: since, MinRuns: opts.MinRuns, SnapshotAt: src.SnapshotAt},
		Cells:     []CostCell{},
		Findings:  []Finding{},
	}

	timing, err := ComputeTiming(src, TimingOptions{Since: since, By: BucketWeek})
	if err != nil {
		return CostReport{}, err
	}
	activeSeconds := map[string]int64{}
	for _, run := range timing.Runs {
		if run.State == RunStateLanded {
			activeSeconds[run.Tag] = run.ActiveSeconds
		}
	}

	groups := map[costCellKey][]costRun{}
	for _, tag := range src.Tags {
		if tag.Committed.UTC().Before(since) {
			continue
		}
		slug, named := fabricengine.ArchiveTagSlug(tag.Name)
		if !named {
			report.Findings = append(report.Findings, Finding{Tag: tag.Name, Error: "the tag names no run: expected a slug and a commit segment after the archive prefix"})
			continue
		}
		usage, finding, err := readRunUsage(src.Records, tag)
		if err != nil {
			return CostReport{}, err
		}
		if finding != nil {
			report.Findings = append(report.Findings, *finding)
			continue
		}
		landed := usage.LandedFound
		if !landed && !slices.Contains(runRecipes, usage.Run.Recipe) {
			var statusFinding *Finding
			if landed, statusFinding, err = terminalRowDone(src.Records, tag, slug); err != nil {
				return CostReport{}, err
			}
			if statusFinding != nil {
				report.Findings = append(report.Findings, *statusFinding)
			}
		}
		if !landed {
			continue
		}

		key := costCellKey{recipe: usage.Run.Recipe, profile: usage.Run.Profile, storedRecord: usage.Run.StoredRecord}
		groups[key] = append(groups[key], priceRun(usage, activeSeconds, tag.Name))
	}

	keys := make([]costCellKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.recipe != b.recipe {
			return a.recipe < b.recipe
		}
		if a.profile != b.profile {
			return a.profile < b.profile
		}
		return !a.storedRecord && b.storedRecord
	})
	for _, key := range keys {
		report.Cells = append(report.Cells, costCell(key, groups[key], opts.MinRuns))
	}
	return report, nil
}

// readRunUsage reads the usage directory at tag's commit and parses it.
// A finding, and no usage, comes back for a file that does not decode and for a directory holding no run record.
func readRunUsage(records Reader, tag gitrepo.TagRef) (shedrun.RunUsage, *Finding, error) {
	dir := filepath.ToSlash(shedrun.UsageDirRel())
	names, err := records.FilesInDirAtRevision(tag.Commit, dir)
	if err != nil {
		return shedrun.RunUsage{}, nil, fmt.Errorf("reportengine: list %s at %s: %w", dir, tag.Name, err)
	}
	files := map[string][]byte{}
	for _, name := range names {
		data, err := records.FileAtRevision(tag.Commit, path.Join(dir, name))
		if err != nil {
			return shedrun.RunUsage{}, nil, fmt.Errorf("reportengine: read %s at %s: %w", path.Join(dir, name), tag.Name, err)
		}
		files[name] = data
	}
	usage, err := shedrun.ParseRunUsage(files)
	if err != nil {
		return shedrun.RunUsage{}, &Finding{Tag: tag.Name, File: dir, Error: err.Error()}, nil
	}
	if !usage.RunFound {
		return shedrun.RunUsage{}, &Finding{Tag: tag.Name, File: dir, Error: "the usage directory holds no run record: the run is left out"}, nil
	}
	return usage, nil, nil
}

// terminalRowDone reports whether the run's status at tag's commit is done, the landed mark of a recipe with no Finalize row.
// A status that does not decode is a finding and not done, and an absent status is not done.
func terminalRowDone(records Reader, tag gitrepo.TagRef, slug string) (bool, *Finding, error) {
	rels, err := shedrun.TreeRunRels(slug)
	if err != nil {
		return false, &Finding{Tag: tag.Name, Error: err.Error()}, nil
	}
	data, err := records.FileAtRevision(tag.Commit, rels.Status)
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("reportengine: read %s at %s: %w", rels.Status, tag.Name, err)
	}
	status, err := shedengine.ParseStatusTolerant(data)
	if err != nil {
		return false, &Finding{Tag: tag.Name, File: rels.Status, Error: err.Error()}, nil
	}
	return status.State == shedengine.StateDone, nil, nil
}

// priceRun reduces a landed run's usage to its dollar and hour figures and its landed size.
// A run the timing report timed takes its active seconds from activeSeconds, keyed by tag.
func priceRun(usage shedrun.RunUsage, activeSeconds map[string]int64, tag string) costRun {
	var run costRun

	if len(usage.Sessions) > 0 {
		total, known := 0.0, true
		for _, session := range usage.Sessions {
			if session.CostUSD == nil {
				known = false
				break
			}
			total += *session.CostUSD
		}
		if known {
			run.usd = &total
		}
	}

	if seconds, timed := activeSeconds[tag]; timed {
		hours := float64(seconds) / secondsPerHour
		run.hours = &hours
	} else if spanned := sessionSpans(usage.Sessions); spanned > 0 {
		hours := spanned.Hours()
		run.hours = &hours
	}

	if usage.LandedFound {
		run.sized = true
		run.size = float64(usage.Landed.LinesAdded+usage.Landed.LinesDeleted) / 100
	}
	return run
}

// sessionSpans returns the time the sessions' started-to-ended spans cover, overlaps counted once.
// A session whose started or ended stamp is absent or not RFC 3339, or whose span is empty, covers nothing.
func sessionSpans(sessions []shedrun.SessionUsageRecord) time.Duration {
	var spans []Interval
	for _, session := range sessions {
		started, startErr := time.Parse(time.RFC3339, session.Started)
		ended, endErr := time.Parse(time.RFC3339, session.Ended)
		if startErr != nil || endErr != nil {
			continue
		}
		spans = append(spans, Interval{From: started.UTC(), To: ended.UTC()})
	}
	return lengthOf(unionIntervals(spans))
}

// costCell builds the cell of key over its runs: only the run count and the note when the cell is under minRuns, else the means and the fits.
func costCell(key costCellKey, runs []costRun, minRuns int) CostCell {
	cell := CostCell{Recipe: key.recipe, Profile: key.profile, StoredRecord: key.storedRecord, Runs: len(runs)}
	if len(runs) < minRuns {
		cell.Note = CostNoteTooFewRuns
		return cell
	}

	var usdPoints, hourPoints []costPoint
	var usdSum, hourSum float64
	for _, run := range runs {
		if run.usd != nil {
			usdSum += *run.usd
			if run.sized {
				usdPoints = append(usdPoints, costPoint{x: run.size, y: *run.usd})
			}
		}
		if run.hours != nil {
			hourSum += *run.hours
			if run.sized {
				hourPoints = append(hourPoints, costPoint{x: run.size, y: *run.hours})
			}
		}
	}
	cell.MeanUSD = meanOf(usdSum, countHolding(runs, func(run costRun) bool { return run.usd != nil }))
	cell.MeanHours = meanOf(hourSum, countHolding(runs, func(run costRun) bool { return run.hours != nil }))
	cell.USDFit = fitCost(usdPoints)
	cell.HoursFit = fitCost(hourPoints)
	return cell
}

// countHolding returns how many of runs satisfy holds.
func countHolding(runs []costRun, holds func(costRun) bool) int {
	count := 0
	for _, run := range runs {
		if holds(run) {
			count++
		}
	}
	return count
}

// meanOf returns sum over count, nil for no count.
func meanOf(sum float64, count int) *float64 {
	if count == 0 {
		return nil
	}
	mean := sum / float64(count)
	return &mean
}

// costPoint is one sized run: its landed lines per 100 and its cost.
type costPoint struct {
	x, y float64
}

// fitCost fits y = fixed + per100 * x by least squares over points.
// It returns nil unless the points hold two distinct x values.
func fitCost(points []costPoint) *CostFit {
	if len(points) < 2 || !slices.ContainsFunc(points, func(point costPoint) bool { return point.x != points[0].x }) {
		return nil
	}
	var sumX, sumY, sumXX, sumXY, sumYY float64
	for _, point := range points {
		sumX += point.x
		sumY += point.y
		sumXX += point.x * point.x
		sumXY += point.x * point.y
		sumYY += point.y * point.y
	}
	n := float64(len(points))
	spreadX := n*sumXX - sumX*sumX
	spreadY := n*sumYY - sumY*sumY
	covariance := n*sumXY - sumX*sumY

	fit := CostFit{Per100Lines: covariance / spreadX, Runs: len(points)}
	fit.Fixed = (sumY - fit.Per100Lines*sumX) / n
	if spreadY > 0 {
		fit.Correlation = covariance / math.Sqrt(spreadX*spreadY)
	}
	return &fit
}

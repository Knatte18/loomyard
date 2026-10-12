// runs.go selects the finished runs of a report: one run per archive tag, full when its loom records show a finished loom run and partial otherwise.

package reportengine

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// SourceBatten is the name of the source holding a run's batten status snapshots.
const SourceBatten = "batten"

// RunRow is one full run.
// Sources maps each source name to whether the run holds it; a source a run lacks is absent, never counted as zero.
type RunRow struct {
	Tag       string          `json:"tag"`
	Slug      string          `json:"slug"`
	Commit    string          `json:"commit"`
	Committed time.Time       `json:"committed"`
	Week      string          `json:"week"`
	Sources   map[string]bool `json:"sources"`

	// Wakes maps each wake source to its wakes, nil for a source the run lacks.
	Wakes map[string]*WakeSource `json:"wakes"`

	// Automatic counts batten's automatic steps by kind, and Paused its paused changes; both are nil without the batten source.
	Automatic map[string]int `json:"automatic"`
	Paused    *int           `json:"paused"`

	// ParentReviewPings sums the keep-alives of every parent-review round, beside the wakes and never as one; nil without the parent-review source.
	ParentReviewPings *int `json:"parent_review_pings"`

	// Alignment counts the alignment attempts per point and verdict word; nil without the alignment source.
	Alignment []AlignmentOutcome `json:"alignment"`

	// Stops lists the run's halts and Failures counts their failure classes; both are nil without the halt-note source.
	Stops    []Stop         `json:"stops"`
	Failures map[string]int `json:"failures"`

	// Arbiter counts the arbiter attempts per verdict word; nil without the arbiter directory.
	Arbiter []ArbiterOutcome `json:"arbiter"`

	// LLMSteps counts the steps an LLM took by hand per source; a source the run lacks has no key.
	LLMSteps map[string]int `json:"llm_steps"`

	// DriveReports is the number of driver attempts reported; nil without the drive-reports directory.
	DriveReports *int `json:"drive_reports"`

	// Segments maps each review segment, and the parent review, to its rounds; a segment the run lacks but another selected run holds is nil.
	Segments map[string]*SegmentRounds `json:"segments"`

	// Decisions counts the entries added after the discussion per author; nil without the decision record.
	Decisions map[string]int `json:"decisions"`

	// battenSnapshots are the batten status files in the run's span, oldest first, the last being the run's batten record.
	battenSnapshots []shedengine.Status

	// childHistory is the loom history at the run's archive tag.
	childHistory []shedengine.HistoryEntry

	// changes are the child state changes batten recorded, and accountedChild the indexes of childHistory entries an automatic step accounts for.
	changes        []battenshed.StateChange
	automaticSteps []battenshed.AutomaticStep
	accountedChild []int

	// Landing is the parent-branch commit that landed the run, nil when no join maps one.
	Landing *Landing `json:"landing"`

	// parentRounds are the parent-review rounds that decoded, and arbiterRecords the arbiter attempts that did.
	parentRounds   []parentreview.Round
	arbiterRecords []loomengine.ArbiterRecord
}

// PartialRun is a tag that names a run which did not finish as a full loom run, with the reason.
// It enters no per-run figure.
type PartialRun struct {
	Tag    string `json:"tag"`
	Slug   string `json:"slug"`
	Week   string `json:"week"`
	Reason string `json:"reason"`
}

// isoWeekLabel spells the ISO week of at, with Monday as its first day, as 2026-W41.
func isoWeekLabel(at time.Time) string {
	year, week := at.UTC().ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

// selectRuns adds a run, a partial run or a finding to report for every archive tag of src committed on or after opts.Since.
func selectRuns(src Sources, opts Options, report *Report) error {
	batten, err := newBattenIndex(src)
	if err != nil {
		return err
	}

	previousTag := map[string]gitrepo.TagRef{}
	for _, tag := range src.Tags {
		slug, named := fabricengine.ArchiveTagSlug(tag.Name)
		var previous *gitrepo.TagRef
		if named {
			if earlier, ok := previousTag[slug]; ok {
				previous = &earlier
			}
			previousTag[slug] = tag
		}
		committed := tag.Committed.UTC()
		if committed.Before(opts.Since) {
			continue
		}
		if !named {
			report.Findings = append(report.Findings, Finding{Tag: tag.Name, Error: "the tag names no run: expected a slug and a commit segment after the archive prefix"})
			continue
		}

		week := isoWeekLabel(committed)
		status, reason, findings, err := readLoomRecords(src.Records, tag, slug)
		if err != nil {
			return err
		}
		report.Findings = append(report.Findings, findings...)
		var earlierCommitted *time.Time
		if previous != nil {
			at := previous.Committed
			earlierCommitted = &at
		}
		if reason != "" {
			report.Partial = append(report.Partial, PartialRun{Tag: tag.Name, Slug: slug, Week: week, Reason: reason})
			report.inputs.selected = append(report.inputs.selected, selectedTag{tag: tag, slug: slug, previous: earlierCommitted, run: -1, noFinalize: reason == noFinalizeReason()})
			continue
		}

		row := RunRow{
			Tag:          tag.Name,
			Slug:         slug,
			Commit:       tag.Commit,
			Committed:    committed,
			Week:         week,
			Sources:      map[string]bool{},
			childHistory: status.History,
		}
		snapshots, battenFindings, err := batten.snapshotsOfRun(slug, tag.Name, earlierCommitted, tag.Committed)
		if err != nil {
			return err
		}
		report.Findings = append(report.Findings, battenFindings...)
		row.battenSnapshots = snapshots
		row.Sources[SourceBatten] = len(snapshots) > 0
		wakeFindings, err := row.readWakes(src.Records, tag)
		if err != nil {
			return err
		}
		stopFindings, err := row.readStops(src.Records, tag)
		if err != nil {
			return err
		}
		roundFindings, err := row.readRounds(src.Records, tag)
		if err != nil {
			return err
		}
		report.Findings = append(report.Findings, wakeFindings...)
		report.Findings = append(report.Findings, stopFindings...)
		report.Findings = append(report.Findings, roundFindings...)
		finalizeDone := slices.ContainsFunc(status.History, func(entry shedengine.HistoryEntry) bool {
			return entry.Producer == loomshed.NameFinalize && entry.Outcome == shedengine.Done
		})
		report.inputs.selected = append(report.inputs.selected, selectedTag{tag: tag, slug: slug, previous: earlierCommitted, run: len(report.Runs), finalizeDone: finalizeDone})
		report.Runs = append(report.Runs, row)
	}

	report.inputs.checkpoints = batten.checkpoints
	report.inputs.newestTag = map[string]time.Time{}
	for slug, tag := range previousTag {
		report.inputs.newestTag[slug] = tag.Committed
	}
	return nil
}

// noFinalizeReason is the reason of a partial run whose loom history holds no Finalize entry.
func noFinalizeReason() string {
	return fmt.Sprintf("the loom history holds no %s entry", loomshed.NameFinalize)
}

// selectedTag is an archive tag the selection read, kept for the landing join.
// run indexes Report.Runs and is -1 for a partial tag; finalizeDone is meaningful for a full run, noFinalize for a partial one.
type selectedTag struct {
	tag          gitrepo.TagRef
	slug         string
	previous     *time.Time
	run          int
	finalizeDone bool
	noFinalize   bool
}

// landingInputs are what run selection hands the landing join: the tags it read, each slug's landing checkpoints oldest first, and each slug's newest archive tag time.
type landingInputs struct {
	selected    []selectedTag
	checkpoints map[string][]time.Time
	newestTag   map[string]time.Time
}

// readLoomRecords reads the loom seed and status at tag's commit through the shared record parsers.
// A non-empty reason means the tag is not a full run: its seed names another recipe, a record is absent or does not decode, or the history holds no Finalize entry.
// A record that exists but does not decode is also a finding.
func readLoomRecords(records Reader, tag gitrepo.TagRef, slug string) (status shedengine.Status, reason string, findings []Finding, err error) {
	rels, relErr := shedrun.TreeRunRels(slug)
	if relErr != nil {
		return shedengine.Status{}, "the slug is no legal run id", []Finding{{Tag: tag.Name, Error: relErr.Error()}}, nil
	}

	seedBytes, err := records.FileAtRevision(tag.Commit, rels.Seed)
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		return shedengine.Status{}, "the tag holds no seed", nil, nil
	}
	if err != nil {
		return shedengine.Status{}, "", nil, fmt.Errorf("reportengine: read %s at %s: %w", rels.Seed, tag.Name, err)
	}
	seed, err := shedrun.ParseSeed(seedBytes)
	if err != nil {
		return shedengine.Status{}, "the seed does not decode", []Finding{{Tag: tag.Name, File: rels.Seed, Error: err.Error()}}, nil
	}
	if seed.Recipe != shedrun.RecipeLoom {
		return shedengine.Status{}, fmt.Sprintf("the seed names the %q recipe, not loom", seed.Recipe), nil, nil
	}

	statusBytes, err := records.FileAtRevision(tag.Commit, rels.Status)
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		return shedengine.Status{}, "the tag holds no status", nil, nil
	}
	if err != nil {
		return shedengine.Status{}, "", nil, fmt.Errorf("reportengine: read %s at %s: %w", rels.Status, tag.Name, err)
	}
	status, err = shedengine.ParseStatusTolerant(statusBytes)
	if err != nil {
		return shedengine.Status{}, "the status does not decode", []Finding{{Tag: tag.Name, File: rels.Status, Error: err.Error()}}, nil
	}
	finalized := slices.ContainsFunc(status.History, func(entry shedengine.HistoryEntry) bool {
		return entry.Producer == loomshed.NameFinalize
	})
	if !finalized {
		return shedengine.Status{}, noFinalizeReason(), nil, nil
	}
	return status, "", nil, nil
}

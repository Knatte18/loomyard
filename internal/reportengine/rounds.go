// rounds.go reads who repaired a run and how its reviews went:
// arbiter attempts, LLM steps, drive reports, review rounds per segment, and the decisions added after the discussion.

package reportengine

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// The sources of a run besides the wake and stop sources.
const (
	SourceArbiter      = "arbiter"
	SourceDriveReports = "drive-reports"
	SourceDecisions    = "decisions"
)

// The keys of a run's LLM-step counts: the loom history and the batten record.
const (
	LLMStepsLoom   = "loom"
	LLMStepsBatten = "batten"
)

// outcomeNone is the outcome of a parent-review round that recorded no verdict.
const outcomeNone = "none"

// ArbiterOutcome counts the arbiter attempts that ended in one verdict word, and how many carry the applied mark.
type ArbiterOutcome struct {
	Verdict string `json:"verdict"`
	Count   int    `json:"count"`
	Applied int    `json:"applied"`
}

// SegmentRounds is the review rounds of one segment: how many, and how the last one ended.
type SegmentRounds struct {
	Rounds  int    `json:"rounds"`
	Outcome string `json:"outcome"`
}

// entryTime reads a history entry's RFC 3339 time, and returns the zero time for text that is not one.
func entryTime(text string) time.Time {
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}
	}
	return at
}

// readRounds fills the run's rows from the records at tag:
// arbiter outcomes, LLM steps, drive reports, review segments and decisions.
// A source whose directory or file the tree lacks stays nil, and a record that does not decode is a finding.
func (row *RunRow) readRounds(records Reader, tag gitrepo.TagRef) ([]Finding, error) {
	row.Segments = map[string]*SegmentRounds{}

	arbiterFindings, err := row.readArbiter(records, tag)
	if err != nil {
		return nil, err
	}
	if err := row.readDriveReports(records, tag); err != nil {
		return nil, err
	}
	if err := row.readSegments(records, tag); err != nil {
		return nil, err
	}
	decisionFindings, err := row.readDecisions(records, tag)
	if err != nil {
		return nil, err
	}
	row.countLLMSteps()
	return append(arbiterFindings, decisionFindings...), nil
}

// readArbiter reads each arbiter attempt directory and counts the attempts per verdict word.
func (row *RunRow) readArbiter(records Reader, tag gitrepo.TagRef) ([]Finding, error) {
	rels, err := shedrun.TreeRunRels(row.Slug)
	if err != nil {
		return nil, err
	}
	attempts, err := records.SubdirsAtRevision(tag.Commit, rels.Arbiter)
	if err != nil {
		return nil, fmt.Errorf("reportengine: list %s at %s: %w", rels.Arbiter, tag.Name, err)
	}
	row.Sources[SourceArbiter] = len(attempts) > 0
	if len(attempts) == 0 {
		return nil, nil
	}

	var findings []Finding
	outcomes := map[string]*ArbiterOutcome{}
	for _, attempt := range attempts {
		dir := path.Join(rels.Arbiter, attempt)
		files, err := filesOfDir(records, tag.Commit, dir)
		if err != nil {
			return nil, err
		}
		record, err := loomengine.ParseArbiterAttemptDir(files)
		if err != nil {
			findings = append(findings, Finding{Tag: tag.Name, File: dir, Error: err.Error()})
			continue
		}
		row.arbiterRecords = append(row.arbiterRecords, record)

		word := verdictNone
		if record.Verdict != nil {
			word = record.Verdict.Verdict
		}
		if outcomes[word] == nil {
			outcomes[word] = &ArbiterOutcome{Verdict: word}
		}
		outcomes[word].Count++
		if record.Applied {
			outcomes[word].Applied++
		}
	}

	row.Arbiter = []ArbiterOutcome{}
	for _, outcome := range outcomes {
		row.Arbiter = append(row.Arbiter, *outcome)
	}
	sort.Slice(row.Arbiter, func(i, j int) bool { return row.Arbiter[i].Verdict < row.Arbiter[j].Verdict })
	return findings, nil
}

// readDriveReports counts the drive reports, one file per driver attempt; their bodies stay unread.
func (row *RunRow) readDriveReports(records Reader, tag gitrepo.TagRef) error {
	rels, err := shedrun.TreeRunRels(row.Slug)
	if err != nil {
		return err
	}
	names, err := records.FilesInDirAtRevision(tag.Commit, rels.DriveReports)
	if err != nil {
		return fmt.Errorf("reportengine: list %s at %s: %w", rels.DriveReports, tag.Name, err)
	}
	row.Sources[SourceDriveReports] = len(names) > 0
	if len(names) > 0 {
		count := len(names)
		row.DriveReports = &count
	}
	return nil
}

// retiredReviewSubdirs maps the review directories older runs wrote to the segment their rounds count under.
// An empty segment drops the rounds.
var retiredReviewSubdirs = map[string]string{
	"webster": "code",
	"merge":   "",
}

// currentReviewSegment returns the segment a review directory counts under.
// It returns false when the directory counts under none.
func currentReviewSegment(subdir string) (string, bool) {
	if segment, retired := retiredReviewSubdirs[subdir]; retired {
		return segment, segment != ""
	}
	return subdir, true
}

// readSegments counts the review rounds of every review segment, and of the parent review, into Segments.
// A Bouncer re-seed's archived generation adds its rounds to its segment's and is no segment of its own.
// A directory retiredReviewSubdirs maps counts under the segment it maps to,
// and a directory that maps to an empty segment is not counted.
func (row *RunRow) readSegments(records Reader, tag gitrepo.TagRef) error {
	reviewsDir := filepath.ToSlash(loomengine.LoomReviewsDirRel())
	subdirs, err := records.SubdirsAtRevision(tag.Commit, reviewsDir)
	if err != nil {
		return fmt.Errorf("reportengine: list %s at %s: %w", reviewsDir, tag.Name, err)
	}

	parentReviewDir := filepath.Base(parentReviewDirRel())
	archivedRounds := map[string]int{}
	for _, subdir := range subdirs {
		if subdir == parentReviewDir {
			continue
		}
		names, err := records.FilesInDirAtRevision(tag.Commit, path.Join(reviewsDir, subdir))
		if err != nil {
			return fmt.Errorf("reportengine: list %s at %s: %w", path.Join(reviewsDir, subdir), tag.Name, err)
		}
		summary := shedadapters.ReviewRounds(names)
		if archivedFrom, archived := shedadapters.ArchivedRunDirSegment(subdir); archived {
			if segment, counted := currentReviewSegment(archivedFrom); counted {
				archivedRounds[segment] += summary.Rounds
			}
			continue
		}
		segment, counted := currentReviewSegment(subdir)
		if !counted {
			continue
		}
		if held := row.Segments[segment]; held != nil {
			held.Rounds += summary.Rounds
			continue
		}
		row.Segments[segment] = &SegmentRounds{Rounds: summary.Rounds, Outcome: summary.Outcome}
	}
	for segment, rounds := range archivedRounds {
		if row.Segments[segment] == nil {
			row.Segments[segment] = &SegmentRounds{}
		}
		row.Segments[segment].Rounds += rounds
	}

	if row.Sources[SourceParentReview] {
		parent := &SegmentRounds{}
		last := 0
		for _, round := range row.parentRounds {
			if round.Request == nil {
				continue
			}
			parent.Rounds++
			if round.Number > last {
				last = round.Number
				parent.Outcome = outcomeNone
				if round.Verdict != nil {
					parent.Outcome = round.Verdict.Kind
				}
			}
		}
		row.Segments[SourceParentReview] = parent
	}
	return nil
}

// readDecisions counts the entries added after the discussion per author.
// A decision record that does not parse is a finding and the run's other counts stand.
func (row *RunRow) readDecisions(records Reader, tag gitrepo.TagRef) ([]Finding, error) {
	file := filepath.ToSlash(loomengine.DiscussionDecisionRecordRel())
	data, err := records.FileAtRevision(tag.Commit, file)
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		row.Sources[SourceDecisions] = false
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reportengine: read %s at %s: %w", file, tag.Name, err)
	}
	row.Sources[SourceDecisions] = true

	decisions, err := discussionparser.ParseAddedDecisions(data)
	if err != nil {
		return []Finding{{Tag: tag.Name, File: file, Error: err.Error()}}, nil
	}
	row.Decisions = map[string]int{}
	for _, decision := range decisions {
		row.Decisions[decision.By]++
	}
	return nil, nil
}

// countLLMSteps counts the steps an LLM took by hand: the goto and reroute entries of the loom history,
// and the goto and reroute entries and unexplained resumes of the batten record.
// Entries an automatic step accounts for are not counted;
// neither are a goto an applied arbiter verdict accounts for or a resume an applied arbiter attempt accounts for.
func (row *RunRow) countLLMSteps() {
	attempts := append([]loomengine.ArbiterRecord(nil), row.arbiterRecords...)
	sort.Slice(attempts, func(i, j int) bool { return attempts[i].Attempt.Number < attempts[j].Attempt.Number })

	accounted := map[int]bool{}
	for _, index := range row.accountedChild {
		accounted[index] = true
	}
	for _, record := range attempts {
		if !record.Applied || record.Verdict == nil || record.Verdict.Goto == "" {
			continue
		}
		for index, entry := range row.childHistory {
			if !accounted[index] && entry.Outcome == shedengine.OutcomeGoto && entry.Producer == record.Verdict.Goto && entryTime(entry.At).After(record.Attempt.SpawnedAt) {
				accounted[index] = true
				break
			}
		}
	}

	row.LLMSteps = map[string]int{LLMStepsLoom: countHandSteps(row.childHistory, accounted)}
	if len(row.battenSnapshots) == 0 {
		return
	}
	batten := countHandSteps(row.battenSnapshots[len(row.battenSnapshots)-1].History, nil)

	// A verify retry resumes the child it halted, so the first resume after the halt is batten's own and no LLM step.
	resumeAccounted := map[int]bool{}
	for _, step := range row.automaticSteps {
		if step.Kind != battenshed.AutomaticKindVerifyRetry {
			continue
		}
		halted := slices.IndexFunc(row.changes, func(change battenshed.StateChange) bool {
			return change.To == shedengine.StateBlocked && change.Producer == step.Producer && change.At.Equal(step.At)
		})
		if halted < 0 {
			continue
		}
		for index := halted + 1; index < len(row.changes); index++ {
			if isResume(row.changes[index].From, row.changes[index].To) {
				resumeAccounted[index] = true
				break
			}
		}
	}
	for _, record := range attempts {
		if !record.Applied {
			continue
		}
		for index, change := range row.changes {
			if resumeAccounted[index] || !isResume(change.From, change.To) {
				continue
			}
			key := record.Attempt.StopKey
			if change.Producer == key.CurrentProducer && string(change.From) == key.State && change.At.After(record.Attempt.SpawnedAt) {
				resumeAccounted[index] = true
				break
			}
		}
	}
	for index, change := range row.changes {
		if isResume(change.From, change.To) && !resumeAccounted[index] {
			batten++
		}
	}
	row.LLMSteps[LLMStepsBatten] = batten
}

// isResume reports whether a change from one state to another resumes a halted child.
func isResume(from, to shedengine.State) bool {
	return (from == shedengine.StateBlocked || from == shedengine.StateFailed) && to == shedengine.StateRunning
}

// countHandSteps counts the goto and reroute entries of history whose index is not accounted for.
func countHandSteps(history []shedengine.HistoryEntry, accounted map[int]bool) int {
	count := 0
	for index, entry := range history {
		if (entry.Outcome == shedengine.OutcomeGoto || entry.Outcome == shedengine.OutcomeReroute) && !accounted[index] {
			count++
		}
	}
	return count
}

// markAbsentSegments sets each segment that some run holds and another lacks to nil in the run lacking it.
// A nil entry tells a reader that the run never had the segment, not that it had zero rounds of it.
func markAbsentSegments(runs []RunRow) {
	union := map[string]bool{}
	for _, run := range runs {
		for segment := range run.Segments {
			union[segment] = true
		}
	}
	for _, run := range runs {
		for segment := range union {
			if _, held := run.Segments[segment]; !held {
				run.Segments[segment] = nil
			}
		}
	}
}

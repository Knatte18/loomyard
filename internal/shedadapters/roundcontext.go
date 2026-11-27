// roundcontext.go builds the round context a review after the first round is judged against.
// It also holds the round changes file the review producer writes at an incremental round's start,
// and the lookup of the report of a round's parent-review ask.

package shedadapters

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/state"
)

// roundChangesPath returns the path of round n's changes file inside runDir.
// The file lists the worktree-relative target paths changed since the round before's snapshot, one per line.
func roundChangesPath(runDir string, n int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-changes.md", n))
}

// writeRoundChanges writes round n's changes file, one path per line.
func writeRoundChanges(runDir string, n int, paths []string) error {
	path := roundChangesPath(runDir, n)
	content := ""
	if len(paths) > 0 {
		content = strings.Join(paths, "\n") + "\n"
	}
	if err := state.WriteFile(path, lock.TrackedFileLockPath(path, recordLockSuffix), []byte(content)); err != nil {
		return fmt.Errorf("shedadapters: write round %d's changes file %s: %w", n, path, err)
	}
	return nil
}

// readRoundChanges returns the paths in round n's changes file, and false when the round has no such file.
func readRoundChanges(runDir string, n int) ([]string, bool, error) {
	content, present, err := readFileIfPresent(roundChangesPath(runDir, n))
	if err != nil || !present {
		return nil, false, err
	}
	paths := []string{}
	for _, line := range strings.Split(string(content), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, line)
		}
	}
	return paths, true, nil
}

// readFileIfPresent returns the bytes of the file at path, and false when the file is absent.
func readFileIfPresent(path string) ([]byte, bool, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("shedadapters: read %s: %w", path, err)
	}
	return content, true, nil
}

// roundParentReportPath returns the report path of round's parent-review ask in the generation runDir holds.
// It is empty when the round holds no ask, or when the generation's seed or the ask record cannot be read.
func roundParentReportPath(runDir string, round int, files burlerengine.ParentReviewFiles) string {
	seed, err := roundSeed(runDir)
	if err != nil {
		return ""
	}
	record, _, err := burlerengine.ReadParentAsks(files)
	if err != nil {
		return ""
	}
	ask, held := record.RoundAsk(seed, round)
	if !held {
		return ""
	}
	return files.ReportPath(ask.Number)
}

// RoundContextSources names what BuildRoundContext reads beyond the run directory.
// An empty path or a nil reader reads nothing.
type RoundContextSources struct {
	// DecisionRecordPath is the decision record whose added entries the context lists.
	DecisionRecordPath string
	// AnchorPath locates the run's parent-review files.
	AnchorPath string
	// AmendmentTitles reads the titles of the pending amending decision entries from the plan amendment record.
	AmendmentTitles func() ([]string, error)
	// ConflictDir is the directory holding the plan executor's halt-reason file.
	ConflictDir string
	// HardenerRound marks a hardener segment's round, whose findings the BLOCKING and changed-file rules do not bind.
	HardenerRound bool
}

// amendingEntryTexts returns the full text of every added entry of the decision record whose title amendmentTitles lists.
// It covers all added entries, in record order, not only those added since an earlier round.
func amendingEntryTexts(recordPath string, amendmentTitles func() ([]string, error)) ([]string, error) {
	if amendmentTitles == nil {
		return nil, nil
	}
	pending, err := amendmentTitles()
	if err != nil {
		return nil, fmt.Errorf("shedadapters: read the plan amendment record: %w", err)
	}
	if len(pending) == 0 {
		return nil, err
	}
	digests, err := addedDecisionDigests(recordPath)
	if err != nil || len(digests) == 0 {
		return nil, err
	}
	entryTexts, err := discussionparser.AddedDecisionTexts(recordPath)
	if err != nil {
		return nil, fmt.Errorf("shedadapters: read the added entries of %q: %w", recordPath, err)
	}
	if len(entryTexts) != len(digests) {
		return nil, fmt.Errorf("shedadapters: decision record %q changed while its added entries were read", recordPath)
	}
	var texts []string
	for i, digest := range digests {
		if slices.Contains(pending, digest.Title) {
			texts = append(texts, entryTexts[i])
		}
	}
	return texts, nil
}

// BuildRoundContext returns what round's review is judged against.
// It holds the findings of round-1 and the fixer's answer to each.
// It holds the decision record entries added since, and the texts of the entries pending as plan amendments.
// It holds the plan executor's halt reason and the files the fixes changed.
// It returns nil, with a nil error, when round-1 is not complete:
// round is 1; round-1's review is absent or does not parse; its snapshot record is absent or holds no `decisions_routed`;
// or its fixer-report is neither one the engine wrote itself nor a fixer-written one that passes the fix check.
// A read error other than an absence is returned.
// The round is incremental unless the run directory's generation is an amendment.
// An empty paths.AnchorPath leaves ParentReportPath empty.
func BuildRoundContext(runDir string, round int, paths RoundContextSources) (*burlerengine.RoundContext, error) {
	if round < 2 {
		return nil, nil
	}
	previous := round - 1
	reviewPath := roundReviewPath(runDir, previous)
	review, present, err := readFileIfPresent(reviewPath)
	if err != nil || !present {
		return nil, err
	}
	_, findings, err := burlerengine.ParseReview(review)
	if err != nil {
		return nil, nil
	}
	snapshot, found, err := readRoundSnapshot(runDir, previous)
	if err != nil {
		return nil, err
	}
	if !found || snapshot.DecisionsRouted == nil {
		return nil, nil
	}
	routed := *snapshot.DecisionsRouted

	fixerReportPath := roundFixerReportPath(runDir, previous)
	reportContent, present, err := readFileIfPresent(fixerReportPath)
	if err != nil || !present {
		return nil, err
	}
	report, err := burlerengine.ParseFixerReport(reportContent)
	if err != nil {
		return nil, nil
	}
	engineWritten := report.Spawned != nil && !*report.Spawned
	if !engineWritten && burlerengine.CheckFixerReport(report, findings, routed) != nil {
		return nil, nil
	}

	titles, texts, err := addedSincePreviousSnapshot(paths.DecisionRecordPath, snapshot.AddedDecisions)
	if err != nil {
		return nil, err
	}
	amendingTexts, err := amendingEntryTexts(paths.DecisionRecordPath, paths.AmendmentTitles)
	if err != nil {
		return nil, err
	}
	haltReason := ""
	if paths.ConflictDir != "" {
		if haltReason, err = runplan.ReadHaltReason(paths.ConflictDir); err != nil {
			return nil, err
		}
	}
	generation, _, err := readGenerationRecord(runDir)
	if err != nil {
		return nil, err
	}
	changed, hasChanges, err := readRoundChanges(runDir, round)
	if err != nil {
		return nil, err
	}

	dispositions := make(map[string]burlerengine.FindingDisposition, len(report.Dispositions))
	for _, disposition := range report.Dispositions {
		dispositions[disposition.ID] = disposition
	}
	inScope, err := InScopeFindings(runDir, previous)
	if err != nil {
		return nil, err
	}
	prior := make([]burlerengine.PriorFinding, len(findings))
	for i, finding := range findings {
		held := routed && finding.Class == burlerengine.ClassDecision
		prior[i] = burlerengine.PriorFinding{Finding: finding, Held: held, InScope: held && slices.Contains(inScope, finding.ID)}
		if disposition, answered := dispositions[finding.ID]; answered {
			prior[i].Disposition = &disposition
		}
	}

	context := &burlerengine.RoundContext{
		Incremental:          !generation.Amendment,
		Prior:                prior,
		AddedTitles:          titles,
		AddedTexts:           texts,
		AmendingTexts:        amendingTexts,
		HaltReason:           haltReason,
		HardenerRound:        paths.HardenerRound,
		HasChanges:           hasChanges,
		Changed:              changed,
		PriorReviewPath:      reviewPath,
		PriorFixerReportPath: fixerReportPath,
	}
	if hasChanges {
		context.ChangesPath = roundChangesPath(runDir, round)
	}
	if paths.AnchorPath != "" {
		context.ParentReportPath = roundParentReportPath(runDir, round, burlerengine.ParentReviewFilesOf(paths.AnchorPath))
	}
	return context, nil
}

// addedSincePreviousSnapshot returns the titles and texts of the decision record's added entries, in record order.
// It lists only entries whose digest the round before's snapshot did not hold.
// An entry under a title a fold kept, with a body of its own, has a new digest and is listed.
func addedSincePreviousSnapshot(recordPath string, recorded []addedDecisionDigest) (titles, texts []string, err error) {
	digests, err := addedDecisionDigests(recordPath)
	if err != nil || len(digests) == 0 {
		return nil, nil, err
	}
	entryTexts, err := discussionparser.AddedDecisionTexts(recordPath)
	if err != nil {
		return nil, nil, fmt.Errorf("shedadapters: read the added entries of %q: %w", recordPath, err)
	}
	if len(entryTexts) != len(digests) {
		return nil, nil, fmt.Errorf("shedadapters: decision record %q changed while its added entries were read", recordPath)
	}
	held := make(map[string]bool, len(recorded))
	for _, digest := range recorded {
		held[digest.Digest] = true
	}
	for i, digest := range digests {
		if !held[digest.Digest] {
			titles, texts = append(titles, digest.Title), append(texts, entryTexts[i])
		}
	}
	return titles, texts, nil
}

// withParentReportPath sets the report path of the round's parent-review ask on profile's round context, when it has one.
// The context is copied first, since the profile may share it with another.
func withParentReportPath(profile *burlerengine.Profile, reportPath string) {
	if profile.Round == nil {
		return
	}
	context := *profile.Round
	context.ParentReportPath = reportPath
	profile.Round = &context
}

// recordRoundChanges writes round's changes file when none exists and the round is incremental.
// It needs a target snapshot, a target id recorded for the round before, and a generation that is no amendment.
// The file lists the target paths changed since that id.
// An answer the snapshot cannot give writes no file, and the round's new findings are held to BLOCKING alone.
// A failed read or write is logged and never fails the round.
func (p *BurlerReviewProducer) recordRoundChanges(round int) {
	if p.snapshot == nil || round < 2 {
		return
	}
	if _, present, err := readRoundChanges(p.runDir, round); err != nil || present {
		if err != nil {
			logger.Warn("shedadapters: burler review could not read the round's changes file; none written", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "error", err)
		}
		return
	}
	previous, found, err := readRoundSnapshot(p.runDir, round-1)
	if err != nil || !found || previous.TargetID == "" {
		if err != nil {
			logger.Warn("shedadapters: burler review could not read the previous round's snapshot record; no changes file written", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "error", err)
		}
		return
	}
	generation, _, err := readGenerationRecord(p.runDir)
	if err != nil {
		logger.Warn("shedadapters: burler review could not read the generation record; no changes file written", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "error", err)
		return
	}
	if generation.Amendment {
		return
	}
	changed, known := p.snapshot.ChangedSince(previous.TargetID)
	if !known {
		return
	}
	if err := writeRoundChanges(p.runDir, round, changed); err != nil {
		logger.Warn("shedadapters: burler review could not write the round's changes file", "producer", p.name, "engine", burlerReviewEngineLabel, "round", round, "error", err)
	}
}

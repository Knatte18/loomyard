// wakes.go reads what woke the orch in a run: batten's classified halts, parent-review rounds that reached the reviewer, and alignment approvals that superseded entries.

package reportengine

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"

	"github.com/Knatte18/loomyard/internal/alignment"
	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// The wake sources besides SourceBatten.
const (
	SourceParentReview   = "parent-review"
	SourceAlignment      = "alignment"
	SourceLandReadyTasks = "land-ready-tasks"
)

// verdictNone is the verdict word of an alignment attempt that wrote no verdict file.
const verdictNone = "none"

// parentReviewWakeKindDelivered is the kind of a parent-review wake whose round has no alignment record.
const parentReviewWakeKindDelivered = "delivered"

// alignmentWakeKindSuperseding is the kind of an alignment wake: an applied approve that superseded board entries.
const alignmentWakeKindSuperseding = "approve-superseding"

// WakeSource is one wake source's wakes of a run: the count per kind and their total.
type WakeSource struct {
	Kinds map[string]int `json:"kinds"`
	Total int            `json:"total"`
}

// AlignmentOutcome counts the alignment attempts of one point that ended in one verdict word, and how many of them Go applied.
type AlignmentOutcome struct {
	Point   string `json:"point"`
	Verdict string `json:"verdict"`
	Count   int    `json:"count"`
	Applied int    `json:"applied"`
}

// newWakeSource tallies kinds into a WakeSource.
func newWakeSource(kinds []string) *WakeSource {
	source := &WakeSource{Kinds: map[string]int{}}
	for _, kind := range kinds {
		source.Kinds[kind]++
		source.Total++
	}
	return source
}

// filesOfDir reads every regular file directly in dir at rev, keyed by file name.
func filesOfDir(records Reader, rev, dir string) (map[string][]byte, error) {
	names, err := records.FilesInDirAtRevision(rev, dir)
	if err != nil {
		return nil, fmt.Errorf("reportengine: list %s at %s: %w", dir, rev, err)
	}
	files := make(map[string][]byte, len(names))
	for _, name := range names {
		data, err := records.FileAtRevision(rev, path.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("reportengine: read %s at %s: %w", path.Join(dir, name), rev, err)
		}
		files[name] = data
	}
	return files, nil
}

// readWakes fills the run's wake sources, automatic steps and paused count from the records at tag.
// A source the run lacks stays nil under its name in Wakes; a record that does not decode is a finding and the run's other sources still count.
func (row *RunRow) readWakes(records Reader, tag gitrepo.TagRef) ([]Finding, error) {
	row.Wakes = map[string]*WakeSource{SourceBatten: nil, SourceParentReview: nil, SourceAlignment: nil, SourceLandReadyTasks: nil}

	if err := row.classifyBatten(records, tag.Commit); err != nil {
		return nil, err
	}
	reviewFindings, err := row.readParentReviewWakes(records, tag)
	if err != nil {
		return nil, err
	}
	alignmentFindings, err := row.readAlignmentWakes(records, tag)
	if err != nil {
		return nil, err
	}
	return append(reviewFindings, alignmentFindings...), nil
}

// classifyBatten classifies the batten snapshots under the rules batten runs with in production.
func (row *RunRow) classifyBatten(records Reader, rev string) error {
	if len(row.battenSnapshots) == 0 {
		return nil
	}
	record := battenshed.BattenRecord{Snapshots: row.battenSnapshots, Child: row.childHistory}
	rules := battenshed.NoticeRules{
		VerifyRows:  []string{loomshed.NameMergeIn, loomshed.NamePublish},
		GateRow:     loomshed.NamePRGate,
		EpisodeRows: []string{battenrecipe.NameLand, battenrecipe.NameWorktreeTeardown},
	}

	rules.Escalations = map[string]int{}
	for _, change := range battenshed.ClassifyRecords(record, rules).Changes {
		if _, counted := rules.Escalations[change.Producer]; counted {
			continue
		}
		count, err := escalationsOf(records, rev, change.Producer)
		if err != nil {
			return err
		}
		rules.Escalations[change.Producer] = count
	}
	result := battenshed.ClassifyRecords(record, rules)

	wakeKinds := make([]string, len(result.Wakes))
	for i, wake := range result.Wakes {
		wakeKinds[i] = wake.Kind
		if wake.Kind != battenshed.WakeKindReviewEscalation && wake.Kind != battenshed.WakeKindPRGate {
			wakeKinds[i] = wake.Kind + " at " + wake.Producer
		}
	}
	row.Wakes[SourceBatten] = newWakeSource(wakeKinds)
	row.Automatic = map[string]int{}
	for _, step := range result.Automatic {
		row.Automatic[step.Kind]++
	}
	paused := len(result.Paused)
	row.Paused = &paused
	row.changes = result.Changes
	row.accountedChild = result.AccountedChild
	return nil
}

// escalationsOf counts the escalation files of the review segment a Bouncer producer owns at rev, archived generations of that segment included.
// A producer that is no Bouncer owns none.
func escalationsOf(records Reader, rev, producer string) (int, error) {
	if producer == "" {
		return 0, nil
	}
	segment, isBouncer, err := loomrecipe.BouncerRunSubdir(producer)
	if err != nil {
		return 0, err
	}
	if !isBouncer {
		return 0, nil
	}

	reviewsDir := filepath.ToSlash(loomengine.LoomReviewsDirRel())
	siblings, err := records.SubdirsAtRevision(rev, reviewsDir)
	if err != nil {
		return 0, fmt.Errorf("reportengine: list %s at %s: %w", reviewsDir, rev, err)
	}
	count := 0
	for _, sibling := range siblings {
		if archived, ok := shedadapters.ArchivedRunDirSegment(sibling); sibling != segment && (!ok || archived != segment) {
			continue
		}
		names, err := records.FilesInDirAtRevision(rev, path.Join(reviewsDir, sibling))
		if err != nil {
			return 0, fmt.Errorf("reportengine: list %s at %s: %w", path.Join(reviewsDir, sibling), rev, err)
		}
		count += len(shedadapters.ReviewRounds(names).EscalatedRounds)
	}
	return count, nil
}

// readParentReviewWakes counts the parent-review rounds that woke the orch and sums every round's keep-alive pings.
// The source is absent when the tree has no parent-review directory.
func (row *RunRow) readParentReviewWakes(records Reader, tag gitrepo.TagRef) ([]Finding, error) {
	root := filepath.ToSlash(loomengine.LoomParentReviewDirRel())
	dirs, err := records.SubdirsAtRevision(tag.Commit, root)
	if err != nil {
		return nil, fmt.Errorf("reportengine: list %s at %s: %w", root, tag.Name, err)
	}
	row.Sources[SourceParentReview] = len(dirs) > 0
	if len(dirs) == 0 {
		return nil, nil
	}

	var findings []Finding
	var kinds []string
	pings := 0
	for _, dir := range dirs {
		number, ok := parentreview.RoundNumber(dir)
		if !ok {
			continue
		}
		files, err := filesOfDir(records, tag.Commit, path.Join(root, dir))
		if err != nil {
			return nil, err
		}
		round, err := parentreview.ParseRoundDir(number, files)
		if err != nil {
			findings = append(findings, Finding{Tag: tag.Name, File: path.Join(root, dir), Error: err.Error()})
			continue
		}
		pings += round.Delivery.Pings
		if round.Delivery.DeliveredAt.IsZero() || (round.Alignment != nil && round.Alignment.State == parentreview.AlignmentApplied) {
			continue
		}
		kind := parentReviewWakeKindDelivered
		if round.Alignment != nil {
			kind = round.Alignment.State
		}
		kinds = append(kinds, kind)
	}
	row.Wakes[SourceParentReview] = newWakeSource(kinds)
	row.ParentReviewPings = &pings
	return findings, nil
}

// readAlignmentWakes counts every alignment attempt per point and verdict, and the applied approvals that superseded board entries as wakes.
// The source is absent when the tree has no alignment directory.
func (row *RunRow) readAlignmentWakes(records Reader, tag gitrepo.TagRef) ([]Finding, error) {
	rels, err := shedrun.TreeRunRels(row.Slug)
	if err != nil {
		return nil, err
	}
	points, err := records.SubdirsAtRevision(tag.Commit, rels.Alignment)
	if err != nil {
		return nil, fmt.Errorf("reportengine: list %s at %s: %w", rels.Alignment, tag.Name, err)
	}
	row.Sources[SourceAlignment] = len(points) > 0
	if len(points) == 0 {
		return nil, nil
	}

	var findings []Finding
	var kinds []string
	type outcomeKey struct{ point, verdict string }
	outcomes := map[outcomeKey]*AlignmentOutcome{}
	for _, point := range points {
		pointDir := path.Join(rels.Alignment, point)
		attempts, err := records.SubdirsAtRevision(tag.Commit, pointDir)
		if err != nil {
			return nil, fmt.Errorf("reportengine: list %s at %s: %w", pointDir, tag.Name, err)
		}
		for _, attemptName := range attempts {
			attemptDir := path.Join(pointDir, attemptName)
			files, err := filesOfDir(records, tag.Commit, attemptDir)
			if err != nil {
				return nil, err
			}
			attempt, verdict, err := alignment.ParseAttemptDir(files)
			if err != nil {
				findings = append(findings, Finding{Tag: tag.Name, File: attemptDir, Error: err.Error()})
				continue
			}

			word := verdictNone
			if verdict != nil {
				word = verdict.Verdict
			}
			key := outcomeKey{point, word}
			if outcomes[key] == nil {
				outcomes[key] = &AlignmentOutcome{Point: point, Verdict: word}
			}
			outcomes[key].Count++
			if attempt.Applied != "" {
				outcomes[key].Applied++
			}
			if attempt.Applied == alignment.VerdictApprove && verdict != nil && len(verdict.Superseded) > 0 {
				kinds = append(kinds, alignmentWakeKindSuperseding)
			}
		}
	}

	row.Wakes[SourceAlignment] = newWakeSource(kinds)
	row.Alignment = []AlignmentOutcome{}
	for _, outcome := range outcomes {
		row.Alignment = append(row.Alignment, *outcome)
	}
	sort.Slice(row.Alignment, func(i, j int) bool {
		a, b := row.Alignment[i], row.Alignment[j]
		if a.Point != b.Point {
			return a.Point < b.Point
		}
		return a.Verdict < b.Verdict
	})
	return findings, nil
}

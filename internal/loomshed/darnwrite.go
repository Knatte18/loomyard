// darnwrite.go implements the Darn-Write row's producer: it wraps the gated darn writer session and does everything Go owns around it.
// That is the two token texts the session is told, the commit of the landing directory after the session, and the removal of the pending rejection a spawn answered.

package loomshed

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// darnResumeWayForward is appended to the Reason of a Darn-Write session whose gate failed.
const darnResumeWayForward = "way forward: run \"lyx loom resume\" in the task worktree, which re-spawns Darn-Write with a fresh verify budget, or, when the parent has moved since this run's verify failure and the task branch and worktree have not, first takes the parent in through Merge-In"

// DarnTold carries the two token texts the producer decides for one darn writer spawn, each empty when it has nothing to tell.
type DarnTold struct {
	// RejectionFindings is the pending rejection's findings.
	RejectionFindings string
	// PriorWork is the note that the change is already under way on the task branch.
	PriorWork string
}

// DarnOutcome is the latest Darn-Write history entry as the producer reads it.
type DarnOutcome struct {
	// Outcome is the entry's outcome.
	Outcome shedengine.Outcome
	// Reason is the halt reason recorded with the entry; empty when the run did not halt there.
	Reason string
}

// DarnDeps carries the told seams NewDarnWrite needs.
type DarnDeps struct {
	// ReadRejection returns the pending rejection; found is false when none is recorded.
	ReadRejection func() (PendingRejection, bool, error)
	// ClearRejection removes the pending rejection record.
	ClearRejection func() error
	// RecordRejection keeps the pending rejection's findings where the darn amendment reads them; it is called before ClearRejection, and nil records nothing.
	RecordRejection func(PendingRejection) error
	// Commit commits the landing directory.
	Commit func() error
	// LatestOutcome returns the latest Darn-Write history entry of the run; found is false when the row has no entry yet.
	LatestOutcome func() (DarnOutcome, bool, error)
	// PublishFailure returns the Publish failure record and its path; found is false when none is present.
	PublishFailure func() (path string, record verifytree.PublishFailure, found bool, err error)
	// RunStart returns the run's start; the zero time when the run's seed carries no stamp.
	RunStart func() (time.Time, error)
	// TaskHead returns the task branch's HEAD commit.
	TaskHead func() (string, error)
	// Dirty returns the task worktree's uncommitted paths, read through the dirty-paths reader Merge-In's clean check uses.
	Dirty func() ([]string, error)
	// ParentTips returns the parent tips Merge-In would merge, read without a fetch.
	ParentTips func() ([]string, error)
	// IsAncestor reports whether sha is an ancestor of the task branch's HEAD.
	IsAncestor func(sha string) (bool, error)
}

// darnWrite decorates the gated darn writer session, built per Call by session, with the Go-owned steps around it.
type darnWrite struct {
	name    string
	session func(DarnTold) shedengine.ShedProducer
	deps    DarnDeps
}

var _ shedengine.ShedProducer = (*darnWrite)(nil)

// NewDarnWrite returns the Darn-Write producer identified as name.
// Each Call builds its session from session, handing it the token texts the producer decides.
func NewDarnWrite(name string, session func(DarnTold) shedengine.ShedProducer, deps DarnDeps) shedengine.ShedProducer {
	return &darnWrite{name: name, session: session, deps: deps}
}

// Call implements shedengine.ShedProducer.
// A failed read of the pending rejection is Stuck naming the reject command.
// A failed read of the latest outcome or the failure record, a Commit failure and a ClearRejection failure are returned errors, never Stuck, because re-running the session cannot fix a git or filesystem fault.
// A failure record this run wrote, on a task branch and worktree unchanged since it and a parent that has moved, returns a rerouting Stuck before any session is built.
// The parent is then merged in instead of the writer re-running against a failure the parent may have fixed.
// The commit runs on every non-empty output pointer, a gate-failed Stuck included, so a halted run leaves the work committed.
// A gate-failed Stuck keeps its pointer and gains the resume way forward.
func (p *darnWrite) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.checkSeams(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	pending, rejected, err := p.deps.ReadRejection()
	if err != nil {
		return stuck(fmt.Sprintf("read the pending rejection: %v; fix it or run %s again", err, reworkRejectCommand))
	}
	latest, found, err := p.deps.LatestOutcome()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: read the latest outcome: %w", p.name, err)
	}
	failureRecord, record, hasRecord, err := p.deps.PublishFailure()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: read the Publish failure record: %w", p.name, err)
	}
	if !hasRecord {
		failureRecord = ""
	} else if reason := p.rerouteReason(failureRecord, record); reason != "" {
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason, Reroute: true}, nil
	}

	told := DarnTold{PriorWork: renderPriorWork(latest, found, failureRecord)}
	if rejected {
		told.RejectionFindings = pending.Findings
	}

	outcome, pointer, err := p.session(told).Call(ctx)
	if err != nil || pointer.Path == "" {
		return outcome, pointer, err
	}

	if err := p.deps.Commit(); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: commit produced artifacts: %w; way forward: the fault is transient, re-step the %s row", p.name, err, p.name)
	}
	switch outcome {
	case shedengine.Done:
		if rejected {
			if p.deps.RecordRejection != nil {
				if err := p.deps.RecordRejection(pending); err != nil {
					return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: record rejection: %w", p.name, err)
				}
			}
			if err := p.deps.ClearRejection(); err != nil {
				return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: clear rejection: %w", p.name, err)
			}
		}
	case shedengine.Stuck:
		pointer.Reason = strings.Join(nonEmpty(pointer.Reason, darnResumeWayForward), "; ")
	}
	return outcome, pointer, nil
}

// checkSeams names the first unwired seam; the constructor returns the bare seam interface and so has no error to refuse a nil through.
func (p *darnWrite) checkSeams() error {
	seams := []struct {
		name    string
		missing bool
	}{
		{"session factory", p.session == nil},
		{"ReadRejection", p.deps.ReadRejection == nil},
		{"ClearRejection", p.deps.ClearRejection == nil},
		{"Commit", p.deps.Commit == nil},
		{"LatestOutcome", p.deps.LatestOutcome == nil},
		{"PublishFailure", p.deps.PublishFailure == nil},
		{"RunStart", p.deps.RunStart == nil},
		{"TaskHead", p.deps.TaskHead == nil},
		{"Dirty", p.deps.Dirty == nil},
		{"ParentTips", p.deps.ParentTips == nil},
		{"IsAncestor", p.deps.IsAncestor == nil},
	}
	for _, s := range seams {
		if s.missing {
			return fmt.Errorf("loomshed: %s: no %s seam wired", p.name, s.name)
		}
	}
	return nil
}

// rerouteReason returns the reason to reroute to Merge-In instead of spawning the writer, or "" when the writer runs.
// It reroutes only when the record at recordPath was written at or after the run's start, the task branch is still at the record's head, the task worktree is clean, and some parent tip is not an ancestor of HEAD.
// A seam error counts as no reroute, except a Dirty error, which counts as a dirty tree; each is logged at Warn.
func (p *darnWrite) rerouteReason(recordPath string, record verifytree.PublishFailure) string {
	writtenAt, err := time.Parse(time.RFC3339, record.WrittenAt)
	if err != nil {
		return ""
	}
	runStart, err := p.deps.RunStart()
	if err != nil {
		logger.Warn("loomshed: could not read the run start; the Darn-Write writer runs", "row", p.name, "error", err)
		return ""
	}
	if runStart.IsZero() || writtenAt.UTC().Before(runStart.UTC().Truncate(time.Second)) {
		return ""
	}
	head, err := p.deps.TaskHead()
	if err != nil {
		logger.Warn("loomshed: could not read the task branch head; the Darn-Write writer runs", "row", p.name, "error", err)
		return ""
	}
	if head != record.Head {
		return ""
	}
	dirty, err := p.deps.Dirty()
	if err != nil {
		logger.Warn("loomshed: could not read the task worktree status; treating it as dirty", "row", p.name, "error", err)
		return ""
	}
	if len(dirty) > 0 {
		return ""
	}
	tips, err := p.deps.ParentTips()
	if err != nil {
		logger.Warn("loomshed: could not read the parent tips; the Darn-Write writer runs", "row", p.name, "error", err)
		return ""
	}
	moved := false
	for _, tip := range tips {
		ancestor, err := p.deps.IsAncestor(tip)
		if err != nil {
			logger.Warn("loomshed: could not tell whether a parent tip is in HEAD; the Darn-Write writer runs", "row", p.name, "tip", tip, "error", err)
			return ""
		}
		if !ancestor {
			moved = true
			break
		}
	}
	if !moved {
		return ""
	}
	return fmt.Sprintf("the parent branch moved since the verify failure recorded at %s, while the task branch and worktree have not changed since it; "+
		"rerouting to Merge-In, which merges the parent and re-runs the failing gate; "+
		"if Merge-In stops without merging, on a parent pair that is dirty, mid-merge, ahead of or diverged from its upstream, "+
		"the way forward is to repair the parent pair as Merge-In's Stuck reason says before any resume, "+
		"since a resume over the unrepaired pair reroutes back to the same stop", recordPath)
}

// renderPriorWork returns the note that the change is already under way on the task branch, naming the latest outcome's halt reason and the failure record's path.
// It is empty when the row has no halt reason and no record to name.
func renderPriorWork(latest DarnOutcome, found bool, failureRecord string) string {
	var reason string
	if found {
		reason = latest.Reason
	}
	if reason == "" && failureRecord == "" {
		return ""
	}
	var note strings.Builder
	note.WriteString("An earlier spawn of this task already began the change, and its work is committed on the task branch.\nRead that work first and continue it instead of starting over.\n")
	if reason != "" {
		fmt.Fprintf(&note, "The run halted there with: %s\n", reason)
	}
	if failureRecord != "" {
		fmt.Fprintf(&note, "The failure record of the last Publish is at %s; read it before you change anything.\n", failureRecord)
	}
	return strings.TrimSuffix(note.String(), "\n")
}

// nonEmpty returns the values that are not empty, in order.
func nonEmpty(values ...string) []string {
	var kept []string
	for _, v := range values {
		if v != "" {
			kept = append(kept, v)
		}
	}
	return kept
}

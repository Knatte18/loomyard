// darnwrite.go implements the Darn row's producer: it wraps the gated darn writer session and does everything Go owns around it.
// That is the two token texts the session is told, the commit of the landing directory after the session, and the removal of the pending rejection a spawn answered.

package loomshed

import (
	"context"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// darnResumeWayForward is appended to the Reason of a Darn session whose gate failed.
const darnResumeWayForward = "way forward: run \"lyx loom resume\" in the task worktree, which re-spawns Darn with a fresh verify budget"

// DarnTold carries the two token texts the producer decides for one darn writer spawn, each empty when it has nothing to tell.
type DarnTold struct {
	// RejectionFindings is the pending rejection's findings.
	RejectionFindings string
	// PriorWork is the note that the change is already under way on the task branch.
	PriorWork string
}

// DarnOutcome is the latest Darn history entry as the producer reads it.
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
	// Commit commits the landing directory.
	Commit func() error
	// LatestOutcome returns the latest Darn history entry of the run; found is false when the row has no entry yet.
	LatestOutcome func() (DarnOutcome, bool, error)
	// PublishFailure returns the path of the Publish failure record; found is false when none is present.
	PublishFailure func() (string, bool, error)
}

// darnWrite decorates the gated darn writer session, built per Call by session, with the Go-owned steps around it.
type darnWrite struct {
	name    string
	session func(DarnTold) shedengine.ShedProducer
	deps    DarnDeps
}

var _ shedengine.ShedProducer = (*darnWrite)(nil)

// NewDarnWrite returns the Darn producer identified as name.
// Each Call builds its session from session, handing it the token texts the producer decides.
func NewDarnWrite(name string, session func(DarnTold) shedengine.ShedProducer, deps DarnDeps) shedengine.ShedProducer {
	return &darnWrite{name: name, session: session, deps: deps}
}

// Call implements shedengine.ShedProducer.
// A failed read of the pending rejection is Stuck naming the reject command.
// A failed read of the latest outcome or the failure record, a Commit failure and a ClearRejection failure are returned errors, never Stuck, because re-running the session cannot fix a git or filesystem fault.
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
	failureRecord, hasRecord, err := p.deps.PublishFailure()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: read the Publish failure record: %w", p.name, err)
	}
	if !hasRecord {
		failureRecord = ""
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
	}
	for _, s := range seams {
		if s.missing {
			return fmt.Errorf("loomshed: %s: no %s seam wired", p.name, s.name)
		}
	}
	return nil
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

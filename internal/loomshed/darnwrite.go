// darnwrite.go implements the Darn-Write row's producer: it wraps the gated darn writer session and does everything Go owns around it.
// That is the token text the session is told and the commit of the landing directory after the session.

package loomshed

import (
	"context"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// darnResumeWayForward is appended to the Reason of a Darn-Write session whose gate failed.
const darnResumeWayForward = "way forward: run \"lym loom resume\" in the task worktree, which re-spawns Darn-Write with a fresh verify budget"

// decisionsWinLine tells the darn writer that a decision recorded after the Discussion wins over a gate finding it forbids fixing.
const decisionsWinLine = "Where a decision under `Added after Discussion` in the decision record forbids fixing a finding, follow the decision and leave that finding unfixed."

// DarnGateReprompt returns the re-prompt line renderer of a Darn-Write gate entry.
// The line names the findings file and decisionRecordPath, and lets a decision recorded after the Discussion win over a finding it forbids fixing, which the default "fix every finding" line contradicts.
// The rendered line carries no newline, as a gate re-prompt requires.
func DarnGateReprompt(decisionRecordPath string) func(findingsPath string) string {
	return func(findingsPath string) string {
		return fmt.Sprintf("Gate findings recorded at %s — read it and fix every finding, bar one a decision under `Added after Discussion` in %s forbids fixing, which you leave unfixed; then end your turn.", findingsPath, decisionRecordPath)
	}
}

// DarnTold carries the token text the producer decides for one darn writer spawn, empty when it has nothing to tell.
type DarnTold struct {
	// PriorWork is the note that the change is already under way on the task branch.
	PriorWork string
}

// DarnOutcome is the latest Darn-Write history entry as the producer reads it.
type DarnOutcome struct {
	// Outcome is the entry's outcome.
	Outcome shedengine.Outcome
	// FindingsPath is the row's kept gate findings file, which holds the text of the last failed gate; empty when no file is kept.
	FindingsPath string
}

// DarnDeps carries the told seams NewDarnWrite needs.
type DarnDeps struct {
	// Commit commits the landing directory.
	Commit func() error
	// LatestOutcome returns the latest Darn-Write history entry of the run that records a spawn; found is false when the row has no such entry yet.
	LatestOutcome func() (DarnOutcome, bool, error)
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
// A failed read of the latest outcome and a Commit failure are returned errors, never Stuck, because re-running the session cannot fix a git or filesystem fault.
// The commit runs on every non-empty output pointer, a gate-failed Stuck included, so a halted run leaves the work committed.
// A gate-failed Stuck keeps its pointer and gains the resume way forward.
func (p *darnWrite) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.checkSeams(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	latest, found, err := p.deps.LatestOutcome()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: read the latest outcome: %w", p.name, err)
	}

	told := DarnTold{PriorWork: renderPriorWork(latest, found)}

	outcome, pointer, err := p.session(told).Call(ctx)
	if err != nil || pointer.Path == "" {
		return outcome, pointer, err
	}

	if err := p.deps.Commit(); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: commit produced artifacts: %w; way forward: the fault is transient, re-step the %s row", p.name, err, p.name)
	}
	if outcome == shedengine.Stuck {
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
		{"Commit", p.deps.Commit == nil},
		{"LatestOutcome", p.deps.LatestOutcome == nil},
	}
	for _, s := range seams {
		if s.missing {
			return fmt.Errorf("loomshed: %s: no %s seam wired", p.name, s.name)
		}
	}
	return nil
}

// renderPriorWork returns the note that the change is already under way on the task branch, naming the latest outcome and its kept gate findings file.
// It tells the writer that a decision recorded after the Discussion wins over a finding it forbids fixing, so a resumed spawn never works against the parent's call.
// It is empty when the row has no earlier entry.
func renderPriorWork(latest DarnOutcome, found bool) string {
	if !found {
		return ""
	}
	lines := []string{
		"An earlier spawn of this task already began the change, and its work is committed on the task branch.",
		"Read that work first and continue it instead of starting over.",
		fmt.Sprintf("That spawn ended %s.", latest.Outcome),
	}
	if latest.FindingsPath != "" {
		lines = append(lines, fmt.Sprintf("Its last gate findings are kept at %s: read them and fix what they name.", latest.FindingsPath))
	}
	lines = append(lines, decisionsWinLine)
	return strings.Join(lines, "\n")
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

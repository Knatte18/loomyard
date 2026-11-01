// darnwrite.go implements the Darn-Write row's producer: it wraps the gated darn writer session and does everything Go owns around it.
// That is the refusal of a run whose decision record or support log is missing, the token text the session is told, the stop marker, and the commit of the landing directory after the session.

package loomshed

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// darnResumeWayForward is appended to the Reason of a Darn-Write session whose gate failed.
const darnResumeWayForward = "way forward: run \"lym loom resume\" in the task worktree, which re-spawns Darn-Write with a fresh verify budget"

// darnRecordGotoWayForward is the way forward of a Darn-Write call that finds the decision record or the support log missing.
// Record-Write writes only the file that is missing, so the goto keeps every entry the record holds.
const darnRecordGotoWayForward = "way forward: run \"lym loom goto --to Record-Write\", then \"lym loom resume\""

// darnRecordRestoreWayForward is the way forward of a Darn-Write call that finds the decision record missing from the working copy while the committed copy holds it.
// Writing the record again from the board entry would drop the committed `Added after Discussion` entries.
const darnRecordRestoreWayForward = "way forward: restore the decision record from the committed copy under _lym with \"git -C _lym checkout HEAD -- discussion/decision-record.md\", then run \"lym loom resume\""

// darnStopWayForward follows the stop report in the Reason of a Darn-Write session whose writer stopped and wrote the stop marker.
const darnStopWayForward = "way forward: record the answer with \"lym loom decision add\", then run \"lym loom resume\""

// darnStopReportMissing stands in for the stop report when the writer wrote the stop marker and no report.
const darnStopReportMissing = "the writer stopped and wrote no stop report"

// darnConditionStopReportMissing is the format of the Reason of a Darn-Write session whose writer stopped on the task body's condition and wrote an empty report; its argument is the stop marker path.
const darnConditionStopReportMissing = "the writer stopped on the task body's condition and wrote no report; the stop marker %s marks the stop"

// decisionRecordRestoreWayForward is the way forward of a Darn-Write session that left the decision record invalid.
// Restoring keeps every `Added after Discussion` entry the record held.
const decisionRecordRestoreWayForward = "way forward: restore each failing file under _lym to its latest version \"lym loom validate-discussion\" accepts (the committed copy when the damage is uncommitted, restored with \"git -C _lym checkout HEAD -- discussion/<file>\"), commit it with \"lym fabric commit\", which the orch runs when a session lym refuses it from reports status: FAILED, then run \"lym loom resume\""

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
	// RecordPath is the absolute path of the decision record Record-Write wrote, the writer's scope.
	RecordPath string
	// SupportLogPath is the absolute path of the support log Record-Write wrote.
	SupportLogPath string
	// StopMarkerPath is the absolute path of the marker the writer creates to say it stopped instead of making the change.
	StopMarkerPath string
	// ConditionStopMarkerPath is the absolute path of the marker the writer creates, beside the stop marker, to say it stopped on a condition the task's body names.
	ConditionStopMarkerPath string
	// CommittedRecord reports whether the committed history holds the decision record, which tells a record removed from the working copy from one that was never written.
	CommittedRecord func() (bool, error)
	// Commit commits the landing directory.
	Commit func() error
	// LatestOutcome returns the latest Darn-Write history entry of the run that records a spawn; found is false when the row has no such entry yet.
	LatestOutcome func() (DarnOutcome, bool, error)
	// CheckRecord validates the decision record once the writer session returns, returning the findings that halt the run; it commits nothing.
	CheckRecord func() ([]discussionparser.Finding, error)
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
// A missing decision record, or a support log missing beside a present record, halts the run Stuck before any session, with the way forward that brings the file back.
// A stop marker and a condition stop marker left by an earlier spawn are removed before each session.
// A failed read of the latest outcome, a Commit failure and a CheckRecord error are returned errors, never Stuck, because re-running the session cannot fix a git or filesystem fault.
// The commit runs on every non-empty output pointer, a gate-failed Stuck included, so a halted run leaves the work committed.
// A gate-failed Stuck keeps its pointer and gains the resume way forward.
// A stop marker the session wrote halts the run Stuck, Done or gate-failed alike, with the session's output as the reason, since the writer stopped instead of making the change.
// When the condition stop marker is present beside it, the writer stopped on a condition the task's body names, and the reason is the stop report alone, whose own way forward stands, or a fixed message naming the stop marker when the report is empty; a condition stop marker with no stop marker is no stop.
// A Done session is then guarded: CheckRecord runs once after the commit, and findings halt the run Stuck with no re-prompt, since the record is written by Go and a re-prompt could only push the writer to edit it.
// The Stuck reason names the findings and the way forward that restores the record.
func (p *darnWrite) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.checkSeams(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	refusal, err := p.scopeRefusal()
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if refusal != "" {
		return shedengine.Stuck, shedengine.OutputPointer{Reason: refusal}, nil
	}
	for _, marker := range []string{p.deps.StopMarkerPath, p.deps.ConditionStopMarkerPath} {
		if err := os.Remove(marker); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: remove the stop marker %s: %w", p.name, marker, err)
		}
	}

	latest, found, err := p.deps.LatestOutcome()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: read the latest outcome: %w", p.name, err)
	}

	told := DarnTold{PriorWork: renderPriorWork(latest, found)}

	outcome, pointer, err := runAndCommit(ctx, p.name, p.session(told), p.deps.Commit)
	if err != nil || pointer.Path == "" {
		return outcome, pointer, err
	}

	stopped, err := fileExists(p.deps.StopMarkerPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: read the stop marker: %w", p.name, err)
	}
	if stopped {
		report, err := readStopReport(pointer.Path)
		if err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: read the stop report: %w", p.name, err)
		}
		conditionStopped, err := fileExists(p.deps.ConditionStopMarkerPath)
		if err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: read the condition stop marker: %w", p.name, err)
		}
		if conditionStopped {
			pointer.Reason = report
			if report == "" {
				pointer.Reason = fmt.Sprintf(darnConditionStopReportMissing, p.deps.StopMarkerPath)
			}
			return shedengine.Stuck, pointer, nil
		}
		pointer.Reason = strings.Join(nonEmpty(report, darnStopWayForward), "; ")
		return shedengine.Stuck, pointer, nil
	}

	if outcome == shedengine.Stuck {
		pointer.Reason = strings.Join(nonEmpty(pointer.Reason, darnResumeWayForward), "; ")
		return outcome, pointer, nil
	}

	findings, err := p.deps.CheckRecord()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: check the decision record: %w", p.name, err)
	}
	if len(findings) > 0 {
		pointer.Reason = fmt.Sprintf("the writer left the decision record invalid: %s; %s", formatDiscussionFindings(findings), decisionRecordRestoreWayForward)
		return shedengine.Stuck, pointer, nil
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
		{"CheckRecord", p.deps.CheckRecord == nil},
		{"CommittedRecord", p.deps.CommittedRecord == nil},
		{"RecordPath", p.deps.RecordPath == ""},
		{"SupportLogPath", p.deps.SupportLogPath == ""},
		{"StopMarkerPath", p.deps.StopMarkerPath == ""},
		{"ConditionStopMarkerPath", p.deps.ConditionStopMarkerPath == ""},
	}
	for _, s := range seams {
		if s.missing {
			return fmt.Errorf("loomshed: %s: no %s seam wired", p.name, s.name)
		}
	}
	return nil
}

// scopeRefusal returns the Stuck reason of a run whose decision record or support log is missing, and empty when both are present.
// A record missing from the working copy but held by the committed history is restored from there: writing it again from the board entry would drop its `Added after Discussion` entries.
func (p *darnWrite) scopeRefusal() (string, error) {
	recordPresent, err := fileExists(p.deps.RecordPath)
	if err != nil {
		return "", fmt.Errorf("loomshed: %s: read the decision record: %w", p.name, err)
	}
	if !recordPresent {
		committed, err := p.deps.CommittedRecord()
		if err != nil {
			return "", fmt.Errorf("loomshed: %s: read the committed decision record: %w", p.name, err)
		}
		if committed {
			return fmt.Sprintf("the decision record %s is missing from the working copy; %s", p.deps.RecordPath, darnRecordRestoreWayForward), nil
		}
		return fmt.Sprintf("the decision record %s is missing; %s", p.deps.RecordPath, darnRecordGotoWayForward), nil
	}
	supportLogPresent, err := fileExists(p.deps.SupportLogPath)
	if err != nil {
		return "", fmt.Errorf("loomshed: %s: read the support log: %w", p.name, err)
	}
	if !supportLogPresent {
		return fmt.Sprintf("the support log %s is missing; %s", p.deps.SupportLogPath, darnRecordGotoWayForward), nil
	}
	return "", nil
}

// fileExists reports whether path exists; any stat error but not-exist is returned.
func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// readStopReport returns the trimmed text of the stop report at path, or darnStopReportMissing when the file is absent.
func readStopReport(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return darnStopReportMissing, nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
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

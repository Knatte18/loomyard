// fixerrun.go implements the merge fixer loop Merge-In and Finalize share: one cold session per row entry, held to a turn budget kept in the attempt record, and judged by what the session's report and the landing gate say after each turn.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// LandingStopPrefix opens the reason of every halt a landing row hands back to the orch, so the hand-back predicate finds it.
const LandingStopPrefix = "landing stop: "

// fixerGateName labels the fixer session's one gate entry.
const fixerGateName = "landing"

// fixerSeats is the seat runner the fixer session runs on, which *seatengine.Engine satisfies.
type fixerSeats interface {
	// Run starts table's seats fresh and returns once the chair has ended.
	Run(table seatengine.Table) (seatengine.Result, error)
}

var _ fixerSeats = (*seatengine.Engine)(nil)

// fixerEnd names how one fixer run ended.
type fixerEnd string

const (
	// fixerEndFixed is a turn whose report says fixed and over which the landing gate passes.
	fixerEndFixed fixerEnd = "fixed"
	// fixerEndEscalate is a report that escalates a conflict between the parent's change and a decision of the record.
	fixerEndEscalate fixerEnd = "escalate"
	// fixerEndSpent is a landing gate that still fails after the last turn the entry had left.
	fixerEndSpent fixerEnd = "spent"
	// fixerEndTimeout is a turn that outran fixer_timeout_min, or a session that outran the bound of every turn the entry had left.
	fixerEndTimeout fixerEnd = "timeout"
	// fixerEndNoReport is a turn that left no report, or one that does not parse.
	fixerEndNoReport fixerEnd = "no-report"
	// fixerEndDirty is a turn that left the working tree dirty.
	fixerEndDirty fixerEnd = "dirty"
	// fixerEndPushed is a session during which the task branch's remote ref moved.
	fixerEndPushed fixerEnd = "pushed"
)

// fixerOutcome is how a fixer run ended and what the halt that follows names.
type fixerOutcome struct {
	// end is how the run ended.
	end fixerEnd
	// reportPath is the report the session writes.
	reportPath string
	// sessionLog is the session's run directory, which holds its log.
	sessionLog string
	// failureLog is the log the landing failure record names; set for a spent end.
	failureLog string
	// conflict is the report's conflict text; set for an escalate end.
	conflict string
	// pushedRef is the remote ref that moved, with its new tip; set for a pushed end.
	pushedRef string
}

// fixerRun runs the merge fixer for one row.
type fixerRun struct {
	deps  Deps
	row   string
	gate  verifyGate
	seats fixerSeats
}

// newFixerRun returns the fixer loop of row over deps' seams, judging each turn through gate.
func newFixerRun(deps Deps, row string, gate verifyGate) fixerRun {
	return fixerRun{deps: deps, row: row, gate: gate, seats: deps.FixerSeats}
}

// run spends the turns rec has left on one fixer session in mode and returns how it ended with the attempt record as it stands then.
// cap is the row's turn cap, and tips are the tip the parent's changes are read from and the merged tip.
// It is never called with no turn left, since a row takes its exhausted-entry path first.
// A turn is charged into the record when it starts, so a turn that never returns to the gate is still spent.
// The first turn's charge is taken back when the session fails to start, an error marked transient, and an escalate report takes its own turn back.
func (f fixerRun) run(ctx context.Context, mode FixerMode, rec FixerAttempts, cap int, tips [2]string) (fixerOutcome, FixerAttempts, error) {
	reportPath := filepath.Join(f.deps.ScratchDir, f.row+"-fixer-report.md")
	attemptsPath := FixerAttemptsPath(f.deps.ScratchDir, f.row)
	turnsLeft := cap - rec.Spent
	turnLimit := time.Duration(f.deps.Config.FixerTimeoutMin) * time.Minute

	table, err := FixerTable(FixerInputs{
		StencilsDir:        f.deps.StencilsDir,
		Mode:               mode,
		DecisionRecordPath: f.deps.DecisionRecordPath,
		MergeRecordPath:    f.deps.MergeRecordPath,
		PlanDir:            f.deps.PlanDir,
		FailureRecordPath:  f.failureRecordPath(),
		AlignedTip:         tips[0],
		MergedTip:          tips[1],
		ReportPath:         reportPath,
		TaskBranch:         f.deps.TaskBranch,
		ParentBranch:       f.deps.ParentBranch,
		Slug:               f.deps.Slug,
		LanguageSkills:     f.deps.LanguageSkills,
	}, f.deps.Config, f.deps.Registry)
	if err != nil {
		return fixerOutcome{}, rec, fmt.Errorf("landingshed: %s: %w", f.row, err)
	}
	if err := f.archiveStaleReport(reportPath); err != nil {
		return fixerOutcome{}, rec, err
	}
	remoteBefore, err := f.deps.TaskRemoteTip()
	if err != nil {
		return fixerOutcome{}, rec, fmt.Errorf("landingshed: %s: read the task branch's remote tip: %w", f.row, err)
	}

	original := rec
	session := &fixerSession{run: f, ctx: ctx, rec: rec, cap: cap, attemptsPath: attemptsPath, turnLimit: turnLimit, outcome: fixerOutcome{reportPath: reportPath}}
	if err := session.chargeTurn(); err != nil {
		return fixerOutcome{}, rec, err
	}
	table.Timeout = turnLimit * time.Duration(turnsLeft)
	table.Gate = shuttleengine.GateSpec{{Name: fixerGateName, Gate: session.check, Attempts: turnsLeft}}

	result, runErr := f.seats.Run(table)
	if session.err != nil {
		return fixerOutcome{}, session.rec, session.err
	}
	if (runErr != nil && errors.Is(runErr, shuttleengine.ErrNotStarted)) || (runErr == nil && result.Chair.NotStarted) {
		cause := runErr
		if cause == nil {
			cause = errors.New(result.Chair.StartError)
		}
		if err := WriteFixerAttempts(attemptsPath, original); err != nil {
			return fixerOutcome{}, session.rec, err
		}
		return fixerOutcome{}, original, shedengine.MarkTransient(shedengine.TransientAgentStart, fmt.Errorf("landingshed: %s: the fixer session did not start: %w", f.row, cause))
	}
	if runErr != nil {
		return fixerOutcome{}, session.rec, fmt.Errorf("landingshed: %s: run the fixer session: %w", f.row, runErr)
	}

	outcome := session.outcome
	outcome.sessionLog = result.Chair.RunDir
	switch {
	case session.end != "":
		outcome.end = session.end
	case !session.reached && result.Chair.Outcome == shuttleengine.OutcomeTimeout:
		outcome.end = fixerEndTimeout
	case !session.reached:
		outcome.end = fixerEndNoReport
	default:
		outcome.end = fixerEndFixed
	}

	remoteAfter, err := f.deps.TaskRemoteTip()
	if err != nil {
		return fixerOutcome{}, session.rec, fmt.Errorf("landingshed: %s: read the task branch's remote tip: %w", f.row, err)
	}
	if remoteAfter != remoteBefore {
		outcome.end = fixerEndPushed
		outcome.pushedRef = fmt.Sprintf("origin/%s at %s", f.deps.TaskBranch, remoteAfter)
	}
	logger.Info("landingshed: fixer run ended", "row", f.row, "mode", string(mode), "end", string(outcome.end), "spent", session.rec.Spent, "cap", cap)
	return outcome, session.rec, nil
}

// failureRecordPath returns the landing failure record's path when a failed gate left one, and "" before any gate has failed.
func (f fixerRun) failureRecordPath() string {
	if _, err := os.Stat(f.gate.paths.LandingFailure); err != nil {
		return ""
	}
	return f.gate.paths.LandingFailure
}

// archiveStaleReport renames a report an earlier fixer run left at reportPath, so a new session never starts beside a finished report.
func (f fixerRun) archiveStaleReport(reportPath string) error {
	stamp := f.gate.now().UTC().Format("20060102T150405Z")
	archived := strings.TrimSuffix(reportPath, ".md") + "-" + stamp + ".md"
	if err := os.Rename(reportPath, archived); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("landingshed: %s: archive the stale fixer report: %w", f.row, err)
	}
	return nil
}

// fixerSession is the state one fixer run shares with the gate closure the seat run calls after each turn.
type fixerSession struct {
	run          fixerRun
	ctx          context.Context
	rec          FixerAttempts
	cap          int
	attemptsPath string
	turnLimit    time.Duration
	turnStart    time.Time
	outcome      fixerOutcome

	// reached is set once the closure has run, which a turn that never returned to the gate leaves false.
	reached bool
	// end is the halting end the closure decided, "" while the run is fixed or undecided.
	end fixerEnd
	// decided holds the closure's last terminal answer, so a second evaluation of a finished run answers the same without charging again.
	decided *shuttleengine.GateResult
	// err is an infrastructure error the closure met, which the run returns after the seat run ends.
	err error
}

// chargeTurn counts the turn that starts now into the record and writes it.
func (s *fixerSession) chargeTurn() error {
	s.rec.Spent++
	s.turnStart = s.run.gate.now()
	return WriteFixerAttempts(s.attemptsPath, s.rec)
}

// halt records end as the run's end and answers the terminal gate result that finalizes the seat run at once.
func (s *fixerSession) halt(end fixerEnd, findings string) (shuttleengine.GateResult, error) {
	s.end = end
	result := shuttleengine.GateResult{Findings: findings, Terminal: true}
	s.decided = &result
	return result, nil
}

// check is the gate closure run after every turn.
// It judges the turn's duration, then the tree, then the report, and on a fixed verdict the landing gate.
// A failing gate with a turn left charges it and re-prompts; one with none left is the spent end.
func (s *fixerSession) check() (shuttleengine.GateResult, error) {
	result, err := s.judge()
	if err != nil {
		s.err = err
		return shuttleengine.GateResult{}, err
	}
	return result, nil
}

func (s *fixerSession) judge() (shuttleengine.GateResult, error) {
	if s.decided != nil {
		return *s.decided, nil
	}
	s.reached = true
	gate := s.run.gate
	if gate.now().Sub(s.turnStart) > s.turnLimit {
		return s.halt(fixerEndTimeout, "the fixer's turn ran past fixer_timeout_min")
	}
	reason, err := gate.clean(s.run.row, "after the fixer's turn")
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if reason != "" {
		return s.halt(fixerEndDirty, reason)
	}
	report, err := ParseFixerReport(s.outcome.reportPath)
	if err != nil {
		return s.halt(fixerEndNoReport, err.Error())
	}
	if report.Verdict == FixerEscalate {
		s.rec.Spent--
		if err := WriteFixerAttempts(s.attemptsPath, s.rec); err != nil {
			return shuttleengine.GateResult{}, err
		}
		s.outcome.conflict = report.Conflict
		return s.halt(fixerEndEscalate, report.Conflict)
	}

	reason, err = gate.check(s.ctx, s.run.row, s.run.deps.ParentBranch)
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if reason == "" {
		passed := shuttleengine.GateResult{Passed: true}
		s.decided = &passed
		return passed, nil
	}
	if s.rec.Spent >= s.cap {
		failure, found, err := verifytree.ReadLandingFailure(gate.paths)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if found {
			s.outcome.failureLog = failure.LogPath
		}
		return s.halt(fixerEndSpent, reason)
	}
	if err := s.chargeTurn(); err != nil {
		return shuttleengine.GateResult{}, err
	}
	findings := reason
	if path := s.run.failureRecordPath(); path != "" {
		findings += "\nlanding failure record: " + path
	}
	return shuttleengine.GateResult{Findings: findings}, nil
}

// haltReason words the halt a run that did not end fixed ends the row with.
// Each reason opens with LandingStopPrefix and ends with a way forward, and is one line.
func (o fixerOutcome) haltReason(row string) string {
	var cause, wayForward string
	switch o.end {
	case fixerEndSpent:
		cause = fmt.Sprintf("the fixer spent every attempt of %s and the landing gate still fails; log: %s", row, o.failureLog)
		wayForward = "fix the failure on the task branch, then `lyx loom resume`"
	case fixerEndEscalate:
		cause = fmt.Sprintf("the fixer of %s found a parent change that conflicts with a decision of the record; report: %s; conflict: %s", row, o.reportPath, singleLine(o.conflict))
		wayForward = "add the deciding decision with `lyx loom decision add` in the task worktree, then `lyx loom resume`"
	case fixerEndTimeout, fixerEndNoReport, fixerEndDirty:
		cause = fmt.Sprintf("the fixer session of %s ended without a usable turn (%s); session log: %s; report: %s", row, o.end, o.sessionLog, o.reportPath)
		wayForward = "fix or clean the task branch, then `lyx loom resume`"
	case fixerEndPushed:
		cause = fmt.Sprintf("the fixer session of %s pushed %s; session log: %s", row, o.pushedRef, o.sessionLog)
		wayForward = "check the pushed commits are the task's, then `lyx loom resume`"
	default:
		return ""
	}
	return LandingStopPrefix + cause + "; way forward: " + wayForward
}

// singleLine collapses every run of whitespace in text, line breaks included, to one space.
func singleLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

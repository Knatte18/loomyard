// fixerrun.go implements the merge fixer loop that Parent-Align and Finalize share for one landing row.
// It starts one cold session per row entry and holds it to a turn budget kept in the attempt record;
// it judges each turn by what the session's report and the landing gate say after it.

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

// LandingStopPrefix opens the reason of every halt a landing row hands back to the orch,
// so the hand-back check finds it.
const LandingStopPrefix = "landing stop: "

// EscalateStopPrefix opens the reason of a merge fixer's escalate halt when the arbiter judges the escalate first.
// Such a halt is no hand-back.
const EscalateStopPrefix = "landing escalate: "

// fixerGateName labels the fixer session's one gate entry.
const fixerGateName = "landing"

// fixerSeats is the seat runner the fixer session runs on; the seat engine satisfies it.
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
	// fixerEndTimeout is a turn that outran landing.fixer_timeout_min,
	// or a session that outran the bound of every turn the entry had left.
	fixerEndTimeout fixerEnd = "timeout"
	// fixerEndNoReport is a turn that left no report, or one that does not parse.
	fixerEndNoReport fixerEnd = "no-report"
	// fixerEndDirty is a turn that left the working tree dirty with no turn left to clean it.
	fixerEndDirty fixerEnd = "dirty"
	// fixerEndPushed is a session during which the task branch's remote ref was deleted or moved.
	// A move counts only when the new tip holds commits that the task head after the session lacks.
	fixerEndPushed fixerEnd = "pushed"
	// fixerEndUnjudged is a landing gate after a turn whose result judged no code:
	// a runner crash or a memory pressure stop.
	fixerEndUnjudged fixerEnd = "unjudged"
)

// fixerOutcome is how a fixer run ended and what the halt that follows names.
type fixerOutcome struct {
	end fixerEnd
	// reportPath is the report the session writes.
	reportPath string
	// session names the session by its engine session id, and by its run directory while that survives.
	session string
	// failureLog is the log the landing failure record names; set for a spent end.
	failureLog string
	// conflict is the report's conflict text; set for an escalate end.
	conflict string
	// remoteRef is the task branch's remote-tracking ref that moved; set for a pushed end.
	remoteRef string
	// remoteTip is remoteRef's tip after the session, "" for a deleted ref; set for a pushed end.
	remoteTip string
	// taskHead is the task head after the session; set for a pushed end whose ref still has a tip.
	taskHead string
	// dirty lists the uncommitted or untracked paths the last turn left; set for a dirty end.
	dirty []string
	// unjudged is the landing gate's halt reason naming the runs that judged no code; set for an unjudged end.
	unjudged string
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

// run spends the turns rec has left on one fixer session in mode.
// It returns how the run ended and the attempt record as it stands then.
// cap is the row's turn cap, and tips are the tip the parent's changes are read from and the merged tip.
// It is never called with no turn left, since a row takes its exhausted-entry path first.
// A turn is charged into the record when it starts, so a turn that never returns to the gate is still spent.
// The first turn's charge is taken back when the session fails to start with an error marked transient.
// An escalate report takes its own turn back.
// A deletion of the task branch's remote ref during the session ends the run pushed.
// A move of that ref does too, except to a commit the task head holds after the session.
// Such a commit is in the task worktree: on a fixed end the landing gate passed over it,
// and any other end halts the row on its own reason.
// That holds for a commit older than the session and for one the session made and the branch's pusher sent.
func (f fixerRun) run(ctx context.Context, mode FixerMode, rec FixerAttempts, cap int, tips [2]string) (fixerOutcome, FixerAttempts, error) {
	reportPath := filepath.Join(f.deps.ScratchDir, f.row+"-fixer-report.md")
	attemptsPath := FixerAttemptsPath(f.deps.RecordsDir, f.row)
	turnsLeft := cap - rec.Spent
	turnLimit := time.Duration(f.deps.Config.FixerTimeoutMin) * time.Minute

	table, err := FixerTable(FixerInputs{
		StencilsDir:        f.deps.StencilsDir,
		Mode:               mode,
		DecisionRecordPath: f.deps.DecisionRecordPath,
		MergeRecordPath:    f.deps.MergeRecordPath,
		PlanDir:            f.deps.PlanDir,
		FailureRecordPath:  f.failureRecordPath(),
		FixReviewPath:      rec.FixReview,
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
	if err := archiveStaleReport(reportPath, f.gate.now(), f.row, "fixer"); err != nil {
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
	if cause := startFailure(result, runErr); cause != nil {
		if err := WriteFixerAttempts(attemptsPath, original); err != nil {
			return fixerOutcome{}, session.rec, err
		}
		return fixerOutcome{}, original, shedengine.MarkTransient(shedengine.TransientAgentStart, fmt.Errorf("landingshed: %s: the fixer session did not start: %w", f.row, cause))
	}
	if runErr != nil {
		return fixerOutcome{}, session.rec, fmt.Errorf("landingshed: %s: run the fixer session: %w", f.row, runErr)
	}

	outcome := session.outcome
	outcome.session = sessionRef(result.Chair)
	switch {
	case session.end != "":
		outcome.end = session.end
	case session.decided != nil && session.decided.Passed:
		outcome.end = fixerEndFixed
	case result.Chair.Outcome == shuttleengine.OutcomeTimeout:
		outcome.end = fixerEndTimeout
	default:
		outcome.end = fixerEndNoReport
	}

	remoteAfter, err := f.deps.TaskRemoteTip()
	if err != nil {
		return fixerOutcome{}, session.rec, fmt.Errorf("landingshed: %s: read the task branch's remote tip: %w", f.row, err)
	}
	if remoteAfter != remoteBefore {
		remoteRef := "origin/" + f.deps.TaskBranch
		var taskHead string
		held := false
		if remoteAfter != "" {
			if taskHead, err = f.deps.TaskHead(); err != nil {
				return fixerOutcome{}, session.rec, fmt.Errorf("landingshed: %s: read the task head: %w", f.row, err)
			}
			if held, err = f.deps.IsAncestor(remoteAfter, taskHead); err != nil {
				return fixerOutcome{}, session.rec, fmt.Errorf("landingshed: %s: test whether the task head holds %s at %s: %w", f.row, remoteRef, remoteAfter, err)
			}
		}
		if held {
			logger.Info("landingshed: the task branch's remote ref moved during the fixer session to a commit the task head holds", "row", f.row, "ref", remoteRef, "tip", remoteAfter, "taskHead", taskHead)
		} else {
			outcome.end = fixerEndPushed
			outcome.remoteRef, outcome.remoteTip, outcome.taskHead = remoteRef, remoteAfter, taskHead
		}
	}
	logger.Info("landingshed: fixer run ended", "row", f.row, "mode", string(mode), "end", string(outcome.end), "spent", session.rec.Spent, "cap", cap)
	return outcome, session.rec, nil
}

// failureRecordPath returns the landing failure record's path when a failed gate left one.
// It returns "" before any gate has failed.
func (f fixerRun) failureRecordPath() string {
	if _, err := os.Stat(f.gate.paths.LandingFailure); err != nil {
		return ""
	}
	return f.gate.paths.LandingFailure
}

// archiveStaleReport renames a report an earlier session of row left at reportPath, stamped with now.
// The rename keeps a new session from starting beside a finished report.
// The what argument names the session in the error.
func archiveStaleReport(reportPath string, now time.Time, row, what string) error {
	stamp := now.UTC().Format("20060102T150405Z")
	archived := strings.TrimSuffix(reportPath, ".md") + "-" + stamp + ".md"
	if err := os.Rename(reportPath, archived); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("landingshed: %s: archive the stale %s report: %w", row, what, err)
	}
	return nil
}

// sessionRef names a finished session for a halt reason by its engine and session id.
// Those outlive the run directory, which the session runner removes after a done outcome;
// the run directory is named too while it still exists, as after a timeout.
func sessionRef(chair shuttleengine.Result) string {
	var parts []string
	if chair.SessionID != "" {
		parts = append(parts, strings.TrimSpace(chair.Engine+" session "+chair.SessionID))
	}
	if chair.RunDir != "" {
		if _, err := os.Stat(chair.RunDir); err == nil {
			parts = append(parts, "run dir "+chair.RunDir)
		}
	}
	if len(parts) == 0 {
		return "none recorded"
	}
	return strings.Join(parts, ", ")
}

// startFailure returns why a seat run's chair session did not start, or nil when it started.
func startFailure(result seatengine.Result, runErr error) error {
	switch {
	case runErr != nil && errors.Is(runErr, shuttleengine.ErrNotStarted):
		return runErr
	case runErr == nil && result.Chair.NotStarted:
		return errors.New(result.Chair.StartError)
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

	// end is the halting end the closure decided, "" while the run is fixed or undecided.
	end fixerEnd
	// decided holds the closure's last terminal answer.
	// A second evaluation of a finished run answers the same without charging again.
	// A passed answer is what makes the run fixed; nil means no turn has ended the run yet.
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

// check is the gate run after every fixer turn.
// It judges the turn's duration, then the tree, then the report, and when the report says fixed, the landing gate.
// A dirty tree with a turn left charges that turn and re-prompts naming the paths,
// so the fixer commits or deletes what it left; with none left, the run ends dirty.
// A failing gate with a turn left charges that turn and re-prompts; with none left, the run ends spent.
// A gate whose result judged no code ends the run as unjudged, whatever turns are left;
// no turn mends such a gate.
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
	gate := s.run.gate
	if gate.now().Sub(s.turnStart) > s.turnLimit {
		return s.halt(fixerEndTimeout, "the fixer's turn ran past "+keyName(fixerTimeoutMinKey))
	}
	const point = "after the fixer's turn"
	dirty, err := gate.dirtyPaths(s.run.row, point)
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if len(dirty) > 0 {
		if s.rec.Spent >= s.cap {
			s.outcome.dirty = dirty
			return s.halt(fixerEndDirty, dirtyReason(point, dirty))
		}
		if err := s.chargeTurn(); err != nil {
			return shuttleengine.GateResult{}, err
		}
		return shuttleengine.GateResult{Findings: verifytree.UncommittedFindings(dirty)}, nil
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

	reason, unjudged, err := gate.check(s.ctx, s.run.row, s.run.deps.ParentBranch)
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if unjudged {
		s.outcome.unjudged = reason
		return s.halt(fixerEndUnjudged, reason)
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

// haltReason returns the reason the row halts with when its fixer run did not end fixed.
// It returns the empty string for a fixed end.
// Each reason is one line.
// Every reason except a tapestry, jacquard, darn or sampler escalate opens with LandingStopPrefix.
// Each of those reasons also ends with a way forward.
// With escalateJudgedFirst true, an escalate end opens with EscalateStopPrefix instead;
// it names `lym loom decision add` as its way forward,
// and a board note with a fix on the task branch once the task's scope is closed.
// With escalateJudgedFirst false, a pawl run's arbiter has no decision verb to settle it with;
// so the way forward is to settle the conflict on the task branch.
// An unjudged end's reason is the landing gate's own;
// it already opens with LandingStopPrefix and names its way forward.
func (o fixerOutcome) haltReason(row string, escalateJudgedFirst bool) string {
	prefix := LandingStopPrefix
	var cause, wayForward string
	switch o.end {
	case fixerEndUnjudged:
		return o.unjudged
	case fixerEndSpent:
		cause = fmt.Sprintf("the fixer spent every attempt of %s and the landing gate still fails; log: %s", row, o.failureLog)
		wayForward = "fix the failure on the task branch, then `lym loom resume`"
	case fixerEndEscalate:
		cause = fmt.Sprintf("the fixer of %s found a parent change that conflicts with a decision of the record; report: %s; conflict: %s", row, o.reportPath, singleLine(o.conflict))
		if escalateJudgedFirst {
			prefix = EscalateStopPrefix
			wayForward = "record the deciding decision with `lym loom decision add` in the task worktree, then `lym loom resume`; once the task's scope is closed, file the call as a board note for a new task with `lym board upsert` and kind note, settle the conflict on the task branch so the record's decisions hold, then `lym loom resume`"
		} else {
			wayForward = "settle the conflict on the task branch, then `lym loom resume`"
		}
	case fixerEndTimeout, fixerEndNoReport:
		cause = fmt.Sprintf("the fixer session of %s ended without a usable turn (%s); session: %s; report: %s", row, o.end, o.session, o.reportPath)
		wayForward = "fix or clean the task branch, then `lym loom resume`"
	case fixerEndDirty:
		cause = fmt.Sprintf("the fixer session of %s left uncommitted or untracked paths in the task worktree after its last turn: %s; session: %s; report: %s", row, listShown(o.dirty), o.session, o.reportPath)
		wayForward = "commit or remove them on the task branch, then `lym loom resume`"
	case fixerEndPushed:
		if o.remoteTip == "" {
			cause = fmt.Sprintf("the remote ref %s was deleted during the fixer session of %s; session: %s", o.remoteRef, row, o.session)
			wayForward = "check why the task branch is gone from origin, then `lym loom resume`"
			break
		}
		cause = fmt.Sprintf("the remote ref %s moved during the fixer session of %s to %s, which holds commits the task head %s lacks after the session; session: %s", o.remoteRef, row, o.remoteTip, o.taskHead, o.session)
		wayForward = fmt.Sprintf("check the commits %s..%s are the task's, then `lym loom resume`", o.taskHead, o.remoteTip)
	default:
		return ""
	}
	return prefix + cause + "; way forward: " + wayForward
}

// singleLine returns text with every run of whitespace, line breaks included, replaced by one space;
// it also drops leading and trailing whitespace.
func singleLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

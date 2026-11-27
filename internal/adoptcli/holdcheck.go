// holdcheck.go runs the adopt check over the subjects of this machine's Bolt hold and settles its end.
// The end keeps the try counts in the hold, clears it on green, and brings each subject's worktree up.

package adoptcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

// The states of a hold check's result, beside the green and failed states of a check record.
const (
	// holdStateNoHold is the state of a check that found no hold of this machine, so it checked nothing.
	holdStateNoHold = "no_hold"
	// holdStateAlreadyRunning is the state of a check that stopped because another check of a subject holds its lock.
	holdStateAlreadyRunning = "already_running"
	// holdStateLost is the state of a check whose hold another machine took, or that cleared, before the check's end.
	holdStateLost = "hold_lost"
	// holdStateNotCleared is the state of a green check whose end could not finish, so the hold stands.
	holdStateNotCleared = "not_cleared"
)

// holdCheckDeps holds the seams of one hold check.
// PullBolt brings the local Bolt level with origin.
// Hold reads the hold standing in the local Bolt for the subject first:
// this machine's hold that covers first, else this machine's first hold, else another machine's first hold.
// It reports whether that hold reads as this machine's, and found false when no hold stands.
// Check runs the adopt check of one subject, reporting whether it ran, with rerun the command that checks the hold again.
// Fix runs the fix loop of a subject's failed check, given the subject's tries from the hold.
// It returns the task filed or standing, its lines, and the refusal the hold write met, nil when none.
// Green runs a subject's green end and returns its lines.
// ClearHold removes this machine's hold, reporting whether one stood.
// DropBoltHold lifts this machine's bolt-hold entries from the pause record.
// BeamUp brings the subject's worktree up; a nil BeamUp brings nothing up.
// Notice posts a line about a subject to its nearest live orch, and LimitNotice posts it as the subject's dobby notice.
type holdCheckDeps struct {
	PullBolt     func() error
	Hold         func(first string) (hold boardengine.Hold, own, found bool, err error)
	Check        func(ctx context.Context, subject, rerun string) (record CheckRecord, ran bool, err error)
	Fix          func(subject string, tries int, record CheckRecord, rerun string) (task string, lines []string, lost *boardengine.HoldRefusal)
	Green        func(subject, rerun string) []string
	ClearHold    func() (bool, error)
	DropBoltHold func() error
	BeamUp       func(subject string) error
	Notice       func(subject, line string) error
	LimitNotice  func(subject, line string) error
}

// holdCheckResult is what one hold check did.
// State is green, failed, no_hold, already_running, hold_lost or not_cleared.
// Green reports whether every subject was green and the hold was cleared.
// Ran reports whether every subject's check ran.
// States holds each checked subject's check state.
// FixTask is the fix task filed or standing, empty when none.
// HoldLost reports that this machine's hold no longer stood at the check's end, so nothing was brought up.
// Lines holds every line posted.
type holdCheckResult struct {
	State    string
	Green    bool
	Ran      bool
	States   map[string]string
	FixTask  string
	HoldLost bool
	Lines    []string
}

// holdCheckRun is one hold check in flight.
type holdCheckRun struct {
	deps   holdCheckDeps
	result holdCheckResult
	// names holds the hold's subjects in the hold's order, and rerun the command that checks them again.
	names []string
	rerun string
}

// say records line in the result and posts it to the orch of subject.
func (r *holdCheckRun) say(subject, line string) {
	r.result.Lines = append(r.result.Lines, line)
	if err := r.deps.Notice(subject, line); err != nil {
		logger.Warn("adoptcli: the adopt check notice was not posted", "subject", subject, "line", line, "error", err)
	}
}

// sayLimit records line in the result and posts it as the dobby notice of subject.
func (r *holdCheckRun) sayLimit(subject, line string) {
	r.result.Lines = append(r.result.Lines, line)
	if err := r.deps.LimitNotice(subject, line); err != nil {
		logger.Warn("adoptcli: the adopt check's dobby notice was not posted", "subject", subject, "line", line, "error", err)
	}
}

// runHoldCheck checks every subject of this machine's hold, first first, and settles the end.
// A hold of this machine that stands is the only thing it checks for:
// with none, or with another machine's, it checks nothing and returns the result no_hold with no error.
// A first outside the hold's subjects is refused, naming the hold's subjects.
// A check that did not run because its lock is held ends the call with Ran false.
// With every subject green it reads the hold once more.
// A take-hold during the checks then leaves the hold and the fix task alone.
// It then removes each subject's fix task and clears the hold.
// It brings each subject's worktree up and lifts the bolt-hold entries.
// With a subject failed it files the fix task of the first failing subject in the hold's order.
// It keeps that subject's tries in the hold.
// It brings each subject's worktree up so the task's dobby starts it.
func runHoldCheck(ctx context.Context, deps holdCheckDeps, first string) (holdCheckResult, error) {
	run := &holdCheckRun{deps: deps, result: holdCheckResult{States: map[string]string{}}}
	if err := deps.PullBolt(); err != nil {
		return run.result, fmt.Errorf("pull Bolt before the adopt check of %s: %w; way forward: re-run lym adopt %s once origin answers", first, err, first)
	}
	hold, own, found, err := deps.Hold(first)
	if err != nil {
		return run.result, fmt.Errorf("read the hold before the adopt check of %s: %w; way forward: fix the cause it names, then run lym adopt %s", first, err, first)
	}
	if !found {
		run.result.State = holdStateNoHold
		run.say(first, fmt.Sprintf("no hold of this machine stands for %s, so nothing was checked; way forward: run lym adopt %s, which writes the hold and checks", first, first))
		return run.result, nil
	}
	if !own {
		run.result.State = holdStateNoHold
		run.say(first, fmt.Sprintf("no hold of this machine stands for %s, so nothing was checked: machine %s holds the hub; way forward: wait for its adopt to finish, then run lym adopt %s", first, hold.Machine, first))
		return run.result, nil
	}

	for _, subject := range hold.Subjects {
		run.names = append(run.names, subject.Subject)
	}
	run.rerun = "lym adopt " + strings.Join(run.names, " ")
	if !slices.Contains(run.names, first) {
		return run.result, fmt.Errorf("the adopt check of %s is outside this machine's hold, which covers %s; way forward: run %s", first, strings.Join(run.names, ", "), run.rerun)
	}

	order := append([]string{first}, slices.DeleteFunc(slices.Clone(run.names), func(name string) bool { return name == first })...)
	records := map[string]CheckRecord{}
	for _, subject := range order {
		record, ran, err := deps.Check(ctx, subject, run.rerun)
		if err != nil {
			return run.result, err
		}
		if !ran {
			run.result.State = holdStateAlreadyRunning
			run.say(subject, fmt.Sprintf("the adopt check of %s is already running and posts its own end; way forward: wait for it, then run %s if the hold still stands", subject, run.rerun))
			return run.result, nil
		}
		records[subject] = record
		run.result.States[subject] = record.State
	}
	run.result.Ran = true

	var failing string
	for _, name := range run.names {
		if records[name].State != checkGreen {
			failing = name
			break
		}
	}
	if failing == "" {
		return run.endGreen(first), nil
	}
	index := slices.IndexFunc(hold.Subjects, func(subject boardengine.HoldSubject) bool { return subject.Subject == failing })
	return run.endFailed(failing, hold.Subjects[index].Tries, records[failing]), nil
}

// endGreen settles a check whose every subject is green.
// It pulls Bolt and reads the hold again first.
// It ends without a later seam when either fails or when this machine's hold no longer stands.
func (r *holdCheckRun) endGreen(first string) holdCheckResult {
	notCleared := func(cause string) holdCheckResult {
		r.result.State = holdStateNotCleared
		r.say(first, fmt.Sprintf("the adopt check of %s is green, but its end did not finish: %s; way forward: re-run %s, whose check goes green again and clears the hold", first, cause, r.rerun))
		return r.result
	}
	if err := r.deps.PullBolt(); err != nil {
		return notCleared(fmt.Sprintf("Bolt was not pulled (%v)", err))
	}
	hold, own, found, err := r.deps.Hold(first)
	if err != nil {
		return notCleared(fmt.Sprintf("the hold was not read (%v)", err))
	}
	if !found || !own {
		holder := ""
		if found {
			holder = hold.Machine
		}
		return r.lost(first, holder)
	}

	for _, subject := range r.names {
		for _, line := range r.deps.Green(subject, r.rerun) {
			r.say(subject, line)
		}
	}
	cleared, err := r.deps.ClearHold()
	var refusal *boardengine.HoldRefusal
	switch {
	case errors.As(err, &refusal) && !refusal.OwnStands:
		return r.lost(first, refusal.Machine)
	case err != nil:
		return notCleared(fmt.Sprintf("the hold was not cleared (%v)", err))
	case !cleared:
		return r.lost(first, "")
	}

	r.bringUp()
	if err := r.deps.DropBoltHold(); err != nil {
		r.say(first, fmt.Sprintf("the bolt-hold entries of the runs were not lifted (%v); the worktree's dobby lifts them at its next tick, or run lym beam start %s when none runs", err, first))
	}
	r.result.State, r.result.Green = checkGreen, true
	return r.result
}

// lost ends the check with the hold lost: holder is the machine that took it, empty when it cleared.
// Nothing is brought up, so a fix task another machine's take-hold now owns stays standing.
func (r *holdCheckRun) lost(subject, holder string) holdCheckResult {
	r.result.State, r.result.HoldLost = holdStateLost, true
	if holder == "" {
		r.say(subject, fmt.Sprintf("the hold of this machine no longer stands: it cleared meanwhile, so nothing was brought up here; way forward: run %s if the runs do not go on", r.rerun))
		return r.result
	}
	r.say(subject, fmt.Sprintf("the hold of this machine no longer stands: machine %s took it, so nothing was brought up here; way forward: wait for its adopt to finish, then run %s if needed", holder, r.rerun))
	return r.result
}

// bringUp brings each subject's worktree up, adding a line naming `lym beam start` for each that fails.
func (r *holdCheckRun) bringUp() {
	if r.deps.BeamUp == nil {
		return
	}
	for _, subject := range r.names {
		if err := r.deps.BeamUp(subject); err != nil {
			r.say(subject, fmt.Sprintf("the beam steps of %s failed (%v); way forward: fix the cause it names, then run lym beam start %s", subject, err, subject))
		}
	}
}

// endFailed settles a check with a failed subject.
// failing is the first failed subject in the hold's order, and tries is its count recorded in the hold.
// A subject that failed before any tier ended gets no fix task.
// A hold write that lost the hold to another machine brings nothing up.
func (r *holdCheckRun) endFailed(failing string, tries int, record CheckRecord) holdCheckResult {
	r.result.State = checkFailed
	if record.FailedTier == nil {
		r.say(failing, fmt.Sprintf("the adopt check of %s failed before any tier ended, so no fix task was filed; way forward: fix what its notice names, then run %s", failing, r.rerun))
		return r.result
	}
	task, lines, lost := r.deps.Fix(failing, tries, record, r.rerun)
	r.result.FixTask = task
	for _, line := range lines {
		if strings.HasPrefix(line, fixLimitLineOpening) {
			r.sayLimit(failing, line)
			continue
		}
		r.say(failing, line)
	}
	if lost != nil && !lost.OwnStands {
		r.result.State, r.result.HoldLost = holdStateLost, true
		return r.result
	}
	r.bringUp()
	return r.result
}

// hubHoldCheckDeps fills holdCheckDeps over the hub of prime.
// A non-nil terminal also receives the gate's output, and noOrch brings each worktree up without its orch.
// Its BeamUp refuses under a test binary, which must start no session or process.
func hubHoldCheckDeps(prime *lymcwd.Location, terminal io.Writer, noOrch bool) (holdCheckDeps, error) {
	boardDir := fabriclayout.BoardDir(prime.HubPath)
	geometry, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		return holdCheckDeps{}, err
	}
	return holdCheckDeps{
		PullBolt: func() error { return boardengine.PullBolt(boardDir) },
		Hold:     func(first string) (boardengine.Hold, bool, bool, error) { return readOwnHold(boardDir, first) },
		Check: func(ctx context.Context, subject, rerun string) (CheckRecord, bool, error) {
			deps, err := hubCheckDeps(prime, subject, terminal)
			if err != nil {
				return CheckRecord{}, false, fmt.Errorf("%w, then re-run %s", err, rerun)
			}
			deps.Rerun = rerun
			return runCheck(ctx, deps)
		},
		Fix: func(subject string, tries int, record CheckRecord, rerun string) (string, []string, *boardengine.HoldRefusal) {
			fix, err := hubFixDeps(prime, subject)
			if err != nil {
				return "", []string{fmt.Sprintf("the fix loop of the adopt check of %s did not run (%v); way forward: fix the cause it names, then re-run %s, the hold standing meanwhile", subject, err, rerun)}, nil
			}
			fix.Tries, fix.Rerun = tries, rerun
			return failedEnd(fix, record)
		},
		Green: func(subject, rerun string) []string {
			green, err := hubGreenDeps(prime, subject, rerun)
			if err != nil {
				return []string{fmt.Sprintf("the green end of the adopt check of %s did not run (%v); way forward: fix the cause it names, then re-run %s", subject, err, rerun)}
			}
			return greenEnd(green)
		},
		ClearHold: func() (bool, error) {
			here, err := machinename.ReadIdentity()
			if err != nil {
				return false, err
			}
			return boardengine.ClearHold(boardDir, here)
		},
		DropBoltHold: func() error { return dropBoltHoldEntries(prime, geometry.RecordPath) },
		BeamUp: func(subject string) error {
			if testing.Testing() {
				return orchcli.ErrBeamUpUnderTest
			}
			worktree := subject
			if subject == boardengine.RootSubject {
				worktree = prime.WorktreeName
			}
			location, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, worktree))
			if err != nil {
				return err
			}
			here, err := machinename.Name()
			if err != nil {
				return err
			}
			_, err = orchcli.BeamUp(location, orchcli.BeamUpOptions{Here: here, SkipOrch: noOrch})
			return err
		},
		Notice: func(subject, line string) error {
			worktree, err := resolveSubjectWorktree(prime, subject)
			if err != nil {
				return err
			}
			return requirePosted(orchcli.NotifyFrom(worktree, checkProgram, line))
		},
		LimitNotice: func(subject, line string) error {
			worktree, err := resolveSubjectWorktree(prime, subject)
			if err != nil {
				return err
			}
			slug := subject
			if subject == boardengine.RootSubject {
				slug = ""
			}
			return requirePosted(orchcli.NotifyFromScoped(worktree, slug, orchcli.DobbyProgram, line))
		},
	}, nil
}

// requirePosted turns a notice that no live orch strand received into an error.
func requirePosted(posted bool, err error) error {
	if err == nil && !posted {
		return errors.New("no live orch strand to post to")
	}
	return err
}

// readOwnHold reads the hold the local Bolt records for the subject first, as holdCheckDeps.Hold does.
func readOwnHold(boardDir, first string) (hold boardengine.Hold, own, found bool, err error) {
	owners, err := boardengine.ReadOwners(boardDir)
	if err != nil {
		return boardengine.Hold{}, false, false, err
	}
	here, err := machinename.ReadIdentity()
	if err != nil {
		return boardengine.Hold{}, false, false, err
	}
	var ownHolds, otherHolds []boardengine.Hold
	for _, standing := range owners.Holds {
		if owners.Registry.ReadsAsHere(standing.Machine, here) {
			ownHolds = append(ownHolds, standing)
		} else {
			otherHolds = append(otherHolds, standing)
		}
	}
	for _, standing := range ownHolds {
		if slices.ContainsFunc(standing.Subjects, func(subject boardengine.HoldSubject) bool { return subject.Subject == first }) {
			return standing, true, true, nil
		}
	}
	switch {
	case len(ownHolds) > 0:
		return ownHolds[0], true, true, nil
	case len(otherHolds) > 0:
		return otherHolds[0], false, true, nil
	}
	return boardengine.Hold{}, false, false, nil
}

// dropBoltHoldEntries lifts every bolt-hold entry of the pause record at recordPath.
// It reads the record again after the lift and lifts once more,
// so an entry a dobby tick wrote from the hold it read before the clear does not outlive it.
func dropBoltHoldEntries(prime *lymcwd.Location, recordPath string) error {
	if err := dropBoltHoldEntriesOnce(prime, recordPath); err != nil {
		return err
	}
	return dropBoltHoldEntriesOnce(prime, recordPath)
}

// dropBoltHoldEntriesOnce lifts the bolt-hold entries the record at recordPath holds now.
// Only a bolt-hold entry of a slug scope is lifted, so an operator's entry stands.
func dropBoltHoldEntriesOnce(prime *lymcwd.Location, recordPath string) error {
	record, err := pauserecord.Read(recordPath)
	if err != nil {
		return err
	}
	var scope []string
	for _, entry := range record.Live {
		if entry.Caller == pauserecord.BoltHoldCaller && entry.Scope != pauserecord.ScopeHub {
			scope = append(scope, entry.Scope)
		}
	}
	if len(scope) == 0 {
		return nil
	}
	deps, _, err := tendercli.HubDeps(prime, os.Getenv(agentname.StrandNameEnv))
	if err != nil {
		return err
	}
	_, err = tenderengine.SyncBoltHold(deps, tenderengine.BoltHoldRequest{Scope: scope})
	return err
}

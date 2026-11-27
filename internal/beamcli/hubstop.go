// hubstop.go implements `lym beam stop --all`: the detached re-start of the caller's process,
// and the sequence the detached process runs to take the hub down on this machine.

package beamcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hibernatecli"
	"github.com/Knatte18/loomyard/internal/hibernateengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/tenderengine"
	"github.com/spf13/cobra"
)

const (
	// stopPauseCaller is the caller label of the hub pause entry the hub stop adds.
	stopPauseCaller = "beam-stop"
	// stopDetachedFlag is the hidden flag the hub stop's detached process runs under.
	stopDetachedFlag = "detached"
)

// detachedStop is the envelope of the caller's `beam stop --all`: the pid of the detached process that runs the stop.
type detachedStop struct {
	PID int `json:"detached_pid"`
}

// hubStopReport is the envelope of the detached `beam stop --all`: what each step of the sequence did.
type hubStopReport struct {
	// PauseOwned reports whether the stop added its own hub pause entry; false when a standing hub entry covered it.
	PauseOwned bool `json:"pause_owned"`
	// Unconfirmed names what still withheld the pause's confirmation when the stop went on without it.
	Unconfirmed string `json:"unconfirmed,omitempty"`
	// Pairs holds the down of each task pair's processes and reed session.
	Pairs []reedcli.DownReport `json:"pairs"`
	// Orchs holds the bring-down of the prime's and each beam's orch and dobby.
	Orchs []orchcli.BeamDownReport `json:"orchs"`
	// Sessions holds the down of each beam's reed session, then the prime's.
	Sessions []reedcli.DownReport `json:"sessions"`
	// Reaped holds each run process no run owned that the stop ended.
	Reaped []tenderengine.ReapedProcess `json:"reaped"`
	// PauseLifted reports whether the stop lifted the pause entry it added.
	PauseLifted bool `json:"pause_lifted"`
	// Failures names each step that failed; the sequence goes on past every one.
	Failures []string `json:"failures"`
}

// hubStopSteps are the effects of the hub stop's sequence, one per kind of step, so a test drives its order over fakes.
type hubStopSteps struct {
	pause       func() (hibernateengine.HubPause, error)
	worktrees   func() (beams, tasks []*lymcwd.Location, err error)
	downSession func(location *lymcwd.Location) (reedcli.DownReport, error)
	beamDown    func(location *lymcwd.Location) (orchcli.BeamDownReport, error)
	reap        func() (tenderengine.ReapReport, error)
	lift        func(ref tenderengine.EntryRef) error
}

// stopAll runs `beam stop --all` and prints its envelope.
// The caller's process re-starts the verb detached in the prime and prints the detached process's pid;
// the detached process runs the hub stop's sequence and prints its report.
// A slug beside --all is refused before anything is stopped.
func stopAll(cmd *cobra.Command, slug string, detached bool) error {
	if slug != "" {
		return finishVerb[detachedStop](cmd, nil, fmt.Errorf("lym beam stop: a slug and --all are exclusive; way forward: drop the slug to stop the whole hub on this machine, or drop --all to stop %s alone", slug))
	}
	location, err := actingLocation(cmd, "stop")
	if err != nil {
		return finishVerb[detachedStop](cmd, nil, err)
	}
	target, err := resolveBeamTarget(location, rootSlug)
	if err != nil {
		return finishVerb[detachedStop](cmd, nil, err)
	}
	prime := target.Location
	if !detached {
		pid, err := spawnDetachedStop(prime)
		if err != nil {
			return finishVerb[detachedStop](cmd, nil, fmt.Errorf("lym beam stop --all: start the detached stop: %w; way forward: fix the cause it names, then run `lym beam stop --all` again", err))
		}
		return finishVerb(cmd, &detachedStop{PID: pid}, nil)
	}
	report := runHubStop(prime, hubStopStepsFor(cmd.Context(), prime, os.Getenv(agentname.StrandNameEnv)))
	logger.Info("beam: the hub stop ended", "pause_owned", report.PauseOwned, "unconfirmed", report.Unconfirmed,
		"pause_lifted", report.PauseLifted, "failures", report.Failures)
	if len(report.Failures) > 0 {
		return finishVerb(cmd, &report, fmt.Errorf("lym beam stop --all: steps failed: %s; way forward: fix the cause each names, then run `lym beam stop --all` again", strings.Join(report.Failures, "; ")))
	}
	return finishVerb(cmd, &report, nil)
}

// detachedStopCommand builds the command that runs `lym beam stop --all --detached` from exe, working in prime's anchor.
// The detached process resolves its location strictly from its working directory, so it starts at the anchor.
func detachedStopCommand(exe string, prime *lymcwd.Location) *exec.Cmd {
	cmd := exec.Command(exe, "beam", "stop", "--all", "--"+stopDetachedFlag)
	cmd.Dir = prime.AnchorPath()
	return cmd
}

// spawnDetachedStop starts `lym beam stop --all` detached in a session of its own, working in prime's anchor.
// It returns the process's pid.
// The process outlives its caller and every reed session, the caller's own included.
func spawnDetachedStop(prime *lymcwd.Location) (int, error) {
	exe, err := selfexec.LymBinary()
	if err != nil {
		return 0, err
	}
	cmd := detachedStopCommand(exe, prime)
	proc.Detach(cmd)
	// Leave stdin/stdout/stderr nil so no handles are inherited from the parent.
	if err := cmd.Start(); err != nil {
		logger.Warn("beam: the detached hub stop did not start", "exe", exe, "args", cmd.Args[1:], "dir", cmd.Dir, "error", err)
		return 0, err
	}
	proc.Reap(cmd)
	logger.Info("beam: spawned the detached hub stop", "exe", exe, "args", cmd.Args[1:], "dir", cmd.Dir, "pid", cmd.Process.Pid)
	return cmd.Process.Pid, nil
}

// hubStopStepsFor wires the hub stop's steps over the hub of prime.
// requester is the strand the stop was asked from; the pause's confirmation leaves its session out when it is an orch's.
func hubStopStepsFor(ctx context.Context, prime *lymcwd.Location, requester string) hubStopSteps {
	tender := func() (tenderengine.Deps, error) {
		deps, _, err := tendercli.HubDeps(prime, stopPauseCaller)
		return deps, err
	}
	return hubStopSteps{
		pause: func() (hibernateengine.HubPause, error) {
			return hibernatecli.PauseHub(prime, stopPauseCaller, requester)
		},
		worktrees: func() ([]*lymcwd.Location, []*lymcwd.Location, error) { return hubStopWorktrees(prime) },
		downSession: func(location *lymcwd.Location) (reedcli.DownReport, error) {
			return reedcli.DownWorktree(ctx, location)
		},
		beamDown: orchcli.BeamDown,
		reap: func() (tenderengine.ReapReport, error) {
			deps, err := tender()
			if err != nil {
				return tenderengine.ReapReport{}, err
			}
			return tenderengine.Reap(deps)
		},
		lift: func(ref tenderengine.EntryRef) error {
			deps, err := tender()
			if err != nil {
				return err
			}
			_, err = tenderengine.Lift(deps, ref)
			return err
		},
	}
}

// hubStopWorktrees lists the hub's beams and task pairs on this machine, whoever owns each.
// A beam is a pair whose origin record carries the beam mark;
// a task pair is every other pair whose origin record lacks it or that has no _lym junction to read one through.
// The prime, a prunable worktree and one still being created are in neither list.
// A pair whose junction or origin record cannot be read is in neither list either:
// the returned error names it, and the beams and tasks read so far are returned beside it.
func hubStopWorktrees(prime *lymcwd.Location) (beams, tasks []*lymcwd.Location, err error) {
	orchs, err := orchcli.HubOrchs(prime)
	if err != nil {
		return nil, nil, err
	}
	beams = orchs[1:]
	worktrees, err := fabricengine.List(prime.WorktreePath())
	if err != nil {
		return nil, nil, err
	}
	var unreadable []error
	for _, worktree := range worktrees {
		if worktree.Main || worktree.Prunable || fabriclayout.InWorktreeStaging(worktree.Path) {
			continue
		}
		pair, err := lymcwd.ResolveWorktree(worktree.Path)
		if err != nil {
			logger.Warn("beam: the hub stop skips a worktree that does not resolve", "worktree", worktree.Path, "error", err)
			continue
		}
		wired, err := fabricengine.LymJunctionWiredHere(pair)
		if err != nil {
			logger.Warn("beam: the hub stop skips a pair whose _lym junction cannot be read", "worktree", pair.WorktreeName, "error", err)
			unreadable = append(unreadable, fmt.Errorf("read the _lym junction of %s: %w", pair.WorktreeName, err))
			continue
		}
		if !wired {
			// No _lym link means no origin record to read through; the pair is not a beam in the hub's orch list.
			tasks = append(tasks, pair)
			continue
		}
		origin, found, err := fabricengine.ReadOrigin(pair)
		if err != nil {
			logger.Warn("beam: the hub stop skips a pair whose origin record cannot be read", "worktree", pair.WorktreeName, "error", err)
			unreadable = append(unreadable, fmt.Errorf("read the origin record of %s: %w", pair.WorktreeName, err))
			continue
		}
		if !found || !origin.Beam {
			tasks = append(tasks, pair)
		}
	}
	return beams, tasks, errors.Join(unreadable...)
}

// runHubStop takes the hub of prime down on this machine, whoever owns each worktree, and returns what it did.
// It pauses the hub and confirms the pause.
// It downs each task pair's processes and reed session.
// It takes down the prime's and each beam's orch and dobby, and downs the beams' reed sessions.
// It then ends the run processes no run owns, lifts the pause entry it added, and downs the prime's reed session last.
// A hub entry that stood before it covers the stop and stays.
// It goes on past an unconfirmed pause, naming what withheld it, and past every failed step, naming each.
func runHubStop(prime *lymcwd.Location, steps hubStopSteps) hubStopReport {
	report := hubStopReport{
		Pairs: []reedcli.DownReport{}, Orchs: []orchcli.BeamDownReport{}, Sessions: []reedcli.DownReport{},
		Reaped: []tenderengine.ReapedProcess{}, Failures: []string{},
	}
	failed := func(step string, err error) {
		logger.Warn("beam: a step of the hub stop failed; the stop goes on", "step", step, "error", err)
		report.Failures = append(report.Failures, step+": "+err.Error())
	}
	downSession := func(location *lymcwd.Location, into *[]reedcli.DownReport) {
		down, err := steps.downSession(location)
		if err != nil {
			failed("down the reed session of "+location.WorktreeName, err)
			return
		}
		*into = append(*into, down)
	}

	paused, err := steps.pause()
	if err != nil {
		failed("pause the hub", err)
	}
	report.PauseOwned = paused.Owned != nil
	switch paused.Outcome {
	case hibernateengine.ConfirmationTimedOut:
		report.Unconfirmed = paused.Withheld
	case hibernateengine.ConfirmationLifted:
		report.Unconfirmed = "the hub pause was lifted before it was confirmed"
	}

	beams, tasks, err := steps.worktrees()
	if err != nil {
		failed("list the hub's worktrees", err)
	}
	for _, task := range tasks {
		downSession(task, &report.Pairs)
	}
	for _, worktree := range append([]*lymcwd.Location{prime}, beams...) {
		down, err := steps.beamDown(worktree)
		report.Orchs = append(report.Orchs, down)
		if err != nil {
			failed("take down the orch and dobby of "+worktree.WorktreeName, err)
		}
	}
	for _, beam := range beams {
		downSession(beam, &report.Sessions)
	}
	reaped, err := steps.reap()
	report.Reaped = append(report.Reaped, reaped.Ended...)
	if err != nil {
		failed("end the run processes no run owns", err)
	}
	// A pause lifted before its confirmation was lifted by the operator; there is no entry left for the stop to lift.
	if paused.Owned != nil && paused.Outcome != hibernateengine.ConfirmationLifted {
		if err := steps.lift(*paused.Owned); err != nil {
			failed("lift the stop's own pause entry", err)
		} else {
			report.PauseLifted = true
		}
	}
	downSession(prime, &report.Sessions)
	return report
}

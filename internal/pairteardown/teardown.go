package pairteardown

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// RemoveQuietWait is the quiet-wait bound `lyx fabric remove` passes.
// It is a constant rather than config: the wait never kills a working driver on the operator path, and an unbounded wait would hang the hub's land step.
const RemoveQuietWait = 2 * time.Minute

// quietPollInterval is the pause between two quiet probes.
// The probe count is capped from Request.QuietWait and this interval, never read off a clock.
const quietPollInterval = 2 * time.Second

// ErrDriverBusy is the sentinel EndSession's error wraps when the bound is spent with Request.RefuseWhenBusy and the pair's driver is still busy.
var ErrDriverBusy = errors.New("pair driver is busy")

// Request names the pair to end and the caller's policy on a busy driver.
type Request struct {
	// Slug names the pair.
	Slug string
	// Force and Remote are passed to the refusal probe and the removal.
	Force  bool
	Remote bool
	// QuietWait bounds the wait for the driver to go quiet; zero probes once.
	QuietWait time.Duration
	// RefuseWhenBusy makes a spent bound an ErrDriverBusy refusal instead of ending the driver.
	RefuseWhenBusy bool
}

// SessionResult reports EndSession's outcome.
type SessionResult struct {
	// Ended is true when the session end ran: Engine.Down on a present task worktree, which reports true whether or not a session existed, or a session found and reaped by name on a gone one.
	Ended bool
	// AbandonedSession names a foreign session reed's Down reported and did not kill; empty in every ordinary teardown.
	AbandonedSession string
	// DriverWasLive is true when the bound was spent with the driver still busy and the caller's policy ended it anyway.
	DriverWasLive bool
}

// Result is Run's outcome: both phases, with the removal passed through whole.
type Result struct {
	Session SessionResult
	Removal fabricengine.RemoveResult
}

// quietState is one quiet probe's answer.
type quietState struct {
	quiet bool
	// driver names the live driver strand blocking quiet; empty when no driver strand blocks.
	driver string
	// loop is true when the pair's loop lock is held, so a detached step loop is live or starting.
	loop bool
}

// Teardown runs the pair-teardown sequence.
// Each substrate sits behind an unexported function field so the untagged tests drive the sequence with fakes.
type Teardown struct {
	prime    *lyxcwd.Location
	interval time.Duration

	gone       func(slug string) (bool, error)
	quiet      func(slug string) (quietState, error)
	refusal    func(slug string, force bool) error
	endSession func(slug string, gone bool) (ended bool, abandoned string, err error)
	remove     func(req Request) (fabricengine.RemoveResult, error)
	sleep      func(ctx context.Context, d time.Duration) error

	// killLoop kills the pair's loop and its in-flight step tree.
	killLoop func(slug string) (bool, error)
	// seizeLoop runs kill, takes the pair's loop lock and leaves the teardown's mark in the pid file;
	// its release function drops the lock, and with sessionEnded false first restores the pid file.
	seizeLoop func(ctx context.Context, slug string, kill func() (bool, error)) (release func(sessionEnded bool) error, err error)
	// awaitRunLock waits for the pair's run lock to be released and names its holder when the bound passes.
	awaitRunLock func(ctx context.Context, slug string) error
}

// New wires the production substrates from the hub's prime location.
func New(prime *lyxcwd.Location) (*Teardown, error) {
	if prime == nil {
		return nil, errors.New("pairteardown: New needs the hub's prime location")
	}
	t := &Teardown{prime: prime, interval: quietPollInterval, sleep: sleepCtx}
	t.gone = t.taskWorktreeGone
	t.quiet = t.probeQuiet
	t.refusal = func(slug string, force bool) error {
		top, err := t.topology()
		if err != nil {
			return err
		}
		return top.RemoveRefusal(prime, slug, force)
	}
	t.endSession = t.endReedSession
	t.killLoop = t.killPairLoop
	t.seizeLoop = t.holdPairLoopLock
	t.awaitRunLock = t.awaitPairRunLock
	t.remove = func(req Request) (fabricengine.RemoveResult, error) {
		top, err := t.topology()
		if err != nil {
			return fabricengine.RemoveResult{}, err
		}
		return top.Remove(prime, req.Slug, req.Force, req.Remote)
	}
	return t, nil
}

// EndSession waits for the pair's driver to go quiet, probes for removal refusals, ends the pair's loop and step tree, waits for the run lock and ends the pair's session.
// A refusal or a busy driver leaves the session and its strands untouched.
// The loop lock is held from the kill until the session has ended, and the teardown's mark stays in the loop's pid file after it, so no step starts once the session end begins.
// A failure before the session end puts the pid file back.
func (t *Teardown) EndSession(ctx context.Context, req Request) (SessionResult, error) {
	var res SessionResult

	gone, err := t.gone(req.Slug)
	if err != nil {
		return res, err
	}
	if !gone {
		res.DriverWasLive, err = t.waitQuiet(ctx, req)
		if err != nil {
			return res, err
		}
	}

	if err := t.refusal(req.Slug, req.Force); err != nil {
		return res, err
	}

	var releaseLoopLock func(sessionEnded bool) error
	if !gone {
		releaseLoopLock, err = t.seizeLoop(ctx, req.Slug, func() (bool, error) { return t.killLoop(req.Slug) })
		if err != nil {
			return res, err
		}
		if err := t.awaitRunLock(ctx, req.Slug); err != nil {
			return res, errors.Join(err, releaseLoopLock(false))
		}
	}

	ended, abandoned, err := t.endSession(req.Slug, gone)
	res.Ended = ended
	res.AbandonedSession = abandoned
	if releaseLoopLock != nil {
		err = errors.Join(err, releaseLoopLock(err == nil))
	}
	if err != nil {
		return res, err
	}
	logger.Info("pairteardown: session ended", "slug", req.Slug, "ended", ended, "task_worktree_gone", gone, "driver_was_live", res.DriverWasLive)
	return res, nil
}

// RemovePair removes the pair.
// It runs only after EndSession succeeded for the same request: Run and batten's one-row producer are its only sequencers.
func (t *Teardown) RemovePair(req Request) (fabricengine.RemoveResult, error) {
	return t.remove(req)
}

// Run is EndSession then RemovePair; a failed EndSession never reaches RemovePair.
func (t *Teardown) Run(ctx context.Context, req Request) (Result, error) {
	session, err := t.EndSession(ctx, req)
	if err != nil {
		return Result{Session: session}, err
	}
	removal, err := t.RemovePair(req)
	return Result{Session: session, Removal: removal}, err
}

// waitQuiet probes until the driver is quiet or the bound is spent, and reports whether a busy driver was left to the caller's policy.
func (t *Teardown) waitQuiet(ctx context.Context, req Request) (driverWasLive bool, err error) {
	attempts := int(req.QuietWait / t.interval)
	for i := 0; ; i++ {
		st, err := t.quiet(req.Slug)
		if err != nil {
			return false, err
		}
		if st.quiet {
			return false, nil
		}
		if i >= attempts {
			if req.RefuseWhenBusy {
				return false, t.busyError(req.Slug, st)
			}
			logger.Warn("pairteardown: ending a driver that is not quiet", "slug", req.Slug, "driver", st.driver)
			return true, nil
		}
		if err := t.sleep(ctx, t.interval); err != nil {
			return false, err
		}
	}
}

// busyError builds the ErrDriverBusy refusal naming the blocker and the way forward.
func (t *Teardown) busyError(slug string, st quietState) error {
	blocker := "the run lock is held"
	switch {
	case st.driver != "":
		blocker = fmt.Sprintf("driver strand %q is live", st.driver)
	case st.loop:
		blocker = "the loop is live"
	}
	attach := fmt.Sprintf("lyx reed attach, run from %s", t.taskAnchor(slug))
	wayForward := "retry once the driver has written its report, or end it with `lyx reed down` in the pair"
	if st.loop {
		wayForward = fmt.Sprintf("pause the run with `lyx shed pause %s`, or retry once the loop stops; %s", slug, wayForward)
	}
	return fmt.Errorf("%w: %s in pair %q; attach with %s; %s", ErrDriverBusy, blocker, slug, attach, wayForward)
}

// taskAnchor returns the task worktree's anchor path.
func (t *Teardown) taskAnchor(slug string) string {
	return filepath.Join(fabricengine.WorktreePath(t.prime, slug), t.prime.AnchorRel)
}

// topology loads the hub's fabric config fresh and builds a topology over it.
func (t *Teardown) topology() (*fabricengine.Topology, error) {
	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(t.prime.HubPath))
	if err != nil {
		return nil, err
	}
	top := fabricengine.NewTopology(cfg)
	top.SetInFlightProbe(t.otherPairSessions)
	return top, nil
}

// otherPairSessions returns the live reed sessions of the hub's pairs other than slug's.
// A hub socket with no tmux server behind it has no sessions.
// The prime's own sessions, the orch among them, are never in the pair set and never count.
func (t *Teardown) otherPairSessions(slug string) ([]string, error) {
	cfg, err := reedengine.LoadConfig(t.prime.AnchorPath(), "reed")
	if err != nil {
		return nil, err
	}
	live, err := reedengine.ListSessions(cfg.Tmux, reedengine.ServerName(t.prime.HubPath))
	if err != nil {
		if reedengine.IsNoServer(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list the hub's live sessions: %w", err)
	}

	entries, err := fabricengine.List(t.prime.WorktreePath())
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}
	removed := filepath.Clean(fabricengine.WorktreePath(t.prime, slug))
	var pairSessions []string
	for _, entry := range entries {
		path := filepath.Clean(filepath.FromSlash(entry.Path))
		if entry.Main || path == removed {
			continue
		}
		pairSessions = append(pairSessions, reedengine.SessionName(path))
	}
	return sessionsOfOtherPairs(live, pairSessions), nil
}

// sessionsOfOtherPairs returns the sessions of live that are named in pairSessions, in live's order.
func sessionsOfOtherPairs(live, pairSessions []string) []string {
	var others []string
	for _, session := range live {
		if slices.Contains(pairSessions, session) {
			others = append(others, session)
		}
	}
	return others
}

// taskWorktreeGone reports whether the task worktree path is not a registered linked worktree of the prime's repo.
func (t *Teardown) taskWorktreeGone(slug string) (bool, error) {
	entries, err := fabricengine.List(t.prime.WorktreePath())
	if err != nil {
		return false, fmt.Errorf("list worktrees: %w", err)
	}
	target := filepath.Clean(fabricengine.WorktreePath(t.prime, slug))
	for _, entry := range entries {
		if !entry.Main && filepath.Clean(filepath.FromSlash(entry.Path)) == target {
			return false, nil
		}
	}
	return true, nil
}

// taskLocation resolves the present task worktree's own location.
func (t *Teardown) taskLocation(slug string) (*lyxcwd.Location, error) {
	return lyxcwd.ResolveWorktree(fabricengine.WorktreePath(t.prime, slug))
}

// reedEngine builds the reed engine for the present task worktree.
func (t *Teardown) reedEngine(task *lyxcwd.Location) (*reedengine.Engine, error) {
	cfg, err := reedengine.LoadConfig(task.AnchorPath(), "reed")
	if err != nil {
		return nil, err
	}
	geom, err := hubgeom.ReedGeometry(task)
	if err != nil {
		return nil, err
	}
	return reedengine.New(cfg, geom), nil
}

// probeQuiet answers whether the present task worktree's driver is quiet: the run lock and the loop lock are free, and the driver strand is absent, dead, retiring or parked.
func (t *Teardown) probeQuiet(slug string) (quietState, error) {
	task, err := t.taskLocation(slug)
	if err != nil {
		return quietState{}, err
	}

	lockFree, err := runLockFree(shedrun.RunLock(task, shedrun.SelfRunID))
	if err != nil {
		return quietState{}, err
	}
	loopFree, err := runLockFree(shedrun.LoopLock(task, shedrun.SelfRunID))
	if err != nil {
		return quietState{}, err
	}

	var driver string
	if !fileExists(shedrun.ParkMarker(task, shedrun.SelfRunID)) {
		eng, err := t.reedEngine(task)
		if err != nil {
			return quietState{}, err
		}
		status, err := eng.Status()
		if err != nil && !errors.Is(err, reedengine.ErrNoSession) {
			return quietState{}, err
		}
		for _, s := range status.Strands {
			if loomengine.IsDriverStrand(s.Name) && s.Live && !s.Retiring {
				driver = s.Name
				break
			}
		}
	}
	return quietState{quiet: lockFree && loopFree && driver == "", driver: driver, loop: !loopFree}, nil
}

// killPairLoop kills the present task worktree's loop and its in-flight step tree.
func (t *Teardown) killPairLoop(slug string) (bool, error) {
	task, err := t.taskLocation(slug)
	if err != nil {
		return false, err
	}
	return killLoop(shedrun.LoopPIDFile(task, shedrun.SelfRunID), shedrun.LoopLock(task, shedrun.SelfRunID), shedrun.LoopJobName(task, shedrun.SelfRunID))
}

// holdPairLoopLock takes the present task worktree's loop lock after kill and leaves the teardown's mark in its loop pid file.
// The steps directory is created first, since the lock file lives in it.
func (t *Teardown) holdPairLoopLock(ctx context.Context, slug string, kill func() (bool, error)) (func(sessionEnded bool) error, error) {
	task, err := t.taskLocation(slug)
	if err != nil {
		return nil, err
	}
	lockPath := shedrun.LoopLock(task, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, err
	}
	return holdLoopLock(ctx, kill, shedrun.LoopPIDFile(task, shedrun.SelfRunID), lockPath, int(runLockWait/quietPollInterval), t.sleep)
}

// awaitPairRunLock waits for the present task worktree's run lock to be released.
func (t *Teardown) awaitPairRunLock(ctx context.Context, slug string) error {
	task, err := t.taskLocation(slug)
	if err != nil {
		return err
	}
	return awaitRunLockNamingHolder(ctx, shedrun.RunLock(task, shedrun.SelfRunID), shedrun.StepsDir(task, shedrun.SelfRunID), slug, int(runLockWait/quietPollInterval), t.sleep)
}

// endReedSession ends the pair's session: Engine.Down while the task worktree is present, EndSessionByName once it is gone.
func (t *Teardown) endReedSession(slug string, gone bool) (bool, string, error) {
	if !gone {
		task, err := t.taskLocation(slug)
		if err != nil {
			return false, "", err
		}
		eng, err := t.reedEngine(task)
		if err != nil {
			return false, "", err
		}
		res, err := eng.Down()
		if err != nil {
			return false, "", err
		}
		return true, res.AbandonedSession, nil
	}

	cfg, err := reedengine.LoadConfig(t.prime.AnchorPath(), "reed")
	if err != nil {
		return false, "", err
	}
	ended, err := reedengine.EndSessionByName(
		cfg.Tmux, cfg.Shell,
		reedengine.ServerName(t.prime.HubPath),
		reedengine.SessionName(fabricengine.WorktreePath(t.prime, slug)),
	)
	return ended, "", err
}

// runLockFree probes the run lock without blocking and releases it at once.
// An absent scratch directory means no run ever took the lock.
func runLockFree(lockPath string) (bool, error) {
	if _, err := os.Stat(filepath.Dir(lockPath)); errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	held, ok, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return true, held.Release()
}

// fileExists reports whether path names an existing file or directory.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// sleepCtx waits d or until ctx is done.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

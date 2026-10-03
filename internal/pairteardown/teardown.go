package pairteardown

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	// Ended is true when a session was ended.
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
	// driver names the live driver strand blocking quiet; empty when only the run lock is held.
	driver string
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
	t.remove = func(req Request) (fabricengine.RemoveResult, error) {
		top, err := t.topology()
		if err != nil {
			return fabricengine.RemoveResult{}, err
		}
		return top.Remove(prime, req.Slug, req.Force, req.Remote)
	}
	return t, nil
}

// EndSession waits for the pair's driver to go quiet, probes for removal refusals and ends the pair's session.
// A refusal or a busy driver leaves the session and its strands untouched.
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

	ended, abandoned, err := t.endSession(req.Slug, gone)
	res.Ended = ended
	res.AbandonedSession = abandoned
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
	if st.driver != "" {
		blocker = fmt.Sprintf("driver strand %q is live", st.driver)
	}
	attach := fmt.Sprintf("lyx reed attach, run from %s", t.taskAnchor(slug))
	return fmt.Errorf("%w: %s in pair %q; attach with %s; retry once the driver has written its report, or end it with `lyx reed down` in the pair", ErrDriverBusy, blocker, slug, attach)
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
	return fabricengine.NewTopology(cfg), nil
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

// probeQuiet answers whether the present task worktree's driver is quiet: the run lock is free, and the driver strand is absent, dead, retiring or parked.
func (t *Teardown) probeQuiet(slug string) (quietState, error) {
	task, err := t.taskLocation(slug)
	if err != nil {
		return quietState{}, err
	}

	lockFree, err := runLockFree(shedrun.RunLock(task, shedrun.SelfRunID))
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
	return quietState{quiet: lockFree && driver == "", driver: driver}, nil
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

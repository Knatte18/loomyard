// revive.go keeps tmux session ids in spawn order across a server restart.
// A worktree about to create its recorded session first has every earlier worktree in the told spawn order revive its own, so a restarted server numbers the sessions as the first server did.

package reedengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
)

// errReviveFirst is the sentinel ensureServerAndSessionLocked returns, having created nothing, when this worktree must wait for the earlier worktrees to revive their sessions.
// It never leaves the package.
var errReviveFirst = errors.New("earlier worktrees revive first")

// recordsSessionLocked reports whether this worktree's reed.json records a session, which a downed or never-booted worktree does not.
// Assumes the op lock is already held.
func (e *Engine) recordsSessionLocked() bool {
	st, err := LoadState(e.stateDir())
	return err == nil && st != nil && st.Session != ""
}

// reviveDueLocked reports whether creating the booter's missing session must wait for the earlier worktrees:
// a spawn order is told, no revival has run for this step yet, and the worktree's recorded session is the one that is not live.
// Assumes the op lock is already held.
func (e *Engine) reviveDueLocked() bool {
	return !e.skipRevival && e.geom.SpawnOrder != nil && e.recordsSessionLocked()
}

// withRevivalFirst runs step through lock.
// When the step reports errReviveFirst, the lock is released, every earlier worktree's Revive runs in spawn order outside it,
// and the whole step runs again from its start under the lock with the skip set, so no two op locks are held at once and the trigger is evaluated once.
func (e *Engine) withRevivalFirst(lock func(fn func() error) error, step func() error) error {
	err := lock(step)
	if !errors.Is(err, errReviveFirst) {
		return err
	}
	e.reviveEarlierWorktrees()
	return lock(func() error {
		e.skipRevival = true
		defer func() { e.skipRevival = false }()
		return step()
	})
}

// reviveEarlierWorktrees calls Revive on each worktree before this one in the told spawn order, sequentially.
// A list that fails or lacks this worktree revives nothing, and a Revive that fails is skipped; each is logged and the boot goes on.
func (e *Engine) reviveEarlierWorktrees() {
	entries, err := e.geom.SpawnOrder()
	if err != nil {
		logger.Warn("reed: could not list the spawn order, booting without reviving earlier sessions", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return
	}
	booterAt := -1
	for i, entry := range entries {
		if entry.Worktree == e.geom.WorktreeName {
			booterAt = i
			break
		}
	}
	if booterAt == -1 {
		return
	}
	for _, entry := range entries[:booterAt] {
		revived, err := entry.Revive()
		if err != nil {
			logger.Warn("reed: could not revive an earlier worktree's session, skipping it", "socket", e.Socket(), "session", e.SessionName(), "worktree", entry.Worktree, "err", err)
			continue
		}
		if revived {
			logger.Info("reed: revived an earlier worktree's session ahead of this one", "socket", e.Socket(), "session", e.SessionName(), "worktree", entry.Worktree)
		}
	}
}

// Revive brings this worktree's recorded session back when it is not live and reports whether it did.
// It runs the ordinary up (session and Selvage, no strand relaunch) under the op lock with the revival skip set, so it never recurses into the earlier worktrees.
// A worktree that records no session, or whose session is live, creates nothing and reports false.
func (e *Engine) Revive() (bool, error) {
	revived := false
	err := e.withOpLock(func() error {
		if !e.recordsSessionLocked() {
			return nil
		}
		up, err := e.tmux.hasSession(e.SessionName())
		if err != nil {
			return fmt.Errorf("check session: %w", err)
		}
		if up {
			return nil
		}
		e.skipRevival = true
		defer func() { e.skipRevival = false }()
		if _, _, err := e.upLocked(); err != nil {
			return err
		}
		revived = true
		return nil
	})
	return revived, err
}

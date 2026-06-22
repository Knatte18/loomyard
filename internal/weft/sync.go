// sync.go — the weft git core for commit, push, pull, and lock management.
//
// Commit stages pathspec-scoped changes and commits them. Push loops with
// rebase-retry on non-fast-forward until everything is pushed. Pull fast-forwards
// from upstream. All operations coordinate via file locks in the weft worktree.
// Skipping behavior is controlled via SyncOptions, not environment variables.

package weft

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/git"
	flock "github.com/Knatte18/loomyard/internal/lock"
)

// SyncOptions controls git sync behavior for Commit, Push, and Pull.
type SyncOptions struct {
	SkipGit  bool // Skip all git operations if true.
	SkipPush bool // Skip push operations if true; affects Push only.
}

// ensureLockDir creates the lock directory inside the weft worktree and returns its path.
func ensureLockDir(weftPath string) (string, error) {
	lockDir := filepath.Join(weftPath, lockDirName)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir lock dir: %w", err)
	}
	return lockDir, nil
}

// Commit stages and commits pathspec-scoped changes in the weft worktree.
// Returns (false, nil) if opts.SkipGit is true or if there is nothing staged.
// Returns (true, nil) if a commit was made.
func Commit(weftPath string, pathspec []string, opts SyncOptions) (committed bool, err error) {
	if opts.SkipGit {
		return false, nil
	}

	// Ensure lock directory exists
	lockDir, err := ensureLockDir(weftPath)
	if err != nil {
		return false, err
	}

	// Acquire write lock
	lock, err := flock.AcquireWriteLock(filepath.Join(lockDir, writeLockFile))
	if err != nil {
		return false, fmt.Errorf("acquire write lock: %w", err)
	}
	defer lock.Release()

	// Stage the pathspec entries
	args := append([]string{"add", "--"}, pathspec...)
	if _, _, code, err := git.RunGit(args, weftPath); err != nil {
		return false, fmt.Errorf("add: %w", err)
	} else if code != 0 {
		return false, fmt.Errorf("add failed")
	}

	// Check if there is anything staged
	_, _, code, err := git.RunGit([]string{"diff", "--cached", "--quiet"}, weftPath)
	if err != nil {
		return false, fmt.Errorf("diff --cached: %w", err)
	}
	if code != 0 {
		// Exit code 1 means changes are staged
		// Commit them
		if _, _, code, err := git.RunGit([]string{"commit", "-m", commitMessage}, weftPath); err != nil {
			return false, fmt.Errorf("commit: %w", err)
		} else if code != 0 {
			return false, fmt.Errorf("commit failed")
		}
		return true, nil
	}

	// Nothing staged
	return false, nil
}

// Push loops pushing unpushed commits to the remote, with rebase-retry on non-fast-forward.
// Returns nil immediately if opts.SkipGit or opts.SkipPush is true.
// No-op if there is nothing unpushed.
func Push(weftPath string, opts SyncOptions) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}

	// Ensure lock directory exists
	lockDir, err := ensureLockDir(weftPath)
	if err != nil {
		return err
	}

	// Acquire push lock
	pushLock, err := flock.AcquireWriteLock(filepath.Join(lockDir, pushLockFile))
	if err != nil {
		return fmt.Errorf("acquire push lock: %w", err)
	}
	defer pushLock.Release()

	for {
		unpushed, err := hasUnpushed(weftPath)
		if err != nil {
			return err
		}
		if !unpushed {
			break
		}

		if err := pushUnpushed(weftPath); err != nil {
			return err
		}
	}

	return nil
}

// Pull fast-forwards from the remote.
// Returns nil immediately if opts.SkipGit is true.
func Pull(weftPath string, opts SyncOptions) error {
	if opts.SkipGit {
		return nil
	}

	_, _, code, err := git.RunGit([]string{"pull", "--ff-only"}, weftPath)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("pull failed")
	}

	return nil
}

// pushUnpushed attempts to push unpushed commits, rebasing once on non-fast-forward.
func pushUnpushed(weftPath string) error {
	for attempt := 0; attempt < 2; attempt++ {
		_, stderr, code, err := git.RunGit([]string{"push"}, weftPath)
		if err != nil {
			return fmt.Errorf("push: %w", err)
		}
		if code == 0 {
			return nil
		}

		if strings.Contains(stderr, "non-fast-forward") ||
			strings.Contains(stderr, "rejected") ||
			strings.Contains(stderr, "fetch first") {
			if _, _, c, err := git.RunGit([]string{"pull", "--rebase"}, weftPath); err != nil || c != 0 {
				git.RunGit([]string{"rebase", "--abort"}, weftPath)
				return fmt.Errorf("rebase failed")
			}
			continue
		}
		return fmt.Errorf("push failed: %s", stderr)
	}
	return fmt.Errorf("push still failing after rebase retry")
}

// hasUnpushed reports whether HEAD is ahead of its upstream.
// If no upstream is configured it returns true (for the first push).
func hasUnpushed(weftPath string) (bool, error) {
	out, _, code, err := git.RunGit([]string{"rev-list", "--count", "@{u}..HEAD"}, weftPath)
	if err != nil {
		return false, fmt.Errorf("rev-list: %w", err)
	}
	if code != 0 {
		return true, nil // no upstream yet
	}
	return strings.TrimSpace(out) != "0", nil
}

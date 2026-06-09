// sync.go — the background pusher that backs up the board to the remote.
//
// Writes only touch the filesystem; Sync is what gets those changes to GitHub.
// It commits any pending working-tree changes and pushes all unpushed commits,
// looping until nothing is left so a burst of writes coalesces into as few
// pushes as possible. A single Sync runs at a time (the push lock); concurrent
// sync processes block, then exit quickly once there is nothing to do. The write
// path launches `mhgo board sync` detached (see spawn_*.go) so it never waits.
package board

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// writeLockFile serialises file-state changes (writers' mutations and the
	// sync commit). pushLockFile guarantees a single active pusher.
	writeLockFile = "tasks.json.lock"
	pushLockFile  = "tasks.json.push.lock"
)

// Sync commits any pending changes and pushes them to the remote, looping until
// the working tree is clean and nothing is unpushed. BOARD_SKIP_GIT disables it
// entirely (used by tests); BOARD_SKIP_PUSH commits locally but skips the push.
func Sync(boardPath string) error {
	if os.Getenv("BOARD_SKIP_GIT") == "1" {
		return nil
	}

	// The lock files live in the board dir; keep git from ever committing them.
	if err := ensureLockfilesIgnored(boardPath); err != nil {
		return err
	}

	// Only one pusher does network work at a time. A second sync process blocks
	// here, then finds nothing to do and returns — that is the coalescing.
	pushLock, err := AcquireWriteLock(filepath.Join(boardPath, pushLockFile))
	if err != nil {
		return fmt.Errorf("acquire push lock: %w", err)
	}
	defer pushLock.Release()

	for {
		committed, err := commitDirty(boardPath)
		if err != nil {
			return err
		}
		if err := pushUnpushed(boardPath); err != nil {
			return err
		}
		// Nothing new arrived this round → done. If a write landed while we were
		// pushing, the tree is dirty again and we loop to catch it.
		if !committed {
			return nil
		}
	}
}

// commitDirty stages and commits the working tree if it has changes, under the
// write lock so it snapshots a state no writer is mid-mutation on. Returns
// whether a commit was made.
func commitDirty(boardPath string) (bool, error) {
	lock, err := AcquireWriteLock(filepath.Join(boardPath, writeLockFile))
	if err != nil {
		return false, fmt.Errorf("acquire write lock: %w", err)
	}
	defer lock.Release()

	out, _, code, err := RunGit([]string{"status", "--porcelain"}, boardPath)
	if err != nil {
		return false, fmt.Errorf("status: %w", err)
	}
	if code != 0 {
		return false, BoardPushError("status failed")
	}
	if strings.TrimSpace(out) == "" {
		return false, nil // clean working tree
	}

	if _, _, code, err := RunGit([]string{"add", "-A"}, boardPath); err != nil {
		return false, fmt.Errorf("add: %w", err)
	} else if code != 0 {
		return false, BoardPushError("add failed")
	}

	if _, _, code, err := RunGit([]string{"commit", "-m", "board sync"}, boardPath); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	} else if code != 0 {
		return false, BoardPushError("commit failed")
	}
	return true, nil
}

// pushUnpushed pushes local commits to the remote, rebasing once on a
// non-fast-forward. No-op if there is nothing unpushed or BOARD_SKIP_PUSH is set.
func pushUnpushed(boardPath string) error {
	if os.Getenv("BOARD_SKIP_PUSH") == "1" {
		return nil
	}

	unpushed, err := hasUnpushed(boardPath)
	if err != nil {
		return err
	}
	if !unpushed {
		return nil
	}

	for attempt := 0; attempt < 2; attempt++ {
		_, stderr, code, err := RunGit([]string{"push"}, boardPath)
		if err != nil {
			return fmt.Errorf("push: %w", err)
		}
		if code == 0 {
			return nil
		}

		if strings.Contains(stderr, "non-fast-forward") ||
			strings.Contains(stderr, "rejected") ||
			strings.Contains(stderr, "fetch first") {
			if _, _, c, err := RunGit([]string{"pull", "--rebase"}, boardPath); err != nil || c != 0 {
				RunGit([]string{"rebase", "--abort"}, boardPath)
				return BoardPushError("rebase failed")
			}
			continue
		}
		return BoardPushError(fmt.Sprintf("push failed: %s", stderr))
	}
	return BoardPushError("push still failing after rebase retry")
}

// ensureLockfilesIgnored adds the lock-file patterns to the board's .gitignore
// (idempotently) so the flock files that live alongside tasks.json are never
// staged or committed. A committed .gitignore is shared with every clone via the
// remote, so the lock files are ignored on every machine from clone time — the
// first sync commits the .gitignore once.
func ensureLockfilesIgnored(boardPath string) error {
	gitignorePath := filepath.Join(boardPath, ".gitignore")
	existing, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read .gitignore: %w", err)
	}

	var missing []string
	for _, pat := range []string{"*.lock", "*.swaplock"} {
		if !strings.Contains(string(existing), pat) {
			missing = append(missing, pat)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open .gitignore: %w", err)
	}
	defer f.Close()
	for _, pat := range missing {
		if _, err := f.WriteString(pat + "\n"); err != nil {
			return fmt.Errorf("write .gitignore: %w", err)
		}
	}
	return nil
}

// hasUnpushed reports whether HEAD is ahead of its upstream. If no upstream is
// configured it returns true, so the first push (which sets it) still happens.
func hasUnpushed(boardPath string) (bool, error) {
	out, _, code, err := RunGit([]string{"rev-list", "--count", "@{u}..HEAD"}, boardPath)
	if err != nil {
		return false, fmt.Errorf("rev-list: %w", err)
	}
	if code != 0 {
		return true, nil // no upstream yet
	}
	return strings.TrimSpace(out) != "0", nil
}

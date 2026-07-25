// sync.go — the background pusher that backs up the board to the remote.
//
// Writes only touch the filesystem; Sync is what gets those changes to GitHub.
// Each loop iteration commits any dirty working-tree state via
// gitrepo.StageAllAndCommit and, unless skipPush is set, pushes anything
// unpushed via gitrepo.PushCoalesced (which owns the hasUnpushed guard and
// the rebase-retry, so a fully-pushed board never touches the network) —
// looping until a commit iteration finds nothing dirty, so a burst of writes
// coalesces into as few pushes as possible. A single top-level push lock
// still serializes pushers across processes; concurrent sync processes
// block, then exit quickly once there is nothing to do. The write path
// launches `lyx board sync` detached (see spawn.go) so it never waits.
package boardengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	flock "github.com/Knatte18/loomyard/internal/lock"
)

const (
	// writeLockFile serialises file-state changes (writers' mutations and the
	// sync commit). pushLockFile guarantees a single active pusher.
	writeLockFile = "tasks.json.lock"
	pushLockFile  = "tasks.json.push.lock"
)

// Sync commits any pending changes and pushes them to the remote, looping until
// the working tree is clean and nothing is unpushed. skipGit disables it
// entirely (used by tests); skipPush commits locally but skips the push.
func Sync(boardPath string, skipGit, skipPush bool) error {
	if skipGit {
		return nil
	}

	// Only one pusher does network work at a time. A second sync process blocks
	// here, then finds nothing to do and returns — that is the coalescing.
	pushLock, err := flock.AcquireWriteLock(filepath.Join(boardPath, pushLockFile))
	if err != nil {
		return fmt.Errorf("acquire push lock: %w", err)
	}
	defer pushLock.Release()

	// The lock files live in the board dir; keep git from ever committing them.
	// Runs under the push lock — Sync is the only .gitignore writer, so the lock
	// serializes concurrent first syncs that would otherwise both read the
	// patterns as missing and append duplicates. Still ahead of any staging, so
	// the ignore patterns are always in place before the first `git add -A`.
	if err := ensureLockfilesIgnored(boardPath); err != nil {
		return err
	}

	repo := gitrepo.New(boardPath)
	for {
		committed, err := commitDirty(repo, boardPath)
		if err != nil {
			return err
		}
		if !skipPush {
			// PushCoalesced no-ops when nothing is ahead of upstream, so a sync
			// with nothing to do never touches the network (and never fails just
			// because the remote is unreachable) — matching the pre-gitrepo
			// pushUnpushed behavior.
			if err := repo.PushCoalesced(); err != nil {
				return fmt.Errorf("sync push: %w", err)
			}
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
func commitDirty(repo *gitrepo.Repo, boardPath string) (bool, error) {
	lock, err := flock.AcquireWriteLock(filepath.Join(boardPath, writeLockFile))
	if err != nil {
		return false, fmt.Errorf("acquire write lock: %w", err)
	}
	defer lock.Release()

	_, committed, err := repo.StageAllAndCommit("board sync")
	if err != nil {
		return false, fmt.Errorf("sync commit: %w", err)
	}
	return committed, nil
}

// ensureLockfilesIgnored adds the lock-file and manifest-sidecar patterns to the
// board's .gitignore (idempotently) so the flock files and the render manifest that
// live alongside tasks.json are never staged or committed. A committed .gitignore is
// shared with every clone via the remote, so the sidecars are ignored on every machine
// from clone time — the first sync commits the .gitignore once.
func ensureLockfilesIgnored(boardPath string) error {
	gitignorePath := filepath.Join(boardPath, ".gitignore")
	existing, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read .gitignore: %w", err)
	}

	var missing []string
	for _, pat := range []string{"*.lock", "*.swaplock", renderManifestFile} {
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

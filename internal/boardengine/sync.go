// sync.go — the background pusher that backs up the board to the remote.
//
// Writes only touch the filesystem; Sync is what gets those changes to
// GitHub. The absorbing-lock loop-until-clean coalescing is owned by
// fabricengine.Bolt.Sync, composed here with board's own step closure:
// ensureLockfilesIgnored + commitDirty (which holds board's own board.lock)
// and, unless skipPush is set, a push through the same Bolt value. A single
// top-level push lock still serializes pushers across processes; concurrent
// sync processes block, then exit quickly once there is nothing to do. The
// write path launches `lyx board sync` detached (see spawn.go) so it never
// waits.
package boardengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	flock "github.com/Knatte18/loomyard/internal/lock"
)

const (
	writeLockFile = "board.lock"
	pushLockFile  = "board.push.lock"
)

// Sync commits pending changes and pushes to the remote, looping until clean and unpushed changes are gone.
// skipGit disables it entirely; skipPush commits locally but skips the push.
// The absorbing-lock coalescing loop is delegated to fabricengine.NewBolt.
func Sync(boardPath string, skipGit, skipPush bool) error {
	if skipGit {
		return nil
	}

	bolt := fabricengine.NewBolt(boardPath)

	step := func() (progressed bool, err error) {
		if err := ensureLockfilesIgnored(boardPath); err != nil {
			return false, err
		}

		committed, err := commitDirty(boardPath, bolt)
		if err != nil {
			return false, err
		}
		if !skipPush {
			if err := bolt.Push(fabricengine.SyncOptions{}); err != nil {
				return false, fmt.Errorf("sync push: %w", err)
			}
		}
		return committed, nil
	}

	return bolt.Sync(step)
}

// commitDirty stages and commits the working tree if it has changes, under board's write lock.
// Returns whether a commit was made.
func commitDirty(boardPath string, bolt *fabricengine.Bolt) (bool, error) {
	lock, err := flock.AcquireWriteLock(filepath.Join(boardPath, writeLockFile))
	if err != nil {
		return false, fmt.Errorf("acquire write lock: %w", err)
	}
	defer lock.Release()

	_, committed, err := bolt.Commit("board sync", fabricengine.SyncOptions{})
	if err != nil {
		return false, fmt.Errorf("sync commit: %w", err)
	}
	return committed, nil
}

// ensureLockfilesIgnored adds lock-file and manifest-sidecar patterns to .gitignore idempotently.
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

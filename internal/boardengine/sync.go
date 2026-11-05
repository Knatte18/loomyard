// sync.go — the background pusher that backs up the board to the remote.
//
// Writes only touch the filesystem;
// Sync is what gets those changes to GitHub.
// The absorbing-lock loop-until-clean coalescing is owned by fabricengine.Bolt.Sync, composed here with board's own step closure: ensureBoardIgnores + commitDirty (which holds the board write lock) and, unless skipPush is set, a push through the same Bolt value.
// A single top-level push lock still serializes pushers across processes;
// concurrent sync processes block, then exit quickly once there is nothing to do.
// The write path launches `lym board sync` detached (see spawn.go) so it never waits.
package boardengine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	flock "github.com/Knatte18/loomyard/internal/lock"
)

// writeLockFile aliases fabricengine.BoardWriteLockFile, which is the single declarer of this
// literal — internal/fabricengine owns the name because a stencil-seeding commit verb outside
// boardengine must take the same lock.
const writeLockFile = fabricengine.BoardWriteLockFile

// retiredIgnorePatterns are the lock patterns the board's .gitignore once held.
// ensureBoardIgnores removes them, so a lock file that strays into the board shows in `git status` instead of hiding.
var retiredIgnorePatterns = []string{"*.lock", "*.swaplock"}

// Sync commits pending changes and pushes to the remote, looping until clean and unpushed changes
// are gone.
// skipGit disables it entirely;
// skipPush commits locally but skips the push.
// The absorbing-lock coalescing loop is delegated to fabricengine.NewBolt.
func Sync(boardPath string, skipGit, skipPush bool) error {
	if skipGit {
		return nil
	}

	bolt := fabricengine.NewBolt(boardPath)

	step := func() (progressed bool, err error) {
		if err := ensureBoardIgnores(boardPath); err != nil {
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
	lockPath, err := fabricengine.BoltLockPath(boardPath, writeLockFile)
	if err != nil {
		return false, err
	}
	lock, err := flock.AcquireWriteLock(lockPath)
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

// ensureBoardIgnores keeps the render manifest ignored in the board's .gitignore and removes the retired lock patterns from it.
// It writes the file only when its content changes.
func ensureBoardIgnores(boardPath string) error {
	gitignorePath := filepath.Join(boardPath, ".gitignore")
	existing, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read .gitignore: %w", err)
	}

	var kept []string
	manifestIgnored := false
	for _, line := range strings.Split(string(existing), "\n") {
		trimmed := strings.TrimSpace(line)
		if slices.Contains(retiredIgnorePatterns, trimmed) {
			continue
		}
		if trimmed == renderManifestFile {
			manifestIgnored = true
		}
		kept = append(kept, line)
	}
	content := strings.Join(kept, "\n")
	if !manifestIgnored {
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += renderManifestFile + "\n"
	}
	if content == string(existing) {
		return nil
	}
	if err := os.WriteFile(gitignorePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}
	return nil
}

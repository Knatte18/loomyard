// sync.go is the background pusher that backs the board up to the remote.
// Writes only touch the filesystem; Sync carries those changes to GitHub.
// Concurrent syncs coalesce in the absorbing-lock loop of the fabricengine package;
// this file supplies the commit and push step.
// A top-level push lock serializes pushers across processes;
// concurrent sync processes block, then exit quickly once there is nothing to do.
// The write path launches `lym board sync` detached (see spawn.go) so it never waits.
package boardengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	flock "github.com/Knatte18/loomyard/internal/lock"
)

// writeLockFile aliases fabricengine.BoardWriteLockFile, which declares the literal.
// The name lives in fabricengine because a stencil-seeding commit verb outside boardengine must take the same lock.
const writeLockFile = fabricengine.BoardWriteLockFile

// Sync commits pending changes and pushes to the remote, looping until clean and unpushed changes are gone.
// skipGit disables it entirely;
// skipPush commits locally but skips the push.
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

// commitDirty stages and commits the working tree when it has changes, holding the board's write lock.
// It reports whether a commit was made.
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

// ensureBoardIgnores keeps the render manifest ignored in the board's .gitignore.
// It writes the file only when the manifest line is missing.
func ensureBoardIgnores(boardPath string) error {
	gitignorePath := filepath.Join(boardPath, ".gitignore")
	existing, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read .gitignore: %w", err)
	}

	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == renderManifestFile {
			return nil
		}
	}
	content := string(existing)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += renderManifestFile + "\n"
	if err := os.WriteFile(gitignorePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}
	return nil
}

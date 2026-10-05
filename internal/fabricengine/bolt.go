// bolt.go defines Bolt, the handle over the unpaired weft:main area (today consumed only by
// _board): it names that area as a self-contained unit through Commit/CommitWritten/Push/Sync,
// so a caller outside this package never has to spell "weft" to reach it.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
)

// Bolt is a handle over a single weft:main-backed repo path with no paired warp side — the
// exception to fabric's one-repo illusion.
// It never constructs any geometry token itself.
type Bolt struct {
	path string
}

// NewBolt returns a Bolt over the repo at repoPath.
func NewBolt(repoPath string) *Bolt {
	return &Bolt{path: repoPath}
}

// Commit stages and commits every change in the Bolt's repo under message, with no warp trailer and
// no correspondence recording.
func (b *Bolt) Commit(message string, opts SyncOptions) (sha string, committed bool, err error) {
	return commitWeftAt(b.path, message, opts)
}

// CommitWritten runs write under the board write lock and commits exactly the repo-relative paths write returns,
// so unrelated dirty files in the repo stay for the next sync.
// The lock is released only after the commit, so a concurrent whole-repo Commit never captures a half-written file.
//
// A write error is returned unwrapped and nothing is committed.
// An empty path list commits nothing and returns committed == false.
// A path that no longer exists stages its deletion.
// With opts.SkipGit, write still runs under the lock but nothing is committed.
// It never pushes; callers push with Push afterwards, outside the write lock.
func (b *Bolt) CommitWritten(message string, write func() ([]string, error), opts SyncOptions) (sha string, committed bool, err error) {
	l, err := lock.AcquireWriteLock(filepath.Join(b.path, BoardWriteLockFile))
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: acquire board write lock: %w", err)
	}
	defer func() { _ = l.Release() }()

	writtenPaths, err := write()
	if err != nil {
		return "", false, err
	}
	if opts.SkipGit {
		return "", false, nil
	}
	return gitrepo.New(b.path).StageAndCommit(message, ScopedPathspec(".", writtenPaths))
}

// Push pushes any unpushed commits in the Bolt's repo.
func (b *Bolt) Push(opts SyncOptions) error {
	return pushWeftAt(b.path, opts)
}

// Sync drives step to completion under an absorbing push lock, looping while step reports progress.
func (b *Bolt) Sync(step func() (progressed bool, err error)) error {
	return coalescePush(filepath.Join(b.path, "board.push.lock"), step)
}

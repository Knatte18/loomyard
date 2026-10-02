// codecommit.go declares CommitCode, the verb that records a repo code on a live hub's board.
// Like CommitSeededStencils it takes the board write lock and commits only its own file through a positive pathspec,
// since the board is live and Bolt.Commit would sweep any pending or half-written board change into the commit.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
)

// CommitCode writes code to the hub's .lyx-code record and commits that file alone onto weft:main, under the board write lock.
// It never pushes; the caller pushes through Bolt.Push.
// rec accumulates the file write and, when a commit was made, the commit.
func CommitCode(hub, code, message string, rec *Mutations) (sha string, committed bool, err error) {
	l, err := lock.AcquireWriteLock(BoardWriteLockPath(hub))
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: acquire board write lock: %w", err)
	}
	defer func() { _ = l.Release() }()

	boardDir := BoardDir(hub)
	if err := WriteCode(boardDir, code); err != nil {
		return "", false, err
	}
	rec.Append(KindFileWritten, filepath.Join(boardDir, CodeFileName), "")

	sha, committed, err = gitrepo.New(boardDir).StageAndCommit(message, ScopedPathspec("", []string{CodeFileName}))
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: commit %s: %w", CodeFileName, err)
	}
	if committed {
		rec.Append(KindCommitCreated, boardDir, sha)
	}
	return sha, committed, nil
}

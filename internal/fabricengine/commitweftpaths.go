// commitweftpaths.go holds the narrow weft commit helpers for explicit paths: a no-push commit and a content check.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// weftCommitPathspec returns the pathspec entries for relPaths, scoped under anchorRel.
// It is the single place the anchor join happens.
// Callers therefore never join AnchorRel or name the durable lym directory in a pathspec themselves.
func weftCommitPathspec(anchorRel string, relPaths []string) []string {
	return ScopedPathspec(anchorRel, relPaths)
}

// CommitRecordsPaths stages and commits exactly relPaths, scoped under anchorRel, in the weft
// worktree at weftPath. It takes the weft write lock itself and never pushes.
//
// Its body order is load-bearing:
//  1. opts.SkipGit returns ("", false, nil) with no lock taken, nothing staged, and nothing recorded.
//  2. An empty relPaths returns ("", false, nil) and records nothing: nothing staged is never reported as a commit.
//  3. The weft write lock is acquired and released by a deferred call;
//     an acquisition failure is wrapped as "fabricengine: acquire weft write lock: %w".
//  4. The paths are staged and committed.
//     Only a nil error with committed true appends KindCommitCreated to rec at weftPath,
//     with the returned sha as the detail.
//
// Per the Mutation Record Invariant, a real weft commit must appear in the calling verb's mutation record.
// Recording therefore happens only once the commit has observably landed.
// A caller with no verb outcome of its own passes a throwaway recorder and discards it.
//
// opts.SkipPush is accepted and ignored because this helper never pushes.
// SyncOptions is kept whole rather than narrowed to a bool so callers can pass their options unchanged.
//
// The lock is taken here, not by the caller, because independent callers would otherwise race on the git index lock.
func CommitRecordsPaths(rec *Mutations, weftPath, anchorRel string, relPaths []string, msg string, opts SyncOptions) (sha string, committed bool, err error) {
	if opts.SkipGit {
		return "", false, nil
	}
	if len(relPaths) == 0 {
		return "", false, nil
	}

	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return "", false, err
	}
	l, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	defer func() { _ = l.Release() }()

	return commitRecordsPathsHeld(rec, weftPath, anchorRel, relPaths, msg, opts)
}

// commitRecordsPathsHeld stages and commits relPaths for a caller that already holds the weft write lock of weftPath.
// It takes no lock, so a caller that try-acquired the lock never blocks on a second acquisition of the same lock file.
// It expects non-empty relPaths and a caller that has already honoured opts.SkipGit; opts is not read here.
func commitRecordsPathsHeld(rec *Mutations, weftPath, anchorRel string, relPaths []string, msg string, opts SyncOptions) (sha string, committed bool, err error) {
	sha, committed, err = gitrepo.New(weftPath).StageAndCommit(msg, weftCommitPathspec(anchorRel, relPaths))
	if err != nil {
		return "", false, err
	}
	if committed {
		rec.Append(KindCommitCreated, weftPath, sha)
	}
	return sha, committed, nil
}

// CommitAnchoredPaths stages and commits relPaths in the run-records worktree of l, with no push.
// It resolves the commit target from l itself, so a caller needs no knowledge that the weft exists.
//
// relPaths are relative to the anchor, so callers never join AnchorRel themselves.
func CommitAnchoredPaths(rec *Mutations, l *lymcwd.Location, relPaths []string, msg string, opts SyncOptions) (sha string, committed bool, err error) {
	return CommitRecordsPaths(rec, RecordsWorktree(l), l.AnchorRel, relPaths, msg, opts)
}

// AnchoredPathHasContent reports whether the anchor-relative relPath matches content in the run-records worktree.
// Matching content is a path the worktree's index tracks or an untracked, non-ignored file on disk.
// Git refuses a commit pathspec that matches nothing,
// so a caller checks this before committing a directory that may be empty and untracked.
// A tracked file deleted from disk still matches, so its deletion can be committed.
func AnchoredPathHasContent(l *lymcwd.Location, relPath string) (bool, error) {
	return entryMatchesWeft(RecordsWorktree(l), weftCommitPathspec(l.AnchorRel, []string{relPath})[0])
}

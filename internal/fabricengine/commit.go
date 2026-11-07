// commit.go holds CommitResult, PartialCommitError, and Fabric.Commit.
// Fabric.Commit splits a mixed file list into a warp-side plain-git commit;
// the weft side is a commit that carries trailers.
// Both sides commit under one combined write lock whenever anything is committed,
// and the async push is spawned after that lock is released.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// CommitResult reports what Fabric.Commit did on each side: the SHA that landed and whether a commit was made.
// Unchanged content is a legitimate no-op, so a side can report that no commit was made.
// It embeds MutationRecord, which holds the mutations the call recorded.
type CommitResult struct {
	MutationRecord
	WarpSHA       string
	WarpCommitted bool
	WeftSHA       string
	WeftCommitted bool
}

// Committed reports whether Fabric.Commit landed a commit on either side.
// Read this rather than the four raw fields, which stay exported because fabriccli prints them.
func (r CommitResult) Committed() bool {
	return r.WarpCommitted || r.WeftCommitted
}

// PartialCommitError reports Fabric.Commit's weft-side failure.
// weftCommitted is true when the weft commit landed but recording it in the correspondence index failed.
// It is false when the weft commit failed entirely.
// warpSHA and weftSHA report whatever landed; warpSHA is empty when no warp commit landed.
// The fields are unexported: callers match the type with errors.As and use only the Error message.
type PartialCommitError struct {
	warpSHA       string
	weftSHA       string
	weftCommitted bool
	Err           error
}

// Error implements the error interface.
// The warp clause appears only when warpSHA is set, since a tags-only call can fail weft-side with no warp commit.
func (e *PartialCommitError) Error() string {
	warpClause := "no warp commit"
	if e.warpSHA != "" {
		warpClause = fmt.Sprintf("warp commit %s landed", e.warpSHA)
	}
	if e.weftCommitted {
		return fmt.Sprintf("fabricengine: %s, weft commit %s landed but was not recorded in the correspondence index: %v", warpClause, e.weftSHA, e.Err)
	}
	return fmt.Sprintf("fabricengine: %s, weft commit failed: %v", warpClause, e.Err)
}

// Unwrap returns the wrapped error, so errors.Is/errors.As reach it.
func (e *PartialCommitError) Unwrap() error {
	return e.Err
}

// spawnDetachedPushFn is a package-level test seam; tests swap it for a recorder.
var spawnDetachedPushFn = SpawnDetachedPush

// Commit classifies files into warp and weft paths against the repo-wide routing set, never a raw pathspec.
// It commits each side under one combined write lock, taken whenever a commit is about to land, warp-only included.
// The lock is released before the push is spawned.
//
// Commit hard-errors, before taking any lock, when the never-committed bucket is non-empty.
// The error names the first such path.
// A caller passing a path under a never-committed directory has a bug that must surface:
// dropping the path would hide it, and committing it would break the never-committed contract.
// Because the check precedes the lock, a rejected call mutates nothing and spawns no push.
//
// The weft side is taken when there are weft files or snapshot tags, unless opts.SkipGit is set.
// Snapshot tags force a weft commit: an empty one carries the tags when there is nothing else to commit.
// Commit(nil, msg, tags, opts) is a supported call: it records a baseline under those tags without weft content.
//
// Commit spawns the async, fire-and-forget push of whatever landed via spawnDetachedPushFn.
// It spawns that push only when something landed.
// The push covers both sides, or in the hub's prime the records side only;
// there the operator pushes the code branch.
// A call that lands nothing spawns no detached child.
// SpawnDetachedPush itself honors the skip environment variables.
func (f *Fabric) Commit(files []string, msg string, snapshotTags []string, opts SyncOptions) (res CommitResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	// Refuse before any classification or locking, as every sibling mutating verb refuses a recorded merge.
	// Foreign git-level merge state with no record, such as a plain git conflicted merge, is refused here too.
	// It pre-empts git's own "cannot do a partial commit during a merge" error.
	// The two refusals return different typed errors on purpose.
	// Fabric has no merge of its own in the foreign case, so *ErrMergeInProgress would misdirect the operator.
	// *ErrForeignMergeState names what clears that state, plain git, as every merge verb does.
	recordExists, err := f.mergeRecordExists()
	if err != nil {
		return CommitResult{}, err
	}
	if recordExists {
		return CommitResult{}, &ErrMergeInProgress{}
	}
	foreignState, err := f.foreignMergeStatePresent()
	if err != nil {
		return CommitResult{}, err
	}
	if foreignState {
		return CommitResult{}, &ErrForeignMergeState{}
	}

	l, err := lymcwd.ResolveWorktree(f.warpPath)
	if err != nil {
		return CommitResult{}, fmt.Errorf("fabricengine: resolve layout for %s: %w", f.warpPath, err)
	}
	cfg, err := LoadConfig(ConfigDirs(l))
	if err != nil {
		return CommitResult{}, err
	}
	routingNames := pathspecNames(cfg)

	// Built before taking any lock: a failure here lands no commit and spawns no push.
	pushRequest, err := statusPushRequest(l, "")
	if err != nil {
		return CommitResult{}, err
	}

	warpFiles, weftFiles, neverCommittedFiles := classifyPaths(l.AnchorRel, routingNames, structuralNeverCommittedDirs, files)
	if len(neverCommittedFiles) > 0 {
		return CommitResult{}, fmt.Errorf("fabricengine: refusing to commit %q: paths under %s are never committed", neverCommittedFiles[0], lymdirs.DotLymDirName)
	}
	weftSide := (len(weftFiles) > 0 || len(snapshotTags) > 0) && !opts.SkipGit

	result, partialErr, err := f.commitBothSides(warpFiles, weftFiles, weftSide, msg, snapshotTags, opts)
	if err != nil {
		return result, err
	}

	// Record from the populated CommitResult fields rather than from control flow.
	// A partial commit (below) still records whichever side actually landed.
	// The target is the worktree path the commit landed in; the detail is the SHA.
	if result.WarpCommitted {
		rec.Append(KindCommitCreated, f.warpPath, result.WarpSHA)
	}
	if result.WeftCommitted {
		rec.Append(KindCommitCreated, f.weftPath, result.WeftSHA)
	}

	if result.WarpCommitted || result.WeftCommitted {
		// The push is fire-and-forget: a child that cannot start leaves the commits local for the next push.
		_ = spawnDetachedPushFn(pushRequest)
	}

	if partialErr != nil {
		return result, partialErr
	}
	return result, nil
}

// commitBothSides runs the locked section that Commit dispatches to: it commits warp first, then weft.
// It takes the combined write lock (`.weft/weft.write.lock`) whenever anything will commit.
// The lock is scoped to this helper, not to Commit, so it is released before Commit spawns the push.
// A defer in Commit would hold the lock across that spawn.
//
// A warp failure returns at once with a zero CommitResult and the wrapped error, since nothing has landed.
// Otherwise, once the weft side, if any, has been attempted, its outcome maps onto a *PartialCommitError:
// the weft commit landed but failed to record, or it failed and did not land.
// On the non-error path the weft commit either landed or was a no-op.
func (f *Fabric) commitBothSides(warpFiles, weftFiles []string, weftSide bool, msg string, snapshotTags []string, opts SyncOptions) (CommitResult, *PartialCommitError, error) {
	committing := len(warpFiles) > 0 || weftSide

	if committing {
		lockDir, err := f.ensureWeftLockDir()
		if err != nil {
			return CommitResult{}, nil, err
		}
		l, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
		if err != nil {
			return CommitResult{}, nil, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
		}
		defer func() { _ = l.Release() }()
	}

	var result CommitResult

	if len(warpFiles) > 0 {
		warpSHA, warpCommitted, err := f.code.StageAndCommit(msg, warpFiles)
		if err != nil {
			return CommitResult{}, nil, fmt.Errorf("fabricengine: warp commit: %w", err)
		}
		result.WarpSHA = warpSHA
		result.WarpCommitted = warpCommitted
	}

	var partialErr *PartialCommitError
	if weftSide {
		weftSHA, weftCommitted, err := f.commitWeftLocked(weftFiles, msg, opts, snapshotTags...)
		switch {
		case err != nil && weftCommitted:
			result.WeftSHA = weftSHA
			result.WeftCommitted = true
			partialErr = &PartialCommitError{warpSHA: result.WarpSHA, weftSHA: weftSHA, weftCommitted: true, Err: err}
		case err != nil && !weftCommitted:
			partialErr = &PartialCommitError{warpSHA: result.WarpSHA, weftCommitted: false, Err: err}
		case err == nil && weftCommitted:
			result.WeftSHA = weftSHA
			result.WeftCommitted = true
		}
	}

	return result, partialErr, nil
}

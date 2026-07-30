// commit.go — CommitResult, PartialCommitError, and Fabric.Commit: the
// classify-and-dispatch two-sided commit that fans a caller's mixed file
// list into a warp-side plain-git commit and a weft-side trailer-bearing
// commit under one held write lock, per the warp-first-then-weft-under-one-lock
// Shared Decision.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
)

// CommitResult reports what Fabric.Commit actually did on each side: the SHA
// landed (if any) and whether a commit was actually made, independently for
// warp and weft — mirroring the "committed" bool gitrepo.StageAndCommit and
// commitWeftLocked already report, since a Fabric.Commit call over unchanged
// content is a legitimate no-op on either or both sides.
type CommitResult struct {
	WarpSHA       string
	WarpCommitted bool
	WeftSHA       string
	WeftCommitted bool
}

// PartialCommitError reports a Fabric.Commit call that landed a warp commit
// but did not complete cleanly on the weft side — see the
// partial-failure-report-three-outcomes Shared Decision. WeftCommitted
// distinguishes the two weft-side failure shapes it can wrap: true means the
// weft commit itself landed but RecordCorrespondence failed to persist an
// index entry afterwards — recoverable via an explicit RebuildIndex, since
// the landed commit's own Warp-SHA trailer remains the correspondence
// index's sole source of truth, and WeftSHAForWarpSHA's own one-shot rebuild
// fires only on a stale hit, never on the index miss a never-written entry
// produces; false means the weft commit itself failed, so nothing landed on
// that side. WarpSHA/WeftSHA name whatever DID land, for a caller (or an
// operator reading the error) to act on without re-deriving it.
type PartialCommitError struct {
	WarpSHA       string
	WeftSHA       string
	WeftCommitted bool
	Err           error
}

// Error implements the error interface.
func (e *PartialCommitError) Error() string {
	if e.WeftCommitted {
		return fmt.Sprintf("fabricengine: warp commit %s landed, weft commit %s landed but was not recorded in the correspondence index: %v", e.WarpSHA, e.WeftSHA, e.Err)
	}
	return fmt.Sprintf("fabricengine: warp commit %s landed, weft commit failed: %v", e.WarpSHA, e.Err)
}

// Unwrap returns the wrapped error, so errors.Is/errors.As reach it.
func (e *PartialCommitError) Unwrap() error {
	return e.Err
}

// spawnDetachedPushFn is a package-level test seam over SpawnDetachedPush —
// see the push-invocation-seam-for-tests Shared Decision. Every
// Fabric.Commit integration test swaps this for a recorder/no-op (with a
// deferred restore, and no t.Parallel()) so no real detached child is
// spawned from the test binary.
var spawnDetachedPushFn = SpawnDetachedPush

// Commit classifies files into warp-side and weft-side paths (via
// classifyPaths, passing relPath == "." per the relpath-is-dot-for-slice-2
// Shared Decision, against the wired name-set WiredNames(f.weftPath)
// resolves) and commits each side separately: the warp side first, with a
// bare msg and no trailer (the plain-git property — no correspondence
// entry), then — when at least one file is weft-side and opts.SkipGit is
// false — the weft side under the fabric-layer write lock, acquired before
// the warp commit and held across both. See the
// warp-first-then-weft-under-one-lock Shared Decision: this is why
// commitWeftLocked exists rather than calling the public, lock-acquiring
// CommitWeft, which would self-deadlock reacquiring its own non-reentrant
// lock. A warp-only commit (no weft-side files, or opts.SkipGit) takes no
// lock and silently drops snapshotTags — they ride the weft commit's
// trailer block, which does not exist on this path.
//
// On a warp commit failure, Commit returns immediately with a zero
// CommitResult and the wrapped warp error — nothing has landed yet, and the
// async push below is never reached. Otherwise, once the weft side (if any)
// has been attempted, Commit maps its three possible outcomes onto a
// *PartialCommitError per the partial-failure-report-three-outcomes Shared
// Decision: a failed-but-landed weft commit, a failed-and-unlanded weft
// commit, or (the non-error path) a landed or no-op weft commit.
//
// Finally, Commit fires the async, fire-and-forget push of whatever landed
// via spawnDetachedPushFn, but only when something actually landed
// (result.WarpCommitted || result.WeftCommitted) — a fully no-op call (e.g.
// an empty files list, or unchanged content on every side) spawns no
// detached child. The push is unconditional on opts here; skip-env gating
// (WEFT_SKIP_GIT/WEFT_SKIP_PUSH) is handled inside SpawnDetachedPush itself,
// per the async-push-both-sides-via-detached-child Shared Decision — the
// WarpCommitted || WeftCommitted guard here is a separate "did anything
// land" gate, not an opts gate.
func (f *Fabric) Commit(files []string, msg string, snapshotTags []string, opts SyncOptions) (CommitResult, error) {
	wiredNames, err := WiredNames(f.weftPath)
	if err != nil {
		return CommitResult{}, err
	}

	warpFiles, weftFiles := classifyPaths(".", wiredNames, files)
	weftSide := len(weftFiles) > 0 && !opts.SkipGit

	if weftSide {
		lockDir, err := f.ensureWeftLockDir()
		if err != nil {
			return CommitResult{}, err
		}
		l, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
		if err != nil {
			return CommitResult{}, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
		}
		defer func() { _ = l.Release() }()
	}

	var result CommitResult

	if len(warpFiles) > 0 {
		warpSHA, warpCommitted, err := f.Warp.StageAndCommit(msg, warpFiles)
		if err != nil {
			return CommitResult{}, fmt.Errorf("fabricengine: warp commit: %w", err)
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
			partialErr = &PartialCommitError{WarpSHA: result.WarpSHA, WeftSHA: weftSHA, WeftCommitted: true, Err: err}
		case err != nil && !weftCommitted:
			partialErr = &PartialCommitError{WarpSHA: result.WarpSHA, WeftCommitted: false, Err: err}
		case err == nil && weftCommitted:
			result.WeftSHA = weftSHA
			result.WeftCommitted = true
		}
	}

	if result.WarpCommitted || result.WeftCommitted {
		_ = spawnDetachedPushFn(f.warpPath, f.weftPath)
	}

	if partialErr != nil {
		return result, partialErr
	}
	return result, nil
}

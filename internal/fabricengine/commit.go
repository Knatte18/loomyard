// commit.go — CommitResult, PartialCommitError, and Fabric.Commit: the classify-and-dispatch
// two-sided commit that fans a caller's mixed file list into a warp-side plain-git commit and a
// weft-side trailer-bearing commit, both performed under one combined write lock whenever the call
// will commit anything at all — warp-only included — per the combined-commit-lock Shared Decision,
// with the lock released before the async push is spawned per commit-lock-scoped-to-commit-only.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// CommitResult reports what Fabric.Commit did on each side: landed SHA and whether a commit was
// made, mirroring gitrepo.StageAndCommit and commitWeftLocked since unchanged content is a
// legitimate no-op.
type CommitResult struct {
	WarpSHA       string
	WarpCommitted bool
	WeftSHA       string
	WeftCommitted bool
}

// Committed reports whether Fabric.Commit landed a commit on either side.
// This is the one result a consumer outside the owner set should read — the four raw fields stay
// exported for fabriccli, which prints them by design.
func (r CommitResult) Committed() bool {
	return r.WarpCommitted || r.WeftCommitted
}

// PartialCommitError reports Fabric.Commit's weft-side failure, distinguishing whether the weft
// commit itself landed (WeftCommitted=true, index recording failed) or failed entirely.
// WarpSHA/WeftSHA report whatever did land;
// does not imply a warp commit (tags-only case).
type PartialCommitError struct {
	WarpSHA       string
	WeftSHA       string
	WeftCommitted bool
	Err           error
}

// Error implements the error interface, including the warp clause only when WarpSHA is populated
// (tags-only calls can hit weft-side failures with no warp commit).
func (e *PartialCommitError) Error() string {
	warpClause := "no warp commit"
	if e.WarpSHA != "" {
		warpClause = fmt.Sprintf("warp commit %s landed", e.WarpSHA)
	}
	if e.WeftCommitted {
		return fmt.Sprintf("fabricengine: %s, weft commit %s landed but was not recorded in the correspondence index: %v", warpClause, e.WeftSHA, e.Err)
	}
	return fmt.Sprintf("fabricengine: %s, weft commit failed: %v", warpClause, e.Err)
}

// Unwrap returns the wrapped error, so errors.Is/errors.As reach it.
func (e *PartialCommitError) Unwrap() error {
	return e.Err
}

// spawnDetachedPushFn is a package-level test seam; tests swap it for a recorder.
var spawnDetachedPushFn = SpawnDetachedPush

// Commit classifies files into warp and weft paths against the repo-wide pathspec, commits each
// side under one combined write lock (acquired whenever anything lands, even warp-only), and fires
// async both-sides push after releasing the lock.
// A fully degenerate no-op takes no lock and spawns no push.
//
// weftSide — and therefore whether committing takes the combined lock and runs ensureWeftLockDir —
// is true whenever there are weft files OR snapshotTags is non-empty (and opts.SkipGit is false),
// per the tags-force-a-weft-commit Shared Decision: commitWeftLocked lands an empty weft commit
// carrying the tags when there is otherwise nothing to commit.
// This means a tags-only or warp-only-but-tagged call now takes the lock and runs ensureWeftLockDir
// where an earlier version of Commit did neither — that is correct, since the call is about to
// write to weft, but it is a real behavioural widening from a predicate that once looked only at
// weftFiles.
// Commit(nil, msg, tags, opts) — tags with zero files at all — is consequently a supported call
// shape, not an accident of the predicate: it is how a caller records a baseline (a warp SHA under
// a snapshot tag) without producing any weft content of its own, the standalone-snapshot use this
// design's write path serves without a new method.
//
// Finally, Commit fires the async, fire-and-forget push of whatever landed via spawnDetachedPushFn,
// but only when something actually landed (result.WarpCommitted || result.WeftCommitted) — a fully
// no-op call (e.g.
// an empty files list,
// or unchanged content on every side) spawns no detached child.
// The push is unconditional on opts here;
// skip-env gating (WEFT_SKIP_GIT/WEFT_SKIP_PUSH) is handled inside SpawnDetachedPush itself, per
// the async-push-both-sides-via-detached-child Shared Decision — the WarpCommitted || WeftCommitted
// guard here is a separate "did anything land" gate, not an opts gate.
func (f *Fabric) Commit(files []string, msg string, snapshotTags []string, opts SyncOptions) (CommitResult, error) {
	l, err := lyxcwd.ResolveWorktree(f.warpPath)
	if err != nil {
		return CommitResult{}, fmt.Errorf("fabricengine: resolve layout for %s: %w", f.warpPath, err)
	}
	wiredNames, err := RepoWiredNames(l)
	if err != nil {
		return CommitResult{}, err
	}

	warpFiles, weftFiles := classifyPaths(l.AnchorRel, wiredNames, files)
	weftSide := (len(weftFiles) > 0 || len(snapshotTags) > 0) && !opts.SkipGit

	result, partialErr, err := f.commitBothSides(warpFiles, weftFiles, weftSide, msg, snapshotTags, opts)
	if err != nil {
		return result, err
	}

	if result.WarpCommitted || result.WeftCommitted {
		_ = spawnDetachedPushFn(f.warpPath, f.weftPath)
	}

	if partialErr != nil {
		return result, partialErr
	}
	return result, nil
}

// commitBothSides performs the locked, warp-then-weft commit critical
// section Commit dispatches to: it acquires the combined write lock
// (`.weft/weft.write.lock`) whenever committing is true — computed by the
// caller as `len(warpFiles) > 0 || weftSide`, per the combined-commit-lock
// Shared Decision — releases it via a helper-scoped defer before returning,
// and performs the same warp-first-then-weft ordering, hard-error-on-warp-
// failure, and three-outcome *PartialCommitError mapping Commit always has.
// Scoping the lock to this helper (rather than a defer in Commit itself) is
// what keeps the lock released before Commit's own spawnDetachedPushFn call,
// per the commit-lock-scoped-to-commit-only Shared Decision: a
// function-scoped defer in Commit would still be held across that spawn,
// which the design forbids and a test asserts against.
//
// On a warp commit failure, commitBothSides returns immediately with a zero
// CommitResult and the wrapped warp error — nothing has landed yet. Otherwise,
// once the weft side (if any) has been attempted, it maps its three possible
// outcomes onto a *PartialCommitError per the
// partial-failure-report-three-outcomes Shared Decision: a failed-but-landed
// weft commit, a failed-and-unlanded weft commit, or (the non-error path) a
// landed or no-op weft commit.
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
		warpSHA, warpCommitted, err := f.Warp.StageAndCommit(msg, warpFiles)
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
			partialErr = &PartialCommitError{WarpSHA: result.WarpSHA, WeftSHA: weftSHA, WeftCommitted: true, Err: err}
		case err != nil && !weftCommitted:
			partialErr = &PartialCommitError{WarpSHA: result.WarpSHA, WeftCommitted: false, Err: err}
		case err == nil && weftCommitted:
			result.WeftSHA = weftSHA
			result.WeftCommitted = true
		}
	}

	return result, partialErr, nil
}

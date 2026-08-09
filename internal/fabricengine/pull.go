// pull.go — the unified Fabric.Pull entry point: weft ff-pull followed by warp
// fetch/inspect/reconcile, detecting a warp history rewrite (rebase or force-push) and, when it is
// safe to do so, re-anchoring weft's own correspondence to the new upstream tip via the existing
// empty-commit machinery.
// This file defines PullResult and *PartialPullError — the result and partial-failure contract
// batches 3-4 (the CLI and docs layers) consume — mirroring PartialCommitError's shape (commit.go),
// with the two sides' roles swapped to match Pull's weft-first ordering.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pattern"
)

// PullResult reports what Fabric.Pull actually did, on both sides independently, and — when a warp
// history rewrite forced a reconcile — the re-anchor baseline and the weft content a caller should
// treat as PATTERN-residue (potentially replayed against the wrong warp baseline).
type PullResult struct {
	// WeftPulled reports whether the weft ff-pull (PullWeft) ran and
	// succeeded. Every field below is only ever populated once this is true —
	// see Fabric.Pull's weft-first ordering.
	WeftPulled bool
	// WarpFetched reports whether the warp fetch (f.warp.Fetch) ran and
	// succeeded.
	WarpFetched bool
	// WarpAdvanced reports whether the warp branch pointer actually moved,
	// either via a clean fast-forward reset or a reconcile reset.
	WarpAdvanced bool
	// NewWarpHEAD is the fetched upstream tip SHA warp advanced to. It is
	// empty when warp did not advance (already up to date, or an abort path).
	NewWarpHEAD string
	// RewriteDetected reports whether the warp pull was a non-fast-forward —
	// i.e. local warp HEAD is not an ancestor of the fetched upstream tip,
	// meaning upstream's history was rewritten (rebase/force-push), not
	// merely advanced.
	RewriteDetected bool
	// Reconciled reports whether a re-anchor weft commit was written to
	// re-bind weft's correspondence to the new warp HEAD after a rewrite.
	Reconciled bool
	// AnchorWarpSHA is the surviving correspondence entry's warp SHA — the
	// confirmed re-anchor baseline reachableAnchor resolved. Populated only
	// when Reconciled is true.
	AnchorWarpSHA string
	// AnchorWeftSHA is that same entry's weft SHA — the baseline the
	// PATTERN-residue range (PatternResidue) starts from. Populated only when
	// Reconciled is true.
	AnchorWeftSHA string
	// ReanchorWeftSHA is the new empty weft anchor commit's own SHA, bound to
	// NewWarpHEAD via its Warp-SHA trailer. Populated only when Reconciled is
	// true.
	ReanchorWeftSHA string
	// PatternResidue lists the post-anchor weft commits (between
	// AnchorWeftSHA and the weft HEAD at reconcile time) that touched
	// _lyx/PATTERN.md or _lyx/pattern/... paths — content a caller should
	// treat as potentially stale against the new warp baseline. Populated
	// only when Reconciled is true.
	PatternResidue []PatternResidueEntry
}

// PatternResidueEntry names one post-anchor weft commit and the
// _lyx/PATTERN.md or _lyx/pattern/... paths it touched, as enumerated by
// Fabric.Pull's reconcile branch (see patternResidueCommits).
type PatternResidueEntry struct {
	WeftSHA string
	Paths   []string
}

// PartialPullError reports a Fabric.Pull call whose weft side completed cleanly but whose warp-side
// work did not — mirroring PartialCommitError's shape (commit.go) with the two sides' roles
// swapped, per the weft-first-ordering / report-not-rollback Shared Decision.
// WeftPulled is always true for this type: a weft-side failure never produces a *PartialPullError
// at all, since Fabric.Pull returns immediately on that path (see Fabric.Pull's doc comment).
// Stage names which warp-side step failed (e.g. "fetch", "reset", "reanchor"), so a caller (or an
// operator reading the error) knows exactly where the call stopped without re-deriving it from
// Err's message.
type PartialPullError struct {
	WeftPulled bool
	Stage      string
	Err        error
}

// Error implements the error interface, stating that weft succeeded and naming the warp-side stage
// that failed.
func (e *PartialPullError) Error() string {
	return fmt.Sprintf("fabricengine: weft pull succeeded, warp %s failed: %v", e.Stage, e.Err)
}

// Unwrap returns the wrapped error, so errors.Is/errors.As reach it.
func (e *PartialPullError) Unwrap() error {
	return e.Err
}

// ErrWarpDivergedUnpushed is returned by Fabric.Pull when the warp remote's history has been
// rewritten AND local warp already carries unpushed commits of its own — the double-conflict case
// Fabric.Pull refuses to reconcile automatically, since resolving it would require deciding what
// happens to the caller's own unpushed work.
// Fabric.Pull makes no change to either repo when this is returned.
var ErrWarpDivergedUnpushed = errors.New("fabricengine: warp remote diverged and local warp has unpushed commits; aborting, no changes")

// ErrNoSurvivingAnchor is returned by Fabric.Pull when the warp remote's history has been rewritten
// so thoroughly that no entry in the correspondence index survives — reachableAnchor found nothing
// reachable from the new upstream tip — leaving no safe baseline to re-anchor weft against.
// Fabric.Pull makes no change to either repo when this is returned.
var ErrNoSurvivingAnchor = errors.New("fabricengine: warp history rewritten and no recorded correspondence survives; aborting, no changes")

// warpUpstreamSHA resolves the warp repo's already-fetched upstream tracking
// ref (`@{u}`) to a plain hex SHA, via `git rev-parse @{u}` in f.warpPath.
// Fabric.Pull calls this AFTER f.warp.Fetch has refreshed the remote-tracking
// ref, so the SHA it returns is the freshly fetched upstream tip — usable
// directly by ResetHard and IsAncestor, which both require a plain commit
// SHA rather than symbolic revision syntax.
func (f *Fabric) warpUpstreamSHA() (string, error) {
	stdout, stderr, code, err := gitexec.RunGit([]string{"rev-parse", "@{u}"}, f.warpPath)
	if err != nil {
		return "", fmt.Errorf("fabricengine: rev-parse @{u} in %s: %w", f.warpPath, err)
	}
	if code != 0 {
		return "", fmt.Errorf("fabricengine: git rev-parse @{u} in %s: %s", f.warpPath, stderr)
	}
	return strings.TrimSpace(stdout), nil
}

// Pull is fabric's unified pull entry point: it pulls weft first, then fetches and inspects warp,
// reconciling weft's correspondence when warp's history has been rewritten.
// A warp-side failure reports the accumulated result rather than unwinding the weft pull
// (weft-first-ordering / report-not-rollback).
func (f *Fabric) Pull(opts SyncOptions) (PullResult, error) {
	if opts.SkipGit {
		return PullResult{}, nil
	}

	var result PullResult

	if err := f.PullWeft(opts); err != nil {
		return PullResult{}, fmt.Errorf("fabricengine: weft pull: %w", err)
	}
	result.WeftPulled = true
	hadUnpushed, err := f.warp.HasUnpushed()
	if err != nil {
		return result, &PartialPullError{WeftPulled: true, Stage: "unpushed-check", Err: err}
	}

	if err := f.warp.Fetch(); err != nil {
		return result, &PartialPullError{WeftPulled: true, Stage: "fetch", Err: err}
	}
	result.WarpFetched = true

	upstreamSHA, err := f.warpUpstreamSHA()
	if err != nil {
		return result, &PartialPullError{WeftPulled: true, Stage: "resolve", Err: err}
	}
	localHEAD, err := f.warp.CurrentSHA()
	if err != nil {
		return result, &PartialPullError{WeftPulled: true, Stage: "resolve", Err: err}
	}

	if localHEAD == upstreamSHA {
		return result, nil
	}

	isFF, err := f.warp.IsAncestor(localHEAD, upstreamSHA)
	if err != nil {
		return result, &PartialPullError{WeftPulled: true, Stage: "classify", Err: err}
	}

	if isFF {
		if err := f.warp.ResetHard(upstreamSHA); err != nil {
			return result, &PartialPullError{WeftPulled: true, Stage: "reset", Err: err}
		}
		result.WarpAdvanced = true
		result.NewWarpHEAD = upstreamSHA
		return result, nil
	}

	result.RewriteDetected = true

	if hadUnpushed {
		return result, ErrWarpDivergedUnpushed
	}

	path, err := f.corrIndexPath()
	if err != nil {
		return result, &PartialPullError{WeftPulled: true, Stage: "load-index", Err: err}
	}
	ix, err := loadCorrIndex(path)
	if err != nil {
		return result, &PartialPullError{WeftPulled: true, Stage: "load-index", Err: err}
	}
	entries := ix.entries()

	if len(entries) == 0 {
		if err := f.warp.ResetHard(upstreamSHA); err != nil {
			return result, &PartialPullError{WeftPulled: true, Stage: "reset", Err: err}
		}
		result.WarpAdvanced = true
		result.NewWarpHEAD = upstreamSHA
		return result, nil
	}

	anchor, found, err := reachableAnchor(entries, func(sha string) (bool, error) {
		return f.warp.IsAncestor(sha, upstreamSHA)
	})
	if err != nil {
		return result, &PartialPullError{WeftPulled: true, Stage: "anchor-walk", Err: err}
	}
	if !found {
		return result, ErrNoSurvivingAnchor
	}

	if err := f.warp.ResetHard(upstreamSHA); err != nil {
		return result, &PartialPullError{WeftPulled: true, Stage: "reset", Err: err}
	}
	result.WarpAdvanced = true
	result.NewWarpHEAD = upstreamSHA
	result.AnchorWarpSHA = anchor.WarpSHA
	result.AnchorWeftSHA = anchor.WeftSHA

	weftHEADBeforeAnchor, _ := f.weft.CurrentSHA()

	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return result, &PartialPullError{WeftPulled: true, Stage: "reanchor", Err: err}
	}
	l, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return result, &PartialPullError{WeftPulled: true, Stage: "reanchor", Err: fmt.Errorf("fabricengine: acquire weft write lock: %w", err)}
	}
	defer func() { _ = l.Release() }()

	msg := appendWarpSHATrailer("fabric: re-anchor weft after warp rebase", upstreamSHA)
	reanchorSHA, _, err := f.commitEmptySnapshot(msg, upstreamSHA)
	if err != nil {
		return result, &PartialPullError{WeftPulled: true, Stage: "reanchor", Err: err}
	}
	result.Reconciled = true
	result.ReanchorWeftSHA = reanchorSHA

	residue, err := f.patternResidueCommits(anchor.WeftSHA, weftHEADBeforeAnchor)
	if err != nil {
		return result, &PartialPullError{WeftPulled: true, Stage: "residue", Err: err}
	}
	result.PatternResidue = residue

	return result, nil
}

// patternResidueCommits enumerates the weft commits in the exclusive range
// fromWeftSHA..toWeftSHA that touch _lyx/PATTERN.md or _lyx/pattern/...
// paths, via one `git log --name-only` invocation in f.weftPath. This is
// Fabric.Pull's reconcile-branch helper: after a re-anchor, every weft
// commit between the old anchor and weft HEAD at reconcile time was written
// against a warp baseline that no longer exists on the rewritten upstream,
// and any of them touching those PATTERN paths is exactly the content a
// caller must treat as potentially stale.
//
// The pathspec strings (pattern.PathspecFile, pattern.PathspecDir) come from
// internal/pattern's exported constants, themselves built from
// lyxdirs.LyxDirName — internal/pattern is the single declarer of the
// PATTERN path segments. Building these strings from lyxdirs.LyxDirName
// rather than an inline literal is a review obligation, not a
// machine-enforced one: TestEnforcement_GeometryLiterals matches whole
// tokens by exact equality and cannot see "_lyx/PATTERN.md".
//
// Separator placement: unlike scanWarpSHATrailers (which uses no
// --name-only), --name-only appends each commit's changed-file list as
// separate lines AFTER that commit's --format output, so a trailing record
// separator would land between one commit's own SHA and its own file list,
// misassigning paths to the wrong commit. The record separator is therefore
// placed at the START of the --format string, delimiting the boundary BEFORE
// each commit's SHA rather than after it. warpSHATrailerFormatUnitSep and
// warpSHATrailerFormatRecordSep (index.go) are reused unchanged, so the split
// can never be confused by ordinary commit content.
//
// Anchor scope: the pathspec is pattern.PathspecFile/PathspecDir joined onto the pair's recorded
// anchor via ScopedPathspec, the same way Fabric.Commit scopes its own routing prefixes.
// A root pathspec would report an empty residue on a subpath-anchored hub — telling a caller
// "nothing needs review" for exactly the commits that do, since that hub's PATTERN content lives at
// <anchor>/_lyx/PATTERN.md and never at the weft worktree root.
//
// If fromWeftSHA == toWeftSHA there are no post-anchor commits at all, so
// this returns (nil, nil) without spawning git. A non-zero git exit returns a
// wrapped error; a real range with zero PATTERN-path-touching commits (empty
// git-log output) also returns (nil, nil).
func (f *Fabric) patternResidueCommits(fromWeftSHA, toWeftSHA string) ([]PatternResidueEntry, error) {
	if fromWeftSHA == toWeftSHA {
		return nil, nil
	}

	l, err := lyxcwd.ResolveWorktree(f.warpPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: resolve anchor for %s: %w", f.warpPath, err)
	}

	format := warpSHATrailerFormatRecordSep + "%H" + warpSHATrailerFormatUnitSep
	rangeArg := fromWeftSHA + ".." + toWeftSHA
	args := []string{"log", "--name-only", "--format=" + format, rangeArg, "--"}
	for _, spec := range ScopedPathspec(l.AnchorRel, []string{pattern.PathspecFile, pattern.PathspecDir}) {
		args = append(args, filepath.ToSlash(spec))
	}

	stdout, stderr, code, err := gitexec.RunGit(args, f.weftPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: git log --name-only %s in %s: %w", rangeArg, f.weftPath, err)
	}
	if code != 0 {
		return nil, fmt.Errorf("fabricengine: git log --name-only %s in %s: %s", rangeArg, f.weftPath, stderr)
	}

	return parsePatternResidueRecords(stdout), nil
}

// parsePatternResidueRecords parses patternResidueCommits' git-log output —
// one warpSHATrailerFormatRecordSep-delimited block per commit, each block
// starting with "<SHA><unitSep>" followed by that commit's changed
// _lyx/PATTERN.md or _lyx/pattern/... paths (one per line, from
// --name-only) — into one
// PatternResidueEntry per commit. Factored out as a pure helper so the
// record-boundary parsing itself is easy to reason about independently of
// the git spawn around it.
func parsePatternResidueRecords(output string) []PatternResidueEntry {
	var entries []PatternResidueEntry
	for _, record := range strings.Split(output, warpSHATrailerFormatRecordSep) {
		record = strings.Trim(record, "\n")
		if record == "" {
			continue
		}

		parts := strings.SplitN(record, warpSHATrailerFormatUnitSep, 2)
		sha := strings.TrimSpace(parts[0])
		if sha == "" {
			continue
		}

		var paths []string
		if len(parts) > 1 {
			for _, line := range strings.Split(parts[1], "\n") {
				path := strings.TrimSpace(line)
				if path != "" {
					paths = append(paths, path)
				}
			}
		}
		entries = append(entries, PatternResidueEntry{WeftSHA: sha, Paths: paths})
	}
	return entries
}

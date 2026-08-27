// pull.go — the unified Fabric.Pull entry point: weft ff-pull followed by warp
// fetch/inspect/reconcile, detecting a warp history rewrite (rebase or force-push) and, when it is
// safe to do so, re-anchoring weft's own correspondence to the new upstream tip via the existing
// empty-commit machinery.
// This file defines PullResult and *PartialPullError — the result and partial-failure contract
// batches 3-4 (the CLI and docs layers) consume — mirroring PartialCommitError's shape (commit.go),
// with the two sides' roles swapped: PartialPullError reports a call whose warp-side work did not
// complete, regardless of whether the weft arm (now non-fatal) completed alongside it.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pattern"
)

// PullResult reports what Fabric.Pull actually did, on both sides independently, and — when a warp
// history rewrite forced a reconcile — the re-anchor baseline and the weft content a caller should
// treat as PATTERN-residue (potentially replayed against the wrong warp baseline).
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type PullResult struct {
	MutationRecord
	// WeftPulled reports whether the weft ff-pull (PullWeft) ran and
	// succeeded — or was skipped as a vacuous no-op because the weft branch
	// has no upstream yet (a freshly bootstrapped hub whose suffixed primary
	// branch exists only locally until the first push lands; there is nothing
	// to fast-forward from, so skipping is success, not failure).
	// A failed upstream probe or a failed weft pull leaves this false and
	// does not stop the call: Fabric.Pull's weft arm is non-fatal, and the
	// warp fetch/reconcile below runs regardless of this field's value.
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

// PartialPullError reports a Fabric.Pull call whose warp-side work did not complete — mirroring
// PartialCommitError's shape (commit.go) with the two sides' roles swapped, per the
// weft-first-ordering / report-not-rollback Shared Decision.
// WeftPulled faithfully reports whether the weft arm completed, which may now be false: since the
// weft arm became non-fatal, a *PartialPullError can be returned with the weft side never having
// pulled at all.
// What has NOT changed: this type still never reports a weft-side failure on its own — a weft-side
// failure alone is no longer an error at all (Fabric.Pull warns and continues), so *PartialPullError
// is only ever constructed alongside a genuine warp-side failure.
// Stage names which warp-side step failed (e.g. "fetch", "reset", "reanchor"), so a caller (or an
// operator reading the error) knows exactly where the call stopped without re-deriving it from
// Err's message.
type PartialPullError struct {
	WeftPulled bool
	Stage      string
	Err        error
}

// Error implements the error interface, naming the warp-side stage that failed and, depending on
// WeftPulled, either confirming the weft pull succeeded alongside it or naming the weft pull as a
// second failure.
func (e *PartialPullError) Error() string {
	if e.WeftPulled {
		return fmt.Sprintf("fabricengine: weft pull succeeded, warp %s failed: %v", e.Stage, e.Err)
	}
	return fmt.Sprintf("fabricengine: weft pull did not complete, warp %s also failed: %v", e.Stage, e.Err)
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

// ErrWarpDirty is returned by Fabric.Pull when warp would have to move (fast-forward or reconcile)
// while the warp worktree carries uncommitted tracked changes.
// Every warp advance goes through ResetHard, which silently discards uncommitted tracked
// modifications — strictly more destructive than the plain `git pull` an external actor would run —
// so Pull refuses before mutating warp instead, mirroring Checkout's dirty-weft refusal.
// The weft side has already been fast-forwarded when this is returned; warp is untouched.
var ErrWarpDirty = errors.New("fabricengine: warp worktree has uncommitted changes; commit or stash them, then re-run pull; aborting, no warp changes")

// weftHasUpstream reports whether the weft worktree's current branch has a configured upstream
// tracking ref.
// A nonzero exit from rev-parse @{u} means no upstream (or a detached HEAD), which for Pull's weft
// step is the nothing-to-pull-from case, never an error.
func (f *Fabric) weftHasUpstream() (bool, error) {
	_, err := gitexec.Run([]string{"rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"}, f.weftPath)
	if err == nil {
		return true, nil
	}
	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) {
		return false, nil
	}
	return false, fmt.Errorf("fabricengine: resolve weft upstream in %s: %w", f.weftPath, err)
}

// warpWorktreeDirty reports whether the warp worktree carries uncommitted TRACKED changes — the
// state ResetHard would silently destroy.
// Untracked files are deliberately excluded: reset --hard leaves them alone, so they are no reason
// to refuse a pull.
func (f *Fabric) warpWorktreeDirty() (bool, error) {
	dirty, _, err := worktreeDirty(scopeTracked, f.warpPath)
	if err != nil {
		return false, fmt.Errorf("fabricengine: git status in %s: %w", f.warpPath, err)
	}
	return dirty, nil
}

// weftSHAOrEmpty returns f's weft repo's current HEAD SHA, mapping an unborn HEAD (no commits) to
// ("", nil) — mirroring coalesce.go's headOrEmpty, the in-repo precedent for this exact tolerance,
// adapted to a repo handle already in hand rather than a path.
func weftSHAOrEmpty(f *Fabric) (string, error) {
	sha, err := f.weft.CurrentSHA()
	if err == nil {
		return sha, nil
	}
	if errors.Is(err, gitrepo.ErrNoCommits) {
		return "", nil
	}
	return "", err
}

// recordWarpAdvance records KindRepoAdvanced at f.warpPath with the new warp HEAD as Detail when a
// ResetHard call this method's caller just made genuinely moved HEAD past before — the same
// before/after CurrentSHA() predicate card 19 uses for push, applied to the warp advance here.
// A reset to the SHA warp already carries advances nothing and must record nothing: ResetHard's own
// worktree_reset entry (destroy.go) still fires either way, since that primitive ran regardless of
// whether it moved anything — this entry names the effect, that one names the primitive.
// If the after-sample errors, nothing is recorded and the error is not propagated: a failure to
// observe is not a failure to advance.
func (f *Fabric) recordWarpAdvance(rec *Mutations, before string) {
	after, err := f.warp.CurrentSHA()
	if err != nil || after == before {
		return
	}
	rec.Append(KindRepoAdvanced, f.warpPath, after)
}

// warpUpstreamSHA resolves the warp repo's already-fetched upstream tracking
// ref (`@{u}`) to a plain hex SHA, via `git rev-parse @{u}` in f.warpPath.
// Fabric.Pull calls this AFTER f.warp.Fetch has refreshed the remote-tracking
// ref, so the SHA it returns is the freshly fetched upstream tip — usable
// directly by ResetHard and IsAncestor, which both require a plain commit
// SHA rather than symbolic revision syntax.
func (f *Fabric) warpUpstreamSHA() (string, error) {
	stdout, err := gitexec.Run([]string{"rev-parse", "@{u}"}, f.warpPath)
	if err != nil {
		return "", fmt.Errorf("fabricengine: rev-parse @{u} in %s: %w", f.warpPath, err)
	}
	return strings.TrimSpace(stdout), nil
}

// Pull is fabric's unified pull entry point: it attempts the weft ff-pull first, then fetches and
// inspects warp, reconciling weft's correspondence when warp's history has been rewritten.
// The weft arm is non-fatal: a failed upstream probe or a failed weft `git pull --ff-only` is
// logged as a warning and leaves PullResult.WeftPulled false, but the warp fetch/reconcile runs
// regardless — a weft that has locally diverged from its own upstream (for example, a status push
// rejected and warned past on an earlier call) must never stall the warp side's own resume.
// A warp-side failure reports the accumulated result rather than unwinding the weft pull
// (weft-first-ordering / report-not-rollback).
// Reconciling a weft that failed to pull is a named manual operator step — `git -C <weft> reset
// --hard origin/<branch>` — never something Pull resolves by rewriting history on the caller's
// behalf, since a push rejection means another machine has already advanced the same FSM state.
func (f *Fabric) Pull(opts SyncOptions) (res PullResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit {
		return PullResult{}, nil
	}

	// A pull hard-resets and re-anchors warp, which would discard an in-progress merge — refuse
	// before any mutation whenever a fabric merge record exists on the pair. Record-only: the
	// foreign-state disposition is Commit's alone, per the lock decision's consequence table.
	recordExists, err := f.mergeRecordExists()
	if err != nil {
		return PullResult{}, err
	}
	if recordExists {
		return PullResult{}, &ErrMergeInProgress{}
	}

	var result PullResult

	// A weft branch with no upstream has nothing to fast-forward from — the freshly bootstrapped
	// hub's suffixed primary exists only locally until the first push lands — so the weft pull is
	// skipped as a vacuous success rather than surfacing git's "no tracking information" failure.
	// A probe failure is non-fatal: it is warned and treated as "nothing to pull from" so the warp
	// side still runs, rather than stalling the whole call on a weft-side observation failure.
	weftHasUpstream, err := f.weftHasUpstream()
	if err != nil {
		logger.Warn("fabricengine: weft pull: resolve upstream failed, continuing to warp", "weft", f.weftPath, "err", err)
	} else if weftHasUpstream {
		// Sample the weft SHA before and after the pull, and record KindRepoAdvanced only on a
		// change — PullWeft's own f.weft.Pull() also returns nil when the weft is already up to
		// date, so an unconditional entry would fabricate a mutation on that no-op path.
		// This is load-bearing: PullWeft can succeed (result.WeftPulled = true below) and Pull can
		// then still return a *PartialPullError on the warp side with no commit ever created and no
		// gate primitive ever run — without this entry the record would be empty and partial would
		// read false while the weft worktree had genuinely been advanced.
		// gitrepo.ErrNoCommits is tolerated exactly as headOrEmpty (coalesce.go) tolerates it: an
		// unborn weft HEAD reports as "" rather than an error, so a genuinely-empty repo before the
		// pull is a legitimate before-sample rather than an observation failure.
		beforeWeftSHA, beforeErr := weftSHAOrEmpty(f)
		if err := f.PullWeft(opts); err != nil {
			// A rejected/diverged weft is routine once a status push may be warned past — warn and
			// fall through to the warp side rather than stalling the operator's whole resume verb.
			// Recovery is a named manual step: `git -C <weft> reset --hard origin/<branch>`.
			logger.Warn("fabricengine: weft pull failed, continuing to warp", "weft", f.weftPath, "err", err)
		} else {
			result.WeftPulled = true
			if beforeErr == nil {
				if afterWeftSHA, afterErr := weftSHAOrEmpty(f); afterErr == nil && afterWeftSHA != beforeWeftSHA {
					rec.Append(KindRepoAdvanced, f.weftPath, afterWeftSHA)
				}
			}
		}
	} else {
		result.WeftPulled = true
	}
	hadUnpushed, err := f.warp.HasUnpushed()
	if err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "unpushed-check", Err: err}
	}

	if err := f.warp.Fetch(); err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "fetch", Err: err}
	}
	result.WarpFetched = true

	upstreamSHA, err := f.warpUpstreamSHA()
	if err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "resolve", Err: err}
	}
	localHEAD, err := f.warp.CurrentSHA()
	if err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "resolve", Err: err}
	}

	if localHEAD == upstreamSHA {
		return result, nil
	}

	// Every remaining branch moves warp via ResetHard, which discards uncommitted tracked changes
	// without a trace — so a dirty warp worktree is refused here, before anything mutates warp.
	dirty, err := f.warpWorktreeDirty()
	if err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "dirty-check", Err: err}
	}
	if dirty {
		return result, ErrWarpDirty
	}

	isFF, err := f.warp.IsAncestor(localHEAD, upstreamSHA)
	if err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "classify", Err: err}
	}

	if isFF {
		if err := f.ResetHard(rec, upstreamSHA); err != nil {
			return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "reset", Err: err}
		}
		f.recordWarpAdvance(rec, localHEAD)
		result.WarpAdvanced = true
		result.NewWarpHEAD = upstreamSHA
		return result, nil
	}

	result.RewriteDetected = true

	if hadUnpushed {
		return result, ErrWarpDivergedUnpushed
	}

	// The index is a rebuildable cache, never authoritative on its own — and here it decides whether
	// the pair can recover at all, so it must be rebuilt from the weft trailer history (the sole
	// source of truth, already fast-forwarded above) before the anchor walk.
	// A re-cloned hub starts with an empty per-pair index while its adopted weft history carries
	// every recorded anchor; without the rebuild, the walk missed those surviving anchors and
	// returned a false ErrNoSurvivingAnchor that no later call could ever clear.
	if err := f.RebuildIndex(); err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "load-index", Err: err}
	}
	path, err := f.corrIndexPath()
	if err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "load-index", Err: err}
	}
	ix, err := loadCorrIndex(path)
	if err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "load-index", Err: err}
	}
	entries := ix.entries()

	if len(entries) == 0 {
		if err := f.ResetHard(rec, upstreamSHA); err != nil {
			return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "reset", Err: err}
		}
		f.recordWarpAdvance(rec, localHEAD)
		result.WarpAdvanced = true
		result.NewWarpHEAD = upstreamSHA
		return result, nil
	}

	anchor, found, err := reachableAnchor(entries, func(sha string) (bool, error) {
		return f.warp.IsAncestor(sha, upstreamSHA)
	})
	if err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "anchor-walk", Err: err}
	}
	if !found {
		return result, ErrNoSurvivingAnchor
	}

	if err := f.ResetHard(rec, upstreamSHA); err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "reset", Err: err}
	}
	f.recordWarpAdvance(rec, localHEAD)
	result.WarpAdvanced = true
	result.NewWarpHEAD = upstreamSHA
	result.AnchorWarpSHA = anchor.WarpSHA
	result.AnchorWeftSHA = anchor.WeftSHA

	weftHEADBeforeAnchor, _ := f.weft.CurrentSHA()

	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "reanchor", Err: err}
	}
	l, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "reanchor", Err: fmt.Errorf("fabricengine: acquire weft write lock: %w", err)}
	}
	defer func() { _ = l.Release() }()

	msg := appendWarpSHATrailer("fabric: re-anchor weft after warp rebase", upstreamSHA)
	reanchorSHA, _, err := f.commitEmptySnapshot(msg, upstreamSHA)
	if err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "reanchor", Err: err}
	}
	result.Reconciled = true
	result.ReanchorWeftSHA = reanchorSHA

	residue, err := f.patternResidueCommits(anchor.WeftSHA, weftHEADBeforeAnchor)
	if err != nil {
		return result, &PartialPullError{WeftPulled: result.WeftPulled, Stage: "residue", Err: err}
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

	stdout, err := gitexec.Run(args, f.weftPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: scan PATTERN residue over %s in %s: %w", rangeArg, f.weftPath, err)
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

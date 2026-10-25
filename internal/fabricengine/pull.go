// pull.go — the unified Fabric.Pull entry point: weft ff-pull followed by warp
// fetch/inspect/reconcile, detecting a warp history rewrite (rebase or force-push) and, when it is
// safe to do so, re-anchoring weft's own correspondence to the new upstream tip via the existing
// empty-commit machinery.
// This file defines PullResult and *PartialPullError — the result and partial-failure contract
// batches 3-4 (the CLI and docs layers) consume — mirroring PartialCommitError's shape (commit.go),
// with the two sides' roles swapped: PartialPullError reports a call whose warp-side work did not
// complete, regardless of whether the weft arm completed alongside it.
// A weft that cannot be brought level with origin is the other half of that contract: a *WeftStopError.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// PullResult reports what Fabric.Pull actually did, on both sides independently, and — when a warp
// history rewrite forced a reconcile — the re-anchor baseline.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type PullResult struct {
	MutationRecord
	// WeftPulled reports whether the weft was brought level with origin, by a fast-forward or a replay of its local commits onto origin's.
	// It is also true when the weft branch has no upstream yet, as on a freshly bootstrapped hub whose suffixed primary branch exists only locally until the first push lands: there is nothing to pull from, so skipping is success, not failure.
	// A weft that origin moved but a conflict or an uncommitted change keeps from being brought level is a *WeftStopError, never a false field.
	// The field is false only on that error and when the weft's origin could not be reached.
	// The warp fetch/reconcile runs regardless of its value.
	WeftPulled bool
	// WarpFetched reports whether the warp fetch (f.code.Fetch) ran and
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
	// weft-side baseline of the re-anchor. Populated only when Reconciled is true.
	AnchorWeftSHA string
	// ReanchorWeftSHA is the new empty weft anchor commit's own SHA, bound to
	// NewWarpHEAD via its Warp-SHA trailer. Populated only when Reconciled is
	// true.
	ReanchorWeftSHA string
}

// PartialPullError reports a Fabric.Pull call whose warp-side work did not complete — mirroring
// PartialCommitError's shape (commit.go) with the two sides' roles swapped, per the
// weft-first-ordering / report-not-rollback Shared Decision.
// WeftPulled faithfully reports whether the weft arm completed, which may be false.
// A *PartialPullError can be returned with the weft side never having pulled at all.
// This type never reports a weft-side failure on its own: a weft stop is a *WeftStopError, joined with this error when both sides failed.
// So *PartialPullError is only ever constructed alongside a genuine warp-side failure.
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
// The weft arm has already RUN when this is returned, but no longer necessarily SUCCEEDED: it
// became non-fatal, so a failed upstream probe or a failed weft `git pull --ff-only` warns and
// falls through to exactly this check. Read PullResult.WeftPulled for which of the two happened.
// Warp itself is untouched either way.
var ErrWarpDirty = errors.New("fabricengine: warp worktree has uncommitted changes; commit or stash them, then re-run pull; aborting, no warp changes")

// ErrWarpRewound is returned, wrapped with the dropped commits and the two ways to settle it, by Fabric.Pull for a task pair's branch that origin rewound past commits this checkout holds.
// Pulling would reset the checkout away from commits origin dropped, so Pull changes nothing.
var ErrWarpRewound error = errors.New("fabricengine: origin rewound this task branch past commits this checkout holds; aborting, no changes")

// WeftStopError is returned by Fabric.Pull when the weft could not be brought level with a moved origin:
// its local commits conflict with origin's, or an uncommitted change blocks the move.
// The warp side has run regardless, so the PullResult returned beside it describes warp.
// Err is ErrReplayConflict or ErrReplayDirty, or the failure of the blocked fast-forward.
type WeftStopError struct {
	// Branch is the weft branch that stopped.
	Branch string
	// Paths are the conflicting paths, or the uncommitted paths that block the move.
	Paths []string
	// WayForward names the commands that settle the stop.
	WayForward string
	// Err is what stopped the weft.
	Err error
}

// Error implements the error interface, naming the weft branch, the cause and the way forward.
func (e *WeftStopError) Error() string {
	return fmt.Sprintf("fabricengine: weft branch %s was not brought level with origin: %v; %s", e.Branch, e.Err, e.WayForward)
}

// Unwrap returns the wrapped error, so errors.Is reaches ErrReplayConflict and ErrReplayDirty.
func (e *WeftStopError) Unwrap() error {
	return e.Err
}

// RecordsStopMessage returns the message of err and true when err is exactly a *WeftStopError, by a direct type assertion.
// An error that joins a stop to another failure is not a stop here, so a caller never drops the other failure.
// It gives a caller outside this package a side-neutral way to tell a records-side stop from any other pull error.
func RecordsStopMessage(err error) (message string, ok bool) {
	stop, ok := err.(*WeftStopError)
	if !ok {
		return "", false
	}
	return stop.Error(), true
}

// NewRecordsStopError returns a *WeftStopError for branch stopped by cause, settled by wayForward.
// It lets a test outside this package build the stop Fabric.Pull returns without spelling the side-named type.
func NewRecordsStopError(branch string, cause error, wayForward string) error {
	return &WeftStopError{Branch: branch, Err: cause, WayForward: wayForward}
}

// weftHasUpstream reports whether the weft worktree's current branch has a configured upstream
// tracking ref.
// No upstream (or a detached HEAD) is, for Pull's weft step, the nothing-to-pull-from case, never an error.
func (f *Fabric) weftHasUpstream() (bool, error) {
	_, hasUpstream, err := upstreamSHAAt(f.weftPath)
	return hasUpstream, err
}

// weftAnchorStopBeforePull runs the checks a weft pull makes before it moves anything, for a weft with an upstream.
// It stops on a standing move record, then on an empty own anchor marker, then fetches the weft branch and stops when taking origin's tip in would cross an anchor change.
// A fetch that fails is warned past: the pull itself fails the same way and is warned past in turn.
func (f *Fabric) weftAnchorStopBeforePull() (*AnchorStopError, error) {
	if stop, err := moveRecordStop(f.weftPath); err != nil || stop != nil {
		return stop, err
	}
	if stop := emptyMarkerStop(f.weftPath); stop != nil {
		return stop, nil
	}
	if err := f.records.Fetch(); err != nil {
		logger.Warn("fabricengine: weft anchor comparison skipped, the fetch failed", "weft", f.weftPath, "err", err)
		return nil, nil
	}
	upstream, hasUpstream, err := upstreamSHAAt(f.weftPath)
	if err != nil || !hasUpstream {
		return nil, err
	}
	return upstreamAnchorStop(f.weftPath, f.warpPath, upstream)
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
	sha, err := f.records.CurrentSHA()
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
	after, err := f.code.CurrentSHA()
	if err != nil || after == before {
		return
	}
	rec.Append(KindRepoAdvanced, f.warpPath, after)
}

// warpUpstreamSHA resolves the warp repo's already-fetched upstream tracking ref to a plain hex SHA.
// Fabric.Pull calls this AFTER f.code.Fetch has refreshed the remote-tracking ref, so the SHA it returns is the freshly fetched upstream tip — usable directly by ResetHard and IsAncestor, which both require a plain commit SHA rather than symbolic revision syntax.
// A branch with no resolvable upstream is an error.
func (f *Fabric) warpUpstreamSHA() (string, error) {
	sha, hasUpstream, err := upstreamSHAAt(f.warpPath)
	if err != nil {
		return "", err
	}
	if !hasUpstream {
		return "", fmt.Errorf("fabricengine: no upstream tracking ref for the checked-out branch in %s", f.warpPath)
	}
	return sha, nil
}

// Pull is fabric's unified pull entry point: it brings the weft level with origin first, then fetches and inspects warp, reconciling weft's correspondence when warp's history has been rewritten.
// The weft arm fast-forwards when it can.
// A weft that both this machine and origin moved has its local commits replayed onto origin's under the weft write lock and push lock.
// The commits of a run origin has ended are kept under a local tag instead.
// A replay that stops on a conflict, or an uncommitted change that blocks the move, never stalls the warp side.
// The warp fetch/reconcile runs regardless, and Pull then returns a *WeftStopError naming the weft branch, the paths and the way forward, beside the warp result.
// A weft whose own anchor marker is empty, and one whose origin tip carries commits and another anchor marker, stop the pull before either side moves:
// Pull returns a *WeftStopError wrapping ErrAnchorChange, naming `lyx fabric anchor` as the way forward, and warp is not touched.
// A weft whose origin cannot be reached is warned past and leaves PullResult.WeftPulled false.
// A weft infrastructure failure that is neither a stop nor an unreachable origin, such as an unreadable hub location, a failed weft write lock or a replay that failed for another reason, returns at once, before the warp arm runs:
// warp is reset against the weft's correspondence, which such a failure leaves unknown.
// A warp-side failure reports the accumulated result rather than unwinding the weft pull (weft-first-ordering / report-not-rollback), joined with a weft stop when both happened.
// The warp arm judges a local commit as unpushed only when no value of the remote-tracking ref's reflog reaches it.
// It refuses a task pair's branch that origin rewound, with ErrWarpRewound, before any reset.
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
	weftStop, err := f.pullWeftArm(rec, &result, opts)
	if err != nil {
		return result, err
	}
	// An anchor stop leaves warp alone as well: a pull that moved warp past a weft it could not bring level would split the pair across the marker change.
	if weftStop != nil && errors.Is(weftStop, ErrAnchorChange) {
		return result, weftStop
	}

	warpErr := f.pullWarpArm(rec, &result)
	switch {
	case weftStop != nil && warpErr != nil:
		return result, errors.Join(weftStop, warpErr)
	case weftStop != nil:
		return result, weftStop
	default:
		return result, warpErr
	}
}

// pullWeftArm brings the weft level with its upstream and records on result whether it did.
// A weft branch with no upstream has nothing to pull from, as on the freshly bootstrapped hub's suffixed primary that exists only locally until the first push lands.
// It counts as pulled rather than surfacing git's "no tracking information" failure.
// A failed probe or an unreachable origin is warned past, so the warp side still runs, and leaves result.WeftPulled false.
// A weft stop is returned as a *WeftStopError beside a nil error, and any other failure as the error.
// Once the weft has an upstream, a standing move record, an empty own anchor marker and an upstream with another anchor marker are weft stops raised before the pull moves anything.
func (f *Fabric) pullWeftArm(rec *Mutations, result *PullResult, opts SyncOptions) (*WeftStopError, error) {
	weftHasUpstream, err := f.weftHasUpstream()
	if err != nil {
		logger.Warn("fabricengine: weft pull: resolve upstream failed, continuing to warp", "weft", f.weftPath, "err", err)
		return nil, nil
	}
	if !weftHasUpstream {
		result.WeftPulled = true
		return nil, nil
	}

	anchorStop, err := f.weftAnchorStopBeforePull()
	if err != nil {
		return nil, err
	}
	if anchorStop != nil {
		branch, err := f.records.CurrentBranch()
		if err != nil {
			return nil, fmt.Errorf("fabricengine: read the weft branch in %s: %w", f.weftPath, err)
		}
		return anchorStop.weftStop(branch), nil
	}

	// Sample the weft SHA before and after the pull, and record KindRepoAdvanced only on a
	// change — PullWeft's own f.records.Pull() also returns nil when the weft is already up to
	// date, so an unconditional entry would fabricate a mutation on that no-op path.
	// This is load-bearing: PullWeft can succeed (result.WeftPulled = true below) and Pull can
	// then still return a *PartialPullError on the warp side with no commit ever created and no
	// gate primitive ever run — without this entry the record would be empty and partial would
	// read false while the weft worktree had genuinely been advanced.
	// gitrepo.ErrNoCommits is tolerated exactly as headOrEmpty (coalesce.go) tolerates it: an
	// unborn weft HEAD reports as "" rather than an error, so a genuinely-empty repo before the
	// pull is a legitimate before-sample rather than an observation failure.
	beforeWeftSHA, beforeErr := weftSHAOrEmpty(f)
	pullErr := f.PullWeft(opts)
	if pullErr != nil {
		return f.settleUnmovedWeft(rec, result, pullErr)
	}
	result.WeftPulled = true
	if beforeErr == nil {
		if afterWeftSHA, afterErr := weftSHAOrEmpty(f); afterErr == nil && afterWeftSHA != beforeWeftSHA {
			rec.Append(KindRepoAdvanced, f.weftPath, afterWeftSHA)
		}
	}
	return nil, nil
}

// settleUnmovedWeft decides what a weft that could not fast-forward needs, after the pull that failed with pullErr fetched origin's tip.
// Origin holding nothing the weft lacks means the fetch itself failed, which is warned past.
// A weft only behind is blocked by an uncommitted change, and a weft both sides moved has its local commits replayed onto origin's.
func (f *Fabric) settleUnmovedWeft(rec *Mutations, result *PullResult, pullErr error) (*WeftStopError, error) {
	upstream, hasUpstream, err := upstreamSHAAt(f.weftPath)
	var head string
	var behind, ahead []string
	if err == nil && hasUpstream {
		head, err = f.records.CurrentSHA()
	}
	if err == nil && hasUpstream {
		behind, err = f.records.CommitsNotIn(upstream, head)
	}
	if err == nil && hasUpstream {
		ahead, err = f.records.CommitsNotIn(head, upstream)
	}
	if err != nil {
		return nil, fmt.Errorf("fabricengine: read the weft's state against its upstream in %s: %w", f.weftPath, errors.Join(err, pullErr))
	}
	if !hasUpstream || len(behind) == 0 {
		logger.Warn("fabricengine: weft pull failed, continuing to warp", "weft", f.weftPath, "err", pullErr)
		return nil, nil
	}

	branch, err := f.records.CurrentBranch()
	if err != nil {
		return nil, fmt.Errorf("fabricengine: read the weft branch in %s: %w", f.weftPath, err)
	}
	if len(ahead) == 0 {
		dirtyPaths, err := f.weftDirtyPaths()
		if err != nil {
			return nil, err
		}
		if len(dirtyPaths) == 0 {
			logger.Warn("fabricengine: weft pull failed, continuing to warp", "weft", f.weftPath, "err", pullErr)
			return nil, nil
		}
		return f.weftDirtyStop(branch, dirtyPaths, pullErr), nil
	}

	l, err := f.hubLocation()
	if err != nil {
		return nil, err
	}
	ended, err := f.weftEndedByOrigin(l, ahead)
	if err != nil {
		return nil, err
	}
	release, err := f.lockWeftForReplay()
	if err != nil {
		return nil, err
	}
	defer release()

	outcome, err := replayWeft(rec, l, f.weftPath, upstream, ended)
	switch {
	case err == nil:
		result.WeftPulled = true
		logWeftReplay(f.weftPath, outcome)
		return nil, nil
	case errors.Is(err, ErrReplayConflict):
		return &WeftStopError{
			Branch:     branch,
			Paths:      outcome.Conflicts,
			WayForward: recordsConflictWayForward(f.weftPath, branch, outcome.Conflicts, ahead, fmt.Sprintf("`lyx fabric push` in %s", f.warpPath)),
			Err:        err,
		}, nil
	case errors.Is(err, ErrReplayDirty):
		dirtyPaths, pathsErr := f.weftDirtyPaths()
		if pathsErr != nil {
			return nil, errors.Join(err, pathsErr)
		}
		return f.weftDirtyStop(branch, dirtyPaths, err), nil
	default:
		return nil, err
	}
}

// weftDirtyStop builds the weft stop for uncommitted tracked changes on paths, which `lyx fabric commit` settles.
func (f *Fabric) weftDirtyStop(branch string, paths []string, cause error) *WeftStopError {
	return &WeftStopError{
		Branch: branch,
		Paths:  paths,
		WayForward: fmt.Sprintf("commit the uncommitted paths in %s (%s) with `lyx fabric commit` in %s, then re-run `lyx fabric pull`",
			f.weftPath, joinOrNone(paths), f.warpPath),
		Err: cause,
	}
}

// weftDirtyPaths returns the weft worktree's tracked uncommitted paths, relative to it, with slash separators.
func (f *Fabric) weftDirtyPaths() ([]string, error) {
	absolute, err := trackedDirtyPaths(f.weftPath)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(absolute))
	for _, path := range absolute {
		rel, err := filepath.Rel(f.weftPath, path)
		if err != nil {
			return nil, err
		}
		paths = append(paths, filepath.ToSlash(rel))
	}
	return paths, nil
}

// hubLocation resolves the location of the warp worktree paired with f's weft, from the weft sibling's own name.
// It never reads f.warpPath, which a caller may hold outside the hub.
func (f *Fabric) hubLocation() (*lyxcwd.Location, error) {
	name := filepath.Base(f.weftPath)
	slug, ok := WeftWarpSlug(name)
	if !ok {
		return nil, fmt.Errorf("fabricengine: %s is no weft sibling worktree", f.weftPath)
	}
	return lyxcwd.ResolveWorktree(filepath.Join(filepath.Dir(f.weftPath), slug))
}

// lockWeftForReplay takes the weft write lock and then the absorbing push lock, and returns the release of both.
func (f *Fabric) lockWeftForReplay() (release func(), err error) {
	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return nil, err
	}
	writeLock, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return nil, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	pushLock, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftPushLockFile))
	if err != nil {
		_ = writeLock.Release()
		return nil, fmt.Errorf("fabricengine: acquire push lock: %w", err)
	}
	return func() {
		_ = pushLock.Release()
		_ = writeLock.Release()
	}, nil
}

// weftEndedByOrigin reads which of the runs the local weft commits ahead touch origin has ended, from the board's origin tip as last fetched.
// It returns an empty set for a hub with no board or a board with no upstream, and reads no network.
func (f *Fabric) weftEndedByOrigin(l *lyxcwd.Location, ahead []string) (EndedSet, error) {
	boardDir := filepath.Join(l.HubPath, BoardDirName)
	if _, err := os.Stat(boardDir); err != nil {
		return nil, nil
	}
	boardRepo := gitrepo.New(boardDir)
	boltTip, err := boardRepo.UpstreamSHA()
	if errors.Is(err, gitrepo.ErrNoUpstream) {
		return nil, nil
	}
	if err != nil {
		logger.Warn("fabricengine: origin's board tip could not be read, so no run origin ended is set aside", "board", boardDir, "error", err)
		return nil, nil
	}

	var slugs []string
	for _, sha := range ahead {
		detail, err := f.records.CommitDetail(sha)
		if err != nil {
			return nil, err
		}
		for _, path := range detail.Paths {
			if slug, found := weftRunSlug(l, path); found && !slices.Contains(slugs, slug) {
				slugs = append(slugs, slug)
			}
		}
	}
	if len(slugs) == 0 {
		return nil, nil
	}

	replay, err := InstalledBoardReplay()
	if err != nil {
		return nil, err
	}
	head, err := boardRepo.CurrentSHA()
	if err != nil {
		return nil, err
	}
	data, err := blobAt(boardRepo, head, boardFileName)
	if err != nil {
		return nil, err
	}
	entries, err := replay.Entries(data)
	if err != nil {
		return nil, fmt.Errorf("%s at %s: %w", boardFileName, head, err)
	}
	created := map[string]string{}
	for _, entry := range entries {
		if slices.Contains(slugs, entry.Slug) && !entry.Created.IsZero() {
			created[entry.Slug] = entry.Created.UTC().Format(time.RFC3339)
		}
	}
	return readEndedSet(boardDir, boltTip, slugs, created)
}

// weftRunSlug returns the slug of the run whose record directory holds path, a records-repository-relative slash path.
func weftRunSlug(l *lyxcwd.Location, path string) (string, bool) {
	root := pathpkg.Join(filepath.ToSlash(l.AnchorRel), filepath.ToSlash(shedrun.RunsRootRel())) + "/"
	rest, found := strings.CutPrefix(path, strings.TrimPrefix(root, "./"))
	if !found {
		return "", false
	}
	slug, _, nested := strings.Cut(rest, "/")
	return slug, nested && slug != ""
}

// logWeftReplay logs what a weft replay dropped or set aside, naming each set-aside tag, so no caller repeats the text.
func logWeftReplay(weftPath string, outcome ReplayOutcome) {
	if len(outcome.Dropped) > 0 {
		logger.Info("fabricengine: dropped commits from the weft", "path", weftPath, "commits", strings.Join(outcome.Dropped, " "))
	}
	for _, aside := range outcome.SetAside {
		logger.Warn("fabricengine: a local weft commit was set aside under a tag", "path", weftPath, "commit", aside.Commit, "tag", aside.Tag)
	}
}

// pullWarpArm fetches warp and moves it level with its upstream, recording what it did on result.
// A warp worktree with uncommitted tracked changes, an unpushed local commit and a task branch origin rewound each refuse before warp moves.
func (f *Fabric) pullWarpArm(rec *Mutations, result *PullResult) error {
	if err := f.code.Fetch(); err != nil {
		return &PartialPullError{WeftPulled: result.WeftPulled, Stage: "fetch", Err: err}
	}
	result.WarpFetched = true

	upstreamSHA, err := f.warpUpstreamSHA()
	if err != nil {
		return &PartialPullError{WeftPulled: result.WeftPulled, Stage: "resolve", Err: err}
	}
	localHEAD, err := f.code.CurrentSHA()
	if err != nil {
		return &PartialPullError{WeftPulled: result.WeftPulled, Stage: "resolve", Err: err}
	}

	if localHEAD == upstreamSHA {
		return nil
	}

	// Every remaining branch moves warp via ResetHard, which discards uncommitted tracked changes
	// without a trace — so a dirty warp worktree is refused here, before anything mutates warp.
	dirty, err := f.warpWorktreeDirty()
	if err != nil {
		return &PartialPullError{WeftPulled: result.WeftPulled, Stage: "dirty-check", Err: err}
	}
	if dirty {
		return ErrWarpDirty
	}

	isFF, err := f.code.IsAncestor(localHEAD, upstreamSHA)
	if err != nil {
		return &PartialPullError{WeftPulled: result.WeftPulled, Stage: "classify", Err: err}
	}

	if isFF {
		if err := f.ResetHard(rec, upstreamSHA); err != nil {
			return &PartialPullError{WeftPulled: result.WeftPulled, Stage: "reset", Err: err}
		}
		f.recordWarpAdvance(rec, localHEAD)
		result.WarpAdvanced = true
		result.NewWarpHEAD = upstreamSHA
		return nil
	}

	result.RewriteDetected = true

	if err := f.refuseUnsafeRewrite(result.WeftPulled, localHEAD, upstreamSHA); err != nil {
		return err
	}
	return f.reanchorAfterRewrite(rec, result, localHEAD, upstreamSHA)
}

// refuseUnsafeRewrite judges a warp branch whose upstream is not a fast-forward of localHEAD by the reflog of its remote-tracking ref, and refuses what a reset would lose.
// A task pair's branch classifyRewrite reads as rewound is refused with ErrWarpRewound, naming the dropped commits.
// The prime's branch takes no such refusal, since its code branch is the operator's and a reset follows the rewind.
// A local commit no reflog value reaches refuses with ErrWarpDivergedUnpushed.
func (f *Fabric) refuseUnsafeRewrite(weftPulled bool, localHEAD, upstreamSHA string) error {
	partial := func(stage string, err error) error {
		return &PartialPullError{WeftPulled: weftPulled, Stage: stage, Err: err}
	}

	trackingRef, err := gitexec.Run([]string{"rev-parse", "--symbolic-full-name", "@{u}"}, f.warpPath)
	if err != nil {
		return partial("reflog", err)
	}
	earlier, err := trackingReflog(f.warpPath, strings.TrimSpace(trackingRef))
	if err != nil {
		return partial("reflog", err)
	}

	kind, dropped, err := classifyRewrite(f.code, localHEAD, upstreamSHA, earlier)
	if err != nil {
		return partial("reflog", err)
	}
	if kind == rewriteRewound {
		l, err := f.hubLocation()
		if err != nil {
			return partial("reflog", err)
		}
		prime, err := IsPrimeWorktree(l)
		if err != nil {
			return partial("reflog", err)
		}
		if !prime {
			branch, err := f.code.CurrentBranch()
			if err != nil {
				return partial("reflog", err)
			}
			return fmt.Errorf("%w: %s", ErrWarpRewound, rewoundWayForward(f.warpPath, branch, upstreamSHA, dropped))
		}
	}

	localOnly, err := f.code.CommitsNotIn(localHEAD, upstreamSHA)
	if err != nil {
		return partial("reflog", err)
	}
	for _, commit := range localOnly {
		reached, err := reachedByAny(f.code, commit, earlier)
		if err != nil {
			return partial("reflog", err)
		}
		if !reached {
			return ErrWarpDivergedUnpushed
		}
	}
	return nil
}

// reanchorAfterRewrite resets warp to upstreamSHA after a rewrite refuseUnsafeRewrite passed, and re-anchors weft's correspondence to it.
func (f *Fabric) reanchorAfterRewrite(rec *Mutations, result *PullResult, localHEAD, upstreamSHA string) error {
	partial := func(stage string, err error) error {
		return &PartialPullError{WeftPulled: result.WeftPulled, Stage: stage, Err: err}
	}

	// The index is a rebuildable cache, never authoritative on its own — and here it decides whether
	// the pair can recover at all, so it must be rebuilt from the weft trailer history (the sole
	// source of truth, already fast-forwarded above) before the anchor walk.
	// A re-cloned hub starts with an empty per-pair index while its adopted weft history carries
	// every recorded anchor; without the rebuild, the walk missed those surviving anchors and
	// returned a false ErrNoSurvivingAnchor that no later call could ever clear.
	if err := f.RebuildIndex(); err != nil {
		return partial("load-index", err)
	}
	path, err := f.corrIndexPath()
	if err != nil {
		return partial("load-index", err)
	}
	ix, err := loadCorrIndex(path)
	if err != nil {
		return partial("load-index", err)
	}
	entries := ix.entries()

	if len(entries) == 0 {
		if err := f.ResetHard(rec, upstreamSHA); err != nil {
			return partial("reset", err)
		}
		f.recordWarpAdvance(rec, localHEAD)
		result.WarpAdvanced = true
		result.NewWarpHEAD = upstreamSHA
		return nil
	}

	anchor, found, err := reachableAnchor(entries, func(sha string) (bool, error) {
		return f.code.IsAncestor(sha, upstreamSHA)
	})
	if err != nil {
		return partial("anchor-walk", err)
	}
	if !found {
		return ErrNoSurvivingAnchor
	}

	if err := f.ResetHard(rec, upstreamSHA); err != nil {
		return partial("reset", err)
	}
	f.recordWarpAdvance(rec, localHEAD)
	result.WarpAdvanced = true
	result.NewWarpHEAD = upstreamSHA
	result.AnchorWarpSHA = anchor.WarpSHA
	result.AnchorWeftSHA = anchor.WeftSHA

	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return partial("reanchor", err)
	}
	l, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return partial("reanchor", fmt.Errorf("fabricengine: acquire weft write lock: %w", err))
	}
	defer func() { _ = l.Release() }()

	msg := appendWarpSHATrailer("fabric: re-anchor weft after warp rebase", upstreamSHA)
	reanchorSHA, _, err := f.commitEmptySnapshot(msg, upstreamSHA)
	if err != nil {
		return partial("reanchor", err)
	}
	result.Reconciled = true
	result.ReanchorWeftSHA = reanchorSHA

	return nil
}

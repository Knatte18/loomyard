// reconcile.go implements the fabric repair-and-adopt sweep for paired warp↔weft worktrees.
//
// Reconcile walks all warp worktrees (never the branch namespace directly) and applies the minimal
// corrective action needed to restore a valid paired topology: it recreates a missing weft worktree
// when the branch still exists, re-points a broken junction, adopts a raw (non-lyx) warp worktree
// by creating the weft side dormant, and reports (but does not touch) a warp worktree on an
// unmanaged branch.
// Wherever a warp branch name needs a weft counterpart, fabric derives it via
// WeftBranchName(warpBranch).
//
// readBranch and checkJunctionHealth are also used by Status;
// they live here because Reconcile needs them first and both verbs share the same package.
//
// The junction name-set checkJunctionHealth/Reconcile/junctionRepointedDetail consult is sourced
// from the repo-wide fabric.yaml at BoardDir(l.HubPath) — via RepoWiredNames — not from any
// individual pair's own weft base, so reconcile converges every worktree to the same repo-wide
// pathspec.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// ReconcileAction describes the corrective action applied to one warp↔weft pair.
type ReconcileAction string

const (
	// ReconcileActionWeftRecreated means a missing weft worktree was recreated from its existing
	// branch.
	ReconcileActionWeftRecreated ReconcileAction = "weft_recreated"

	// ReconcileActionJunctionRepointed means at least one broken or dangling warp junction was
	// re-pointed to its correct weft directory.
	// WireJunctions repairs every junction in one call, so the outcome's Detail (via
	// junctionRepointedDetail) names all of them, not just the one that failed checkJunctionHealth.
	ReconcileActionJunctionRepointed ReconcileAction = "junction_repointed"

	// ReconcileActionRawAdopted means a warp worktree created outside lyx had its weft side created
	// (branch + worktree) as a dormant counterpart.
	// No junction is wired;
	// re-running Reconcile is what wires it once the pair exists.
	ReconcileActionRawAdopted ReconcileAction = "raw_adopted"

	// ReconcileActionUnmanagedReported means a warp worktree is on an unmanaged branch with no weft
	// sibling;
	// it was reported but left untouched.
	ReconcileActionUnmanagedReported ReconcileAction = "unmanaged_reported"

	// ReconcileActionAlreadyHealthy means the pair required no corrective action.
	ReconcileActionAlreadyHealthy ReconcileAction = "already_healthy"

	// ReconcileActionPortalRestored means the pair's junctions were healthy but its hub-level portal
	// junction or launcher directory was missing and has been recreated.
	// It is reported instead of ReconcileActionAlreadyHealthy for the same reason
	// ReconcileActionStaleRemoved is: a consumer keying off Action must see that convergence altered
	// the pair.
	ReconcileActionPortalRestored ReconcileAction = "portal_restored"

	// ReconcileActionStaleRemoved means the pair's junction/repoint check found nothing to add or
	// re-point,
	// but declarative stale-removal deleted at least one on-disk junction absent from the repo-wide
	// pathspec.
	// It is reported instead of ReconcileActionAlreadyHealthy so consumers keying off Action — not
	// just Detail — see that convergence altered the pair.
	ReconcileActionStaleRemoved ReconcileAction = "stale_removed"

	// ReconcileActionVanishedMidWalk means the warp worktree directory existed when `git worktree
	// list` enumerated it and was gone by the time this pass reached it — a concurrent `lyx fabric
	// remove` or `prune`, not a fault in this hub.
	//
	// It exists because the alternative was actively misleading: the vanished directory made
	// readBranch fail, which was reported as ReconcileActionUnmanagedReported — a verdict meaning
	// something entirely different ("this pair is not fabric's to manage") — carrying os/exec's raw
	// `chdir <path>: no such file or directory` as its reason. Naming the race lets an operator, and a
	// scripted caller reading the failure a per-pair Error now produces, tell a transient from a real
	// defect without decoding a Go runtime message.
	ReconcileActionVanishedMidWalk ReconcileAction = "vanished_mid_walk"
)

// WarpBindingOutcome describes the result of the once-per-Reconcile warp-URL binding backfill.
type WarpBindingOutcome string

const (
	// WarpBindingOutcomeRecorded means no binding existed yet and Reconcile wrote one from the warp
	// side's origin remote.
	WarpBindingOutcomeRecorded WarpBindingOutcome = "recorded"

	// WarpBindingOutcomePresent means a binding already existed and normalizes to the same URL as the
	// warp side's origin remote, so nothing was written.
	WarpBindingOutcomePresent WarpBindingOutcome = "present"

	// WarpBindingOutcomeDiverged means a binding already existed but normalizes to a different URL
	// than the warp side's origin remote; the record is left untouched and reconcile still succeeds.
	WarpBindingOutcomeDiverged WarpBindingOutcome = "diverged"

	// WarpBindingOutcomeSkipped means the warp side has no origin remote to read, which is a
	// legitimate state (a synthetic test hub, a locally-initialised warp) rather than an error.
	WarpBindingOutcomeSkipped WarpBindingOutcome = "skipped"

	// WarpBindingOutcomeDeferred means no binding existed and Reconcile declined to write one this
	// pass — either the board worktree was dirty, or the write itself failed — leaving the backfill
	// for the next Reconcile call.
	WarpBindingOutcomeDeferred WarpBindingOutcome = "deferred"

	// WarpBindingOutcomeRecordFailed means the CLI handler committed or pushed the backfilled record
	// and that commit or push failed.
	// Topology.Reconcile never returns this value — only the CLI handler sets it, because the commit
	// and push that can fail happen after Reconcile has already returned.
	WarpBindingOutcomeRecordFailed WarpBindingOutcome = "record_failed"
)

// ReconcilePairResult describes the outcome for one warp↔weft pair.
type ReconcilePairResult struct {
	// WarpWorktree is the absolute path to the warp worktree.
	WarpWorktree string `json:"warp_worktree"`
	// WeftWorktree is the absolute path to the expected weft sibling.
	WeftWorktree string `json:"weft_worktree"`
	// Action is the corrective action taken (or reported).
	Action ReconcileAction `json:"action"`
	// Detail provides human-readable context for the action.
	Detail string `json:"detail,omitempty"`
	// Error is non-empty when the reconcile step encountered an error.
	Error string `json:"error,omitempty"`
}

// ReconcileResult is the top-level result returned by Reconcile.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type ReconcileResult struct {
	MutationRecord
	// Pairs is the ordered list of per-worktree reconcile outcomes.
	Pairs []ReconcilePairResult `json:"pairs"`

	// WarpBinding is the outcome of the once-per-Reconcile warp-URL binding backfill.
	// The check runs exactly once per Reconcile call, after the pair loop, and is never reported
	// per-pair — the binding is a once-per-hub fact written to the board directory, so running it
	// inside the per-worktree loop would repeat it N times and leave "which pair owns a repo-wide
	// fact" unanswerable.
	// runReconcile hand-builds its own envelope and never serializes this struct, so this tag
	// documents intent rather than driving output.
	WarpBinding WarpBindingOutcome `json:"warp_binding"`

	// WarpBindingDetail provides human-readable context for WarpBinding.
	// Like WarpBinding, it is a once-per-Reconcile, repo-wide field, never a per-pair one, and its
	// json tag documents intent only — runReconcile never serializes this struct directly.
	WarpBindingDetail string `json:"warp_binding_detail,omitempty"`
}

// Reconcile walks all warp worktrees reachable from layout l and applies corrective actions to
// restore a valid paired warp↔weft topology.
// For each warp worktree it applies a sequence of rules: recreate missing weft worktrees, re-point
// broken junctions, adopt raw (non-lyx) worktrees, or report unmanaged pairs.
// Per-worktree errors are recorded in ReconcilePairResult.Error.
func (t *Topology) Reconcile(l *lyxcwd.Location) (res ReconcileResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	if err := refuseEmptyAnchorMarker(l); err != nil {
		return ReconcileResult{}, err
	}

	entries, err := List(l.WorktreePath())
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("list worktrees: %w", err)
	}

	var result ReconcileResult

	for _, entry := range entries {
		warpPath := filepath.FromSlash(entry.Path)
		warpPath = filepath.Clean(warpPath)

		slug := filepath.Base(warpPath)
		weftPath := WeftWorktreePath(l, slug)

		pr := ReconcilePairResult{
			WarpWorktree: filepath.ToSlash(warpPath),
			WeftWorktree: filepath.ToSlash(weftPath),
		}

		// The worktree list was read before this loop began, so a concurrent remove/prune can delete a
		// pair's directory between the enumeration and this iteration. Naming that race here keeps it
		// from surfacing further down as a layout or branch-read failure whose Action
		// (unmanaged_reported) means something entirely different and whose reason is os/exec's raw
		// chdir error.
		if _, statErr := os.Stat(warpPath); os.IsNotExist(statErr) {
			markVanishedMidWalk(l.WorktreePath(), warpPath, &pr)
			result.Pairs = append(result.Pairs, pr)
			continue
		}

		warpLayout, layoutErr := warpLayoutFor(l, warpPath)
		if layoutErr != nil {
			pr.Error = fmt.Sprintf("resolve layout: %v", layoutErr)
			pr.Action = ReconcileActionUnmanagedReported
			markVanishedMidWalk(l.WorktreePath(), warpPath, &pr)
			result.Pairs = append(result.Pairs, pr)
			continue
		}

		weftStat, weftStatErr := os.Stat(weftPath)
		weftWorktreeExists := weftStatErr == nil && weftStat.IsDir()

		warpBranch, branchErr := readBranch(warpPath)
		if branchErr != nil {
			pr.Error = fmt.Sprintf("read warp branch: %v", branchErr)
			pr.Action = ReconcileActionUnmanagedReported
			markVanishedMidWalk(l.WorktreePath(), warpPath, &pr)
			result.Pairs = append(result.Pairs, pr)
			continue
		}

		if !weftWorktreeExists {
			pr.Action = t.reconcileMissingWeft(rec, warpLayout, warpPath, weftPath, slug, warpBranch, &pr)
		}

		// A pair whose weft worktree was just recreated has its junctions still pointing at
		// directories that vanished with it, so it falls through to the same wiring repair a pair
		// with an existing weft gets — without it, reconcile reported success while leaving the
		// pair unhealthy until a SECOND reconcile run, and the interim `pairs` reason was a raw
		// EvalSymlinks error rather than a fabric verdict.
		// The freshly-recreated pair keeps its own weft_recreated Action rather than being
		// relabelled junction_repointed, so the report still names what actually happened;
		// ReconcileActionRawAdopted deliberately does NOT fall through, since a raw-adopted pair is
		// dormant by design and wired by the next pass.
		repairWiring := weftWorktreeExists || (pr.Action == ReconcileActionWeftRecreated && pr.Error == "")
		if repairWiring {
			t.repairPairWiring(rec, warpLayout, slug, &pr, weftWorktreeExists)
		}

		// The pre-check above closes only the window between enumeration and the start of this
		// iteration; a concurrent teardown can just as easily land in the middle of the repair steps,
		// which then fail with whatever raw error git or the filesystem produced for a directory that
		// is no longer there. Re-checking here converts every one of those windows, not just the
		// first, and it cannot mask a real defect: a repair that genuinely failed leaves the warp
		// worktree exactly where it was.
		if pr.Error != "" {
			markVanishedMidWalk(l.WorktreePath(), warpPath, &pr)
		}

		result.Pairs = append(result.Pairs, pr)
	}

	result.WarpBinding, result.WarpBindingDetail = t.reconcileWarpBinding(rec, l)

	return result, nil
}

// refuseEmptyAnchorMarker aborts a reconcile pass when the hub's recorded lyx-anchor marker exists
// but is empty after trimming.
//
// lyxcwd deliberately treats an empty marker as ABSENT, so `Resolve` falls back to the `"."` anchor
// and succeeds at the warp worktree root — correct for a hub that never recorded one, and a trap
// for a hub that did.
// Reconcile is the only verb that wires junctions, so on a subpath-anchored hub whose marker was
// truncated it is the verb that materialises a SECOND junction set at the repo root beside the
// still-live set at the real anchor — exactly the damage lyxcwd.ErrStaleAnchorMarker exists to
// prevent for the pre-rename spelling.
// A present-but-empty marker is a corrupt record rather than an absent one, so the repair verb
// refuses it here instead, leaving lyxcwd's own documented fallback untouched.
//
// A marker that cannot be read at all (including a genuinely absent one) is the legitimate
// root-anchored case and passes.
func refuseEmptyAnchorMarker(l *lyxcwd.Location) error {
	markerPath := filepath.Join(BoardDir(l.HubPath), lyxcwd.AnchorFileName)

	data, err := os.ReadFile(markerPath)
	if err != nil {
		return nil
	}
	if strings.TrimSpace(string(data)) != "" {
		return nil
	}

	return fmt.Errorf(
		"recorded anchor marker at %s is empty; write the repo's subpath into it (or %q for the repo root) in the hub's %s worktree and commit, then re-run — reconciling against an empty marker would wire a second junction set at the warp repo root",
		markerPath, ".", BoardDirName)
}

// reconcileWarpBinding backfills the once-per-hub .lyx-warp record from the warp side's origin
// remote, for every hub that predates the binding.
// It runs exactly once per Reconcile call, after the pair loop, never per-pair, and it never
// returns an error: a binding backfill is a convenience that may never fail or downgrade a
// reconcile verdict, so any failure is folded into a Deferred outcome instead.
// rec is Reconcile's own recorder; it records KindFileWritten at the boardDir's .lyx-warp path on the
// branch that actually calls writeWarpBinding, mirroring CloneHub's own record for the same file —
// without it a reconcile-driven backfill is an uncovered hub-visible addition batch 7's omission
// direction would catch.
func (t *Topology) reconcileWarpBinding(rec *Mutations, l *lyxcwd.Location) (WarpBindingOutcome, string) {
	boardDir := BoardDir(l.HubPath)

	// Re-seed the weft repo's operational excludes on every reconcile pass. The seeding is
	// idempotent and otherwise only runs from a weft-git verb or a wiring call, so a hub wired by an
	// earlier binary would never pick up a newly added artifact pattern — and the dirty-board gate
	// below reads exactly the status those patterns govern, so a hub that never runs a weft-git verb
	// would defer its backfill forever on an artifact the current binary excludes.
	// A seeding failure is not fatal: the gate simply sees whatever git reports.
	// Best-effort: the board worktree's artifact excludes are self-healing (every weft-git verb
	// re-seeds them), so a failure here must not stop the binding backfill this function exists for.
	_ = seedWeftArtifactExcludes(boardDir)

	// git remote get-url is read-only, so it falls outside the Fabric Git Invariant's
	// mutating-warp-git rule even though it targets the warp worktree.
	originOut, err := gitexec.Run([]string{"remote", "get-url", "origin"}, l.WorktreePath())
	origin := strings.TrimSpace(originOut)
	if err != nil || origin == "" {
		// An absent origin remote is a legitimate state (a synthetic test hub, a locally-initialised
		// warp), not an error condition.
		return WarpBindingOutcomeSkipped, ""
	}

	recorded, found := readWarpBinding(boardDir)
	if found {
		if normalizeWarpURL(recorded) == normalizeWarpURL(origin) {
			return WarpBindingOutcomePresent, ""
		}

		detail := fmt.Sprintf("recorded warp binding %s does not match warp origin %s", recorded, origin)
		if warpURLTransportIdentity(recorded) == warpURLTransportIdentity(origin) {
			// The record does not describe the transport in use, so a transport-only spelling
			// difference is advisory rather than a genuine divergence — surfaced but never fatal.
			detail += "; the two spellings differ only by transport, which is advisory: the record does not describe the transport in use"
		}
		// Divergence is reported, never overwritten and never fatal: the same never-silently-re-point
		// rule clone follows, but reconcile is the repair verb and must not be blocked by an unrelated
		// URL mismatch.
		return WarpBindingOutcomeDiverged, detail
	}

	// Bolt.Commit is stage-all: safe at clone time, when the board was created seconds earlier, but
	// not at reconcile time, when a long-lived board may carry unrelated uncommitted content a
	// backfill commit would sweep up and push. This check is read-only and runs before anything is
	// written, so no half-written state is ever left behind.
	dirty, _, statusErr := worktreeDirty(scopeAll, boardDir)
	if statusErr != nil {
		return WarpBindingOutcomeDeferred, fmt.Sprintf("board status check failed: %v", statusErr)
	}
	if dirty {
		return WarpBindingOutcomeDeferred, "board worktree has uncommitted changes; backfill deferred to avoid sweeping them into an unrelated commit"
	}

	if err := writeWarpBinding(boardDir, origin); err != nil {
		return WarpBindingOutcomeDeferred, fmt.Sprintf("write warp binding: %v", err)
	}
	rec.Append(KindFileWritten, filepath.Join(boardDir, WarpBindingFileName), "")
	return WarpBindingOutcomeRecorded, fmt.Sprintf("recorded warp binding %s", origin)
}

// repairPairWiring converges one pair's junctions: it re-wires whatever checkJunctionHealth reports
// broken, and applies declarative stale-removal.
//
// setAction distinguishes the two callers. A pair whose weft worktree already existed has no Action
// yet, so this call assigns the verdict (already_healthy / junction_repointed). A pair whose weft
// worktree this same pass recreated already carries weft_recreated, which names what actually
// happened and must survive the repair, so that caller passes false and gets Detail notes only.
// rec is Reconcile's own recorder, threaded through to every gate-reaching call this helper makes.
func (t *Topology) repairPairWiring(rec *Mutations, warpLayout *lyxcwd.Location, slug string, pr *ReconcilePairResult, setAction bool) {
	junctionHealthy, unhealthyReason := checkJunctionHealth(warpLayout)

	if !junctionHealthy {
		if setAction {
			pr.Action = ReconcileActionJunctionRepointed
		}
		// Record WHAT was broken, not just that something was: "missing", "not a junction" and
		// "points at the wrong weft" are different operator problems with the same repair.
		appendPrDetail(pr, unhealthyReason)
		names, namesErr := RepoWiredNames(warpLayout)
		if namesErr != nil {
			pr.Error = fmt.Sprintf("re-point junction: load fabric config: %v", namesErr)
		} else if wireErr := WireJunctionsWith(rec, warpLayout, slug, names); wireErr != nil {
			pr.Error = fmt.Sprintf("re-point junction: %v", wireErr)
		} else {
			appendPrDetail(pr, junctionRepointedDetail(warpLayout))
		}
	} else if setAction {
		pr.Action = ReconcileActionAlreadyHealthy
	}

	if restorePortalAndLaunchers(rec, warpLayout, slug, pr) && setAction && pr.Action == ReconcileActionAlreadyHealthy {
		pr.Action = ReconcileActionPortalRestored
	}

	applyStaleRemoval(rec, warpLayout, slug, pr)
}

// restorePortalAndLaunchers recreates the pair's hub-level portal junction and launcher directory
// when either is missing, and reports whether it restored anything.
//
// Both are part of the managed topology — Add creates them, Remove and Prune tear them down — but
// nothing repaired them, so a pair whose portal had been deleted was reported already_healthy
// forever and could only be recovered by removing and re-adding the pair.
// The hub's prime worktree is skipped: it never had a portal or a launcher directory in the first
// place, and Reconcile is a repair verb, not the place to start creating artefacts Clone does not.
// A restore failure is a Detail note, never an Error or a changed Action: the portal is convenience
// plumbing, and failing to rebuild it must not downgrade a verdict about the pair's git topology.
// rec is Reconcile's own recorder, threaded through to createPortal (and, from batch 5's card 21
// onward, writeLaunchers).
func restorePortalAndLaunchers(rec *Mutations, warpLayout *lyxcwd.Location, slug string, pr *ReconcilePairResult) bool {
	primeName, primeErr := PrimeName(warpLayout)
	if primeErr != nil || slug == primeName {
		return false
	}

	restored := false

	if _, err := os.Lstat(PortalLink(warpLayout, slug)); os.IsNotExist(err) {
		if portalErr := createPortal(rec, warpLayout, slug); portalErr != nil {
			appendPrDetail(pr, fmt.Sprintf("portal restore failed: %v", portalErr))
		} else {
			appendPrDetail(pr, "portal junction restored")
			restored = true
		}
	}

	if _, err := os.Stat(LauncherDir(warpLayout, slug)); os.IsNotExist(err) {
		if launcherErr := writeLaunchers(rec, warpLayout, slug); launcherErr != nil {
			appendPrDetail(pr, fmt.Sprintf("launcher restore failed: %v", launcherErr))
		} else {
			appendPrDetail(pr, "launcher scripts restored")
			restored = true
		}
	}

	return restored
}

// reconcileMissingWeft determines and applies the corrective action when a weft worktree
// does not exist for the given warp worktree: recreate from the existing branch,
// adopt a raw worktree, or report unmanaged.
// A missing weft REPO (the prime weft checkout that holds the weft gitdir) is diagnosed first and
// reported by name — every corrective branch below needs the weft repo, and without this check each
// of them failed with a raw chdir error that named a path instead of the actual problem.
func (t *Topology) reconcileMissingWeft(
	rec *Mutations,
	warpLayout *lyxcwd.Location,
	warpPath, weftPath, slug, warpBranch string,
	pr *ReconcilePairResult,
) ReconcileAction {
	weftBranch := WeftBranchName(warpBranch)

	if !weftRepoExists(warpLayout) {
		weftRepoRoot, weftRepoRootErr := WeftRepoRoot(warpLayout)
		if weftRepoRootErr != nil {
			pr.Error = fmt.Sprintf("resolve weft repo root: %v", weftRepoRootErr)
		} else {
			pr.Error = fmt.Sprintf("weft repo missing at %s; restore it or re-clone the hub", weftRepoRoot)
		}
		return ReconcileActionUnmanagedReported
	}

	if weftBranchExists(warpLayout, weftBranch) {
		if weftRepoRoot, weftRepoRootErr := WeftRepoRoot(warpLayout); weftRepoRootErr == nil {
			// Bookkeeping only: a failed prune leaves the stale registration the adopt below
			// re-reports, and must not abort the repair.
			_, _ = gitexec.Run([]string{"worktree", "prune"}, weftRepoRoot)
		}

		if err := adoptWeftWorktree(warpLayout, weftPath, weftBranch); err != nil {
			pr.Error = fmt.Sprintf("recreate weft worktree: %v", err)
			return ReconcileActionWeftRecreated
		}
		pr.Detail = fmt.Sprintf("recreated weft worktree at %s (branch %s existed)", weftPath, weftBranch)
		return ReconcileActionWeftRecreated
	}

	isRaw := isRawWarpWorktree(warpLayout)
	if isRaw {
		if err := createDormantWeftForRawWarp(rec, warpLayout, slug, weftBranch); err != nil {
			pr.Error = fmt.Sprintf("adopt raw warp worktree: %v", err)
			return ReconcileActionRawAdopted
		}
		pr.Detail = fmt.Sprintf("adopted raw warp worktree at %s; weft branch %s created dormant (re-run lyx fabric reconcile to wire it)", warpPath, weftBranch)
		return ReconcileActionRawAdopted
	}

	pr.Detail = fmt.Sprintf(
		"warp worktree %s is on branch %s with no weft sibling; run `lyx fabric add` or `lyx fabric reconcile`",
		warpPath, warpBranch,
	)
	return ReconcileActionUnmanagedReported
}

// adoptWeftWorktree creates a git worktree at weftPath for the existing branch in
// the weft repo. The branch already exists, so no -b flag is used.
func adoptWeftWorktree(warpLayout *lyxcwd.Location, weftPath, branch string) error {
	weftRepoRoot, weftRepoRootErr := WeftRepoRoot(warpLayout)
	if weftRepoRootErr != nil {
		return fmt.Errorf("resolve weft repo root: %w", weftRepoRootErr)
	}
	// Through containedWorktreeAdd so a symlink toggled at weftPath during reconcile's own
	// enumerate-then-adopt window cannot carry the worktree outside the hub (R5's create-side escape).
	if err := containedWorktreeAdd(weftRepoRoot, warpLayout.HubPath, weftPath, func(worktreePath string) []string {
		return []string{"worktree", "add", worktreePath, branch}
	}); err != nil {
		return fmt.Errorf("adopt weft worktree %q for branch %q: %w", weftPath, branch, err)
	}
	return nil
}

// isRawWarpWorktree reports whether warpLayout's worktree lacks any lyx management markers.
// A worktree is raw when it has no _lyx junction or directory at its ANCHORED directory — the only
// place fabric ever wires one.
// Probing the worktree root instead misclassified every subpath-anchored, fully lyx-managed
// worktree as raw the moment its weft side went missing, driving reconcile into raw-adoption where
// a root-anchored hub in the identical state was reported unmanaged.
func isRawWarpWorktree(warpLayout *lyxcwd.Location) bool {
	lyxPath := filepath.Join(warpLayout.AnchorPath(), lyxdirs.LyxDirName)
	_, err := os.Lstat(lyxPath)
	return os.IsNotExist(err)
}

// createDormantWeftForRawWarp creates a weft branch and worktree for a raw warp
// worktree, leaving it dormant (no junction wiring). The weft branch forks from
// the current weft HEAD.
// rec is Reconcile's own recorder, threaded through to createWeftWorktree.
func createDormantWeftForRawWarp(rec *Mutations, warpLayout *lyxcwd.Location, slug, weftBranch string) error {
	weftRoot, err := WeftRepoRoot(warpLayout)
	if err != nil {
		return fmt.Errorf("resolve weft repo root: %w", err)
	}

	parentWeftOut, err := gitexec.Run(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		weftRoot,
	)
	if err != nil {
		return fmt.Errorf("capture parent weft branch: %w", err)
	}
	parentWeftBranch := strings.TrimSpace(parentWeftOut)

	if err := createWeftWorktree(rec, warpLayout, slug, weftBranch, parentWeftBranch); err != nil {
		return fmt.Errorf("create dormant weft worktree: %w", err)
	}

	return nil
}

// markVanishedMidWalk reports whether the pair at warpPath stopped existing during this pass, and
// when it did, rewrites pr as the vanished-mid-walk verdict — clearing any Error the disappearance
// produced along the way.
//
// The Error is cleared rather than kept alongside the new Action because nothing failed to
// reconcile: the pair stopped existing, which is a concurrent `remove`/`prune` doing exactly its
// job. Leaving the Error would make an ordinary concurrent teardown fail every enclosing reconcile,
// since a non-empty per-pair Error is what drives the verb's own non-zero exit.
//
// "Stopped existing" is decided by git's own worktree registration, not by the directory alone, and
// the difference is not academic: this pass may itself have RECREATED the directory on its way past,
// because wiring a junction creates the link's parent. Checking only the directory therefore missed
// exactly the interleaving where reconcile's own repair steps raced the teardown — the common case,
// not the rare one. A path git no longer lists is gone whether or not something left a directory
// standing there.
// The stat runs first purely as a cheap short-circuit: an absent directory needs no git spawn to
// settle, and the registration read only happens for a pair that is already reporting a problem.
// Neither check can mask a genuine defect: a repair that really failed leaves the worktree both on
// disk and registered.
func markVanishedMidWalk(repoDir, warpPath string, pr *ReconcilePairResult) bool {
	if _, statErr := os.Stat(warpPath); !os.IsNotExist(statErr) {
		if isAnyWorktreeOf(repoDir, warpPath) {
			return false
		}
	}
	pr.Action = ReconcileActionVanishedMidWalk
	pr.Detail = "warp worktree removed by a concurrent remove or prune after this pass enumerated it"
	pr.Error = ""
	return true
}

// readBranch returns the current branch name for the worktree at dir, reporting "HEAD" for a
// detached HEAD exactly as `git rev-parse --abbrev-ref HEAD` does.
//
// The rev-parse spelling alone is not enough: it exits 128 on an UNBORN branch (a branch with zero
// commits), which is the ordinary state of the weft primary immediately after a clone against an
// empty remote — the documented first-ever-setup path. Reporting that as an error made a
// just-cloned hub describe itself as out of sync and made Healthy fail loudly at loom preflight
// until the first sync landed a commit. `git branch --show-current` answers correctly on an unborn
// branch, so it is consulted as the fallback, and only a genuinely branch-less HEAD falls through
// to an error.
func readBranch(dir string) (string, error) {
	out, err := gitexec.Run(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		dir,
	)
	if err == nil {
		return strings.TrimSpace(out), nil
	}

	// The first call's *GitError stays bound across the fallback, because both downstream messages
	// below cite its exit code — this is the merge-rule-carve-outs "prior call" case, not a plain
	// two-message merge.
	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) {
		return "", fmt.Errorf("read current branch: %w", err)
	}

	unbornOut, unbornErr := gitexec.Run(
		[]string{"branch", "--show-current"},
		dir,
	)
	if unbornErr != nil {
		var unbornGitErr *gitexec.GitError
		if !errors.As(unbornErr, &unbornGitErr) {
			return "", fmt.Errorf("read current branch via the unborn-branch fallback: %w", unbornErr)
		}
		return "", fmt.Errorf("rev-parse exited %d and the unborn-branch fallback also failed: %w", gitErr.ExitCode, unbornErr)
	}
	branch := strings.TrimSpace(unbornOut)
	if branch == "" {
		return "", fmt.Errorf("rev-parse exited %d and no current branch is set", gitErr.ExitCode)
	}
	return branch, nil
}

// checkJunctionHealth verifies that every junction in WarpJunctionsHere(warpLayout, names)
// is a link resolving to its Target, reporting the first unhealthy one found.
// Returns (ok, reason) where ok is true only if every junction is correctly configured.
func checkJunctionHealth(warpLayout *lyxcwd.Location) (bool, string) {
	names, err := RepoWiredNames(warpLayout)
	if err != nil {
		return false, fmt.Sprintf("warp junction check unavailable: cannot load fabric.yaml: %v", err)
	}

	for _, j := range WarpJunctionsHere(warpLayout, names) {
		_, err := os.Lstat(j.Link)
		if err != nil {
			if os.IsNotExist(err) {
				return false, fmt.Sprintf("warp %s junction missing", j.Name)
			}
			return false, fmt.Sprintf("lstat error: %v", err)
		}

		isLink, err := fslink.IsLink(j.Link)
		if err != nil || !isLink {
			return false, fmt.Sprintf("warp %s is not a junction", j.Name)
		}

		warpResolved, err := fslink.PointsTo(j.Link)
		if err != nil {
			return false, fmt.Sprintf("resolve warp link: %v", err)
		}

		weftResolved, err := filepath.EvalSymlinks(filepath.Clean(j.Target))
		if err != nil {
			return false, fmt.Sprintf("resolve weft target: %v", err)
		}

		if filepath.Clean(warpResolved) != filepath.Clean(weftResolved) {
			return false, fmt.Sprintf("warp %s junction points elsewhere", j.Name)
		}
	}

	return true, ""
}

// junctionRepointedDetail formats ReconcileActionJunctionRepointed's Detail string,
// naming every junction in WarpJunctionsHere(warpLayout, names) as "Link → Target".
func junctionRepointedDetail(warpLayout *lyxcwd.Location) string {
	names, err := RepoWiredNames(warpLayout)
	if err != nil {
		return "junction re-pointed: cannot load fabric.yaml: " + err.Error()
	}

	junctions := WarpJunctionsHere(warpLayout, names)
	parts := make([]string, len(junctions))
	for i, j := range junctions {
		parts[i] = fmt.Sprintf("%s → %s", j.Link, j.Target)
	}
	return "junction re-pointed: " + strings.Join(parts, "; ")
}

// scanOnDiskJunctionNames lists the names of the fabric-owned link entries directly under the slug
// worktree's anchored directory, excluding hub-reserved names (_board/_portals/_launchers).
// Returns (nil, err) if the directory cannot be read;
// callers must treat a scan error as "skip removal", not as "the on-disk set is empty".
//
// Ownership, not merely link-ness, is the membership test, and it is load-bearing rather than
// defensive: the anchored directory is ordinary warp-repo content, so a hand-authored symlink
// checked into the user's repo (`latest -> v2`, `README.md -> docs/README.md`) sits right beside
// fabric's junctions.
// Treating every link as fabric's made `applyStaleRemoval` delete such a symlink out of the user's
// working tree, which is exactly the "fabric never deletes what might be user content" rule
// seedLyxJunction and unseedJunctionRecords already enforce everywhere else.
// A link is fabric-owned only when it resolves inside the paired weft worktree — the only root any
// fabric junction is ever created with.
// A link that cannot be resolved at all is deliberately NOT claimed: an unresolvable link cannot be
// proven fabric's, and unseedJunctionRecords already refuses to remove one for the same reason.
func scanOnDiskJunctionNames(l *lyxcwd.Location, slug string) ([]string, error) {
	dir := filepath.Join(WorktreePath(l, slug), l.AnchorRel)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	reserved := make(map[string]bool)
	for _, r := range HubReservedNames() {
		reserved[r] = true
	}

	var names []string
	for _, entry := range entries {
		if reserved[entry.Name()] {
			continue
		}
		link := filepath.Join(dir, entry.Name())
		isLink, err := fslink.IsLink(link)
		if err != nil {
			return nil, err
		}
		if !isLink {
			continue
		}
		owned, err := linkIsFabricOwned(l, slug, link)
		if err != nil {
			return nil, err
		}
		if owned {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// linkIsFabricOwned reports whether the link at linkPath resolves to a location fabric itself
// would have pointed a junction at: somewhere inside the slug's paired weft worktree.
// It returns (false, nil) — never an error — for a link whose target does not resolve, so an
// unreadable or dangling link is left alone rather than swept.
//
// The Hub Containment Invariant is why the weft worktree is the only root considered: with no
// fabric junction pointing at the board any more, claiming a link that resolves onto
// BoardDir(l.HubPath) would let the sweep remove an operator hand-made link pointing at
// <hub>/_board, which is exactly what the invariant says is never fabric's to claim.
func linkIsFabricOwned(l *lyxcwd.Location, slug, linkPath string) (bool, error) {
	resolved, err := fslink.PointsTo(linkPath)
	if err != nil {
		return false, nil
	}
	resolved = filepath.Clean(resolved)

	root := WeftWorktreePath(l, slug)
	normalizedRoot, rootErr := filepath.EvalSymlinks(root)
	if rootErr != nil {
		return false, nil
	}
	normalizedRoot = filepath.Clean(normalizedRoot)
	if resolved == normalizedRoot || strings.HasPrefix(resolved, normalizedRoot+string(filepath.Separator)) {
		return true, nil
	}
	return false, nil
}

// appendPrDetail appends text to pr.Detail, joining on "; " when a prior
// detail is already present. Shared by every reconcile step that annotates a
// pair's outcome without touching its Action or Error — applyStaleRemoval's
// skip-reasons above.
func appendPrDetail(pr *ReconcilePairResult, text string) {
	if pr.Detail == "" {
		pr.Detail = text
	} else {
		pr.Detail = pr.Detail + "; " + text
	}
}

// applyStaleRemoval converges warpLayout's on-disk junctions to the repo-wide pathspec
// by removing any junction present on disk but absent from RepoWiredNames. Fail-closed:
// if repo-wide fabric.yaml cannot be loaded or the on-disk scan fails, nothing is removed.
// rec is the calling verb's own recorder, threaded through to removeWarpJunction.
func applyStaleRemoval(rec *Mutations, warpLayout *lyxcwd.Location, slug string, pr *ReconcilePairResult) {
	desired, err := RepoWiredNames(warpLayout)
	if err != nil {
		appendPrDetail(pr, fmt.Sprintf("stale-removal skipped: cannot load repo-wide fabric.yaml: %v", err))
		return
	}

	onDisk, err := scanOnDiskJunctionNames(warpLayout, slug)
	if err != nil {
		appendPrDetail(pr, fmt.Sprintf("stale-removal skipped: cannot scan on-disk junctions: %v", err))
		return
	}

	desiredSet := make(map[string]bool, len(desired))
	for _, name := range desired {
		desiredSet[name] = true
	}

	var stale []string
	for _, name := range onDisk {
		if !desiredSet[name] {
			stale = append(stale, name)
		}
	}
	if len(stale) == 0 {
		return
	}

	var removed []string
	for _, name := range stale {
		// The exclude-strip and the removed-tally both run only after a nil-error removal: stripping
		// a still-present junction's .git/info/exclude entry (because its removal was refused or
		// failed) would leave that junction showing as untracked dirt in git status, and counting it
		// as removed would report an effect that did not land.
		if removeErr := removeWarpJunction(rec, warpLayout, slug, []string{name}); removeErr != nil {
			// applyStaleRemoval is a void helper with no propagation path, so a failed removal is
			// logged rather than silently discarded. A gate refusal and an operational failure are
			// logged distinctly, but neither counts the junction as removed — both leave it on disk.
			var refusal *destructiveRefusal
			if errors.As(removeErr, &refusal) {
				logger.Warn("fabricengine: reconcile stale-junction removal refused", "worktree", slug, "junction", name, "error", refusal.Error())
			} else {
				logger.Warn("fabricengine: reconcile stale-junction removal failed", "worktree", slug, "junction", name, "error", removeErr.Error())
			}
			continue
		}
		_, _ = unseedGitExclude(rec, warpLayout, slug, []string{name})
		removed = append(removed, name)
	}

	// Report convergence only when a junction actually came off disk. An all-refused (or all-failed)
	// pass converged nothing, so it must not append a possibly-empty removed-detail or flip Action to
	// stale_removed — the same report-the-effect-not-the-intent rule the reconcile honesty fix (M2)
	// established for the success verdict.
	if len(removed) == 0 {
		return
	}

	appendPrDetail(pr, fmt.Sprintf("stale junction(s) removed: %s", strings.Join(removed, ", ")))

	if pr.Action == ReconcileActionAlreadyHealthy {
		pr.Action = ReconcileActionStaleRemoved
	}
}

// reconcile.go implements the fabric repair-and-adopt sweep over paired warp and weft worktrees.
// It also holds readBranch and checkJunctionHealth, which the Status verb shares.
//
// The junction name set comes from the repo-wide fabric.yaml, so every worktree converges to one pathspec.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// Reconcile walks the warp worktrees reachable from layout l and restores a valid paired topology.
// For each warp worktree it applies these rules:
// recreate a missing weft, re-point broken junctions, adopt raw (non-lym) worktrees, or report unmanaged pairs.
// Per-worktree errors are recorded in the Error field of that pair's result.
// It judges each present weft's anchor marker before wiring that pair, with the prime weft first.
// A blank marker fails that pair alone and leaves its wiring untouched.
// A weft that carries a move record fails its pair the same way, with the record's way forward as the error.
// A weft with no committed marker gets the one its content implies, committed and handed to a detached push.
// After the pair loop it compares the recorded warp binding with the warp origin and reports the outcome.
func (t *Topology) Reconcile(l *lymcwd.Location) (res fabriclayout.ReconcileResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	entries, err := List(l.WorktreePath())
	if err != nil {
		return fabriclayout.ReconcileResult{}, fmt.Errorf("list worktrees: %w", err)
	}

	var result fabriclayout.ReconcileResult

	for _, entry := range entries {
		warpPath := filepath.FromSlash(entry.Path)
		warpPath = filepath.Clean(warpPath)

		slug := filepath.Base(warpPath)
		weftPath := RecordsWorktreePath(l, slug)

		pr := fabriclayout.ReconcilePairResult{
			CodeWorktree:    filepath.ToSlash(warpPath),
			RecordsWorktree: filepath.ToSlash(weftPath),
		}

		// The worktree list is read before the loop, so a concurrent remove or prune can delete a pair's directory.
		// Checking here keeps that race from surfacing as a layout or branch-read failure:
		// those report the unmanaged_reported Action and a raw os/exec chdir error.
		if _, statErr := os.Stat(warpPath); os.IsNotExist(statErr) {
			markVanishedMidWalk(l.WorktreePath(), warpPath, &pr)
			result.Pairs = append(result.Pairs, pr)
			continue
		}

		warpLayout, layoutErr := warpLayoutFor(l, warpPath)
		if layoutErr != nil {
			pr.Error = fmt.Sprintf("resolve layout: %v", layoutErr)
			pr.Action = fabriclayout.ReconcileActionUnmanagedReported
			markVanishedMidWalk(l.WorktreePath(), warpPath, &pr)
			result.Pairs = append(result.Pairs, pr)
			continue
		}

		weftStat, weftStatErr := os.Stat(weftPath)
		weftWorktreeExists := weftStatErr == nil && weftStat.IsDir()

		warpBranch, branchErr := readBranch(warpPath)
		if branchErr != nil {
			pr.Error = fmt.Sprintf("read warp branch: %v", branchErr)
			pr.Action = fabriclayout.ReconcileActionUnmanagedReported
			markVanishedMidWalk(l.WorktreePath(), warpPath, &pr)
			result.Pairs = append(result.Pairs, pr)
			continue
		}

		if !weftWorktreeExists {
			pr.Action = t.reconcileMissingWeft(rec, warpLayout, warpPath, weftPath, slug, warpBranch, &pr)
		}

		// A pair whose weft worktree was just recreated has junctions still pointing at directories that vanished with it,
		// so it takes the same wiring repair as a pair with an existing weft.
		// The recreated pair keeps its weft_recreated Action, not junction_repointed, so the report names what happened.
		// ReconcileActionRawAdopted does not fall through: it is dormant by design and wired by the next pass.
		repairWiring := weftWorktreeExists || (pr.Action == fabriclayout.ReconcileActionWeftRecreated && pr.Error == "")
		// A standing move record fails the pair before anything of it is touched:
		// its weft is mid-change, and wiring it at either anchor could lay a second junction set.
		moveStands := false
		if weftWorktreeExists {
			wayForward, standing, moveErr := MoveInProgress(slug, weftPath)
			switch {
			case moveErr != nil:
				pr.Error = moveErr.Error()
				moveStands = true
			case standing:
				pr.Error = wayForward
				moveStands = true
			}
		}
		if weftWorktreeExists && !moveStands {
			restoreWeftLockDir(rec, weftPath, &pr)
		}
		if moveStands {
			if pr.Action == "" {
				pr.Action = fabriclayout.ReconcileActionUnmanagedReported
			}
		} else if repairWiring {
			// Read after any recreation above, so a pair whose weft came back carrying its own marker is wired at that anchor.
			wiredLayout, markerOK := settleWeftMarker(rec, l.HubPath, weftPath, slug, &pr)
			if markerOK {
				t.repairPairWiring(rec, wiredLayout, slug, warpBranch, &pr, weftWorktreeExists)
			} else if pr.Action == "" {
				pr.Action = fabriclayout.ReconcileActionUnmanagedReported
			}
		}

		// The pre-check above covers only the window before this iteration starts.
		// A concurrent teardown can also land mid-repair, and the repair step then fails with a raw git or filesystem error.
		// Re-checking here covers every such window, not only the first, and it cannot mask a real defect:
		// a repair that genuinely failed leaves the warp worktree exactly where it was.
		if pr.Error != "" {
			markVanishedMidWalk(l.WorktreePath(), warpPath, &pr)
		}

		result.Pairs = append(result.Pairs, pr)
	}

	result.WarpBinding, result.WarpBindingDetail = reconcileWarpBinding(l)

	regenerateAfterVerb(rec, l, "reconcile")

	return result, nil
}

// settleWeftMarker judges the anchor marker of the weft worktree at weftPath, which belongs to the pair slug.
// It reports the pair's Location, read from its own marker, and whether wiring may go on.
//
// A marker that is blank in the working tree fails the pair in pr.Error.
// lymcwd reads a blank marker as absent.
// Wiring against that fallback would lay a second junction set at the repo root beside the live one.
// A weft with no committed marker gets an anchor derived from the tree committed on it,
// and the commit goes to a detached push; the anchor value is noted in pr.Detail.
// A failed spawn of that push is noted in pr.Detail too, since the next detached push carries the commit.
func settleWeftMarker(rec *Mutations, hubPath, weftPath, slug string, pr *fabriclayout.ReconcilePairResult) (*lymcwd.Location, bool) {
	current, err := lymcwd.NamedWorktree(hubPath, slug)
	if err != nil {
		pr.Error = fmt.Sprintf("read the anchor: %v", err)
		return nil, false
	}
	if err := refuseEmptyOwnMarker(weftPath); err != nil {
		pr.Error = err.Error()
		return current, false
	}
	_, found, err := committedMarker(weftPath, "HEAD")
	if err != nil {
		pr.Error = err.Error()
		return current, false
	}
	if found {
		return current, true
	}

	names, err := RepoWiredNames(current)
	if err != nil {
		pr.Error = fmt.Sprintf("record the anchor: load fabric config: %v", err)
		return current, false
	}
	anchor, err := treeAnchor(weftPath, "HEAD", names, current.AnchorRel)
	if err != nil {
		pr.Error = fmt.Sprintf("record the anchor: %v", err)
		return current, false
	}
	if _, err := commitAnchorMarker(rec, weftPath, anchor, "fabric reconcile: record anchor "+anchor); err != nil {
		pr.Error = fmt.Sprintf("record the anchor: %v", err)
		return current, false
	}
	appendPrDetail(pr, fmt.Sprintf("recorded anchor %s in %s", anchor, lymcwd.AnchorFileName))

	migrated, err := lymcwd.NamedWorktree(hubPath, slug)
	if err != nil {
		pr.Error = fmt.Sprintf("read the recorded anchor: %v", err)
		return current, false
	}
	if err := SpawnPairPush(migrated); err != nil {
		appendPrDetail(pr, fmt.Sprintf("the recorded anchor is not pushed yet: %v", err))
	}
	return migrated, true
}

// reconcileWarpBinding compares the .lym-warp record with the warp origin remote.
// It runs once per reconcile call, after the pair loop, and never per pair.
// It writes nothing and never returns an error:
// the comparison is a convenience that must not fail or downgrade a reconcile verdict.
func reconcileWarpBinding(l *lymcwd.Location) (fabriclayout.WarpBindingOutcome, string) {
	// The read-only `git remote get-url` falls outside the Fabric Git Invariant's mutating-warp-git rule.
	// It still targets the warp worktree.
	originOut, err := gitexec.Run([]string{"remote", "get-url", "origin"}, l.WorktreePath())
	origin := strings.TrimSpace(originOut)
	if err != nil || origin == "" {
		// An absent origin remote is a legitimate state, not an error condition.
		// It is normal for a synthetic test hub or a locally initialised warp.
		return fabriclayout.WarpBindingOutcomeSkipped, ""
	}

	recorded, found := readWarpBinding(fabriclayout.BoltDir(l.HubPath))
	if !found {
		return fabriclayout.WarpBindingOutcomeSkipped, "no warp binding is recorded"
	}
	if normalizeWarpURL(recorded) == normalizeWarpURL(origin) {
		return fabriclayout.WarpBindingOutcomePresent, ""
	}

	detail := fmt.Sprintf("recorded warp binding %s does not match warp origin %s", recorded, origin)
	if warpURLTransportIdentity(recorded) == warpURLTransportIdentity(origin) {
		// The record does not describe the transport in use,
		// so a transport-only spelling difference is advisory, not a genuine divergence: it is surfaced but never fatal.
		detail += "; the two spellings differ only by transport, which is advisory: the record does not describe the transport in use"
	}
	// Divergence is reported, never overwritten and never fatal.
	// Reconcile is the repair verb, and an unrelated URL mismatch must not block it.
	return fabriclayout.WarpBindingOutcomeDiverged, detail
}

// restoreWeftLockDir recreates an existing weft worktree's missing .weft lock directory.
// It records the creation in rec and notes it in pr's Detail, and it never changes pr.Action.
// A failure sets pr.Error.
func restoreWeftLockDir(rec *Mutations, weftPath string, pr *fabriclayout.ReconcilePairResult) {
	lockDir := filepath.Join(weftPath, fabriclayout.RecordsLockDirName)
	if _, err := os.Stat(lockDir); err == nil || !os.IsNotExist(err) {
		return
	}
	if _, err := ensureWeftLockDirAt(weftPath); err != nil {
		pr.Error = fmt.Sprintf("restore weft lock directory: %v", err)
		return
	}
	rec.Append(fabriclayout.KindDirCreated, lockDir, "")
	appendPrDetail(pr, fmt.Sprintf("weft lock directory restored at %s", lockDir))
}

// repairPairWiring converges one pair's junctions.
// It re-wires whatever checkJunctionHealth reports broken, and it applies declarative stale-removal.
//
// setAction says whether this call assigns the Action verdict.
// A pair whose weft worktree already existed has no Action yet, so this call assigns the verdict:
// already_healthy or junction_repointed.
// A pair whose weft worktree this same pass recreated already carries weft_recreated, which records what happened.
// That caller passes false and gets Detail notes only, so the value survives the repair.
// rec is Reconcile's own recorder, threaded through to every gate-reaching call this helper makes.
func (t *Topology) repairPairWiring(rec *Mutations, warpLayout *lymcwd.Location, slug, warpBranch string, pr *fabriclayout.ReconcilePairResult, setAction bool) {
	junctionHealthy, unhealthyReason := checkJunctionHealth(warpLayout)

	if !junctionHealthy {
		if setAction {
			pr.Action = fabriclayout.ReconcileActionJunctionRepointed
		}
		// Record WHAT was broken, not just that something was:
		// "missing", "not a junction" and "points at the wrong weft" are different operator problems with the same repair.
		appendPrDetail(pr, unhealthyReason)
		names, namesErr := RepoWiredNames(warpLayout)
		if namesErr != nil {
			pr.Error = fmt.Sprintf("re-point junction: load fabric config: %v", namesErr)
		} else if wireErr := wireJunctionSet(rec, warpLayout, slug, names); wireErr != nil {
			pr.Error = fmt.Sprintf("re-point junction: %v", wireErr)
		} else {
			appendPrDetail(pr, junctionRepointedDetail(warpLayout))
		}
	} else if setAction {
		pr.Action = fabriclayout.ReconcileActionAlreadyHealthy
	}

	// The scratch step runs on the healthy and the re-wired branch alike and never touches Action or Error:
	// scratch is agent convenience and must not downgrade a verdict about the pair.
	if detail := wireScratchLink(rec, warpLayout, slug).detail(); detail != "" {
		appendPrDetail(pr, detail)
	}

	applyStaleRemoval(rec, warpLayout, slug, pr)
}

// reconcileMissingWeft determines and applies the corrective action for a warp worktree that has no weft worktree.
// It recreates the worktree from the existing branch, adopts a raw warp worktree, or reports it unmanaged.
// A missing weft repo (the prime weft checkout that holds the weft gitdir) is diagnosed first and reported by name.
// Every corrective branch below needs the weft repo.
func (t *Topology) reconcileMissingWeft(
	rec *Mutations,
	warpLayout *lymcwd.Location,
	warpPath, weftPath, slug, warpBranch string,
	pr *fabriclayout.ReconcilePairResult,
) fabriclayout.ReconcileAction {
	weftBranch := fabriclayout.RecordsBranchName(warpBranch)

	if !weftRepoExists(warpLayout) {
		weftRepoRoot, weftRepoRootErr := RecordsRepoRoot(warpLayout)
		if weftRepoRootErr != nil {
			pr.Error = fmt.Sprintf("resolve weft repo root: %v", weftRepoRootErr)
		} else {
			pr.Error = fmt.Sprintf("weft repo missing at %s; restore it or re-clone the hub", weftRepoRoot)
		}
		return fabriclayout.ReconcileActionUnmanagedReported
	}

	exists, fromOrigin, resolveErr := resolveWeftBranch(rec, warpLayout, weftBranch, true)
	if resolveErr != nil {
		pr.Error = fmt.Sprintf("recreate weft worktree: %v", resolveErr)
		return fabriclayout.ReconcileActionWeftRecreated
	}
	if exists {
		weftRepoRoot, weftRepoRootErr := RecordsRepoRoot(warpLayout)
		if weftRepoRootErr == nil {
			// Bookkeeping only: a failed prune leaves a stale registration, which the adopt below re-reports.
			// A failed prune must not abort the repair.
			_, _ = gitexec.Run([]string{"worktree", "prune"}, weftRepoRoot)
		}

		if err := adoptWeftWorktree(warpLayout, weftPath, weftBranch); err != nil {
			pr.Error = fmt.Sprintf("recreate weft worktree: %v", err)
			if fromOrigin && weftRepoRootErr == nil {
				t.deleteAdoptedWeftBranch(rec, warpLayout, weftRepoRoot, weftBranch)
			}
			return fabriclayout.ReconcileActionWeftRecreated
		}
		source := "existed"
		if fromOrigin {
			source = "adopted from " + originRemoteName
		}
		pr.Detail = fmt.Sprintf("recreated weft worktree at %s (branch %s %s)", weftPath, weftBranch, source)
		return fabriclayout.ReconcileActionWeftRecreated
	}

	isRaw := isRawWarpWorktree(warpLayout)
	if isRaw {
		if err := createDormantWeftForRawWarp(rec, warpLayout, slug, weftBranch); err != nil {
			pr.Error = fmt.Sprintf("adopt raw warp worktree: %v", err)
			return fabriclayout.ReconcileActionRawAdopted
		}
		pr.Detail = fmt.Sprintf("adopted raw warp worktree at %s; weft branch %s created dormant (re-run lym fabric reconcile to wire it)", warpPath, weftBranch)
		return fabriclayout.ReconcileActionRawAdopted
	}

	pr.Detail = fmt.Sprintf(
		"warp worktree %s is on branch %s with no weft sibling; run `lym fabric add` or `lym fabric reconcile`",
		warpPath, warpBranch,
	)
	return fabriclayout.ReconcileActionUnmanagedReported
}

// deleteAdoptedWeftBranch deletes a local weft branch created from origin after adopting its worktree failed.
// The deletion keeps a surviving local weft branch an honest proof of a live pair.
// It never touches origin.
// A refused or failed deletion is a logged warning; the adopt error already on the pair stays.
func (t *Topology) deleteAdoptedWeftBranch(rec *Mutations, warpLayout *lymcwd.Location, weftRepoRoot, weftBranch string) {
	req := branchRequest{
		what:      "delete weft branch adopted from origin",
		repoDir:   weftRepoRoot,
		branch:    weftBranch,
		ownership: ownedManagedBranch(warpLayout),
		dirtiness: dirtyCheckedOutBranch(),
		force:     false,
	}
	err := deleteBranch(rec, req)
	if err == nil {
		return
	}
	var refusal *destructiveRefusal
	if errors.As(err, &refusal) {
		logger.Warn("fabricengine: reconcile's adopted-branch deletion was refused by the destructive gate", "branch", weftBranch, "check", string(refusal.Check))
		return
	}
	logger.Warn("fabricengine: reconcile could not delete the adopted weft branch", "branch", weftBranch, "error", err.Error())
}

// adoptWeftWorktree creates a git worktree at weftPath for the existing branch in the weft repo.
// The branch already exists, so no -b flag is used.
func adoptWeftWorktree(warpLayout *lymcwd.Location, weftPath, branch string) error {
	weftRepoRoot, weftRepoRootErr := RecordsRepoRoot(warpLayout)
	if weftRepoRootErr != nil {
		return fmt.Errorf("resolve weft repo root: %w", weftRepoRootErr)
	}
	// The worktree is added through containedWorktreeAdd, which keeps it inside the hub.
	// A symlink toggled at weftPath during reconcile's enumerate-then-adopt window cannot carry it out.
	if err := containedWorktreeAdd(weftRepoRoot, warpLayout.HubPath, weftPath, func(worktreePath string) []string {
		return []string{"worktree", "add", worktreePath, branch}
	}); err != nil {
		return fmt.Errorf("adopt weft worktree %q for branch %q: %w", weftPath, branch, err)
	}
	if _, err := ensureWeftLockDirAt(weftPath); err != nil {
		return fmt.Errorf("create weft lock dir in %q: %w", weftPath, err)
	}
	return nil
}

// isRawWarpWorktree reports whether warpLayout's worktree lacks any lym management markers.
// A worktree is raw when it has no _lym junction or directory at its anchored directory.
// Fabric wires that marker only there.
// The probe reads the anchored directory, not the worktree root.
func isRawWarpWorktree(warpLayout *lymcwd.Location) bool {
	lymPath := filepath.Join(warpLayout.AnchorPath(), lymdirs.LymDirName)
	_, err := os.Lstat(lymPath)
	return os.IsNotExist(err)
}

// createDormantWeftForRawWarp creates a weft branch and worktree for a raw warp worktree.
// The worktree is left dormant, with no junction wiring.
// The weft branch forks from the current weft HEAD.
// rec is the caller's recorder, passed on to the weft worktree creation.
func createDormantWeftForRawWarp(rec *Mutations, warpLayout *lymcwd.Location, slug, weftBranch string) error {
	weftRoot, err := RecordsRepoRoot(warpLayout)
	if err != nil {
		return fmt.Errorf("resolve weft repo root: %w", err)
	}

	parentWeftBranch, err := readBranch(weftRoot)
	if err != nil {
		return fmt.Errorf("capture parent weft branch: %w", err)
	}

	if err := createWeftWorktree(rec, warpLayout, slug, weftBranch, parentWeftBranch); err != nil {
		return fmt.Errorf("create dormant weft worktree: %w", err)
	}

	return nil
}

// markVanishedMidWalk reports whether the pair at warpPath stopped existing during this pass.
// When it did, it rewrites pr as the vanished-mid-walk verdict and clears any Error the disappearance produced.
//
// The Error is cleared because nothing failed to reconcile:
// the pair stopped existing, a concurrent `remove` or `prune` doing its job.
// A non-empty per-pair Error drives the verb's non-zero exit:
// keeping it would fail every enclosing reconcile over an ordinary teardown.
//
// Existence is decided by git's worktree registration, not by the directory alone.
// This pass may itself recreate the directory on its way past, because wiring a junction creates the link's parent.
// A path git no longer lists is gone whether or not a directory stands there.
// The stat runs first only as a cheap short-circuit: an absent directory needs no git spawn,
// and the registration read happens only for a pair already reporting a problem.
// Neither check can mask a genuine defect:
// a repair that failed leaves the worktree both on disk and registered.
func markVanishedMidWalk(repoDir, warpPath string, pr *fabriclayout.ReconcilePairResult) bool {
	if _, statErr := os.Stat(warpPath); !os.IsNotExist(statErr) {
		if isAnyWorktreeOf(repoDir, warpPath) {
			return false
		}
	}
	pr.Action = fabriclayout.ReconcileActionVanishedMidWalk
	pr.Detail = "warp worktree removed by a concurrent remove or prune after this pass enumerated it"
	pr.Error = ""
	return true
}

// readBranch returns the current branch name for the worktree at dir, reporting "HEAD" for a
// detached HEAD.
//
// An unborn branch (a branch with zero commits) reports its name.
// It is the ordinary state of the weft primary right after a clone of an empty remote.
// Reporting it as an error would make a freshly cloned hub look out of sync.
// Healthy would then fail at loom preflight until the first sync lands a commit.
func readBranch(dir string) (string, error) {
	branch, detached, err := gitrepo.New(dir).HeadRef()
	if err != nil {
		return "", fmt.Errorf("read current branch: %w", err)
	}
	if detached {
		return "HEAD", nil
	}
	return branch, nil
}

// checkJunctionHealth reports whether every junction the warp worktree wires is a link resolving to its target.
// When it is not, reason names the first unhealthy junction found.
func checkJunctionHealth(warpLayout *lymcwd.Location) (bool, string) {
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

// junctionRepointedDetail formats the Detail string of `ReconcileActionJunctionRepointed`.
// It names every warp junction as "Link → Target".
func junctionRepointedDetail(warpLayout *lymcwd.Location) string {
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
// worktree's anchored directory, excluding `HubReservedNames`.
// It returns a nil slice and the error when the directory cannot be read;
// callers must treat a scan error as "skip removal", not as an empty on-disk set.
//
// Ownership, not link-ness, is the membership test, because the anchored directory is ordinary warp-repo content:
// a hand-authored symlink checked into the user's repo, such as `latest -> v2`, sits beside fabric's junctions.
// Treating every link as fabric's would let `applyStaleRemoval` delete such a symlink.
// That breaks the rule that fabric never deletes what might be user content.
// A link is fabric-owned only when it resolves inside the paired weft worktree:
// the only root any fabric junction is created with.
// A link that cannot be resolved is not claimed, because it cannot be proven fabric's.
func scanOnDiskJunctionNames(l *lymcwd.Location, slug string) ([]string, error) {
	dir := filepath.Join(WorktreePath(l, slug), l.AnchorRel)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	reserved := make(map[string]bool)
	for _, r := range fabriclayout.HubReservedNames() {
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
// It returns (false, nil), never an error, for a link whose target does not resolve,
// so an unreadable or dangling link is left alone rather than swept.
//
// The Hub Containment Invariant is why the weft worktree is the only root considered.
// Claiming a link onto `BoltDir(l.HubPath)` would let the sweep remove an operator's own link to the Bolt checkout:
// the invariant says such a link is never fabric's to claim.
func linkIsFabricOwned(l *lymcwd.Location, slug, linkPath string) (bool, error) {
	resolved, err := fslink.PointsTo(linkPath)
	if err != nil {
		return false, nil
	}
	resolved = filepath.Clean(resolved)

	root := RecordsWorktreePath(l, slug)
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

// appendPrDetail appends text to pr.Detail, joining on "; " when a prior detail is already present.
// Reconcile steps use it to annotate a pair's outcome without touching its Action or Error.
func appendPrDetail(pr *fabriclayout.ReconcilePairResult, text string) {
	if pr.Detail == "" {
		pr.Detail = text
	} else {
		pr.Detail = pr.Detail + "; " + text
	}
}

// applyStaleRemoval converges warpLayout's on-disk junctions to the repo-wide pathspec
// by removing any junction present on disk but absent from RepoWiredNames.
// It fails closed: if repo-wide fabric.yaml cannot be loaded or the on-disk scan fails, nothing is removed.
// The scratch link is never a stale junction, so the sweep leaves it and its exclude line alone.
// rec is the calling verb's recorder, passed on to `removeWarpJunction`.
func applyStaleRemoval(rec *Mutations, warpLayout *lymcwd.Location, slug string, pr *fabriclayout.ReconcilePairResult) {
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
	onDisk = withoutScratchLink(onDisk)

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
		// The exclude-strip and the removed-tally run only after a nil-error removal.
		// A refused or failed removal leaves the junction on disk.
		// Stripping its exclude entry would show it as untracked dirt in git status.
		// Counting it as removed would report an effect that did not land.
		if removeErr := removeWarpJunction(rec, warpLayout, slug, []string{name}); removeErr != nil {
			// applyStaleRemoval is a void helper with no propagation path,
			// so a failed removal is logged rather than silently discarded.
			// A gate refusal and an operational failure are logged distinctly.
			// Neither counts the junction as removed; both leave it on disk.
			var refusal *destructiveRefusal
			if errors.As(removeErr, &refusal) {
				logger.Warn("fabricengine: reconcile stale-junction removal refused", "worktree", slug, "junction", name, "error", refusal.Error())
			} else {
				logger.Warn("fabricengine: reconcile stale-junction removal failed", "worktree", slug, "junction", name, "error", removeErr.Error())
			}
			continue
		}
		_, _ = unseedGitExclude(rec, warpLayout, slug, []string{name}, false)
		removed = append(removed, name)
	}

	// Report convergence only when a junction actually came off disk.
	// An all-refused or all-failed pass converged nothing,
	// so it must not append a possibly-empty removed-detail or flip Action to stale_removed.
	if len(removed) == 0 {
		return
	}

	appendPrDetail(pr, fmt.Sprintf("stale junction(s) removed: %s", strings.Join(removed, ", ")))

	if pr.Action == fabriclayout.ReconcileActionAlreadyHealthy {
		pr.Action = fabriclayout.ReconcileActionStaleRemoved
	}
}

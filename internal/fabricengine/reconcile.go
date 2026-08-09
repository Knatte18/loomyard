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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
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

	// ReconcileActionStaleRemoved means the pair's junction/repoint check found nothing to add or
	// re-point,
	// but declarative stale-removal deleted at least one on-disk junction absent from the repo-wide
	// pathspec.
	// It is reported instead of ReconcileActionAlreadyHealthy so consumers keying off Action — not
	// just Detail — see that convergence altered the pair.
	ReconcileActionStaleRemoved ReconcileAction = "stale_removed"
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
type ReconcileResult struct {
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
func (t *Topology) Reconcile(l *lyxcwd.Location) (ReconcileResult, error) {
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

		warpLayout, layoutErr := warpLayoutFor(l, warpPath)
		if layoutErr != nil {
			pr.Error = fmt.Sprintf("resolve layout: %v", layoutErr)
			pr.Action = ReconcileActionUnmanagedReported
			result.Pairs = append(result.Pairs, pr)
			continue
		}

		weftStat, weftStatErr := os.Stat(weftPath)
		weftWorktreeExists := weftStatErr == nil && weftStat.IsDir()

		warpBranch, branchErr := readBranch(warpPath)
		if branchErr != nil {
			pr.Error = fmt.Sprintf("read warp branch: %v", branchErr)
			pr.Action = ReconcileActionUnmanagedReported
			result.Pairs = append(result.Pairs, pr)
			continue
		}

		if !weftWorktreeExists {
			pr.Action = t.reconcileMissingWeft(warpLayout, warpPath, weftPath, slug, warpBranch, &pr)
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
			t.repairPairWiring(warpLayout, slug, &pr, weftWorktreeExists)
		}

		result.Pairs = append(result.Pairs, pr)
	}

	result.WarpBinding, result.WarpBindingDetail = t.reconcileWarpBinding(l)

	return result, nil
}

// reconcileWarpBinding backfills the once-per-hub .lyx-warp record from the warp side's origin
// remote, for every hub that predates the binding.
// It runs exactly once per Reconcile call, after the pair loop, never per-pair, and it never
// returns an error: like wireBoardLink's board-junction repair, a binding backfill is a convenience
// that may never fail or downgrade a reconcile verdict, so any failure is folded into a Deferred
// outcome instead.
func (t *Topology) reconcileWarpBinding(l *lyxcwd.Location) (WarpBindingOutcome, string) {
	boardDir := BoardDir(l.HubPath)

	// Re-seed the weft repo's operational excludes on every reconcile pass. The seeding is
	// idempotent and otherwise only runs from a weft-git verb or a wiring call, so a hub wired by an
	// earlier binary would never pick up a newly added artifact pattern — and the dirty-board gate
	// below reads exactly the status those patterns govern, so a hub that never runs a weft-git verb
	// would defer its backfill forever on an artifact the current binary excludes.
	// A seeding failure is not fatal: the gate simply sees whatever git reports.
	_ = seedWeftArtifactExcludes(boardDir)

	// git remote get-url is read-only, so it falls outside the Fabric Git Invariant's
	// mutating-warp-git rule even though it targets the warp worktree.
	originOut, _, exitCode, err := gitexec.RunGit([]string{"remote", "get-url", "origin"}, l.WorktreePath())
	origin := strings.TrimSpace(originOut)
	if err != nil || exitCode != 0 || origin == "" {
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
	statusOut, _, statusExit, statusErr := gitexec.RunGit([]string{"status", "--porcelain"}, boardDir)
	if statusErr != nil || statusExit != 0 {
		return WarpBindingOutcomeDeferred, fmt.Sprintf("board status check failed: %v (exit %d)", statusErr, statusExit)
	}
	if strings.TrimSpace(statusOut) != "" {
		return WarpBindingOutcomeDeferred, "board worktree has uncommitted changes; backfill deferred to avoid sweeping them into an unrelated commit"
	}

	if err := writeWarpBinding(boardDir, origin); err != nil {
		return WarpBindingOutcomeDeferred, fmt.Sprintf("write warp binding: %v", err)
	}
	return WarpBindingOutcomeRecorded, fmt.Sprintf("recorded warp binding %s", origin)
}

// repairPairWiring converges one pair's junctions: it re-wires whatever checkJunctionHealth reports
// broken, always re-wires the operator-convenience _board link, and applies declarative
// stale-removal.
//
// setAction distinguishes the two callers. A pair whose weft worktree already existed has no Action
// yet, so this call assigns the verdict (already_healthy / junction_repointed). A pair whose weft
// worktree this same pass recreated already carries weft_recreated, which names what actually
// happened and must survive the repair, so that caller passes false and gets Detail notes only.
//
// The _board re-wire is unconditional with respect to junction health: checkJunctionHealth only ever
// inspects the pathspec name-set, which _board is deliberately outside, so a pair whose only broken
// link is _board reports healthy and would never be repaired if this call sat inside the
// unhealthy branch. A wiring failure there is surfaced as a Detail note, never as an Error or a
// changed Action — this convenience link must never be able to downgrade a reconcile verdict.
func (t *Topology) repairPairWiring(warpLayout *lyxcwd.Location, slug string, pr *ReconcilePairResult, setAction bool) {
	junctionHealthy, _ := checkJunctionHealth(warpLayout)

	if !junctionHealthy {
		if setAction {
			pr.Action = ReconcileActionJunctionRepointed
		}
		names, namesErr := RepoWiredNames(warpLayout)
		if namesErr != nil {
			pr.Error = fmt.Sprintf("re-point junction: load fabric config: %v", namesErr)
		} else if wireErr := WireJunctions(warpLayout, slug, names); wireErr != nil {
			pr.Error = fmt.Sprintf("re-point junction: %v", wireErr)
		} else {
			appendPrDetail(pr, junctionRepointedDetail(warpLayout))
		}
	} else if setAction {
		pr.Action = ReconcileActionAlreadyHealthy
	}

	if boardErr := wireBoardLink(warpLayout, slug); boardErr != nil {
		appendPrDetail(pr, fmt.Sprintf("board junction wiring failed: %v", boardErr))
	}

	applyStaleRemoval(warpLayout, slug, pr)
}

// reconcileMissingWeft determines and applies the corrective action when a weft worktree
// does not exist for the given warp worktree: recreate from the existing branch,
// adopt a raw worktree, or report unmanaged.
func (t *Topology) reconcileMissingWeft(
	warpLayout *lyxcwd.Location,
	warpPath, weftPath, slug, warpBranch string,
	pr *ReconcilePairResult,
) ReconcileAction {
	weftBranch := WeftBranchName(warpBranch)

	if weftBranchExists(warpLayout, weftBranch) {
		if weftRepoRoot, weftRepoRootErr := WeftRepoRoot(warpLayout); weftRepoRootErr == nil {
			_, _, _, _ = gitexec.RunGit([]string{"worktree", "prune"}, weftRepoRoot)
		}

		if err := adoptWeftWorktree(warpLayout, weftPath, weftBranch); err != nil {
			pr.Error = fmt.Sprintf("recreate weft worktree: %v", err)
			return ReconcileActionWeftRecreated
		}
		pr.Detail = fmt.Sprintf("recreated weft worktree at %s (branch %s existed)", weftPath, weftBranch)
		return ReconcileActionWeftRecreated
	}

	isRaw := isRawWarpWorktree(warpPath)
	if isRaw {
		if err := createDormantWeftForRawWarp(warpLayout, slug, weftBranch); err != nil {
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
	_, _, exitCode, err := gitexec.RunGit(
		[]string{"worktree", "add", weftPath, branch},
		weftRepoRoot,
	)
	if err != nil {
		return fmt.Errorf("git worktree add: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("adopt weft worktree %q for branch %q failed (git exit %d)", weftPath, branch, exitCode)
	}
	return nil
}

// isRawWarpWorktree reports whether the worktree at warpPath lacks any lyx management
// markers. A worktree is raw when it has no _lyx junction or directory.
func isRawWarpWorktree(warpPath string) bool {
	lyxPath := filepath.Join(warpPath, lyxdirs.LyxDirName)
	_, err := os.Lstat(lyxPath)
	return os.IsNotExist(err)
}

// createDormantWeftForRawWarp creates a weft branch and worktree for a raw warp
// worktree, leaving it dormant (no junction wiring). The weft branch forks from
// the current weft HEAD.
func createDormantWeftForRawWarp(warpLayout *lyxcwd.Location, slug, weftBranch string) error {
	weftRoot, err := WeftRepoRoot(warpLayout)
	if err != nil {
		return fmt.Errorf("resolve weft repo root: %w", err)
	}

	parentWeftOut, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		weftRoot,
	)
	if err != nil {
		return fmt.Errorf("capture parent weft branch: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("capture parent weft branch failed with exit code %d", exitCode)
	}
	parentWeftBranch := strings.TrimSpace(parentWeftOut)

	if err := createWeftWorktree(warpLayout, slug, weftBranch, parentWeftBranch); err != nil {
		return fmt.Errorf("create dormant weft worktree: %w", err)
	}

	return nil
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
	out, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		dir,
	)
	if err != nil {
		return "", fmt.Errorf("rev-parse: %w", err)
	}
	if exitCode == 0 {
		return strings.TrimSpace(out), nil
	}

	unbornOut, _, unbornExit, unbornErr := gitexec.RunGit(
		[]string{"branch", "--show-current"},
		dir,
	)
	if unbornErr != nil {
		return "", fmt.Errorf("branch --show-current: %w", unbornErr)
	}
	if unbornExit != 0 {
		return "", fmt.Errorf("rev-parse exited %d and branch --show-current exited %d", exitCode, unbornExit)
	}
	branch := strings.TrimSpace(unbornOut)
	if branch == "" {
		return "", fmt.Errorf("rev-parse exited %d and no current branch is set", exitCode)
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
// A link is fabric-owned only when it resolves inside the paired weft worktree or onto the hub's
// board directory — the only two targets any fabric junction is ever created with.
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
// would have pointed a junction at: somewhere inside the slug's paired weft worktree, or the hub's
// board directory.
// It returns (false, nil) — never an error — for a link whose target does not resolve, so an
// unreadable or dangling link is left alone rather than swept.
func linkIsFabricOwned(l *lyxcwd.Location, slug, linkPath string) (bool, error) {
	resolved, err := fslink.PointsTo(linkPath)
	if err != nil {
		return false, nil
	}
	resolved = filepath.Clean(resolved)

	for _, root := range []string{WeftWorktreePath(l, slug), BoardDir(l.HubPath)} {
		normalizedRoot, rootErr := filepath.EvalSymlinks(root)
		if rootErr != nil {
			continue
		}
		normalizedRoot = filepath.Clean(normalizedRoot)
		if resolved == normalizedRoot || strings.HasPrefix(resolved, normalizedRoot+string(filepath.Separator)) {
			return true, nil
		}
	}
	return false, nil
}

// appendPrDetail appends text to pr.Detail, joining on "; " when a prior
// detail is already present. Shared by every reconcile step that annotates a
// pair's outcome without touching its Action or Error — applyStaleRemoval's
// skip-reasons and wireBoardLink's failure note in Reconcile above.
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
func applyStaleRemoval(warpLayout *lyxcwd.Location, slug string, pr *ReconcilePairResult) {
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
		_ = removeWarpJunction(warpLayout, slug, []string{name})
		_, _ = unseedGitExclude(warpLayout, slug, []string{name})
		removed = append(removed, name)
	}

	appendPrDetail(pr, fmt.Sprintf("stale junction(s) removed: %s", strings.Join(removed, ", ")))

	if pr.Action == ReconcileActionAlreadyHealthy {
		pr.Action = ReconcileActionStaleRemoved
	}
}

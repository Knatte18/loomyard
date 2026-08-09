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
			pairedAction := t.reconcileMissingWeft(warpLayout, warpPath, weftPath, slug, warpBranch, &pr)
			pr.Action = pairedAction
		} else {
			junctionHealthy, _ := checkJunctionHealth(warpLayout)

			if !junctionHealthy {
				names, namesErr := RepoWiredNames(warpLayout)
				if namesErr != nil {
					pr.Error = fmt.Sprintf("re-point junction: load fabric config: %v", namesErr)
					pr.Action = ReconcileActionJunctionRepointed
				} else if wireErr := WireJunctions(warpLayout, slug, names); wireErr != nil {
					pr.Error = fmt.Sprintf("re-point junction: %v", wireErr)
					pr.Action = ReconcileActionJunctionRepointed
				} else {
					pr.Action = ReconcileActionJunctionRepointed
					pr.Detail = junctionRepointedDetail(warpLayout)
				}
			} else {
				pr.Action = ReconcileActionAlreadyHealthy
			}

			// Re-wire the operator-convenience _board junction unconditionally,
			// regardless of junctionHealthy above: checkJunctionHealth only
			// ever inspects the pathspec name-set, which _board is
			// deliberately outside, so a pair whose *only* broken link is
			// _board would report junctionHealthy == true and never reach
			// here if this call sat inside the `!junctionHealthy` branch. A
			// wiring failure is surfaced as a Detail note, never as an Error
			// or a changed Action — this convenience link must never be able
			// to downgrade a reconcile verdict.
			if boardErr := wireBoardLink(warpLayout, slug); boardErr != nil {
				appendPrDetail(&pr, fmt.Sprintf("board junction wiring failed: %v", boardErr))
			}

			applyStaleRemoval(warpLayout, slug, &pr)
		}

		result.Pairs = append(result.Pairs, pr)
	}

	return result, nil
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

// readBranch returns the current branch name for the worktree at dir via rev-parse.
func readBranch(dir string) (string, error) {
	out, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		dir,
	)
	if err != nil {
		return "", fmt.Errorf("rev-parse: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("rev-parse exited %d", exitCode)
	}
	return strings.TrimSpace(out), nil
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

// scanOnDiskJunctionNames lists the names of link entries directly under
// filepath.Join(worktreeRoot, relPath), excluding hub-reserved names
// (_board/_portals/_launchers). Returns (nil, err) if the directory
// cannot be read; callers must treat a scan error as "skip removal", not
// as "the on-disk set is empty".
func scanOnDiskJunctionNames(worktreeRoot, relPath string) ([]string, error) {
	dir := filepath.Join(worktreeRoot, relPath)
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
		isLink, err := fslink.IsLink(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if isLink {
			names = append(names, entry.Name())
		}
	}
	return names, nil
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

	onDisk, err := scanOnDiskJunctionNames(warpLayout.WorktreePath(), warpLayout.AnchorRel)
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

// cleanup.go implements the Cleanup verb: it finds weft branches that have no
// corresponding warp worktree sibling and deletes them according to a flag matrix.
//
// Flag matrix:
//   - apply == false → dry-run/report only; nothing is deleted.
//   - apply == true  → deletes every orphan weft branch that is not the primary weft branch, not
//     checked out at a worktree, and not unmanaged (no "-weft" suffix, e.g. inherited from history
//     predating fabric's uniform naming scheme).
//
// force is reserved and currently consulted by no gate in this verb; see Topology.Cleanup's own
// doc comment.
//
// A weft branch's warp sibling is recovered via WeftWarpSlug(branch) —
// inverting WeftBranchName's suffix. The weft repo may also hold non-suffixed weft
// branches inherited from history predating fabric's uniform naming scheme;
// WeftWarpSlug rejects those (ok == false), and by definition a non-suffixed weft
// branch is not fabric-managed — it is reported but never deleted, matching the
// report-but-don't-touch rule Reconcile applies to unmanaged branches, rather than
// the raddle-fold-back gate.
//
// Liveness is judged in BRANCH space, not against worktree directory names: a weft
// branch <warpBranch>-weft is a live pair iff some warp worktree is currently checked
// out on <warpBranch>. A weft branch that is itself checked out at some worktree is
// additionally always protected, whatever the liveness verdict says: git branch -D
// can never delete a checked-out branch, and a checked-out weft branch means the pair
// is still materialized on disk — e.g. a live pair whose warp worktree sits on a
// detached HEAD, which branch-space liveness cannot see. Liveness-in-branch-space is
// the one point where fabric must diverge from warp's original logic. warp compared the weft branch's stripped slug against warp worktree
// *directory* base names; that comparison is wrong for the primary pair, whose warp
// worktree directory is the repo name (e.g. "lyx-fabric-test") while its branch is
// "main". Under warp the mistake is harmless because warp's primary weft branch is the
// unsuffixed "main" (WeftWarpSlug rejects it → protected as unmanaged). Under fabric's
// uniform suffix scheme the primary weft branch is "main-weft", which WeftWarpSlug
// accepts — so a directory-name comparison would misclassify it as a deletable orphan
// and delete the very branch board's reserved weft:main branch requires to stay
// permanent, distinct from it. Comparing
// against live warp *branches* protects every task pair with no BranchPrefix juggling — but NOT the
// primary weft branch, which needs the explicit carve-out described below.
//
// Board needs no explicit exclusion: its weft:main branch is the warp's own
// unsuffixed default branch (e.g. "main"), which never matches the
// -weft-suffixed pattern WeftWarpSlug looks for, so it is never enumerated
// as a Cleanup candidate in the first place — not because of any
// repo-level carve-out, but because Cleanup only ever walks weft branches
// and compares them against the set of known warp worktree slugs.
//
// The PRIMARY WEFT branch ("main-weft") does need an explicit carve-out, and gets one via
// primaryWeftBranch. Branch-space liveness protects it only while the prime warp worktree happens to
// sit on the repo's default branch, which `lyx fabric checkout` exists to change; one checkout onto
// any other branch promoted the durable weft line to a deletable orphan, and
// `cleanup --apply --force` then deleted it along with any unpushed weft commit it alone carried.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// CleanupBranchEntry describes the fate of one orphaned weft branch under Cleanup.
type CleanupBranchEntry struct {
	// Branch is the weft branch name.
	Branch string `json:"branch"`
	// Deleted reports whether the branch was actually deleted from the weft repo.
	// It is false on a dry run, when the entry was skipped due to gate protection or
	// unmanaged (non-suffixed) status, and when deletion itself failed.
	Deleted bool `json:"deleted"`
	// Protected reports whether the branch was skipped rather than deleted —
	// because the branch is not fabric-managed (no "-weft" suffix, e.g. inherited from history
	// predating fabric's uniform naming scheme), or because the branch is
	// currently checked out at a worktree (git branch -D could never delete it).
	Protected bool `json:"protected,omitempty"`
	// Error is non-empty when apply is true and branch deletion failed.
	Error string `json:"error,omitempty"`
}

// CleanupResult is the top-level result type returned by Cleanup.
// It lists every orphaned weft branch, whether deleted, protected, or reported only, and embeds
// MutationRecord, which carries the mutation record accumulated over the call.
type CleanupResult struct {
	MutationRecord
	// Entries lists the orphaned weft branches and their dispositions.
	Entries []CleanupBranchEntry `json:"entries"`
}

// Cleanup finds weft branches with no corresponding warp worktree sibling and reports or deletes
// them per the flag matrix: apply gates whether any deletion happens, checked-out branches are
// always protected. The repo's primary weft branch is protected unconditionally, in every mode —
// see primaryWeftBranch for why branch-space liveness alone cannot protect it.
// force is reserved and currently consulted by no gate in this verb: deleteWeftBranch already
// hardcodes force: false for its own request, and that stays true.
func (t *Topology) Cleanup(l *lyxcwd.Location, apply, force bool) (res CleanupResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	// Enumerate warp worktrees using git-registered entries only.
	entries, err := List(l.WorktreePath())
	if err != nil {
		return CleanupResult{}, fmt.Errorf("list warp worktrees: %w", err)
	}

	primaryWeft, err := primaryWeftBranch(l)
	if err != nil {
		return CleanupResult{}, err
	}

	// Build the set of live warp branches; unreadable branches (stale registrations) skip.
	liveWarpBranches := make(map[string]bool, len(entries))
	for _, entry := range entries {
		warpPath := filepath.Clean(filepath.FromSlash(entry.Path))
		branch, branchErr := readBranch(warpPath)
		if branchErr != nil {
			continue
		}
		liveWarpBranches[branch] = true
	}

	// Enumerate all branches in the weft repo to find orphans.
	weftBranches, err := listWeftBranches(l)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("list weft branches: %w", err)
	}

	var result CleanupResult

	for _, weftBranch := range weftBranches {
		branch := weftBranch.Branch

		// Recover the warp branch by inverting WeftBranchName's suffix.
		// Non-fabric-managed branches are reported but never deleted.
		warpBranch, ok := WeftWarpSlug(branch)
		if !ok {
			result.Entries = append(result.Entries, CleanupBranchEntry{
				Branch:    branch,
				Protected: true,
			})
			continue
		}

		if liveWarpBranches[warpBranch] {
			// Live pair: a warp worktree is on this weft branch's paired warp branch.
			continue
		}

		entry := CleanupBranchEntry{
			Branch: branch,
		}

		if branch == primaryWeft {
			// The repo's primary weft line, never an orphan however the prime worktree is
			// currently checked out.
			entry.Protected = true
			result.Entries = append(result.Entries, entry)
			continue
		}

		if weftBranch.WorktreePath != "" {
			// Checked-out branch: always protected, never deletable.
			entry.Protected = true
			result.Entries = append(result.Entries, entry)
			continue
		}

		if !apply {
			result.Entries = append(result.Entries, entry)
			continue
		}

		entry.Deleted = deleteWeftBranch(rec, l, branch, t.cfg.BranchPrefix, &entry)
		result.Entries = append(result.Entries, entry)
	}

	return result, nil
}

// primaryWeftBranch returns the weft branch paired with the repo's primary warp branch — the one
// weft line Cleanup must never enumerate as an orphan.
//
// The source is the branch `<Hub>/_board` is checked out on: ensureBoardWorktree materialises that
// worktree on the warp's own unsuffixed default branch and nothing moves it afterwards, so it is the
// hub's durable record of which warp branch is primary.
// Branch-space liveness cannot answer this on its own, and the difference is a data-loss bug rather
// than a nicety: liveness asks whether some warp worktree is CURRENTLY checked out on the paired
// warp branch, so a single `lyx fabric checkout <other>` on the prime pair makes `main` not-live and
// promotes `main-weft` — the durable `_lyx` line, unpushed commits included — to a deletable orphan.
//
// A hub whose board branch cannot be read is refused rather than swept: Cleanup's deletions are
// irreversible, so an unreadable primary is the one direction that must fail closed.
func primaryWeftBranch(l *lyxcwd.Location) (string, error) {
	boardDir := BoardDir(l.HubPath)
	boardBranch, err := readBranch(boardDir)
	if err != nil {
		return "", fmt.Errorf(
			"cannot determine the repo's primary weft branch from the %s worktree at %s (%w); refusing to enumerate orphan weft branches",
			BoardDirName, boardDir, err)
	}
	boardBranch = strings.TrimSpace(boardBranch)
	if boardBranch == "" || boardBranch == "HEAD" {
		return "", fmt.Errorf(
			"the %s worktree at %s is not on a named branch; refusing to enumerate orphan weft branches",
			BoardDirName, boardDir)
	}
	return WeftBranchName(boardBranch), nil
}

// weftBranchCheckout pairs a weft branch name with its checked-out worktree path if any.
type weftBranchCheckout struct {
	Branch       string
	WorktreePath string
}

// listWeftBranches returns every branch in the weft repo with its checked-out worktree path if any.
func listWeftBranches(l *lyxcwd.Location) ([]weftBranchCheckout, error) {
	weftRepoRoot, err := WeftRepoRoot(l)
	if err != nil {
		return nil, fmt.Errorf("resolve weft repo root: %w", err)
	}
	out, err := gitexec.Run(
		[]string{"branch", "--format=%(refname:short)\x1f%(worktreepath)"},
		weftRepoRoot,
	)
	if err != nil {
		return nil, fmt.Errorf("list weft branches failed: %w", err)
	}

	raw := strings.TrimSpace(out)
	if raw == "" {
		return nil, nil
	}

	var branches []weftBranchCheckout
	for _, line := range strings.Split(raw, "\n") {
		name, worktreePath, _ := strings.Cut(strings.TrimSpace(line), "\x1f")
		if name == "" {
			continue
		}
		branches = append(branches, weftBranchCheckout{
			Branch:       name,
			WorktreePath: strings.TrimSpace(worktreePath),
		})
	}
	return branches, nil
}

// deleteWeftBranch deletes a weft branch through the gate's deleteBranch executor, recording errors
// in entry. force is always false here even when Topology.Cleanup was called with force true:
// --force there answers the folded-back-raddle gate above, evaluated before this call is reached, and
// may not answer the gate's own primary-weft carve-out or checked-out check.
// rec is the calling verb's own recorder, passed straight through to deleteBranch.
func deleteWeftBranch(rec *Mutations, l *lyxcwd.Location, branch, branchPrefix string, entry *CleanupBranchEntry) bool {
	weftRepoRoot, err := WeftRepoRoot(l)
	if err != nil {
		entry.Error = fmt.Sprintf("resolve weft repo root: %v", err)
		return false
	}
	req := branchRequest{
		what:      "delete weft branch",
		repoDir:   weftRepoRoot,
		branch:    branch,
		ownership: ownedManagedBranch(l, branchPrefix),
		dirtiness: dirtyCheckedOutBranch(),
		force:     false,
	}
	if err := deleteBranch(rec, req); err != nil {
		entry.Error = fmt.Sprintf("delete weft branch %q failed: %v", branch, err)
		return false
	}
	return true
}

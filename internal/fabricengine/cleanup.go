// cleanup.go implements the Cleanup verb: it finds weft branches that have no
// corresponding warp worktree sibling and deletes them according to a flag matrix.
//
// Flag matrix:
//   - apply == false → dry-run/report only; nothing is deleted.
//   - apply == true  → deletes every orphan weft branch that is not the primary weft branch, not
//     checked out at a worktree, and not unmanaged (no "-weft" suffix, e.g. inherited from history
//     predating fabric's uniform naming scheme).
//   - remote is independent of --force and requires apply to delete anything (remote alone with
//     apply false is still a dry run): when true, every orphan weft branch actually deleted locally
//     is also deleted on the weft repo's origin remote. A weft repo with no origin remote configured
//     reports that once per verb, on the result's RemoteSkippedReason, rather than once per branch.
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
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
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
	// Error is non-empty when apply is true and branch deletion failed. A non-empty Error makes
	// `lyx fabric cleanup --apply` exit non-zero, because it names a genuine failure of the local
	// `git branch -D` rather than a designed refusal — Protected and unmanaged entries set no Error
	// at all.
	Error string `json:"error,omitempty"`
	// RemoteDeleted reports whether this branch's copy on the remote was observably removed. It is
	// true only when the remote deletion was attempted and actually removed a ref.
	RemoteDeleted bool `json:"remote_deleted,omitempty"`
	// RemoteError is non-empty when the remote deletion was attempted and did not succeed. Its text
	// always names the layer that said no — the gate's own refusal, or the remote deletion itself.
	RemoteError string `json:"remote_error,omitempty"`
}

// CleanupResult is the top-level result type returned by Cleanup.
// It lists every orphaned weft branch, whether deleted, protected, or reported only, and embeds
// MutationRecord, which carries the mutation record accumulated over the call.
type CleanupResult struct {
	MutationRecord
	// Entries lists the orphaned weft branches and their dispositions.
	Entries []CleanupBranchEntry `json:"entries"`
	// RemoteSkippedReason carries a once-per-verb reason no remote deletion was attempted at all —
	// today only a weft repo with no origin remote configured. It is deliberately not RemoteError,
	// because RemoteError is the field the CLI switches its exit code on and a missing origin must
	// exit 0.
	RemoteSkippedReason string `json:"remote_skipped_reason,omitempty"`
}

// Cleanup finds weft branches with no corresponding warp worktree sibling and reports or deletes
// them per the flag matrix: apply gates whether any deletion happens, checked-out branches are
// always protected. The repo's primary weft branch is protected unconditionally, in every mode —
// see primaryWeftBranch for why branch-space liveness alone cannot protect it.
// force is reserved and currently consulted by no gate in this verb: deleteWeftBranch already
// hardcodes force: false for its own request, and that stays true.
// remote gates whether an orphan weft branch actually deleted locally is also deleted on the weft
// repo's origin remote; see this file's header for the flag matrix. A remote deletion failure never
// makes Cleanup return a non-nil error — it is recorded on the entry and the sweep continues.
func (t *Topology) Cleanup(l *lyxcwd.Location, apply, force, remote bool) (res CleanupResult, err error) {
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

	// The no-origin pre-check runs once per call, ahead of the per-branch loop, and only when remote
	// is true. It runs under a dry run too, since RemoteURL is a go-git local config read that spawns
	// no process and contacts no network, so telling the operator up front that --apply --remote
	// would do nothing remotely is the whole value of reporting it. With remote false the pre-check
	// does not run at all, so a plain cleanup --apply against a remoteless repo reports no reason.
	var result CleanupResult
	remoteOK := false
	var weftRepoRootForRemote string
	if remote {
		weftRepoRoot, weftRootErr := WeftRepoRoot(l)
		if weftRootErr != nil {
			result.RemoteSkippedReason = fmt.Sprintf(
				"no remote deletion attempted: cannot resolve the weft repo root: %v", weftRootErr)
		} else if _, urlErr := gitrepo.New(weftRepoRoot).RemoteURL(originRemoteName); urlErr != nil {
			result.RemoteSkippedReason = fmt.Sprintf(
				"no remote deletion attempted: the weft repo has no %q remote configured: %v",
				originRemoteName, urlErr)
		} else {
			remoteOK = true
			weftRepoRootForRemote = weftRepoRoot
		}
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
		if entry.Deleted && remote && remoteOK {
			deleteWeftBranchOnRemote(rec, l, branch, t.cfg.BranchPrefix, weftRepoRootForRemote, &entry)
		}
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
// in entry. force is always false here because deleteWeftBranch's own request never had a
// force-answerable gate to begin with: Topology.Cleanup's force parameter is reserved and consulted
// by no gate in this verb, and may not answer the gate's own primary-weft carve-out or checked-out
// check either way.
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

// deleteWeftBranchOnRemote deletes branch on the weft repo's origin remote through the gate's
// deleteRemoteBranch executor, recording the outcome on entry. It runs only after deleteWeftBranch
// has already returned true for the same branch: the gate's ownership and dirtiness answers come
// from local state, so a branch it refuses to delete locally must never lose its remote copy either.
// A remote failure never aborts the sweep. It distinguishes a gate refusal from an operational
// failure so entry.RemoteError always names the layer that actually said no.
func deleteWeftBranchOnRemote(rec *Mutations, l *lyxcwd.Location, branch, branchPrefix, weftRepoRoot string, entry *CleanupBranchEntry) {
	req := remoteBranchRequest{
		what:      "delete weft branch on remote",
		repoDir:   weftRepoRoot,
		remote:    originRemoteName,
		branch:    branch,
		ownership: ownedManagedBranch(l, branchPrefix),
		dirtiness: dirtyCheckedOutBranch(),
		force:     false,
	}
	deleted, err := deleteRemoteBranch(rec, req)
	if err != nil {
		if refusal, ok := RefusalOf(err); ok {
			entry.RemoteError = fmt.Sprintf("gate refused remote deletion of %q: %s", branch, refusal.Reason)
		} else {
			entry.RemoteError = fmt.Sprintf("delete remote branch %q on %q failed: %v", branch, originRemoteName, err)
		}
		return
	}
	entry.RemoteDeleted = deleted
}

// PairBranchResult is RemovePairBranch's outcome.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type PairBranchResult struct {
	MutationRecord
	// Branch is the pair's other-side branch the call addressed.
	Branch string `json:"branch"`
	// LocalDeleted reports whether the branch was present locally and deleted.
	LocalDeleted bool `json:"local_deleted"`
	// RemoteDeleted reports whether a copy on the origin remote was observably removed.
	RemoteDeleted bool `json:"remote_deleted,omitempty"`
	// RemoteSkippedReason is non-empty when no remote deletion was attempted, today only because the
	// repository has no origin remote.
	RemoteSkippedReason string `json:"remote_skipped_reason,omitempty"`
}

// RemovePairBranch deletes slug's pair's other-side branch, locally when present and on the origin
// remote when present there, for a caller whose pair is already removed from disk.
// It exists for the same vocabulary reason PairSiblingRemnant does: a teardown re-entered after
// Remove was interrupted past its worktree removals, or whose remote deletion failed, must finish
// the branch deletion Remove would have done, without naming the branch's side of the pair.
// It refuses while the pair's other-side worktree is still on disk, since that is Remove's and
// Prune's to take down. Unlike Remove, a failed remote deletion is a returned error: this call's
// whole purpose is the deletion, so a caller must be able to retry it.
func (t *Topology) RemovePairBranch(l *lyxcwd.Location, slug string) (res PairBranchResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	if err := validateWorktreeSlug(slug, t.cfg.Dirs()); err != nil {
		return PairBranchResult{}, err
	}
	if _, present, err := PairSiblingRemnant(l, slug); err != nil {
		return PairBranchResult{}, err
	} else if present {
		return PairBranchResult{}, fmt.Errorf("the pair for %q still has its other-side worktree on disk; remove the pair first", slug)
	}

	branch := WeftBranchName(t.cfg.BranchPrefix + slug)
	result := PairBranchResult{Branch: branch}
	entry := CleanupBranchEntry{Branch: branch}

	if weftBranchExists(l, branch) {
		if !deleteWeftBranch(rec, l, branch, t.cfg.BranchPrefix, &entry) {
			return PairBranchResult{}, errors.New(entry.Error)
		}
		result.LocalDeleted = true
	}

	weftRepoRoot, err := WeftRepoRoot(l)
	if err != nil {
		return PairBranchResult{}, fmt.Errorf("resolve weft repo root: %w", err)
	}
	if _, urlErr := gitrepo.New(weftRepoRoot).RemoteURL(originRemoteName); urlErr != nil {
		result.RemoteSkippedReason = fmt.Sprintf("no remote deletion attempted: the weft repo has no %q remote configured: %v", originRemoteName, urlErr)
		return result, nil
	}
	deleteWeftBranchOnRemote(rec, l, branch, t.cfg.BranchPrefix, weftRepoRoot, &entry)
	if entry.RemoteError != "" {
		return PairBranchResult{}, errors.New(entry.RemoteError)
	}
	result.RemoteDeleted = entry.RemoteDeleted
	return result, nil
}

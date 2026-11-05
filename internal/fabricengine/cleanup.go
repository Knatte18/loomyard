// cleanup.go implements the Cleanup verb: it finds weft branches that have no
// corresponding warp worktree sibling and deletes them according to a flag matrix.
//
// Flag matrix:
//   - apply == false → dry-run/report only; nothing is deleted.
//   - apply == true  → deletes every orphan weft branch that is not the primary weft branch, not
//     checked out at a worktree, and not unmanaged (no "-weft" suffix, e.g. inherited from history
//     predating fabric's uniform naming scheme), whose slug's teardown lock the sweep takes by one
//     try and whose pair is not found live under that lock.
//   - remote is independent of --force and requires apply to delete anything (remote alone with
//     apply false is still a dry run): when true, every orphan weft branch actually deleted locally
//     is also deleted on the weft repo's origin remote, leased on the origin tip its archive read, so a
//     branch origin moved since is kept and reported on the entry's RemoteKeptReason. A weft repo with
//     no origin remote configured reports that once per verb, on the result's RemoteSkippedReason,
//     rather than once per branch.
//
// force is reserved and currently consulted by no gate in this verb; see Topology.Cleanup's own
// doc comment.
//
// A weft branch's warp sibling is recovered via WeftWarpSlug(branch) —
// inverting RecordsBranchName's suffix. The weft repo may also hold non-suffixed weft
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
// worktree directory is the repo name (e.g. "lym-fabric-test") while its branch is
// "main". Under warp the mistake is harmless because warp's primary weft branch is the
// unsuffixed "main" (WeftWarpSlug rejects it → protected as unmanaged). Under fabric's
// uniform suffix scheme the primary weft branch is "main-weft", which WeftWarpSlug
// accepts — so a directory-name comparison would misclassify it as a deletable orphan
// and delete the very branch board's reserved weft:main branch requires to stay
// permanent, distinct from it. Comparing
// against live warp *branches* protects every task pair whatever its branch prefix — but NOT the
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
// sit on the repo's default branch, which `lym fabric checkout` exists to change; one checkout onto
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
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// removeBeforeWeftDeleteHook, nil in production, runs right before each leased delete of a weft branch on origin, so a test can move origin's branch between the archive's read and the delete.
var removeBeforeWeftDeleteHook func()

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
	// A non-empty Error makes `lym fabric cleanup --apply` exit non-zero, because it names a genuine failure of the local `git branch -D` rather than a designed refusal — Protected and unmanaged entries set no Error at all.
	Error string `json:"error,omitempty"`
	// RemoteDeleted reports whether this branch's copy on the remote was observably removed. It is
	// true only when the remote deletion was attempted and actually removed a ref.
	RemoteDeleted bool `json:"remote_deleted,omitempty"`
	// RemoteError is non-empty when the remote deletion was attempted and did not succeed. Its text
	// always names the layer that said no — the gate's own refusal, or the remote deletion itself.
	RemoteError string `json:"remote_error,omitempty"`
	// RemoteKeptReason is non-empty when this branch's copy on the remote was left in place because it moved after the archive read its tip.
	// A kept branch is another process's live branch, not a failure, so it sets no RemoteError.
	RemoteKeptReason string `json:"remote_kept_reason,omitempty"`
	// ArchiveTag names the archive/<slug>/<tip> tag made on the local tip before this branch was deleted, whose push to the weft origin is the teardown child's; empty on a dry run, when no origin is configured, and when the archive failed.
	// An archive failure sets Error and leaves the branch, local and remote, in place.
	ArchiveTag string `json:"archive_tag,omitempty"`
	// OriginArchiveTag names the second archive tag, at origin's tip of this branch, pushed when that tip is neither the local tip nor an ancestor of it; empty when none was needed.
	OriginArchiveTag string `json:"origin_archive_tag,omitempty"`
	// Skipped is true when the sweep left the branch whole because the teardown lock of its slug was held; SkippedReason names the lock and the re-run.
	// A skipped entry sets no Error, and a later sweep takes the slug.
	Skipped bool `json:"skipped,omitempty"`
	// SkippedReason is the reason a Skipped entry was skipped.
	SkippedReason string `json:"skipped_reason,omitempty"`
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
	// ArchiveSkippedReason carries a once-per-verb reason no archive tag was pushed — today only a weft repo with no origin remote, which makes every branch's archive skip; the branches are then deleted as usual.
	// It is set from the first skip and never makes the verb exit non-zero.
	ArchiveSkippedReason string `json:"archive_skipped_reason,omitempty"`
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
// The remote deletion is leased on the origin tip the archive read, so a branch origin moved since stays on origin and is reported in the entry's RemoteKeptReason; an orphan origin holds no copy of gets no remote deletion.
// Under apply, each orphan whose slug's teardown lock one try takes, and whose pair is not found live under it, has its branch tip archived (archiveWeftTip) before the branch is deleted, whatever remote says; an archive failure fills the entry's Error and keeps the branch, local and remote, without aborting the sweep.
// An orphan whose slug's teardown lock is held is skipped whole, with Skipped and SkippedReason set, and a lock error other than a held lock fills Error and keeps the branch; an orphan a worktree turned live since the enumeration gets no entry.
// The sweep pushes no weft branch, and an orphan whose origin tip the local tip does not contain gets a second archive tag at that tip, named in the entry's OriginArchiveTag.
func (t *Topology) Cleanup(l *lymcwd.Location, apply, force, remote bool) (res CleanupResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

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
		weftRepoRoot, weftRootErr := RecordsRepoRoot(l)
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

	liveWarpBranches, err := liveWarpBranchSet(l)
	if err != nil {
		return CleanupResult{}, err
	}

	// Enumerate all branches in the weft repo to find orphans.
	weftBranches, err := listWeftBranches(l)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("list weft branches: %w", err)
	}

	for _, weftBranch := range weftBranches {
		branch := weftBranch.Branch

		// Recover the warp branch by inverting RecordsBranchName's suffix.
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

		remoteWeftRoot := ""
		if remote && remoteOK {
			remoteWeftRoot = weftRepoRootForRemote
		}
		if t.cleanupOrphan(rec, l, warpBranch, &entry, remoteWeftRoot, &result) {
			result.Entries = append(result.Entries, entry)
		}
	}

	return result, nil
}

// cleanupSweepRerun is the re-run the sweeps name when a slug's teardown lock is held: no verb is named, since more than one command reaches the sweeps.
const cleanupSweepRerun = "re-run the cleanup once it finishes"

// teardownLockHeldReason is why a sweep keeps a slug whose teardown lock a push or a removal holds.
func teardownLockHeldReason(slug string) string {
	return fmt.Sprintf("kept: the teardown lock of the pair %q is held by a push or a removal; %s", slug, cleanupSweepRerun)
}

// liveWarpBranchSet lists the branches some warp worktree has checked out, from the git-registered worktrees only.
// A worktree whose branch cannot be read, a stale registration, is skipped.
func liveWarpBranchSet(l *lymcwd.Location) (map[string]bool, error) {
	entries, err := List(l.WorktreePath())
	if err != nil {
		return nil, fmt.Errorf("list warp worktrees: %w", err)
	}
	live := make(map[string]bool, len(entries))
	for _, entry := range entries {
		branch, branchErr := readBranch(filepath.Clean(filepath.FromSlash(entry.Path)))
		if branchErr != nil {
			continue
		}
		live[branch] = true
	}
	return live, nil
}

// cleanupOrphan tags and deletes one orphan weft branch locally under the teardown lock of its slug, taken by one try, and hands the remote half to the lock's holder:
// pushing the archive tags and, under remoteWeftRoot when it is non-empty, deleting the branch on origin leased on the tip the archive read.
// A held lock marks entry skipped and any other lock error fills entry.Error, each keeping the branch whole.
// It re-reads the live warp branches under the lock and reports false, so the caller records no entry, when a worktree now sits on the orphan's paired warp branch, since the sweep listed them before an Add of the slug may have finished.
// A failed local tag fills entry.Error and keeps the branch; a remote half that ran in process fills the entry's tags and remote outcome, and a failed one fills entry.Error.
func (t *Topology) cleanupOrphan(rec *Mutations, l *lymcwd.Location, warpBranch string, entry *CleanupBranchEntry, remoteWeftRoot string, result *CleanupResult) (record bool) {
	slug := slugOfBranch(t.cfg, warpBranch)
	if t.cleanupAfterEnumerate != nil {
		t.cleanupAfterEnumerate()
	}
	// The lock file is the sweep's scratch state, not a mutation of the pair, so it joins no record: a sweep whose archive fails before any deletion stays unmutated.
	hold, lockErr := t.holdTeardown(NewMutations(l.HubPath), rec, l, slug, true, cleanupSweepRerun, true)
	switch {
	case errors.Is(lockErr, ErrPairBusy):
		entry.Skipped = true
		entry.SkippedReason = teardownLockHeldReason(slug)
		return true
	case lockErr != nil:
		entry.Error = fmt.Sprintf("take the teardown lock of weft branch %q failed: %v", entry.Branch, lockErr)
		return true
	}
	var job *remoteTeardownJob
	defer func() {
		if job == nil {
			hold.finish(nil)
		}
	}()

	live, err := liveWarpBranchSet(l)
	if err != nil {
		entry.Error = fmt.Sprintf("re-read the warp worktrees of weft branch %q failed: %v", entry.Branch, err)
		return true
	}
	if live[warpBranch] {
		return false
	}

	tip, archiveTag, archiveSkipped, err := tagWeftTipForArchive(l, slug, entry.Branch)
	if err != nil {
		entry.Error = fmt.Sprintf("archive weft branch %q failed: %v", entry.Branch, err)
		return true
	}
	entry.ArchiveTag = archiveTag
	if archiveSkipped != "" && result.ArchiveSkippedReason == "" {
		result.ArchiveSkippedReason = archiveSkipped
	}

	entry.Deleted = deleteWeftBranch(rec, l, entry.Branch, entry)
	if archiveSkipped != "" {
		return true
	}
	job = &remoteTeardownJob{Kind: remoteTeardownCleanup, Slug: slug, Worktree: l.WorktreePath(), WeftBranch: entry.Branch, WeftTip: tip, DeleteWeftOnOrigin: entry.Deleted && remoteWeftRoot != ""}
	out := hold.finish(job)
	if out.ran {
		if out.err != nil {
			entry.Error = fmt.Sprintf("archive weft branch %q failed: %v", entry.Branch, out.err)
			return true
		}
		entry.ArchiveTag, entry.OriginArchiveTag = out.archive.localTag, out.archive.originTag
		entry.RemoteDeleted, entry.RemoteError, entry.RemoteKeptReason = out.weft.RemoteDeleted, out.weft.RemoteError, out.weft.RemoteKeptReason
	}
	return true
}

// primaryWeftBranch returns the weft branch paired with the repo's primary warp branch — the one
// weft line Cleanup must never enumerate as an orphan.
//
// The source is the branch the Bolt checkout is checked out on: ensureBoardWorktree materialises that
// worktree on the warp's own unsuffixed default branch and nothing moves it afterwards, so it is the
// hub's durable record of which warp branch is primary.
// Branch-space liveness cannot answer this on its own, and the difference is a data-loss bug rather
// than a nicety: liveness asks whether some warp worktree is CURRENTLY checked out on the paired
// warp branch, so a single `lym fabric checkout <other>` on the prime pair makes `main` not-live and
// promotes `main-weft` — the durable `_lym` line, unpushed commits included — to a deletable orphan.
//
// A hub whose board branch cannot be read is refused rather than swept: Cleanup's deletions are
// irreversible, so an unreadable primary is the one direction that must fail closed.
func primaryWeftBranch(l *lymcwd.Location) (string, error) {
	boardDir := BoltDir(l.HubPath)
	boardBranch, err := readBranch(boardDir)
	if err != nil {
		return "", fmt.Errorf(
			"cannot determine the repo's primary weft branch from the %s worktree at %s (%w); refusing to enumerate orphan weft branches",
			BoltDirName, boardDir, err)
	}
	boardBranch = strings.TrimSpace(boardBranch)
	if boardBranch == "" || boardBranch == "HEAD" {
		return "", fmt.Errorf(
			"the %s worktree at %s is not on a named branch; refusing to enumerate orphan weft branches",
			BoltDirName, boardDir)
	}
	return RecordsBranchName(boardBranch), nil
}

// weftBranchCheckout pairs a weft branch name with its checked-out worktree path if any.
type weftBranchCheckout struct {
	Branch       string
	WorktreePath string
}

// listWeftBranches returns every branch in the weft repo with its checked-out worktree path if any.
func listWeftBranches(l *lymcwd.Location) ([]weftBranchCheckout, error) {
	weftRepoRoot, err := RecordsRepoRoot(l)
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
func deleteWeftBranch(rec *Mutations, l *lymcwd.Location, branch string, entry *CleanupBranchEntry) bool {
	weftRepoRoot, err := RecordsRepoRoot(l)
	if err != nil {
		entry.Error = fmt.Sprintf("resolve weft repo root: %v", err)
		return false
	}
	req := branchRequest{
		what:      "delete weft branch",
		repoDir:   weftRepoRoot,
		branch:    branch,
		ownership: ownedManagedBranch(l),
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
// The deletion is leased on leaseSHA, the origin tip the caller's archive read: an empty leaseSHA means origin held no copy, so nothing is deleted and nothing is reported,
// and a branch origin moved since is kept and reported in entry.RemoteKeptReason, never in entry.RemoteError.
// A remote failure never aborts the sweep. It distinguishes a gate refusal from an operational
// failure so entry.RemoteError always names the layer that actually said no.
func deleteWeftBranchOnRemote(rec *Mutations, l *lymcwd.Location, branch, weftRepoRoot, leaseSHA string, entry *CleanupBranchEntry) {
	if leaseSHA == "" {
		return
	}
	req := remoteBranchRequest{
		what:      "delete weft branch on remote",
		repoDir:   weftRepoRoot,
		remote:    originRemoteName,
		branch:    branch,
		ownership: ownedManagedBranch(l),
		dirtiness: dirtyCheckedOutBranch(),
		leaseSHA:  leaseSHA,
		force:     false,
	}
	if removeBeforeWeftDeleteHook != nil {
		removeBeforeWeftDeleteHook()
	}
	deleted, err := deleteRemoteBranch(rec, req)
	if err != nil {
		if errors.Is(err, gitrepo.ErrLeaseRejected) {
			entry.RemoteKeptReason = fmt.Sprintf("kept: origin's weft branch %q moved during the removal, so it is another process's live branch", branch)
			return
		}
		if refusal, ok := RefusalOf(err); ok {
			entry.RemoteError = fmt.Sprintf("gate refused remote deletion of %q: %s", branch, refusal.Reason)
		} else {
			entry.RemoteError = fmt.Sprintf("delete remote branch %q on %q failed: %v", branch, originRemoteName, err)
		}
		return
	}
	entry.RemoteDeleted = deleted
}

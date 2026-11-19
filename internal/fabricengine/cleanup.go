// cleanup.go implements the Cleanup verb.
// It finds weft branches with no warp worktree sibling and deletes them according to a flag matrix.
//
// Flag matrix:
//   - apply == false: dry run; nothing is deleted.
//   - apply == true: deletes every orphan weft branch that is fabric-managed, is not the primary weft branch,
//     and is not checked out at a worktree.
//     Its slug's teardown lock is taken by one try, and the branch is deleted only when its pair is not found
//     live under that lock.
//   - remote is independent of force and requires apply to delete anything; with apply false it is a dry run.
//     When true, every orphan weft branch deleted locally is also deleted on the weft repo's origin remote,
//     leased on the origin tip its archive read.
//     A branch that origin moved since is kept and reported on the entry's RemoteKeptReason.
//     A weft repo with no origin remote reports that once per verb, on the result's RemoteSkippedReason.
//
// force is reserved; no gate in this verb consults it.
//
// A weft branch's warp sibling is recovered by WeftWarpSlug, which inverts RecordsBranchName's suffix.
// WeftWarpSlug rejects a branch without the suffix (ok is false).
// Such a branch is not fabric-managed: it is reported but never deleted, as Reconcile treats unmanaged branches.
//
// Liveness is judged in branch space, not against worktree directory names.
// A weft branch <warpBranch>-weft is a live pair when some warp worktree is checked out on <warpBranch>.
// A weft branch checked out at a worktree is always protected, whatever the liveness verdict:
// git branch -D cannot delete a checked-out branch.
// Such a branch means the pair is still on disk, for example when its warp worktree sits on a detached HEAD.
// Branch-space liveness cannot see that case.
// Directory names are not compared, because the primary pair's worktree directory is the repo name:
// its branch is the default branch.
//
// primaryWeftBranch names the primary weft branch, the default branch plus "-weft", and protects it explicitly.
// Branch-space liveness protects it only while the prime warp worktree sits on the default branch,
// and `lym fabric checkout` can move that worktree, which would let the sweep delete the durable weft line.
//
// The board's weft:main branch is the unsuffixed default branch, so it is never a candidate:
// Cleanup walks only suffixed weft branches.

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

// removeBeforeWeftDeleteHook is nil in production.
// It runs before each leased delete of a weft branch on origin;
// a test can use it to move origin's branch between the archive's read and that delete.
var removeBeforeWeftDeleteHook func()

// CleanupBranchEntry records what a cleanup sweep did with one orphaned weft branch.
type CleanupBranchEntry struct {
	// Branch is the weft branch name.
	Branch string `json:"branch"`
	// Deleted reports whether the branch was actually deleted from the weft repo.
	// It is false on a dry run.
	// It is also false for a protected or skipped entry, and when deletion itself failed.
	Deleted bool `json:"deleted"`
	// Protected reports whether the branch was kept rather than deleted.
	// A branch without the "-weft" suffix is protected, because it is not fabric-managed.
	// A branch checked out at a worktree is protected too, since git branch -D cannot delete it.
	// Such an entry sets no Error.
	Protected bool `json:"protected,omitempty"`
	// Error is non-empty when apply is true and branch deletion failed.
	// It makes `lym fabric cleanup --apply` exit non-zero; a designed refusal sets no Error.
	Error string `json:"error,omitempty"`
	// RemoteDeleted reports whether this branch's copy on the remote was observably removed.
	// It is true only when the remote deletion was attempted and actually removed a ref.
	RemoteDeleted bool `json:"remote_deleted,omitempty"`
	// RemoteError is non-empty when the remote deletion was attempted and did not succeed.
	// Its text names the layer that refused: the gate's own refusal, or the remote deletion itself.
	RemoteError string `json:"remote_error,omitempty"`
	// RemoteKeptReason is non-empty when the remote copy was left because it moved after the archive read its tip.
	// Such a branch is another process's live branch, not a failure, so it sets no RemoteError.
	RemoteKeptReason string `json:"remote_kept_reason,omitempty"`
	// ArchiveTag names the archive/<slug>/<tip> tag made on the local tip before the branch was deleted.
	// It is empty on a dry run or with no origin; a failed archive sets Error and keeps the branch, local and remote.
	ArchiveTag string `json:"archive_tag,omitempty"`
	// OriginArchiveTag names the archive tag at origin's tip, made when that tip is neither the local tip nor its ancestor.
	OriginArchiveTag string `json:"origin_archive_tag,omitempty"`
	// Skipped is true when the sweep left the branch whole because its slug's teardown lock was held.
	// A skipped entry sets no Error, and a later sweep takes the slug.
	Skipped bool `json:"skipped,omitempty"`
	// SkippedReason names the held teardown lock and the re-run that takes the slug.
	SkippedReason string `json:"skipped_reason,omitempty"`
}

// CleanupResult reports the outcome of one cleanup sweep.
// It lists every orphaned weft branch: deleted, protected, or only reported.
// It embeds MutationRecord, which carries the mutations accumulated over the call.
type CleanupResult struct {
	MutationRecord
	// Entries lists the orphaned weft branches and their dispositions.
	Entries []CleanupBranchEntry `json:"entries"`
	// RemoteSkippedReason gives the reason no remote deletion was attempted at all.
	// It is set only when the weft repo has no origin remote configured.
	// It is kept out of RemoteError, which decides the exit code, so a missing origin exits 0.
	// The skip is reported once for the verb, not once per branch.
	RemoteSkippedReason string `json:"remote_skipped_reason,omitempty"`
	// ArchiveSkippedReason gives the once-per-verb reason no archive tag was pushed.
	// It is set when no origin is configured; the branches are still deleted, and it never fails the verb.
	ArchiveSkippedReason string `json:"archive_skipped_reason,omitempty"`
}

// Cleanup finds weft branches with no warp worktree sibling and reports or deletes them.
// Apply gates whether any deletion happens, and a branch checked out at a worktree is always protected.
// The repo's primary weft branch is protected in every mode, because branch-space liveness alone cannot protect it.
// force is reserved; no gate in this verb consults it.
// remote gates whether an orphan weft branch deleted locally is also deleted on the weft repo's origin.
// A remote deletion failure never makes Cleanup return a non-nil error;
// it is recorded on the entry and the sweep continues.
// The remote deletion is leased on the origin tip the archive read,
// so a branch that origin moved since stays on origin and is reported in the entry's RemoteKeptReason.
// An orphan that origin does not hold gets no remote deletion.
// Under apply, an orphan is archived and deleted only when its slug's teardown lock is taken by one try,
// and its pair is not found live under that lock.
// The archive runs whatever remote says;
// an archive failure fills the entry's Error and keeps the branch, local and remote, without aborting the sweep.
// An orphan whose slug's teardown lock is held is skipped whole, with Skipped and SkippedReason set;
// any other lock error fills Error and keeps the branch.
// An orphan that a worktree turned live since the enumeration gets no entry.
// The sweep pushes no weft branch,
// and an orphan whose origin tip the local tip does not contain gets a second archive tag at that tip;
// that tag is recorded in the entry's OriginArchiveTag.
func (t *Topology) Cleanup(l *lymcwd.Location, apply, force, remote bool) (res CleanupResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	primaryWeft, err := primaryWeftBranch(l)
	if err != nil {
		return CleanupResult{}, err
	}

	// The no-origin pre-check runs once per call, ahead of the per-branch loop, and only when remote is true.
	// It runs under a dry run too.
	// RemoteURL is a go-git local config read that spawns no process and contacts no network;
	// so telling the operator up front that --apply --remote would do nothing remotely is the point of the check.
	// With remote false the pre-check does not run, so a plain cleanup --apply against a remoteless repo reports no reason.
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

// cleanupSweepRerun is the re-run the sweeps name when a slug's teardown lock is held:
// no verb is named, since more than one command reaches the sweeps.
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

// cleanupOrphan tags and deletes one orphan weft branch locally under the teardown lock of its slug.
// It takes that lock with one try, and hands the remote half to the lock's holder.
// The holder pushes the archive tags, and when remoteWeftRoot is non-empty, it deletes the branch on origin.
// That deletion is leased on the tip the archive read.
// A held lock marks entry skipped, and any other lock error fills entry.Error, keeping the branch whole.
// It re-reads the live warp branches under the lock.
// When a worktree now sits on the orphan's paired warp branch, it reports false.
// The caller then records no entry, because the sweep listed the branches before an Add of the slug may have finished.
// A failed local tag fills entry.Error and keeps the branch;
// a remote half that ran in process fills the entry's tags and remote outcome, and a failed one fills entry.Error.
func (t *Topology) cleanupOrphan(rec *Mutations, l *lymcwd.Location, warpBranch string, entry *CleanupBranchEntry, remoteWeftRoot string, result *CleanupResult) (record bool) {
	slug := slugOfBranch(t.cfg, warpBranch)
	if t.cleanupAfterEnumerate != nil {
		t.cleanupAfterEnumerate()
	}
	// The lock file is the sweep's scratch state, not a mutation of the pair, so it joins no record:
	// a sweep whose archive fails before any deletion stays unmutated.
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

// primaryWeftBranch returns the weft branch paired with the repo's primary warp branch.
// It is the one weft line the orphan sweep must never enumerate.
//
// The source is the branch the Bolt checkout is on:
// ensureBoardWorktree materialises that worktree on the warp's own unsuffixed default branch;
// and nothing moves it afterwards, so it is the hub's durable record of which warp branch is primary.
// Branch-space liveness cannot answer this alone, and a wrong answer loses data.
// Liveness asks whether some warp worktree is currently checked out on the paired warp branch.
// A single `lym fabric checkout <other>` on the prime pair makes `main` not-live;
// and promotes `main-weft`, the durable `_lym` line with its unpushed commits, to a deletable orphan.
//
// A hub whose board branch cannot be read is refused rather than swept:
// deletions are irreversible, so an unreadable primary must fail closed.
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

// deleteWeftBranch deletes a weft branch through the gate's deleteBranch executor and records errors in entry.
// force is always false here: it cannot answer the gate's primary-weft carve-out or checked-out check either way.
// rec is the mutation recorder of the calling verb.
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

// deleteWeftBranchOnRemote deletes branch from the weft repo's origin remote through the gate.
// It records the outcome on entry.
// It must run only after the same branch was deleted locally.
// The gate's ownership and dirtiness answers come from local state,
// so a branch it refuses locally must keep its remote copy too.
// The deletion is leased on leaseSHA, the origin tip the caller read.
// An empty leaseSHA means origin held no copy, so nothing is deleted or reported.
// A branch that origin moved since leaseSHA is kept and reported in entry.RemoteKeptReason, never in entry.RemoteError.
// A remote failure never aborts the sweep.
// It distinguishes a gate refusal from an operational failure, so entry.RemoteError names the layer that said no.
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

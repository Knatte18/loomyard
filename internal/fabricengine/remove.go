// remove.go implements Remove: it tears down the portal and launchers after the slug and
// target-exists checks, so a refused slug never loses another pair's launchers, and the teardown
// still runs when the worktree dir itself is already gone.
// The weft branch it removes is WeftBranchName(warpBranch).
// Its link sweep is anchored and ownership-filtered — see the sweep's own comment for why reading
// the worktree root and trusting link-ness alone was wrong on both hub geometries.
// It never deletes a directory git declined to remove unless that directory is a registered LINKED
// worktree of this repo — see removeWarpWorktreeDir for the data-loss this rule exists to prevent.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// RemoveResult contains the result of successfully removing a worktree pair.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type RemoveResult struct {
	MutationRecord
	Slug         string `json:"slug"`
	Path         string `json:"path"`
	LinksRemoved int    `json:"links_removed"`
}

// Remove removes a paired warp and weft git worktree with all associated artifacts.
// If force is false, both worktrees must be clean;
// if force is true, uncommitted changes are forcefully removed.
// It validates slug through the same validator Add uses (slug.go), so hub geometry — `_board`,
// `_portals`, `_launchers`, `_lyx`, `.lyx`, and every weft sibling — can never be handed to a
// teardown verb as if it were a pair.
// It refuses the hub's prime worktree outright too: the prime is the warp repository itself, not a
// pair this verb can tear down, and git's own refusal to remove a main working tree is not a
// licence to delete the clone.
// Portal and launcher cleanup run after those checks but before the git removal, so they still run
// when the worktree directory is already gone.
func (t *Topology) Remove(l *lyxcwd.Location, slug string, force bool) (res RemoveResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	warpBranch := t.cfg.BranchPrefix + slug
	weftBranch := WeftBranchName(warpBranch)

	if err := validateWorktreeSlug(slug, t.cfg.Dirs()); err != nil {
		return RemoveResult{}, err
	}

	if err := refusePrimeSlug(l, slug); err != nil {
		return RemoveResult{}, err
	}

	target := WorktreePath(l, slug)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return RemoveResult{}, fmt.Errorf("worktree %q not found", target)
	}

	// Refuse before any teardown for the named pair: a mid-merge pair is not force's to override —
	// force answers dirtiness only, never a live merge record.
	blocked, err := mergeBlocksMutation(target, WeftWorktreePath(l, slug))
	if err != nil {
		return RemoveResult{}, err
	}
	if blocked {
		return RemoveResult{}, &ErrMergeInProgress{}
	}

	// Refuse for the other direction too: this pair may be idle itself while some OTHER pair in the
	// hub is mid-merge ON its branches. Removing it there deletes the weft branch that merge is
	// resolving against, so an abort would leave the source work reachable only from the remote.
	inFlight, err := mergeSourceInFlight(l, warpBranch)
	if err != nil {
		return RemoveResult{}, err
	}
	if inFlight {
		return RemoveResult{}, &ErrMergeInProgress{}
	}

	// removePortal and removeLaunchers are best-effort: an operational failure is discarded exactly as
	// before, but a gate refusal must surface rather than vanish at the verb the slice's worst defect
	// came from.
	if err := surfaceRefusal(removePortal(rec, l, slug)); err != nil {
		return RemoveResult{}, err
	}
	if err := surfaceRefusal(removeLaunchers(rec, l, slug)); err != nil {
		return RemoveResult{}, err
	}

	if !force {
		dirty, _, err := worktreeDirty(scopeAll, target)
		if err != nil {
			return RemoveResult{}, nameStrandedPortalTeardown(rec, fmt.Errorf("check warp worktree status: %w", err))
		}
		if dirty {
			return RemoveResult{}, nameStrandedPortalTeardown(rec, fmt.Errorf("worktree has uncommitted changes; use --force"))
		}
	}

	if !force {
		if err := refuseDirtyWeftWorktree(WeftWorktreePath(l, slug)); err != nil {
			return RemoveResult{}, nameStrandedPortalTeardown(rec, err)
		}
	}

	// Sweep the ANCHORED directory, and only the links fabric itself created there.
	// The previous sweep read the worktree ROOT and removed every symlink it found: on a
	// subpath-anchored hub that saw none of the pair's junctions (they live at
	// <worktree>/<anchorRel>) and reported LinksRemoved: 0, and at a root anchor it deleted the
	// user's own checked-in symlinks alongside fabric's.
	linksRemoved := 0
	if ownedNames, scanErr := scanOnDiskJunctionNames(l, slug); scanErr == nil {
		removeErr := removeWarpJunction(rec, l, slug, ownedNames)
		if err := surfaceRefusal(removeErr); err != nil {
			return RemoveResult{}, err
		}
		if removeErr == nil {
			linksRemoved = len(ownedNames)
		}
	}
	if err := removeWarpWorktreeDir(rec, l, target, force); err != nil {
		return RemoveResult{}, err
	}

	// A weft-teardown failure is tolerated only when the weft worktree is actually gone (already
	// absent, or removed with just a branch/prune step failing) — a weft worktree still on disk
	// after a "successful" Remove is a half-torn pair the operator was never told about.
	weftErr := removeWeftWorktree(rec, l, slug, weftBranch, force, true, t.cfg.BranchPrefix)
	if weftErr != nil {
		weftTarget := WeftWorktreePath(l, slug)
		if _, statErr := os.Stat(weftTarget); statErr == nil {
			return RemoveResult{}, fmt.Errorf(
				"warp worktree removed, but weft teardown failed and the weft worktree remains at %s: %w",
				weftTarget, weftErr)
		}
	}

	return RemoveResult{
		Slug:         slug,
		Path:         target,
		LinksRemoved: linksRemoved,
	}, nil
}

// nameStrandedPortalTeardown appends the reconcile remedy to refusal when Remove's portal and
// launcher teardown has already recorded a mutation, and returns refusal unchanged otherwise.
//
// Remove tears the portal and launchers down before the no-force dirtiness gates, deliberately: the
// teardown must still run when the worktree directory is already gone (see this file's header).
// The consequence is that an operator who is REFUSED — told to commit their work or pass --force —
// has nonetheless already lost that pair's portal junction and launcher scripts by the time they
// read the message.
// The loss is fully self-healing, since `lyx fabric reconcile` re-wires both and reports
// ReconcileActionPortalRestored for the pair, and the mutation record already carries the entries on
// the failure path with partial=true. What was missing is the last step: the operator has no reason
// to suspect their launchers just vanished, and no reason to reach for reconcile.
// Naming it in the refusal itself closes that gap without reordering the teardown, whose position
// this file's header justifies on its own grounds.
//
// The remedy is appended only when something was actually recorded, so a refusal that stranded
// nothing — the ordinary case once a first refused attempt has already torn the portal down — does
// not tell the operator to repair a hub that is intact.
func nameStrandedPortalTeardown(rec *Mutations, refusal error) error {
	// Len has a value receiver, so a nil recorder would panic on the auto-dereference rather than
	// answering zero. Remove always constructs one, but this helper must not depend on that.
	if rec == nil || rec.Len() == 0 {
		return refusal
	}
	return fmt.Errorf(
		"%w; this pair's portal junction and launcher scripts were already torn down before the refusal — run \"lyx fabric reconcile\" to restore them",
		refusal)
}

// refuseDirtyWeftWorktree returns an error when the weft worktree at weftTarget carries
// uncommitted changes, or when its status could not be read at all.
//
// An ABSENT weft worktree is not a refusal: there is no uncommitted work to lose, and tearing down
// a half-present pair is exactly what Remove is for.
// An unreadable one IS a refusal, and that is the whole point of this helper: the probe used to
// swallow its own spawn error in an empty if-branch, so a git that failed to run silently reported
// the weft side clean and the no-force gate simply disappeared.
func refuseDirtyWeftWorktree(weftTarget string) error {
	if _, statErr := os.Stat(weftTarget); os.IsNotExist(statErr) {
		return nil
	}

	dirty, _, err := worktreeDirty(scopeAll, weftTarget)
	if err != nil {
		return fmt.Errorf("check weft worktree status: %w", err)
	}
	if dirty {
		return fmt.Errorf("weft worktree has uncommitted changes; run \"lyx fabric sync\" or use --force")
	}
	return nil
}

// refusePrimeSlug returns an error when slug names the hub's prime (main) warp worktree.
// The prime is the warp repository itself rather than a pair Remove can tear down, and git refuses
// to remove a main working tree — so without this guard the removal reaches the directory-removal
// fallback and deletes the whole clone, gitdir included.
// A prime-name resolution failure is not fatal here: it means the hub geometry is already broken,
// and removeWarpWorktreeDir's own registered-worktree rule still refuses to delete anything git
// declined to remove.
func refusePrimeSlug(l *lyxcwd.Location, slug string) error {
	primeName, err := PrimeName(l)
	if err != nil {
		return nil
	}
	if slug != primeName {
		return nil
	}
	return fmt.Errorf(
		"refusing to remove %q: it is this hub's prime worktree — the warp repository itself, not a pair; remove the whole hub directory instead if that is what you meant",
		slug)
}

// removeWarpWorktreeDir removes the warp worktree at target via the gate's removeGitWorktree
// executor, falling back to a second gated call ONLY when target is a registered LINKED worktree of
// this repo.
//
// The narrow fallback is the whole point of this helper.
// `git worktree remove` refuses a main working tree, a path that is not a worktree of this repo at
// all, and a worktree carrying state it will not discard;
// treating every one of those refusals as licence to delete the directory turned an ordinary typo
// (`lyx fabric remove <prime>`, `lyx fabric remove _board`) into the loss of a whole git clone.
// A registered linked worktree is fabric's own pair member and nothing else, so deleting it after a
// git refusal is recoverable bookkeeping rather than data loss.
//
// The fallback is itself gated because it fires on ANY nonzero exit from `git worktree remove`, and
// `git worktree remove` without `--force` refuses on untracked files — an ungated fallback would
// therefore delete exactly the untracked files git had just declined to discard.
func removeWarpWorktreeDir(rec *Mutations, l *lyxcwd.Location, target string, force bool) error {
	req := pathRequest{
		what:      "remove warp worktree",
		container: l.HubPath,
		target:    target,
		slug:      nil,
		ownership: ownedRegisteredLinkedWorktree(l.WorktreePath()),
		dirtiness: dirtyScopeAll(),
		force:     force,
	}

	err := removeGitWorktree(rec, req, l.WorktreePath())
	if err == nil {
		return nil
	}

	var refusal *destructiveRefusal
	if errors.As(err, &refusal) && !isRegisteredLinkedWorktree(l, target) {
		// The gate refused before git ever ran: target fails the exact same
		// isRegisteredLinkedWorktree predicate the post-git-failure branch below would have
		// applied, just evaluated earlier. Report the identical, pre-existing message rather
		// than a gate-internal one — git's own exit code and stderr are unavailable here
		// because git was never invoked.
		return fmt.Errorf(
			"refusing to remove worktree %s: %s; it is not a linked worktree of this repo, so fabric will not delete the directory itself",
			target, refusal.Reason)
	}

	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) {
		// git never ran, or the gate refused before it could: destroy nothing.
		return fmt.Errorf("run git worktree remove for %s: %w", target, err)
	}

	if !isRegisteredLinkedWorktree(l, target) {
		return fmt.Errorf(
			"git refused to remove worktree %s (git exit %d): %s; it is not a linked worktree of this repo, so fabric will not delete the directory itself",
			target, gitErr.ExitCode, strings.TrimSpace(gitErr.Stderr))
	}

	// force must travel from the primary request into the fallback, and its absence here was a real
	// defect: an operator who passed --force against a worktree git declined for some OTHER reason
	// (a `git worktree lock`, say) got a refusal whose stated remedy was "use --force" — the one
	// thing they had already done — and a half-torn-down pair.
	// Propagating it preserves the protection the fallback exists for, rather than weakening it: in
	// the NO-force case git refused precisely because of untracked files, and the fallback must not
	// delete what git just declined to discard; in the force case git was already invoked WITH
	// --force, so untracked files cannot have been its reason.
	// pathRequest.force is a bool with no unset state, so it is the one field a call site can omit to
	// a silent zero value — the exact failure mode the type's own doc comment names for the others.
	fallbackReq := pathRequest{
		what:      "remove warp worktree",
		container: l.HubPath,
		target:    target,
		ownership: ownedRegisteredLinkedWorktree(l.WorktreePath()),
		dirtiness: dirtyScopeAll(),
		force:     force,
	}
	if removeErr := removePath(rec, fallbackReq); removeErr != nil {
		// A *destructiveRefusal propagates unwrapped so errors.As still works at the caller; only an
		// operational failure gets the "fallback removal failed" wrapper.
		var refusal *destructiveRefusal
		if errors.As(removeErr, &refusal) {
			return removeErr
		}
		return fmt.Errorf("fallback removal failed: %w", removeErr)
	}
	// Best-effort: a failed prune leaves a stale registration the next reconcile or prune
	// re-reports, and it must not turn a completed removal into an error.
	_, _ = gitexec.Run([]string{"worktree", "prune"}, l.WorktreePath())
	return nil
}

// isRegisteredLinkedWorktree reports whether target is registered in this repo's worktree list as a
// worktree OTHER than the main one.
// A failure to enumerate answers false, the conservative direction: an unenumerable repo is exactly
// where a blind directory removal is least defensible.
func isRegisteredLinkedWorktree(l *lyxcwd.Location, target string) bool {
	return isRegisteredLinkedWorktreeIn(l.WorktreePath(), target)
}

// isRegisteredLinkedWorktreeIn is the repo-agnostic form of isRegisteredLinkedWorktree: it asks the
// repo at repoDir whether target is one of its registered LINKED worktrees.
//
// It is shared rather than duplicated because both sides of the pair need the same rule.
// Remove asks it of the WARP repo before falling back to a directory removal;
// Prune asks it of the WEFT repo for the same reason, and for the same data loss — a hub directory
// whose name merely ends in the weft suffix is not fabric's to delete, however loudly git refuses to
// remove it as a worktree.
// A failure to enumerate answers false, the conservative direction.
func isRegisteredLinkedWorktreeIn(repoDir, target string) bool {
	entries, err := List(repoDir)
	if err != nil {
		return false
	}
	cleanTarget := filepath.Clean(target)
	for _, entry := range entries {
		if entry.Main {
			continue
		}
		if filepath.Clean(filepath.FromSlash(entry.Path)) == cleanTarget {
			return true
		}
	}
	return false
}

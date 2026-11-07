// prune.go implements the Prune verb, which finds and optionally removes stale or orphaned warp↔weft pairs.
// A pair is stale when its warp worktree directory is missing.
// A pair is orphaned when a weft worktree has no warp sibling.
// Prune works on directory names (`<slug>-weft`), so fabric's branch-naming scheme does not affect this file.
//
// Removal uses `git worktree remove --force`; uncommitted tracked changes protect a weft worktree unless force is set.
// This is the refuse-then-offer-force posture of Remove: uncommitted work is never discarded silently.
//
// Ownership is checked before either mode acts, and force does not bypass it.
// The orphan pass chooses hub children by directory name alone,
// so an ordinary `<hub>/notes-weft/` directory and an unrelated git clone at `<hub>/proj-weft/` both match.
// Fabric deletes a path only when the hub's weft repo registers it as a linked worktree.
// Everything else is reported Unowned and left alone, in both modes.
// The same rule governs removeWarpWorktreeDir on the warp side:
// a refusal by `git worktree remove` is not permission to delete the path.
//
// A third pass reports each hub directory named as a valid pair slug that no worktree registration covers.
// Such a directory may be a task worktree path that a late writer refilled after a removal.
// It blocks a new pair of its slug.
// Nothing shows fabric created it, so it is reported Unowned in both modes and never removed.
//
// Prune removes weft worktrees only and deletes no weft branch: orphaned branches are left to Cleanup.
// So it has no archive step; archiveWeftTip belongs to the verbs that delete a branch.
//
// The verdict is the same in both modes, so a dry run's Protected and Unowned flags match what --apply would do.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// PruneEntry describes one stale or orphaned pair that Prune has identified.
type PruneEntry struct {
	// CodeWorktree is the absolute path to the (missing or absent) warp worktree.
	CodeWorktree string `json:"code_worktree"`
	// RecordsWorktree is the absolute path to the weft worktree sibling.
	RecordsWorktree string `json:"records_worktree"`
	// Reason describes why this pair was flagged for pruning.
	Reason string `json:"reason"`
	// Removed reports whether the weft worktree was actually deleted.
	// It is false on a dry run and true only when apply is true and removal succeeded.
	Removed bool `json:"removed"`
	// Protected reports whether this entry's weft worktree has uncommitted tracked changes
	// that force was not given to discard.
	// It is computed in both modes, so a dry run reports what --apply would act on.
	Protected bool `json:"protected,omitempty"`
	// Unowned reports that the enumerated path is not a linked worktree of this hub's weft repo.
	// Fabric never removes it, force included.
	// The orphan pass matches on directory name alone.
	// So an ordinary user directory or an unrelated git clone with a weft-suffixed name is reported here, not deleted.
	// A directory at a pair's task worktree path that no registration covers is reported the same way.
	// Like Protected, it is computed in both modes.
	Unowned bool `json:"unowned,omitempty"`
	// Error is non-empty when an entry was not removed and the operator needs to know why.
	// It covers a failed removal, a protected entry, and an unowned path fabric refuses to touch.
	Error string `json:"error,omitempty"`
}

// PruneResult is the result of Prune.
// It lists every stale or orphaned pair, whether or not it was removed.
// The embedded MutationRecord holds the mutations recorded over the call.
type PruneResult struct {
	MutationRecord
	// Entries lists the pairs that were identified, with Removed set on those actually deleted.
	// It is never nil, so the CLI envelope renders `"entries":[]` rather than `"entries":null` when nothing was stale.
	Entries []PruneEntry `json:"entries"`
}

// Prune identifies stale or orphaned warp↔weft pairs.
// When apply is true, it removes their stale weft worktrees and associated launcher directories.
// A weft worktree with uncommitted tracked changes is protected unless force is true.
// The forced removal would otherwise discard those changes without a trace.
// Per-entry removal errors and protection reasons are recorded in PruneEntry.Error.
// A hub directory named as a pair slug that is not a registered worktree is reported Unowned and never removed.
// Its PruneEntry.Error says how to clear it.
func (t *Topology) Prune(l *lymcwd.Location, apply, force bool) (res PruneResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	entries, err := List(l.WorktreePath())
	if err != nil {
		return PruneResult{}, fmt.Errorf("list worktrees: %w", err)
	}

	// A dry run changes nothing, so only an applying Prune rewrites the hub workspace file.
	if apply {
		defer regenerateAfterVerb(rec, l, "prune")
	}

	liveWarpSlugs := make(map[string]bool)

	// The warp-missing loop records its slugs here so the orphan pass does not report the same weft twice.
	pass1Slugs := make(map[string]bool)

	// Seeded non-nil, because a nil Entries marshals to `"entries":null` rather than an empty array.
	result := PruneResult{Entries: []PruneEntry{}}
	for _, entry := range entries {
		warpPath := filepath.FromSlash(entry.Path)
		warpPath = filepath.Clean(warpPath)
		slug := filepath.Base(warpPath)

		weftPath := RecordsWorktreePath(l, slug)

		_, warpStatErr := os.Stat(warpPath)
		warpMissing := warpStatErr != nil

		if warpMissing {
			pe := PruneEntry{
				CodeWorktree:    filepath.ToSlash(warpPath),
				RecordsWorktree: filepath.ToSlash(weftPath),
				Reason:          "warp worktree directory missing",
			}

			applyStalePairOwnership(l, weftPath, &pe)
			applyStalePairProtection(weftPath, force, &pe)
			if apply && !pe.Protected && !pe.Unowned {
				pe.Removed = removeStalePair(rec, l, slug, weftPath, &pe)
			}

			pass1Slugs[slug] = true
			result.Entries = append(result.Entries, pe)
		} else {
			liveWarpSlugs[slug] = true
		}
	}
	hubEntries, err := os.ReadDir(l.HubPath)
	if err != nil {
		// A missing or unreadable hub is a fatal error; we cannot scan for orphans.
		return PruneResult{}, fmt.Errorf("read hub directory: %w", err)
	}

	for _, dirEntry := range hubEntries {
		if !dirEntry.IsDir() {
			continue
		}

		name := dirEntry.Name()

		warpSlug, ok := WeftWarpSlug(name)
		if !ok {
			continue
		}

		if liveWarpSlugs[warpSlug] || pass1Slugs[warpSlug] {
			continue
		}

		weftPath := filepath.Join(l.HubPath, name)
		warpPath := filepath.Join(l.HubPath, warpSlug)

		pe := PruneEntry{
			CodeWorktree:    filepath.ToSlash(warpPath),
			RecordsWorktree: filepath.ToSlash(weftPath),
			Reason:          "weft worktree has no warp sibling",
		}

		applyStalePairOwnership(l, weftPath, &pe)
		applyStalePairProtection(weftPath, force, &pe)
		if apply && !pe.Protected && !pe.Unowned {
			pe.Removed = removeStalePair(rec, l, warpSlug, weftPath, &pe)
		}

		result.Entries = append(result.Entries, pe)
	}

	// No pair slug starts with a dot, and a hidden hub directory is tooling's, not a pair's.
	for _, dirEntry := range hubEntries {
		name := dirEntry.Name()
		if !dirEntry.IsDir() || liveWarpSlugs[name] || strings.HasPrefix(name, ".") {
			continue
		}
		if validateWorktreeSlug(name, t.cfg.Dirs()) != nil {
			continue
		}
		strayPath := filepath.Join(l.HubPath, name)
		result.Entries = append(result.Entries, PruneEntry{
			CodeWorktree:    filepath.ToSlash(strayPath),
			RecordsWorktree: filepath.ToSlash(RecordsWorktreePath(l, name)),
			Reason:          "directory at a pair's task worktree path is not a worktree",
			Unowned:         true,
			Error: fmt.Sprintf(
				"%q is not a worktree of this hub's repository, so fabric will not remove it, and it blocks a new pair of %q: save anything worth keeping from it, then delete it",
				strayPath, name),
		})
	}

	return result, nil
}

// applyStalePairOwnership marks pe unowned unless weftPath is registered as a linked worktree of the hub's weft repo.
//
// It runs in both modes, and force does not bypass it.
// It does not answer whether there is work worth keeping; it answers whether fabric may delete the path at all.
// No flag can make a directory fabric never created fabric's.
// The orphan pass chooses paths by directory name alone (WeftWarpSlug over every hub child).
// A `<hub>/notes-weft/` holding an operator's notes looks like a stale pair until this check separates it.
// So does a separate git clone at `<hub>/proj-weft/`.
//
// An absent path is not unowned: there is nothing to delete.
// removeStalePair still clears launcher and worktree-registration debris for a pair whose weft worktree is gone.
func applyStalePairOwnership(l *lymcwd.Location, weftPath string, pe *PruneEntry) {
	if _, statErr := os.Stat(weftPath); os.IsNotExist(statErr) {
		return
	}

	weftRepoRoot, err := RecordsRepoRoot(l)
	if err != nil {
		pe.Unowned = true
		pe.Error = fmt.Sprintf("cannot resolve this hub's weft repo to confirm %q is one of its worktrees (%v); refusing to remove it", weftPath, err)
		return
	}

	if isRegisteredLinkedWorktreeIn(weftRepoRoot, weftPath) {
		return
	}

	pe.Unowned = true
	pe.Error = fmt.Sprintf(
		"%q is not a linked worktree of this hub's weft repo at %s; its name merely ends in the weft suffix, so fabric will not remove it — delete it yourself if it really is debris",
		weftPath, weftRepoRoot)
}

// applyStalePairProtection marks pe protected when force is unset and the weft worktree is dirty.
//
// The probe checks tracked files only.
// `git worktree remove --force` discards tracked modifications silently; this gate stops that loss.
// Untracked files are the ordinary residue of an abandoned pair.
// Refusing on them would protect nothing and make prune useless on the debris it exists to clear.
// A probe that cannot run leaves the entry unprotected.
// Git cannot read such a worktree, so it has no tracked work to lose.
func applyStalePairProtection(weftPath string, force bool, pe *PruneEntry) {
	if force {
		return
	}
	if _, statErr := os.Stat(weftPath); statErr != nil {
		return
	}

	dirty, _, err := worktreeDirty(scopeTracked, weftPath)
	if err != nil {
		return
	}
	if !dirty {
		return
	}

	pe.Protected = true
	pe.Error = "weft worktree has uncommitted changes; commit them or re-run with --force to discard them"
}

// removeStalePair removes the stale weft worktree at weftPath when it exists.
// It tears down the slug's launcher directory and prunes administrative state in both repos.
// Errors are recorded in pe.Error.
// It returns true only when a weft worktree existed and was removed without error.
//
// It is reached only for an entry that applyStalePairOwnership has already cleared.
// So weftPath is either absent or a registered linked worktree of the hub's weft repo.
// The directory-removal fallback below relies on that.
// It runs only after git declines to remove a path that git still registers as a worktree.
// That is recoverable bookkeeping, not data loss.
// The same rule and justification apply to removeWarpWorktreeDir's fallback.
//
// Launcher teardown runs after the ownership gate, since its slug comes from a directory name.
// Tearing it down first would let a stray `<hub>/my-task-weft/` directory remove a live pair's launcher directory.
// Nothing has yet shown the entry is fabric's at that point.
// rec is the calling verb's own recorder, threaded through every gate call this helper makes.
func removeStalePair(rec *Mutations, l *lymcwd.Location, slug, weftPath string, pe *PruneEntry) bool {
	weftRepoRoot, weftRepoRootErr := RecordsRepoRoot(l)
	if weftRepoRootErr != nil {
		pe.Error = fmt.Sprintf("resolve weft repo root: %v", weftRepoRootErr)
		return false
	}

	// The launcher teardown is keyed on a slug the orphan pass derived from a directory name.
	// A refusal here is most likely about that input, so it is recorded rather than swallowed as an ordinary failure.
	if err := surfaceRefusal(removeLaunchers(rec, l, slug)); err != nil {
		pe.Error = err.Error()
		return false
	}

	removed := false

	if _, statErr := os.Stat(weftPath); statErr == nil {
		req := pathRequest{
			what:      "remove weft worktree",
			container: l.HubPath,
			target:    weftPath,
			ownership: ownedRegisteredLinkedWorktree(weftRepoRoot),
			dirtiness: dirtyScopeTracked(),
			force:     true,
		}
		err := removeGitWorktree(rec, req, weftRepoRoot)
		if err != nil {
			var gitErr *gitexec.GitError
			if !errors.As(err, &gitErr) {
				// git never ran, or the gate refused before it could: destroy nothing.
				pe.Error = fmt.Sprintf("git worktree remove: %v", err)
				return false
			}

			if !isRegisteredLinkedWorktreeIn(weftRepoRoot, weftPath) {
				// The registration vanished between the ownership gate and here (a concurrent
				// prune, an external `git worktree prune`). Report git's own reason and delete
				// nothing: an unregistered path is never fabric's to remove.
				pe.Error = fmt.Sprintf(
					"git refused to remove weft worktree %q (git exit %d): %s; it is no longer a linked worktree of %s, so fabric will not delete the directory itself",
					weftPath, gitErr.ExitCode, strings.TrimSpace(gitErr.Stderr), weftRepoRoot)
				return false
			}
			// force: true mirrors the primary request above.
			// applyStalePairProtection has already run and respects force.
			// A tracked-dirty weft worktree that reaches here is one the operator asked to discard.
			// Refusing it would demand --force from an operator who already passed it, and leave the pair half torn down.
			fallbackReq := pathRequest{
				what:      "remove weft worktree",
				container: l.HubPath,
				target:    weftPath,
				ownership: ownedRegisteredLinkedWorktree(weftRepoRoot),
				dirtiness: dirtyScopeTracked(),
				force:     true,
			}
			if removeErr := removePath(rec, fallbackReq); removeErr != nil {
				// The %d is this worktree-remove call's exit code; the %v is the removePath fallback's own failure.
				pe.Error = fmt.Sprintf("remove weft worktree %q failed (git exit %d); fallback cleanup also failed: %v", weftPath, gitErr.ExitCode, removeErr)
				return false
			}
		}
		removed = true
	}

	// The warp repo's `.git/worktrees/<slug>` registration can outlive the warp worktree directory it describes.
	// That is the situation a stale pair starts from, and the `git worktree prune` below clears the registration.
	// Mutations are recorded only after their effect is observed.
	// So probe the admin path just before the best-effort prunes.
	// Append KindWorktreeRemoved at the pair's warp worktree path only if the probe found it and it is then gone.
	warpAdminPath := filepath.Join(l.WorktreePath(), ".git", "worktrees", slug)
	_, warpAdminStatErr := os.Lstat(warpAdminPath)
	warpAdminWasRegistered := warpAdminStatErr == nil

	// Best-effort: a failed prune leaves a stale registration the next reconcile or prune
	// re-reports, and it must not turn a completed removal into an error.
	_, _ = gitexec.Run([]string{"worktree", "prune"}, weftRepoRoot)
	// Best-effort, same reasoning as the weft-side prune above.
	_, _ = gitexec.Run([]string{"worktree", "prune"}, l.WorktreePath())

	if warpAdminWasRegistered {
		if _, statErr := os.Lstat(warpAdminPath); os.IsNotExist(statErr) {
			rec.Append(KindWorktreeRemoved, filepath.Join(l.HubPath, slug), "git worktree prune")
		}
	}

	return removed
}

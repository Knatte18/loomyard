// ancestors.go implements the empty-directory sweeper that prunes empty ancestors of removed
// worktrees and portals, and the containment assertion its destructive callers share.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// refuseUncontainedPath returns an error unless target RESOLVES strictly below container.
//
// It guards the teardown helpers that delete a path derived by joining a caller-supplied slug onto
// hub geometry.
// Slug validation is the primary defence and catches every input this repo can produce today, but a
// derived path that has escaped its container is being handed to os.RemoveAll, so the cost of
// asserting it is nothing against the cost of being wrong once.
//
// Both sides are put through resolveAncestorSymlinks/containmentPath before they are related, and
// that resolution is the whole point rather than a refinement: a purely lexical filepath.Rel
// comparison trusts the NOMINAL path, so a symlink planted at any intermediate segment of target
// let a gated removal act on a real path far outside container while the check happily passed —
// reproduced live against `removeLaunchers` by planting a link at <Hub>/_launchers/<slug>, which
// then deleted two files outside the hub and reported success.
// The final component of target is deliberately NOT resolved (see containmentPath): a junction
// removal's target IS a link, and resolving it would place the target inside the weft worktree it
// points at rather than the warp worktree it lives in, refusing every legitimate unwire.
//
// The refusal is a *destructiveRefusal, not a bare fmt.Errorf, and that is load-bearing rather than
// cosmetic. Both callers (removeLaunchers, removePortal) are reached from best-effort call sites in
// Remove and Prune that wrap them in surfaceRefusal, which by design discards anything that is NOT a
// *destructiveRefusal. While this guard returned a plain error it was therefore silently swallowed:
// a static symlink planted at the _launchers/_portals container made the whole teardown refuse —
// correctly, nothing escaped — while `lyx fabric remove` reported "ok":true, "partial":false and an
// exit 0, with the launcher scripts still on disk and no signal to the operator that a containment
// guard had fired at all (fabric's R8 crucible round, finding L2; R2's M2 dishonest-success shape on
// the teardown path). Typing it as the gate's own refusal is what makes CONSTRAINTS.md's "a gate
// refusal is never discarded on a best-effort path" true for this guard too, with no change at any
// call site, and makes RefusalOf answer for it like any other.
func refuseUncontainedPath(container, target, what string) error {
	if err := containmentFailure(container, target); err != nil {
		return &destructiveRefusal{
			Check:  CheckContainment,
			What:   "remove " + what,
			Target: target,
			Reason: err.Error(),
		}
	}
	return nil
}

// containmentFailure returns the bare reason target does not resolve strictly below container, or
// nil when it does.
//
// It exists so the destructive gate can attribute its own refusal without inheriting a second
// "refusing to remove …" prefix: checkPathRequest already renders one through *destructiveRefusal,
// and wrapping refuseUncontainedPath's whole message produced "refusing to remove remove launcher
// script" in a real refusal. The two callers that are NOT the gate (launchers.go, portals.go) keep
// the prefixed wrapper, where it reads correctly.
func containmentFailure(container, target string) error {
	resolvedContainer := resolveAncestorSymlinks(container)
	resolvedTarget := containmentPath(target)

	rel, err := filepath.Rel(resolvedContainer, resolvedTarget)
	if err != nil {
		return fmt.Errorf("cannot relate %s to %s: %w", target, container, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is not inside %s", target, container)
	}
	return nil
}

// containmentPath renders path for a containment comparison: every symlink in its PARENT ancestry
// is resolved, while the final component is carried through unresolved.
//
// Keeping the final component is what lets a link be a legitimate containment target. Every
// junction the gate removes is itself a link living inside the warp worktree and pointing into the
// weft one, so resolving the leaf would relocate the target into the weft worktree and make the
// warp-worktree container refuse it.
// Resolving everything ABOVE the leaf is what closes the bypass: a link at any intermediate segment
// moves the real inode elsewhere, and that relocation is exactly what the comparison must see.
func containmentPath(path string) string {
	cleaned := filepath.Clean(path)
	parent := filepath.Dir(cleaned)
	if parent == cleaned {
		return cleaned
	}
	return filepath.Join(resolveAncestorSymlinks(parent), filepath.Base(cleaned))
}

// resolveAncestorSymlinks returns dir with every symlink in it resolved, falling back component by
// component for the part of dir that does not exist on disk yet.
//
// filepath.EvalSymlinks fails outright on a path whose leaf is absent, which is an ordinary state
// for a gate target (a launcher directory already swept, a hub about to be created), so a bare
// EvalSymlinks call would answer "cannot resolve" far more often than "resolved". Walking upward to
// the deepest ancestor that DOES exist, resolving that, and re-joining the missing tail keeps the
// answer usable in exactly those cases while still resolving every link that is actually there.
// An unresolvable path degrades to its cleaned lexical form, which is the pre-existing behaviour
// and therefore never stricter than it was — the fallback direction matters on Windows, where
// EvalSymlinks over a directory junction is the case least covered by this repo's own testing.
func resolveAncestorSymlinks(dir string) string {
	cleaned := filepath.Clean(dir)
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return resolved
	}
	parent := filepath.Dir(cleaned)
	if parent == cleaned {
		return cleaned
	}
	return filepath.Join(resolveAncestorSymlinks(parent), filepath.Base(cleaned))
}

// pruneEmptyAncestors walks upward from start, removing empty directories up to
// (but not including) stop. All errors are silently swallowed. The helper is
// idempotent: calling it on an already-pruned tree is safe.
func pruneEmptyAncestors(start, stop string) {
	cur := start

	for {
		// Boundary guard: check if cur is still strictly under stop
		rel, err := filepath.Rel(stop, cur)
		if err != nil {
			// Filesystem error on Rel (rare)
			return
		}

		// Check if cur is the stop dir or above it (outside the target subtree)
		if rel == "." || strings.HasPrefix(rel, "..") {
			// We've reached or passed the boundary; halt
			return
		}

		// Attempt to remove the empty directory
		if err := os.Remove(cur); err != nil {
			// Directory is not empty, already gone, or other error; halt
			return
		}

		// Successfully removed; move to parent
		cur = filepath.Dir(cur)
	}
}

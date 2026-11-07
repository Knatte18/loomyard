// ancestors.go implements the empty-directory sweeper that prunes empty ancestors of removed worktrees and launchers.
// It also holds the containment check that the destructive callers share.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// refuseUncontainedPath returns an error unless target resolves strictly below container.
//
// It guards the teardown helpers that delete a path derived by joining a caller-supplied slug onto hub geometry.
// Slug validation is the primary defence.
// A derived path that has escaped its container would reach os.RemoveAll.
// Asserting containment costs far less than being wrong once.
//
// The comparison resolves the symlinks in both paths before it relates them.
// A lexical comparison trusts the nominal path.
// A symlink at an intermediate segment of target lets a gated removal act outside container while the check passes.
// The final component of target stays unresolved.
// A link being removed must be judged where it lives, not where it points.
//
// The failure is returned as a refusal, not a plain error.
// Best-effort callers discard every error that is not a refusal, so a plain error would be swallowed silently.
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

// containmentFailure returns the reason target does not resolve strictly below container, or nil when it does.
//
// The reason carries no message prefix, so a caller that already names the action does not repeat it.
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

// containmentPath renders path for a containment comparison, resolving every symlink in its parent directories.
// The final component is kept as written.
//
// Keeping the final component lets a link be a valid containment target.
// A link being removed lives inside the container and points elsewhere.
// Resolving that link would move the target outside the container, so the container would refuse it.
// Resolving every ancestor closes the bypass:
// a link at an intermediate segment moves the real directory elsewhere, and the comparison must see that.
func containmentPath(path string) string {
	cleaned := filepath.Clean(path)
	parent := filepath.Dir(cleaned)
	if parent == cleaned {
		return cleaned
	}
	return filepath.Join(resolveAncestorSymlinks(parent), filepath.Base(cleaned))
}

// resolveAncestorSymlinks returns dir with every symlink in it resolved.
//
// filepath.EvalSymlinks fails when the leaf of a path is absent.
// That is ordinary for a gate target, such as a launcher directory already swept or a hub not yet created.
// So the function resolves the deepest ancestor that exists and joins the missing tail back on.
// Every link that is present is still resolved.
// A path that cannot be resolved falls back to its cleaned lexical form.
// The result is therefore never stricter than a lexical comparison.
// The fallback direction matters on Windows.
// There, EvalSymlinks over a directory junction is the case least covered by this repo's own testing.
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

// pruneEmptyAncestors walks upward from start, removing empty directories up to, but not including, stop.
// All errors are silently swallowed, and the walk halts at the first one.
// It is idempotent: calling it on an already-pruned tree is safe.
//
// Removal goes through an os.Root opened at stop.
// A component that escapes stop is refused by the OS rather than followed.
// Callers walk a hub-level container that an attacker can plant a link in, so the nominal path is not trusted.
func pruneEmptyAncestors(start, stop string) {
	root, err := os.OpenRoot(stop)
	if err != nil {
		// The stop directory is gone or cannot be opened, so there is nothing above start left to prune.
		return
	}
	defer func() { _ = root.Close() }()

	cur := start
	for {
		// Boundary guard: cur must still be strictly under stop.
		// The check is lexical because it only ends the loop; the removal below refuses any escape.
		// A lexical comparison can stop the walk early, but it cannot widen it.
		rel, err := filepath.Rel(stop, cur)
		if err != nil {
			// The paths cannot be related, so the walk stops here.
			return
		}

		if rel == "." || strings.HasPrefix(rel, "..") {
			return
		}

		// Remove through the root deletes a single entry, so the OS refuses a non-empty directory.
		// A component that escapes stop is refused rather than followed.
		// A final-component link is unlinked as a link, since os.Root never follows the last component.
		if err := root.Remove(rel); err != nil {
			// Any failure halts the walk, including a directory that is not empty.
			return
		}

		cur = filepath.Dir(cur)
	}
}

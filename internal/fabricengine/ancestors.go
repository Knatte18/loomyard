// ancestors.go holds the containment check that the destructive callers share.
// It resolves the symlinks in a path's ancestors before it relates the path to its container.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"strings"
)

// containmentFailure returns the reason target does not resolve strictly below container, or nil when it does.
//
// The comparison resolves the symlinks in both paths before it relates them.
// A lexical comparison trusts the nominal path.
// A symlink at an intermediate segment of target lets a gated removal act outside container while the check passes.
// The final component of target stays unresolved.
// A link being removed must be judged where it lives, not where it points.
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
// That is ordinary for a gate target, such as a path already removed or a hub not yet created.
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

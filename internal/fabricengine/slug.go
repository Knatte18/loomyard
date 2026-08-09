// slug.go holds the single worktree-slug validator every topology verb that takes a slug shares.
// It lives in its own file because the rule binds both directions of the pair lifecycle: Add must
// refuse a slug that would create a booby-trapped pair, and Remove must refuse the same names for
// the stronger reason that they already exist as hub geometry — `<Hub>/_board`, `<Hub>/.lyx`, and
// the weft siblings are real directories a teardown verb handed one of those names would otherwise
// walk straight into.

package fabricengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/weftname"
)

// validateWorktreeSlug reports whether slug may name a warp↔weft worktree pair, returning a
// specific error when it may not.
//
// junctionNames is the repo's own configured pathspec name-set (Config.Dirs()), folded into the
// reserved set by IsReservedHubName so a name that wires a junction can never also be a worktree.
//
// A slug is by contract a single path component: every consumer re-derives it from a warp worktree
// path via filepath.Base, and the hub scan only looks at the hub's top level, so a
// separator-containing slug would create a pair the rest of the module cannot re-identify.
// Both separators are rejected on every platform — a slash-free contract must not depend on GOOS.
func validateWorktreeSlug(slug string, junctionNames []string) error {
	if strings.TrimSpace(slug) == "" {
		return fmt.Errorf("invalid slug %q: a slug must not be empty", slug)
	}

	if strings.ContainsAny(slug, `/\`) {
		return fmt.Errorf("invalid slug %q: a slug must be a single path component (no '/' or '\\')", slug)
	}

	if strings.HasSuffix(slug, weftname.Suffix) {
		return fmt.Errorf("invalid slug %q: a slug must not end in %q (that suffix is reserved for weft worktrees)", slug, weftname.Suffix)
	}

	if IsReservedHubName(slug, junctionNames) {
		return fmt.Errorf("invalid slug %q: that name is reserved for lyx hub geometry", slug)
	}

	return nil
}

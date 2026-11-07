// slug.go holds validateWorktreeSlug, the one slug validator that every topology verb taking a slug shares.
// The rule binds both directions of the pair lifecycle.
// Adding a pair must refuse a slug that would create a booby-trapped pair.
// Removing a pair must refuse names that already are hub geometry:
// the Bolt checkout and the weft siblings are real directories.
// A teardown given one of those names would otherwise walk straight into it.
// `.lym` is refused as a structural reservation, whatever a repo's fabric.yaml configures.
// `.` and `..` are refused too: they carry no separator and name no reserved directory,
// yet they resolve onto the hub itself once joined onto hub geometry.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/weftname"
)

// ErrInvalidSlug is the sentinel that every slug refusal in this package matches under errors.Is.
// The refusal's own text still carries the specific reason.
var ErrInvalidSlug = errors.New("invalid slug")

// invalidSlug builds a validateWorktreeSlug refusal that renders the formatted reason and matches ErrInvalidSlug.
func invalidSlug(format string, args ...any) error {
	return &sentinelError{sentinel: ErrInvalidSlug, msg: fmt.Sprintf(format, args...)}
}

// ValidatePairSlug checks that slug may name a pair of the hub.
// The refusal it returns matches ErrInvalidSlug under errors.Is.
// junctionNames is the repo's configured junction name set (Config.Dirs()).
func ValidatePairSlug(slug string, junctionNames []string) error {
	return validateWorktreeSlug(slug, junctionNames)
}

// validateWorktreeSlug returns nil when slug may name a warp↔weft worktree pair, and a specific error when it may not.
//
// junctionNames is the repo's configured pathspec name set (Config.Dirs()).
// Its names are reserved, so a name that wires a junction can never also be a worktree.
//
// A slug is by contract a single path component.
// Every consumer re-derives it from a warp worktree path with filepath.Base.
// The hub scan only reads the hub's top level,
// so a slug with a separator would create a pair the module cannot re-identify.
// Both separators are rejected on every platform, since a slash-free contract must not depend on GOOS.
func validateWorktreeSlug(slug string, junctionNames []string) error {
	if strings.TrimSpace(slug) == "" {
		return invalidSlug("invalid slug %q: a slug must not be empty", slug)
	}

	if strings.ContainsAny(slug, `/\`) {
		return invalidSlug("invalid slug %q: a slug must be a single path component (no '/' or '\\')", slug)
	}

	// "." and ".." carry no separator and name no reserved directory, so the checks around this one let them through.
	// filepath.Join resolves them, though: a slug of ".." turns <hub>/_launchers/<anchor>/<slug> into the hub itself,
	// so a teardown's os.RemoveAll would delete the whole hub, warp clone and weft clone included.
	// A slug must denote a directory below its parent, never the parent or the parent's parent.
	// So joining it must not move anywhere: filepath.Clean must leave it untouched.
	if slug != filepath.Clean(slug) || slug == "." || slug == ".." {
		return invalidSlug("invalid slug %q: a slug must name a directory, not a relative path element like \".\" or \"..\"", slug)
	}

	if strings.HasSuffix(slug, weftname.Suffix) {
		return invalidSlug("invalid slug %q: a slug must not end in %q (that suffix is reserved for weft worktrees)", slug, weftname.Suffix)
	}

	if IsReservedHubName(slug, junctionNames) {
		return invalidSlug("invalid slug %q: that name is reserved for lym hub geometry", slug)
	}

	return nil
}

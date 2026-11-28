// Package lymcwd is the entry gate for lym commands.
// It resolves a working directory to the coordinates of a legal lym worktree, or reports why the directory is not one.
// It exposes typed accessors for the paths every caller derives from that Location,
// so no other package recomputes geometry from raw os.Getwd or git --show-toplevel calls.
// WithCwd and CwdFrom carry a working directory through a context.Context instead of the process cwd.
// The value they carry is always absolute.
//
// # The hub suffix
//
// `-LYMHUB` is the only hub container suffix; no code parses, trims or recognises the older `-HUB`.
// The suffix is declared twice on purpose:
// this package's private hubSuffix derives RepoName, and internal/fabricengine's exported HubSuffix backs HubPath.
// Both change together, with the geometryTokenOwners row of TestEnforcement_GeometryLiterals as the third site.
// Hub discovery ignores the name:
// the hub is the worktree root's parent, and looksLikeHub checks its structure.
// A hub that still carries the older suffix therefore resolves, and only the display-only RepoName degrades.
// Such a hub is never renamed in place.
// The tmux socket key is bound to the hub's absolute path.
// An operator removes the old container by hand and re-clones; `clone --reset` resolves the new suffix only.
package lymcwd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// Location and geometry constants name directories and files for lym configuration and fabric, board and hub geometry.
// All path construction must use these constants, never inline string literals.
const (
	// hubSuffix is the suffix appended to a repo name to form the hub container directory
	// (e.g. "loomyard" → "loomyard-LYMHUB").
	// RepoName derives from it.
	// HubPath is exported from internal/fabricengine, which declares its own copy of this literal.
	hubSuffix = "-LYMHUB"
)

// ErrNotAGitRepo is returned when a directory is not within a git repository.
var ErrNotAGitRepo = errors.New("not a git repository")

// Location represents the resolved coordinates of a legal lym worktree.
// It records the repo name, the hub path, the worktree name within that hub, and the anchor subpath in AnchorRel.
// It stores neither the cwd nor the worktree path, since both derive from the other fields.
// Under the strict cwd gate, the cwd equals AnchorPath() after a successful resolve.
// The worktree path is always a direct child of HubPath.
type Location struct {
	RepoName     string
	HubPath      string
	WorktreeName string
	AnchorRel    string
}

// WorktreePath returns the path to this worktree: a direct child of the hub named WorktreeName.
func (l *Location) WorktreePath() string {
	return filepath.Join(l.HubPath, l.WorktreeName)
}

// AnchorPath returns the path to the anchored subpath lym operates at within this worktree.
func (l *Location) AnchorPath() string {
	return filepath.Join(l.WorktreePath(), l.AnchorRel)
}

// Getwd returns the current working directory.
//
// It wraps os.Getwd. Code outside cmd/lym/main.go calls this instead of os.Getwd.
// It returns an error if the current directory cannot be determined.
func Getwd() (string, error) {
	return os.Getwd()
}

// Resolve builds a Location from cwd.
// It finds the worktree root in-process and reads the recorded .lym-anchor marker for AnchorRel.
// It then requires cwd to equal the anchored directory exactly.
//
// AnchorRel defaults to "." when no anchor is recorded.
// Resolve does not check for _lym/; internal/configengine checks it.
//
// It returns the Location on success.
// It returns ErrNotAGitRepo when cwd is outside a git repo.
// It returns ErrCwdOutsideAnchor when cwd is not exactly the anchored directory.
func Resolve(cwd string) (*Location, error) {
	return resolveCore(cwd, true)
}

// ResolveWorktree builds a Location like Resolve, without the cwd gate.
//
// It serves callers that hold a worktree root rather than an acting cwd, for whom the gate would fire wrongly.
// Only Resolve applies the gate; layout construction above a subpath anchor does not need it.
func ResolveWorktree(worktreeRoot string) (*Location, error) {
	return resolveCore(worktreeRoot, false)
}

// NamedWorktree builds the Location of the worktree called name in the hub at hubPath.
// AnchorRel is that worktree's own recorded anchor, with the hub-wide fallback and then "." when none is recorded.
// It applies no cwd gate, spawns no git, and needs neither the worktree nor its records sibling to exist.
// It is how code outside this package gets the Location of a worktree other than the acting one.
func NamedWorktree(hubPath, name string) (*Location, error) {
	anchorRel, err := anchorRelOrRoot(hubPath, name)
	if err != nil {
		return nil, err
	}
	return &Location{
		RepoName:     strings.TrimSuffix(filepath.Base(hubPath), hubSuffix),
		HubPath:      hubPath,
		WorktreeName: name,
		AnchorRel:    anchorRel,
	}, nil
}

// resolveCore is the shared body behind Resolve and ResolveWorktree.
// It finds the worktree root and reads the worktree's recorded anchor for AnchorRel.
// It applies the strict cwd gate when applyGate is set.
// applyGate is true only for Resolve, whose input is the acting cwd.
// ResolveWorktree passes false, because its input is a worktree root rather than an acting cwd,
// and it must never be gated against itself.
func resolveCore(cwd string, applyGate bool) (*Location, error) {
	workTreeRoot, err := gitWorktreeRoot(cwd)
	if err != nil {
		return nil, err
	}

	hubPath := filepath.Dir(workTreeRoot)
	anchorRel, err := anchorRelOrRoot(hubPath, filepath.Base(workTreeRoot))
	if err != nil {
		return nil, err
	}

	return buildLocation(cwd, workTreeRoot, hubPath, anchorRel, applyGate)
}

// ResolveWithin builds the Location of the worktree whose anchored directory or records sibling holds cwd.
// It serves a read-only verb that a session runs from wherever its harness has moved the working directory.
// An example is a directory in the records sibling, where the strict cwd gate of Resolve refuses.
// A cwd anywhere in a records sibling resolves to the Location of the worktree that sibling belongs to.
//
// It returns ErrNotAGitRepo when cwd is outside a git repo.
// It returns ErrCwdOutsideAnchor when cwd lies in a worktree but outside its anchored directory.
// That error names the anchored directory.
func ResolveWithin(cwd string) (*Location, error) {
	workTreeRoot, err := gitWorktreeRoot(cwd)
	if err != nil {
		return nil, err
	}

	hubPath := filepath.Dir(workTreeRoot)
	ownerName, inRecordsSibling := ownerOfRecordsSibling(hubPath, filepath.Base(workTreeRoot))
	anchorRel, err := anchorRelOrRoot(hubPath, ownerName)
	if err != nil {
		return nil, err
	}

	ownerRoot := filepath.Join(hubPath, ownerName)
	if !inRecordsSibling {
		if err := checkCwdWithinAnchor(filepath.Clean(cwd), anchorRel, ownerRoot); err != nil {
			return nil, err
		}
	}
	return buildLocation(cwd, ownerRoot, hubPath, anchorRel, false)
}

// ownerOfRecordsSibling returns the name of the worktree whose records sibling is the worktree called name in hubPath.
// It also reports whether name is such a records sibling.
// A name that ends in the records-sibling suffix with no worktree of the trimmed name beside it is not a records sibling.
// Such a name comes back unchanged.
func ownerOfRecordsSibling(hubPath, name string) (string, bool) {
	owner, hasSuffix := strings.CutSuffix(name, weftname.Suffix)
	if !hasSuffix || owner == "" {
		return name, false
	}
	info, err := os.Stat(filepath.Join(hubPath, owner))
	if err != nil || !info.IsDir() {
		return name, false
	}
	return owner, true
}

// anchorRelOrRoot returns the recorded anchor of the worktree called name in hubPath, or "." when none is recorded.
// The fallback is never a path derived from cwd:
// a cwd-dependent value would make Location lie.
// _lym would resolve to a different place depending on where the user stood.
func anchorRelOrRoot(hubPath, name string) (string, error) {
	anchor, found, err := readRecordedAnchor(hubPath, name)
	if err != nil {
		return "", err
	}
	if !found {
		return ".", nil
	}
	return anchor, nil
}

// gitWorktreeRoot finds the worktree root at or above cwd in-process, as `git rev-parse --show-toplevel` does.
// It resolves the symlinks in cwd first, so the walk climbs the real parent directories.
// A cwd reached through a symlinked path then gets the same hub path and reed socket key as through its real path.
// Any failure returns the bare ErrNotAGitRepo sentinel.
// That covers a missing cwd, a pruned worktree, and a gitfile naming a missing git dir.
// It is returned unwrapped, so callers that pin the exact error text still see ErrNotAGitRepo.
func gitWorktreeRoot(cwd string) (string, error) {
	resolved, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", ErrNotAGitRepo
	}
	workTreeRoot, _, err := dotgit.FindRoot(resolved)
	if err != nil {
		return "", ErrNotAGitRepo
	}
	return workTreeRoot, nil
}

// buildLocation assembles the Location from its resolved parts.
// When applyGate is set, it applies the strict cwd gate before returning.
func buildLocation(cwd, workTreeRoot, hubPath, anchorRel string, applyGate bool) (*Location, error) {
	if applyGate {
		if err := checkCwdAnchorGate(filepath.Clean(cwd), anchorRel, workTreeRoot); err != nil {
			return nil, err
		}
	}

	return &Location{
		RepoName:     strings.TrimSuffix(filepath.Base(hubPath), hubSuffix),
		HubPath:      hubPath,
		WorktreeName: filepath.Base(workTreeRoot),
		AnchorRel:    anchorRel,
	}, nil
}

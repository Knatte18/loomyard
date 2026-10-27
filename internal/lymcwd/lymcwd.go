// Package lymcwd is the entry gate that converts "the process started somewhere" into "these are
// the coordinates of a legal lym worktree,
// or here is why this is not one".
// It is no longer a geometry owner — constructing paths from structural tokens is precisely what it
// stops doing — it resolves the active Location from a working directory and exposes typed
// accessors for the paths every caller derives from that Location, so no other package recomputes
// geometry from raw os.Getwd or git --show-toplevel calls.
// WithCwd/CwdFrom are the per-call cwd-injection seam that carries this working directory through a
// context.Context instead of the process cwd, and the value they carry is always absolute.
//
// # The hub suffix
//
// `-LYMHUB` is the sole hub container suffix, and no code parses, trims or recognises the retired `-HUB`.
// The suffix is declared twice by sanction: this package's private hubSuffix, for RepoName derivation, and internal/fabricengine's exported HubSuffix, used by HubPath.
// Both move together, with TestEnforcement_GeometryLiterals's geometryTokenOwners row as the third site.
// Hub discovery is name-independent: the hub is the worktree root's parent and looksLikeHub is structural,
// so a hub still carrying the retired suffix resolves, and only the display-only RepoName degrades.
// Such a hub is never renamed in place, because launcher scripts and the tmux socket key are bound to the hub's absolute path;
// the operator removes the old container by hand and re-clones, and `clone --reset` resolves the new suffix only.
package lymcwd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/dotgit"
)

// Location and geometry constants define directory and file names used by lym configuration and fabric/board/hub geometry.
// All path construction must use these constants, never inline string literals.
const (
	// hubSuffix is the suffix appended to a repo name to form the hub container directory
	// (e.g. "loomyard" → "loomyard-LYMHUB"). It stays private to lymcwd: RepoName derives
	// from it below, but the exported HubPath(parent, name) constructor moved to
	// internal/fabricengine, which declares its own copy of this literal.
	hubSuffix = "-LYMHUB"
)

// ErrNotAGitRepo is returned when a directory is not within a git repository.
var ErrNotAGitRepo = errors.New("not a git repository")

// Location represents the resolved coordinates of a legal lym worktree: the repo it belongs to, the hub container it lives inside, its own name within that hub, and the anchored subpath lym operates at.
// It deliberately does not store Cwd or the worktree path themselves — under the strict cwd gate, cwd is provably equal to AnchorPath() after a successful resolve, and the worktree path is a direct child of HubPath by construction, so both are derivable rather than stored.
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
// It wraps os.Getwd and is the ONLY permitted os.Getwd call outside cmd/lym/main.go.
// Returns an error if the current directory cannot be determined.
func Getwd() (string, error) {
	return os.Getwd()
}

// Resolve builds a Location from the given cwd by finding the worktree root in-process and reading
// the recorded .lym-anchor marker for AnchorRel, then requires cwd to equal the anchored directory
// exactly.
//
// AnchorRel defaults to "."
// when no anchor is recorded.
// Resolve does NOT check for _lym/ (that stays in internal/configengine).
//
// Returns the Location on success, ErrNotAGitRepo when cwd is outside a git repo, or
// ErrCwdOutsideAnchor when cwd is not exactly the anchored directory.
func Resolve(cwd string) (*Location, error) {
	return resolveCore(cwd, true)
}

// ResolveWorktree builds a Location like Resolve but applies NO cwd gate.
//
// It exists for callers holding a worktree root (not an acting cwd) where the gate would spuriously
// fire.
// The gate applies only to Resolve(cwd), not internal sibling-layout construction above a subpath
// anchor.
func ResolveWorktree(worktreeRoot string) (*Location, error) {
	return resolveCore(worktreeRoot, false)
}

// ResolveWithAnchor builds a Location exactly as Resolve does,
// but takes the anchor as a parameter instead of reading the recorded marker,
// and applies NO cwd gate.
// It is a deliberate bypass, not a general-purpose resolver: a caller reaching for it to escape a
// gate failure is misusing it — the correct fix is to stand in the anchored directory.
// It must stay ungated because both its callers stand somewhere the gate would reject: fabric's
// clone passes the freshly-cloned worktree root while the anchor may be a non-"."
// subpath,
// and gitkit injects anchors into its fixture repos standing in for hubs.
func ResolveWithAnchor(cwd, anchor string) (*Location, error) {
	return resolveWithAnchorCore(cwd, anchor, false)
}

// NamedWorktree builds the Location of the worktree called name in the hub at hubPath.
// AnchorRel is that worktree's own recorded anchor, with the hub-wide fallback and then "." when none is recorded.
// It applies no cwd gate, spawns no git, and needs neither the worktree nor its records sibling to exist.
// It is how code outside this package gets the Location of a worktree other than the acting one.
func NamedWorktree(hubPath, name string) (*Location, error) {
	anchorRel := "."
	anchor, found, err := readRecordedAnchor(hubPath, name)
	if err != nil {
		return nil, err
	}
	if found {
		anchorRel = anchor
	}
	return &Location{
		RepoName:     strings.TrimSuffix(filepath.Base(hubPath), hubSuffix),
		HubPath:      hubPath,
		WorktreeName: name,
		AnchorRel:    anchorRel,
	}, nil
}

// resolveCore is the shared body behind Resolve and ResolveWorktree.
// It finds the worktree root, reads the worktree's own recorded anchor for AnchorRel, and optionally applies the strict cwd gate.
// applyGate is true only for Resolve's entry-point cwd.
// ResolveWorktree passes false because its input is a worktree root, not an acting cwd, and must never be gated against itself.
func resolveCore(cwd string, applyGate bool) (*Location, error) {
	workTreeRoot, err := gitWorktreeRoot(cwd)
	if err != nil {
		return nil, err
	}

	hubPath := filepath.Dir(workTreeRoot)

	// AnchorRel falls back to "." rather than a cwd-derived relative path: the
	// name Location makes a lie of a cwd-dependent answer, and _lym would
	// otherwise resolve to a different place depending on where the user
	// happened to stand.
	anchorRel := "."
	anchor, found, err := readRecordedAnchor(hubPath, filepath.Base(workTreeRoot))
	if err != nil {
		return nil, err
	}
	if found {
		anchorRel = anchor
	}

	return buildLocation(cwd, workTreeRoot, hubPath, anchorRel, applyGate)
}

// resolveWithAnchorCore is the shared body behind ResolveWithAnchor.
// It finds the worktree root exactly as resolveCore does, but takes anchor
// as a parameter instead of reading the recorded marker.
func resolveWithAnchorCore(cwd, anchor string, applyGate bool) (*Location, error) {
	workTreeRoot, err := gitWorktreeRoot(cwd)
	if err != nil {
		return nil, err
	}

	hubPath := filepath.Dir(workTreeRoot)
	return buildLocation(cwd, workTreeRoot, hubPath, anchor, applyGate)
}

// gitWorktreeRoot resolves the symlinks in cwd, then finds the worktree root at or above the resolved path in-process, as `git rev-parse --show-toplevel` does.
// Resolving the start rather than the root found from it makes the walk climb the real parents, so a cwd reached through any symlinked path, to the root or below it, gets the same hub path, and with it the same reed socket key, as through its real path.
// Any failure, including a missing cwd, a pruned worktree or a gitfile naming a missing git dir, is the bare sentinel.
// It is returned unwrapped, so errors.Is(err, ErrNotAGitRepo) keeps matching at every consumer that pins its exact rendering.
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

// buildLocation assembles the Location from its resolved parts and optionally
// applies the strict cwd gate before returning it.
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

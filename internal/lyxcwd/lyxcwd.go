// Package lyxcwd is the entry gate that converts "the process started
// somewhere" into "these are the coordinates of a legal lyx worktree, or here
// is why this is not one". It is no longer a geometry owner — constructing
// paths from structural tokens is precisely what it stops doing — it resolves
// the active Location from a working directory and exposes typed accessors
// for the paths every caller derives from that Location, so no other package
// recomputes geometry from raw os.Getwd or git --show-toplevel calls.
package lyxcwd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// Location and geometry constants define directory and file names used by lyx
// configuration and weft/board/hub geometry. All path construction must use these
// constants, never inline string literals.
const (
	// lyxDirName is the directory name for the lyx system directory within a worktree.
	// internal/configengine.LyxDirName is the single exported declarer of this token now;
	// this private const is a transitional second declarer for lyxcwd's own
	// remaining _lyx-anchored method (PortalTarget), removed once that method relocates.
	lyxDirName = "_lyx"

	// hubSuffix is the suffix appended to a repo name to form the hub container directory
	// (e.g. "loomyard" → "loomyard-HUB"). It stays private to lyxcwd: RepoName derives
	// from it below, but the exported HubPath(parent, name) constructor moved to
	// internal/fabricengine, which declares its own copy of this literal.
	hubSuffix = "-HUB"

	// PatternDirName is the directory name for the PATTERN constraint-injection surface
	// within a worktree (i.e. <worktree>/_pattern). It is a transitional second
	// declarer of that token, read as a git pathspec by internal/fabricengine and
	// its tests until batch 6's card 35 cuts them over to that package's own
	// const; internal/pattern is the new owner for everything else. Use
	// internal/pattern's Dir/File/FileHere to obtain the paths built from it.
	PatternDirName = "_pattern"
)

// ErrNotAGitRepo is returned when a directory is not within a git repository.
var ErrNotAGitRepo = errors.New("not a git repository")

// Location represents the resolved coordinates of a legal lyx worktree: the
// repo it belongs to, the hub container it lives inside, its own name within
// that hub, and the anchored subpath lyx operates at. It deliberately does not
// store Cwd or the worktree path themselves — under the strict cwd gate, cwd
// is provably equal to AnchorPath() after a successful resolve, and the
// worktree path is a direct child of HubPath by construction, so both are
// derivable rather than stored.
type Location struct {
	RepoName     string
	HubPath      string
	WorktreeName string
	AnchorRel    string
}

// WorktreePath returns the path to this worktree: a direct child of the hub
// named WorktreeName.
func (l *Location) WorktreePath() string {
	return filepath.Join(l.HubPath, l.WorktreeName)
}

// AnchorPath returns the path to the anchored subpath lyx operates at within
// this worktree.
func (l *Location) AnchorPath() string {
	return filepath.Join(l.WorktreePath(), l.AnchorRel)
}

// Getwd returns the current working directory.
//
// It wraps os.Getwd and is the ONLY permitted os.Getwd call outside cmd/lyx/main.go.
// Returns an error if the current directory cannot be determined.
func Getwd() (string, error) {
	return os.Getwd()
}

// Resolve builds a Location from the given cwd by running
// git rev-parse --show-toplevel and reading the recorded .lyx-anchor
// marker for AnchorRel, then requires cwd to equal the anchored directory
// exactly.
//
// AnchorRel defaults to "." when no anchor is recorded.
// Resolve does NOT check for _lyx/ (that stays in internal/configengine).
//
// Returns the Location on success, ErrNotAGitRepo when git fails or cwd is
// outside a git repo, or ErrCwdOutsideAnchor when cwd is not exactly the
// anchored directory.
func Resolve(cwd string) (*Location, error) {
	return resolveCore(cwd, true)
}

// ResolveWorktree builds a Location like Resolve but applies NO cwd gate.
//
// It exists for callers holding a worktree root (not an acting cwd) where the gate would
// spuriously fire. The gate applies only to Resolve(cwd), not internal sibling-layout
// construction above a subpath anchor.
func ResolveWorktree(worktreeRoot string) (*Location, error) {
	return resolveCore(worktreeRoot, false)
}

// ResolveWithAnchor builds a Location exactly as Resolve does, but takes the
// anchor as a parameter instead of reading the recorded marker, and applies
// NO cwd gate. It is a deliberate bypass, not a general-purpose resolver: a
// caller reaching for it to escape a gate failure is misusing it — the
// correct fix is to stand in the anchored directory. It must stay ungated
// because both its callers stand somewhere the gate would reject: fabric's
// clone passes the freshly-cloned worktree root while the anchor may be a
// non-"." subpath, and lyxtest injects anchors into synthetic hubs.
func ResolveWithAnchor(cwd, anchor string) (*Location, error) {
	return resolveWithAnchorCore(cwd, anchor, false)
}

// resolveCore is the shared body behind Resolve and ResolveWorktree: it runs
// git rev-parse --show-toplevel, reads the recorded anchor for AnchorRel, and
// optionally applies the strict cwd gate. applyGate is true only for
// Resolve's entry-point cwd; ResolveWorktree passes false because its input is
// a worktree root, not an acting cwd, and must never be gated against itself.
func resolveCore(cwd string, applyGate bool) (*Location, error) {
	workTreeRoot, err := gitWorktreeRoot(cwd)
	if err != nil {
		return nil, err
	}

	hubPath := filepath.Dir(workTreeRoot)

	// AnchorRel falls back to "." rather than a cwd-derived relative path: the
	// name Location makes a lie of a cwd-dependent answer, and _lyx would
	// otherwise resolve to a different place depending on where the user
	// happened to stand.
	anchorRel := "."
	if anchor, found := readRecordedAnchor(hubPath); found {
		anchorRel = anchor
	}

	return buildLocation(cwd, workTreeRoot, hubPath, anchorRel, applyGate)
}

// resolveWithAnchorCore is the shared body behind ResolveWithAnchor: it runs
// git rev-parse --show-toplevel exactly as resolveCore does, but takes anchor
// as a parameter instead of reading the recorded marker.
func resolveWithAnchorCore(cwd, anchor string, applyGate bool) (*Location, error) {
	workTreeRoot, err := gitWorktreeRoot(cwd)
	if err != nil {
		return nil, err
	}

	hubPath := filepath.Dir(workTreeRoot)
	return buildLocation(cwd, workTreeRoot, hubPath, anchor, applyGate)
}

// gitWorktreeRoot runs git rev-parse --show-toplevel at cwd and returns the
// cleaned, OS-native worktree root path.
func gitWorktreeRoot(cwd string) (string, error) {
	stdout, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--show-toplevel"}, cwd)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotAGitRepo, err)
	}
	if exitCode != 0 {
		return "", ErrNotAGitRepo
	}

	workTreeRoot := filepath.FromSlash(strings.TrimSpace(stdout))
	return filepath.Clean(workTreeRoot), nil
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

// WeftPatternDir returns the path to the _pattern directory in the current worktree's weft sibling.
// It mirrors fabricengine.WeftLyxDir exactly and is the junction target for pattern weft.
// Its own weft-sibling base is inlined via weftname.SiblingPath rather than a Location
// method: WeftWorktree relocated to fabricengine in this same batch, and this accessor's
// own relocation (card 35) is a deletion, not a move, so it does not adopt the accessor.
func (l *Location) WeftPatternDir() string {
	return filepath.Join(weftname.SiblingPath(l.HubPath, filepath.Base(l.WorktreePath())), l.AnchorRel, PatternDirName)
}

// WeftPatternDirFor returns the path to the _pattern directory within a named slug's weft worktree.
// It mirrors fabricengine.WeftLyxDirFor exactly and pairs with HostPatternLink(slug) as junction endpoints.
func (l *Location) WeftPatternDirFor(slug string) string {
	return filepath.Join(weftname.SiblingPath(l.HubPath, slug), l.AnchorRel, PatternDirName)
}

// HostPatternLink returns the path to the _pattern junction link in a named slug's host worktree.
// It mirrors fabricengine.HostLyxLink exactly and points into the paired weft worktree via WeftPatternDirFor(slug).
func (l *Location) HostPatternLink(slug string) string {
	return filepath.Join(l.HubPath, slug, l.AnchorRel, PatternDirName)
}

// HostPatternLinkHere returns the path to the _pattern junction link in the current host worktree.
// Derived from WorktreePath()+AnchorRel. It mirrors fabricengine.HostLyxLinkHere exactly and serves
// as the host-side junction endpoint paired with WeftPatternDir().
func (l *Location) HostPatternLinkHere() string {
	return filepath.Join(l.WorktreePath(), l.AnchorRel, PatternDirName)
}

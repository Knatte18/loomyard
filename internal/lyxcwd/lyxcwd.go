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
	"runtime"
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
	// remaining _lyx-anchored methods (LyxDir, PortalTarget, HostLyxLink,
	// HostLyxLinkHere, WeftLyxDir, WeftLyxDirFor), removed once those methods relocate.
	lyxDirName = "_lyx"

	// dotLyxDirName is the directory name for ephemeral, machine-bound lyx state (e.g. reed runtime
	// state), distinct from lyxDirName ("_lyx") which is durable and weft-synced.
	dotLyxDirName = ".lyx"

	// BoardDirName is the name of the board data directory inside the hub (i.e. <hub>/_board).
	// It is the single source of this literal; use BoardDir(hub) to obtain the full path.
	BoardDirName = "_board"

	// HubSuffix is the suffix appended to a repo name to form the hub container directory
	// (e.g. "loomyard" → "loomyard-HUB"). Use HubPath(parent, name) to obtain the full path.
	HubSuffix = "-HUB"

	// PatternDirName is the directory name for the PATTERN constraint-injection surface
	// within a worktree (i.e. <worktree>/_pattern). Use PatternDir/PatternFile to obtain paths.
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
		RepoName:     strings.TrimSuffix(filepath.Base(hubPath), HubSuffix),
		HubPath:      hubPath,
		WorktreeName: filepath.Base(workTreeRoot),
		AnchorRel:    anchorRel,
	}, nil
}

// BoardDir returns the absolute path to the board data directory inside hub.
func BoardDir(hub string) string {
	return filepath.Join(hub, BoardDirName)
}

// HubPath returns the absolute path to the hub container directory for the given repo name inside parent.
func HubPath(parent, name string) string {
	return filepath.Join(parent, name+HubSuffix)
}

// WeftHostSlug parses a weft sibling directory name and returns the host slug it corresponds to.
// It reports whether name ends with weftname.Suffix AND the stripped prefix is non-empty.
func WeftHostSlug(name string) (slug string, ok bool) {
	if !strings.HasSuffix(name, weftname.Suffix) {
		return "", false
	}
	s := strings.TrimSuffix(name, weftname.Suffix)
	if s == "" {
		return "", false
	}
	return s, true
}

// HubReservedNames returns the hub-structural reserved name-set that lyxcwd owns:
// _raddle, _board, _portals, _launchers. It deliberately excludes configengine.LyxDirName and PatternDirName,
// which are config-migrated junction names folded into the reserved set by IsReservedHubName's
// junctionNames parameter instead.
func HubReservedNames() []string {
	return []string{BoardDirName, "_portals", "_launchers", "_raddle"}
}

// IsReservedHubName reports whether name is one of the hub-level entry names a worktree slug
// must never claim: HubReservedNames UNION the caller-supplied junctionNames (the weft-backed
// junction name-set injected from fabric config). lyxcwd stays config-blind; the junction
// portion is passed in by the caller.
func IsReservedHubName(name string, junctionNames []string) bool {
	for _, reserved := range HubReservedNames() {
		if name == reserved {
			return true
		}
	}
	for _, junctionName := range junctionNames {
		if name == junctionName {
			return true
		}
	}
	return false
}

// LyxDir returns the path to the _lyx directory in the current working directory.
func (l *Location) LyxDir() string {
	return filepath.Join(l.AnchorPath(), lyxDirName)
}

// DotLyxDir returns the path to the ephemeral .lyx directory (machine-bound runtime state),
// distinct from the durable, weft-synced LyxDir().
func (l *Location) DotLyxDir() string {
	return filepath.Join(l.AnchorPath(), dotLyxDirName)
}

// PortalsDir returns the path to the _portals directory in the hub.
func (l *Location) PortalsDir() string {
	return filepath.Join(l.HubPath, "_portals")
}

// PortalLink returns the path to the mirrored portal junction link for the given slug.
// It is mirrored into the repo subpath structure, including RelPath segments.
func (l *Location) PortalLink(slug string) string {
	return filepath.Join(l.HubPath, "_portals", l.AnchorRel, slug)
}

// PortalTarget returns the path to the _lyx directory within a portal for the given slug.
func (l *Location) PortalTarget(slug string) string {
	return filepath.Join(l.HubPath, slug, l.AnchorRel, lyxDirName)
}

// LaunchersDir returns the path to the _launchers directory in the hub.
func (l *Location) LaunchersDir() string {
	return filepath.Join(l.HubPath, "_launchers")
}

// LauncherDir returns the path to the mirrored launcher directory for the given slug.
// It is mirrored into the repo subpath structure, including RelPath segments.
func (l *Location) LauncherDir(slug string) string {
	return filepath.Join(l.HubPath, "_launchers", l.AnchorRel, slug)
}

// MenuLauncherPath returns the path to the per-subpath menu launcher script.
// It is mirrored into the repo subpath structure. The extension is GOOS-selected: ".cmd" on Windows, ".sh" elsewhere.
func (l *Location) MenuLauncherPath() string {
	return filepath.Join(l.HubPath, "_launchers", l.AnchorRel, menuLauncherName())
}

// menuLauncherName returns the OS-appropriate filename for the menu launcher script.
func menuLauncherName() string {
	if runtime.GOOS == "windows" {
		return "ide-menu.cmd"
	}
	return "ide-menu.sh"
}

// LauncherSpawnRel returns the relative path from a launcher directory to the target worktree's
// subpath for spawning.
func (l *Location) LauncherSpawnRel(slug string) string {
	rel, _ := filepath.Rel(l.LauncherDir(slug), filepath.Join(filepath.Join(l.HubPath, slug), l.AnchorRel))
	return rel
}

// MenuLauncherRel returns the relative path from the menu launcher directory to the
// primeName worktree's subpath for menu spawning. primeName is the main worktree's
// base name, sourced by the caller via fabricengine.PrimeName(l) — lyxcwd no
// longer resolves the main worktree itself (that subprocess-backed lookup is
// fabricengine's, per the Hub Geometry Invariant).
func (l *Location) MenuLauncherRel(primeName string) string {
	rel, _ := filepath.Rel(filepath.Dir(l.MenuLauncherPath()), filepath.Join(l.HubPath, primeName, l.AnchorRel))
	return rel
}

// WeftWorktreePath returns the path to a sibling weft worktree with the given slug.
func (l *Location) WeftWorktreePath(slug string) string {
	return weftname.SiblingPath(l.HubPath, slug)
}

// WeftWorktree returns the path to the weft worktree paired with the current host worktree.
func (l *Location) WeftWorktree() string {
	return weftname.SiblingPath(l.HubPath, filepath.Base(l.WorktreePath()))
}

// WeftLyxDir returns the path to the _lyx directory in the current worktree's weft sibling.
// It is the junction target for lyx weft and the pathspec base for weft operations.
func (l *Location) WeftLyxDir() string {
	return filepath.Join(l.WeftWorktree(), l.AnchorRel, lyxDirName)
}

// WeftLyxDirFor returns the path to the _lyx directory within a named slug's weft worktree.
// It is the junction target paired by spawn seeds and pairs with HostLyxLink(slug).
func (l *Location) WeftLyxDirFor(slug string) string {
	return filepath.Join(l.WeftWorktreePath(slug), l.AnchorRel, lyxDirName)
}

// WeftPatternDir returns the path to the _pattern directory in the current worktree's weft sibling.
// It mirrors WeftLyxDir exactly and is the junction target for pattern weft.
func (l *Location) WeftPatternDir() string {
	return filepath.Join(l.WeftWorktree(), l.AnchorRel, PatternDirName)
}

// WeftPatternDirFor returns the path to the _pattern directory within a named slug's weft worktree.
// It mirrors WeftLyxDirFor exactly and pairs with HostPatternLink(slug) as junction endpoints.
func (l *Location) WeftPatternDirFor(slug string) string {
	return filepath.Join(l.WeftWorktreePath(slug), l.AnchorRel, PatternDirName)
}

// WeftRaddleDir returns the path to the _raddle directory in the current worktree's weft sibling.
//
// Returns filepath.Join(WeftWorktree(), RelPath, "_raddle").
func (l *Location) WeftRaddleDir() string {
	return filepath.Join(l.WeftWorktree(), l.AnchorRel, "_raddle")
}

// HostLyxLink returns the path to the _lyx junction link in a named slug's host worktree.
// It is the host-side junction endpoint that points into the paired weft worktree via WeftLyxDirFor(slug).
func (l *Location) HostLyxLink(slug string) string {
	return filepath.Join(l.HubPath, slug, l.AnchorRel, lyxDirName)
}

// HostLyxLinkHere returns the path to the _lyx junction link in the current host worktree.
// Derived from WorktreePath()+AnchorRel. It serves as the host-side junction endpoint
// paired with WeftLyxDir().
func (l *Location) HostLyxLinkHere() string {
	return filepath.Join(l.WorktreePath(), l.AnchorRel, lyxDirName)
}

// HostPatternLink returns the path to the _pattern junction link in a named slug's host worktree.
// It mirrors HostLyxLink exactly and points into the paired weft worktree via WeftPatternDirFor(slug).
func (l *Location) HostPatternLink(slug string) string {
	return filepath.Join(l.HubPath, slug, l.AnchorRel, PatternDirName)
}

// HostPatternLinkHere returns the path to the _pattern junction link in the current host worktree.
// Derived from WorktreePath()+AnchorRel. It mirrors HostLyxLinkHere exactly and serves
// as the host-side junction endpoint paired with WeftPatternDir().
func (l *Location) HostPatternLinkHere() string {
	return filepath.Join(l.WorktreePath(), l.AnchorRel, PatternDirName)
}

// HostJunction represents a directory junction in the host worktree that links to a weft directory.
type HostJunction struct {
	Name   string // Name is the directory name (e.g., "_lyx")
	Link   string // Link is the host-side path to the junction
	Target string // Target is the weft-side path the junction points to
}

// HostJunctions returns the list of host junctions for a given slug, one record per name in names,
// in names's own order (no forced sort). For each name, the record is {Name, Link, Target} where
// Link is HubPath/slug-anchored (inlined here since fabricengine.WorktreePath is not importable from
// this in-module method) and Target is computed from WeftWorktreePath and AnchorRel.
// HostJunctions is HubPath/slug-anchored; HostJunctionsHere below is the Here-anchored counterpart.
func (l *Location) HostJunctions(slug string, names []string) []HostJunction {
	junctions := make([]HostJunction, 0, len(names))
	for _, name := range names {
		junctions = append(junctions, HostJunction{
			Name:   name,
			Link:   filepath.Join(l.HubPath, slug, l.AnchorRel, name),
			Target: filepath.Join(l.WeftWorktreePath(slug), l.AnchorRel, name),
		})
	}
	return junctions
}

// HostJunctionsHere returns the same HostJunction records as HostJunctions(slug, names),
// but resolved against the current worktree rather than a named slug: Link is built from
// WorktreePath() and each Target from WeftWorktree(). This mirrors HostLyxLinkHere()/HostLyxLink(slug).
// It exists for health-check sites that are Here-anchored and have no slug available.
func (l *Location) HostJunctionsHere(names []string) []HostJunction {
	junctions := make([]HostJunction, 0, len(names))
	for _, name := range names {
		junctions = append(junctions, HostJunction{
			Name:   name,
			Link:   filepath.Join(l.WorktreePath(), l.AnchorRel, name),
			Target: filepath.Join(l.WeftWorktree(), l.AnchorRel, name),
		})
	}
	return junctions
}

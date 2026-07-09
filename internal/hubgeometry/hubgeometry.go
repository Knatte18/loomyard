// Package hubgeometry is the single owner of Loomyard worktree and container geometry.
// It resolves the active Layout from a working directory and exposes typed
// accessors for every derived path, so no other package recomputes geometry
// from raw os.Getwd or git --show-toplevel calls.
package hubgeometry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// Layout and geometry constants centralize the directory and file names used by the lyx
// configuration system and the weft/board/hub geometry vocabulary. All code that constructs
// paths from these names must import this package and use these constants — never inline
// the string literals.
const (
	// LyxDirName is the directory name for the lyx system directory within a worktree.
	LyxDirName = "_lyx"

	// dotLyxDirName is the directory name for the ephemeral, machine-bound lyx state
	// directory within a worktree. It is deliberately distinct from LyxDirName ("_lyx"):
	// "_lyx" (underscore) is durable and weft-synced, while ".lyx" (dot) is ephemeral and
	// local to the machine (e.g. mux's runtime state and lock files never travel with weft).
	dotLyxDirName = ".lyx"

	// configDirName is the subdirectory name within LyxDirName that holds configuration files.
	configDirName = "config"

	// dotEnvName is the filename for environment variable overrides.
	dotEnvName = ".env"

	// WeftSuffix is the suffix appended to a host-worktree slug to form the weft sibling
	// directory name (e.g. "feat" → "feat-weft"). It is the single source of this literal
	// for the whole repo; use WeftSiblingPath/WeftRepoRoot/WeftWorktreePath rather than
	// constructing the path from this constant directly.
	WeftSuffix = "-weft"

	// BoardDirName is the name of the board data directory inside the hub (i.e. <hub>/_board).
	// It is the single source of this literal; use BoardDir(hub) to obtain the full path.
	BoardDirName = "_board"

	// HubSuffix is the suffix appended to a repo name to form the hub container directory
	// (e.g. "loomyard" → "loomyard-HUB"). It is the single source of this literal;
	// use HubPath(parent, name) to obtain the full path.
	HubSuffix = "-HUB"
)

// ErrNotAGitRepo is returned when a directory is not within a git repository.
var ErrNotAGitRepo = errors.New("not a git repository")

// Layout represents the geometry of a worktree and container within a git repository.
//
// Fields:
//   - Cwd: the current working directory (normalized via filepath.Clean)
//   - WorktreeRoot: the root of the git repository (from git rev-parse --show-toplevel)
//   - Hub: the parent directory of WorktreeRoot (the container directory, not a git repo)
//   - RelPath: the relative path from WorktreeRoot to Cwd
//   - Prime: the path to the main (first) worktree from List()
type Layout struct {
	Cwd          string
	WorktreeRoot string
	Hub          string
	RelPath      string
	Prime        string
}

// Getwd returns the current working directory.
//
// It wraps os.Getwd and is the ONLY permitted os.Getwd call outside cmd/lyx/main.go.
// Returns an error if the current directory cannot be determined.
func Getwd() (string, error) {
	return os.Getwd()
}

// Resolve builds a Layout from the given cwd, running git rev-parse --show-toplevel
// to determine the repository root.
//
// Steps:
//  1. Run git rev-parse --show-toplevel from cwd
//  2. On error or non-zero exit, return ErrNotAGitRepo (with context)
//  3. Normalize the output via filepath.FromSlash + filepath.Clean → WorktreeRoot
//  4. Set Cwd = filepath.Clean(cwd)
//  5. Set Hub = filepath.Dir(WorktreeRoot)
//  6. Set RelPath = filepath.Rel(WorktreeRoot, Cwd)
//  7. Call List(cwd) and set Prime to the Main==true entry's Path
//
// Resolve does NOT check for _lyx/ (that authority stays in internal/configengine).
//
// Returns the Layout on success, ErrNotAGitRepo (wrapped with exec-layer context) when
// the git subprocess itself fails to spawn, or the bare ErrNotAGitRepo sentinel (with
// no appended text) when git ran but reported a non-zero exit.
func Resolve(cwd string) (*Layout, error) {
	// Step 1-2: Run git rev-parse --show-toplevel. stderr is discarded: it is git's raw,
	// unwrapped text and must never leak into our JSON error envelope.
	stdout, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--show-toplevel"}, cwd)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotAGitRepo, err)
	}
	if exitCode != 0 {
		// git ran and reported failure (e.g. cwd is outside any git repository);
		// return the bare sentinel with no appended content.
		return nil, ErrNotAGitRepo
	}

	// Step 3: Normalize output
	workTreeRoot := filepath.FromSlash(strings.TrimSpace(stdout))
	workTreeRoot = filepath.Clean(workTreeRoot)

	// Step 4-6: Set layout fields
	cleanCwd := filepath.Clean(cwd)
	hub := filepath.Dir(workTreeRoot)
	relPath, _ := filepath.Rel(workTreeRoot, cleanCwd)

	// Step 7: Get Prime from List
	entries, err := List(cwd)
	if err != nil {
		return nil, fmt.Errorf("failed to get main worktree: %w", err)
	}

	prime := ""
	for _, entry := range entries {
		if entry.Main {
			// Normalize prime path (git may emit forward slashes)
			prime = filepath.FromSlash(entry.Path)
			prime = filepath.Clean(prime)
			break
		}
	}

	return &Layout{
		Cwd:          cleanCwd,
		WorktreeRoot: workTreeRoot,
		Hub:          hub,
		RelPath:      relPath,
		Prime:        prime,
	}, nil
}

// ConfigDir returns the path to the config directory within a baseDir.
//
// The config directory is where YAML configuration files are stored, organized by module.
// Returns filepath.Join(baseDir, LyxDirName, configDirName).
func ConfigDir(baseDir string) string {
	return filepath.Join(baseDir, LyxDirName, configDirName)
}

// ConfigFile returns the path to a module-specific configuration YAML file within a baseDir.
//
// The file is constructed by joining the module name with ".yaml" and placing it
// in the ConfigDir. This is used by callers like configengine.Load to resolve config paths.
// Returns filepath.Join(ConfigDir(baseDir), module+".yaml").
func ConfigFile(baseDir, module string) string {
	return filepath.Join(ConfigDir(baseDir), module+".yaml")
}

// PerchRunsDir returns the path to the base directory for perch block run
// dirs within a baseDir: the parent of every <PerchRunsDir>/<run-id>/
// directory a perch run writes its round artifacts into. It lives under
// _lyx so run artifacts are weft-synced via the host _lyx junction, like
// every other durable lyx state. Per the Hub Geometry Invariant, no other
// package may construct this path.
//
// Returns filepath.Join(baseDir, LyxDirName, "perch").
func PerchRunsDir(baseDir string) string {
	return filepath.Join(baseDir, LyxDirName, "perch")
}

// DotEnv returns the path to the .env file within a baseDir.
//
// The .env file provides environment variable overrides for the worktree.
// Returns filepath.Join(baseDir, dotEnvName).
func DotEnv(baseDir string) string {
	return filepath.Join(baseDir, dotEnvName)
}

// WeftSiblingPath returns the absolute path to the weft sibling worktree for the
// given slug inside hub.
//
// It is a pure bootstrap helper for callers that have no resolved Layout. The result
// is filepath.Join(hub, slug+WeftSuffix), which is the canonical form of the
// <hub>/<slug>-weft directory. The three weft Layout methods delegate here so that
// the WeftSuffix constant is consumed in exactly one place.
func WeftSiblingPath(hub, slug string) string {
	return filepath.Join(hub, slug+WeftSuffix)
}

// BoardDir returns the absolute path to the board data directory inside hub.
//
// It is a pure bootstrap helper for callers that have no resolved Layout. The result
// is filepath.Join(hub, BoardDirName), which is the canonical form of the
// <hub>/_board directory used by the board engine.
func BoardDir(hub string) string {
	return filepath.Join(hub, BoardDirName)
}

// HubPath returns the absolute path to the hub container directory for the given repo name
// inside parent.
//
// It is a pure bootstrap helper for callers that have no resolved Layout. The result
// is filepath.Join(parent, name+HubSuffix), which is the canonical form of the
// <parent>/<name>-HUB directory.
func HubPath(parent, name string) string {
	return filepath.Join(parent, name+HubSuffix)
}

// WeftHostSlug parses a weft sibling directory name and returns the host slug it
// corresponds to.
//
// It reports whether name ends with WeftSuffix AND the stripped prefix is non-empty.
// The non-empty guard rejects a bare "-weft" entry (which would yield an empty slug),
// matching the skip condition in warpengine/prune.go's hub scan. When ok is true,
// slug is the result of strings.TrimSuffix(name, WeftSuffix) and may be passed
// directly to any of the geometry constructors as the host slug.
func WeftHostSlug(name string) (slug string, ok bool) {
	if !strings.HasSuffix(name, WeftSuffix) {
		return "", false
	}
	// Strip the suffix; reject a bare "-weft" name (empty slug).
	s := strings.TrimSuffix(name, WeftSuffix)
	if s == "" {
		return "", false
	}
	return s, true
}

// LyxDir returns the path to the _lyx directory in the current working directory.
//
// Returns filepath.Join(Cwd, LyxDirName).
func (l *Layout) LyxDir() string {
	return filepath.Join(l.Cwd, LyxDirName)
}

// DotLyxDir returns the path to the ephemeral .lyx directory in the current working
// directory. This is where machine-bound, non-weft-synced runtime state lives (e.g. mux's
// mux.json and mux.lock), distinct from the durable, weft-synced LyxDir() ("_lyx").
//
// Returns filepath.Join(Cwd, dotLyxDirName).
func (l *Layout) DotLyxDir() string {
	return filepath.Join(l.Cwd, dotLyxDirName)
}

// WorktreePath returns the path to a sibling worktree with the given slug.
//
// Returns filepath.Join(Hub, slug).
func (l *Layout) WorktreePath(slug string) string {
	return filepath.Join(l.Hub, slug)
}

// PortalsDir returns the path to the _portals directory in the hub.
//
// Returns filepath.Join(Hub, "_portals").
func (l *Layout) PortalsDir() string {
	return filepath.Join(l.Hub, "_portals")
}

// PortalLink returns the path to the mirrored portal junction link for the given slug.
//
// The portal link is mirrored into the repo subpath structure. At RelPath == ".",
// this collapses to <Hub>/_portals/<slug>. For subpaths, it includes the
// RelPath segments: <Hub>/_portals/<RelPath>/<slug>.
//
// Returns filepath.Join(Hub, "_portals", RelPath, slug).
func (l *Layout) PortalLink(slug string) string {
	return filepath.Join(l.Hub, "_portals", l.RelPath, slug)
}

// PortalTarget returns the path to the _lyx directory within a portal for the given slug.
//
// The path is: <Hub>/<slug>/<RelPath>/_lyx
//
// Returns filepath.Join(Hub, slug, RelPath, LyxDirName).
func (l *Layout) PortalTarget(slug string) string {
	return filepath.Join(l.Hub, slug, l.RelPath, LyxDirName)
}

// LaunchersDir returns the path to the _launchers directory in the hub.
//
// This is the un-mirrored root used as a prune boundary and base for MkdirAll.
//
// Returns filepath.Join(Hub, "_launchers").
func (l *Layout) LaunchersDir() string {
	return filepath.Join(l.Hub, "_launchers")
}

// LauncherDir returns the path to the mirrored launcher directory for the given slug.
//
// The launcher directory is mirrored into the repo subpath structure. At RelPath == ".",
// this collapses to <Hub>/_launchers/<slug>. For subpaths, it includes the
// RelPath segments: <Hub>/_launchers/<RelPath>/<slug>.
//
// Returns filepath.Join(Hub, "_launchers", RelPath, slug).
func (l *Layout) LauncherDir(slug string) string {
	return filepath.Join(l.Hub, "_launchers", l.RelPath, slug)
}

// MenuLauncherPath returns the path to the per-subpath menu launcher script.
//
// The menu launcher is mirrored into the repo subpath structure. At RelPath == ".",
// this collapses to <Hub>/_launchers/ide-menu.cmd. For subpaths, it includes
// the RelPath segments: <Hub>/_launchers/<RelPath>/ide-menu.cmd.
//
// Returns filepath.Join(Hub, "_launchers", RelPath, "ide-menu.cmd").
func (l *Layout) MenuLauncherPath() string {
	return filepath.Join(l.Hub, "_launchers", l.RelPath, "ide-menu.cmd")
}

// LauncherSpawnRel returns the relative path from a launcher directory to the
// target worktree's subpath for spawning.
//
// This climbs from <Hub>/_launchers/<RelPath>/<slug> to
// <Hub>/<slug>/<RelPath>, yielding paths like (..\)^(2+N)<slug>\<sub>
// on Windows (N = RelPath segment count). At RelPath == ".", it collapses to
// ..\..\<slug>.
//
// Returns filepath.Rel(LauncherDir(slug), filepath.Join(WorktreePath(slug), RelPath)).
func (l *Layout) LauncherSpawnRel(slug string) string {
	rel, _ := filepath.Rel(l.LauncherDir(slug), filepath.Join(l.WorktreePath(slug), l.RelPath))
	return rel
}

// MenuLauncherRel returns the relative path from the menu launcher directory to
// the main worktree's subpath for menu spawning.
//
// This climbs from <Hub>/_launchers/<RelPath> to
// <Hub>/<Prime>/<RelPath>, yielding paths like (..\)^(1+N)<prime>\<sub>
// (N = RelPath segment count). At RelPath == ".", it collapses to ..\<prime>.
//
// Returns filepath.Rel(filepath.Dir(MenuLauncherPath()), filepath.Join(Prime, RelPath)).
func (l *Layout) MenuLauncherRel() string {
	rel, _ := filepath.Rel(filepath.Dir(l.MenuLauncherPath()), filepath.Join(l.Prime, l.RelPath))
	return rel
}

// PrimeName returns the base name of the main worktree.
//
// Returns filepath.Base(Prime).
func (l *Layout) PrimeName() string {
	return filepath.Base(l.Prime)
}

// Weft geometry methods
//
// The host link and the weft target for the same slug form the two ends of a seeded
// junction: HostLyxLink(slug) ↔ WeftLyxDirFor(slug) are junctions that connect
// the host and weft worktrees for that slug. Similarly, HostLyxLinkHere() ↔ WeftLyxDir()
// are the junctions for the current worktree.

// WeftRepoRoot returns the path to the weft Prime worktree (the git -C target for weft worktree add/remove).
//
// Returns WeftSiblingPath(Hub, PrimeName()), which is filepath.Join(Hub, PrimeName()+WeftSuffix).
func (l *Layout) WeftRepoRoot() string {
	return WeftSiblingPath(l.Hub, l.PrimeName())
}

// WeftWorktreePath returns the path to a sibling weft worktree with the given slug.
//
// Returns WeftSiblingPath(Hub, slug), parallel to WorktreePath(slug).
func (l *Layout) WeftWorktreePath(slug string) string {
	return WeftSiblingPath(l.Hub, slug)
}

// WeftWorktree returns the path to the weft worktree paired with the current host worktree.
//
// Returns WeftSiblingPath(Hub, filepath.Base(WorktreeRoot)), the weft analog of
// WorktreeRoot. At the main worktree, this equals WeftRepoRoot().
func (l *Layout) WeftWorktree() string {
	return WeftSiblingPath(l.Hub, filepath.Base(l.WorktreeRoot))
}

// WeftLyxDir returns the path to the _lyx directory in the current worktree's weft sibling.
//
// The path is: <hub>/<current-worktree>-weft/<RelPath>/_lyx. This is the junction target
// for lyx weft and the pathspec base for weft operations, with RelPath-mirroring like
// PortalTarget (collapses to <weft>/_lyx at RelPath ".").
//
// Returns filepath.Join(WeftWorktree(), RelPath, LyxDirName).
func (l *Layout) WeftLyxDir() string {
	return filepath.Join(l.WeftWorktree(), l.RelPath, LyxDirName)
}

// WeftLyxDirFor returns the path to the _lyx directory within a named slug's weft worktree.
//
// The path is: <hub>/<slug>-weft/<RelPath>/_lyx. This is the junction target paired
// by spawn seeds for <slug>, and pairs with HostLyxLink(slug) as the junction endpoints.
// Parallel to HostLyxLink(slug).
//
// Returns filepath.Join(WeftWorktreePath(slug), RelPath, LyxDirName).
func (l *Layout) WeftLyxDirFor(slug string) string {
	return filepath.Join(l.WeftWorktreePath(slug), l.RelPath, LyxDirName)
}

// WeftRaddleDir returns the path to the _raddle directory in the current worktree's weft sibling.
//
// Returns filepath.Join(WeftWorktree(), RelPath, "_raddle").
func (l *Layout) WeftRaddleDir() string {
	return filepath.Join(l.WeftWorktree(), l.RelPath, "_raddle")
}

// HostLyxLink returns the path to the _lyx junction link in a named slug's host worktree.
//
// The path is: <hub>/<slug>/<RelPath>/_lyx. This is the host-side junction endpoint that
// points into the paired weft worktree via WeftLyxDirFor(slug).
//
// Returns filepath.Join(WorktreePath(slug), RelPath, LyxDirName).
func (l *Layout) HostLyxLink(slug string) string {
	return filepath.Join(l.WorktreePath(slug), l.RelPath, LyxDirName)
}

// HostLyxLinkHere returns the path to the _lyx junction link in the current host worktree.
//
// The path is: <hub>/<current-worktree>/<RelPath>/_lyx, derived from WorktreeRoot+RelPath,
// not from Cwd. This is intentionally distinct from LyxDir() (which is Cwd-based) and serves
// as the host-side junction endpoint paired with WeftLyxDir().
//
// Returns filepath.Join(WorktreeRoot, RelPath, LyxDirName).
func (l *Layout) HostLyxLinkHere() string {
	return filepath.Join(l.WorktreeRoot, l.RelPath, LyxDirName)
}

// HostJunction represents a directory junction in the host worktree that links to a weft directory.
//
// It carries three fields because the two seeding operations (junction creation and
// git-exclude entry) consume different ones:
//   - Link: used by junction creation (fslink.CreateDirLink)
//   - Target: used by junction creation (fslink.CreateDirLink)
//   - Name: used by git-exclude seeding
type HostJunction struct {
	Name   string // Name is the directory name (e.g., "_lyx")
	Link   string // Link is the host-side path to the junction
	Target string // Target is the weft-side path the junction points to
}

// HostJunctions returns the list of host junctions for a given slug.
//
// Currently, this returns a single-element slice containing the _lyx junction.
// The junction record carries Name, Link, and Target fields for use by the
// seeders in internal/warpengine.
//
// Returns a slice with exactly one entry: {Name: LyxDirName, Link: HostLyxLink(slug), Target: WeftLyxDirFor(slug)}.
func (l *Layout) HostJunctions(slug string) []HostJunction {
	return []HostJunction{
		{
			Name:   LyxDirName,
			Link:   l.HostLyxLink(slug),
			Target: l.WeftLyxDirFor(slug),
		},
	}
}

// Package paths is the single owner of Loomyard worktree and container geometry.
// It resolves the active Layout from a working directory and exposes typed
// accessors for every derived path, so no other package recomputes geometry
// from raw os.Getwd or git --show-toplevel calls.
package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// Config layout constants centralize the directory and file names used by the lyx configuration system.
// These ensure that all code paths derive their paths from a single source of truth,
// so the layout can be changed in one place without scattering updates across the codebase.
const (
	// LyxDirName is the directory name for the lyx system directory within a worktree.
	LyxDirName = "_lyx"

	// configDirName is the subdirectory name within LyxDirName that holds configuration files.
	configDirName = "config"

	// dotEnvName is the filename for environment variable overrides.
	dotEnvName = ".env"
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
// Resolve does NOT check for _lyx/ (that authority stays in internal/config).
//
// Returns the Layout on success, or ErrNotAGitRepo (wrapped with context) on failure.
func Resolve(cwd string) (*Layout, error) {
	// Step 1-2: Run git rev-parse --show-toplevel
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--show-toplevel"}, cwd)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotAGitRepo, err)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotAGitRepo, stderr)
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
// in the ConfigDir. This is used by callers like config.Load to resolve config paths.
// Returns filepath.Join(ConfigDir(baseDir), module+".yaml").
func ConfigFile(baseDir, module string) string {
	return filepath.Join(ConfigDir(baseDir), module+".yaml")
}

// DotEnv returns the path to the .env file within a baseDir.
//
// The .env file provides environment variable overrides for the worktree.
// Returns filepath.Join(baseDir, dotEnvName).
func DotEnv(baseDir string) string {
	return filepath.Join(baseDir, dotEnvName)
}

// LyxDir returns the path to the _lyx directory in the current working directory.
//
// Returns filepath.Join(Cwd, LyxDirName).
func (l *Layout) LyxDir() string {
	return filepath.Join(l.Cwd, LyxDirName)
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
// Returns filepath.Join(Hub, PrimeName()+"-weft").
func (l *Layout) WeftRepoRoot() string {
	return filepath.Join(l.Hub, l.PrimeName()+"-weft")
}

// WeftWorktreePath returns the path to a sibling weft worktree with the given slug.
//
// Returns filepath.Join(Hub, slug+"-weft"), parallel to WorktreePath(slug).
func (l *Layout) WeftWorktreePath(slug string) string {
	return filepath.Join(l.Hub, slug+"-weft")
}

// WeftWorktree returns the path to the weft worktree paired with the current host worktree.
//
// Returns filepath.Join(Hub, filepath.Base(WorktreeRoot)+"-weft"), the weft analog
// of WorktreeRoot. At the main worktree, this equals WeftRepoRoot().
func (l *Layout) WeftWorktree() string {
	return filepath.Join(l.Hub, filepath.Base(l.WorktreeRoot)+"-weft")
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

// WeftCodeguideDir returns the path to the _codeguide directory in the current worktree's weft sibling.
//
// Returns filepath.Join(WeftWorktree(), RelPath, "_codeguide").
func (l *Layout) WeftCodeguideDir() string {
	return filepath.Join(l.WeftWorktree(), l.RelPath, "_codeguide")
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
// seeders in internal/worktree.
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

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

	"github.com/Knatte18/loomyard/internal/git"
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
	stdout, stderr, exitCode, err := git.RunGit([]string{"rev-parse", "--show-toplevel"}, cwd)
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

// LyxDir returns the path to the _lyx directory in the current working directory.
//
// Returns filepath.Join(Cwd, "_lyx").
func (l *Layout) LyxDir() string {
	return filepath.Join(l.Cwd, "_lyx")
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
// Returns filepath.Join(Hub, slug, RelPath, "_lyx").
func (l *Layout) PortalTarget(slug string) string {
	return filepath.Join(l.Hub, slug, l.RelPath, "_lyx")
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

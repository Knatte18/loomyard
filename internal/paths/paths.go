// Package paths is the single owner of mhgo worktree and container geometry.
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

	"github.com/Knatte18/mhgo/internal/git"
)

// ErrNotAGitRepo is returned when a directory is not within a git repository.
var ErrNotAGitRepo = errors.New("not a git repository")

// Layout represents the geometry of a worktree and container within a git repository.
//
// Fields:
//   - Cwd: the current working directory (normalized via filepath.Clean)
//   - WorktreeRoot: the root of the git repository (from git rev-parse --show-toplevel)
//   - Container: the parent directory of WorktreeRoot
//   - RelPath: the relative path from WorktreeRoot to Cwd
//   - MainWorktree: the path to the main (first) worktree from List()
type Layout struct {
	Cwd           string
	WorktreeRoot  string
	Container     string
	RelPath       string
	MainWorktree  string
}

// Getwd returns the current working directory.
//
// It wraps os.Getwd and is the ONLY permitted os.Getwd call outside cmd/mhgo/main.go.
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
//  5. Set Container = filepath.Dir(WorktreeRoot)
//  6. Set RelPath = filepath.Rel(WorktreeRoot, Cwd)
//  7. Call List(cwd) and set MainWorktree to the Main==true entry's Path
//
// Resolve does NOT check for _mhgo/ (that authority stays in internal/config).
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
	container := filepath.Dir(workTreeRoot)
	relPath, _ := filepath.Rel(workTreeRoot, cleanCwd)

	// Step 7: Get MainWorktree from List
	entries, err := List(cwd)
	if err != nil {
		return nil, fmt.Errorf("failed to get main worktree: %w", err)
	}

	mainWorktree := ""
	for _, entry := range entries {
		if entry.Main {
			// Normalize mainWorktree path (git may emit forward slashes)
			mainWorktree = filepath.FromSlash(entry.Path)
			mainWorktree = filepath.Clean(mainWorktree)
			break
		}
	}

	return &Layout{
		Cwd:          cleanCwd,
		WorktreeRoot: workTreeRoot,
		Container:    container,
		RelPath:      relPath,
		MainWorktree: mainWorktree,
	}, nil
}

// MhgoDir returns the path to the _mhgo directory in the current working directory.
//
// Returns filepath.Join(Cwd, "_mhgo").
func (l *Layout) MhgoDir() string {
	return filepath.Join(l.Cwd, "_mhgo")
}

// WorktreePath returns the path to a sibling worktree with the given slug.
//
// Returns filepath.Join(Container, slug).
func (l *Layout) WorktreePath(slug string) string {
	return filepath.Join(l.Container, slug)
}

// PortalsDir returns the path to the _portals directory in the container.
//
// Returns filepath.Join(Container, "_portals").
func (l *Layout) PortalsDir() string {
	return filepath.Join(l.Container, "_portals")
}

// PortalLink returns the path to the mirrored portal junction link for the given slug.
//
// The portal link is mirrored into the repo subpath structure. At RelPath == ".",
// this collapses to <Container>/_portals/<slug>. For subpaths, it includes the
// RelPath segments: <Container>/_portals/<RelPath>/<slug>.
//
// Returns filepath.Join(Container, "_portals", RelPath, slug).
func (l *Layout) PortalLink(slug string) string {
	return filepath.Join(l.Container, "_portals", l.RelPath, slug)
}

// PortalTarget returns the path to the _mhgo directory within a portal for the given slug.
//
// The path is: <Container>/<slug>/<RelPath>/_mhgo
//
// Returns filepath.Join(Container, slug, RelPath, "_mhgo").
func (l *Layout) PortalTarget(slug string) string {
	return filepath.Join(l.Container, slug, l.RelPath, "_mhgo")
}

// LaunchersDir returns the path to the _launchers directory in the container.
//
// This is the un-mirrored root used as a prune boundary and base for MkdirAll.
//
// Returns filepath.Join(Container, "_launchers").
func (l *Layout) LaunchersDir() string {
	return filepath.Join(l.Container, "_launchers")
}

// LauncherDir returns the path to the mirrored launcher directory for the given slug.
//
// The launcher directory is mirrored into the repo subpath structure. At RelPath == ".",
// this collapses to <Container>/_launchers/<slug>. For subpaths, it includes the
// RelPath segments: <Container>/_launchers/<RelPath>/<slug>.
//
// Returns filepath.Join(Container, "_launchers", RelPath, slug).
func (l *Layout) LauncherDir(slug string) string {
	return filepath.Join(l.Container, "_launchers", l.RelPath, slug)
}

// MenuLauncherPath returns the path to the per-subpath menu launcher script.
//
// The menu launcher is mirrored into the repo subpath structure. At RelPath == ".",
// this collapses to <Container>/_launchers/ide-menu.cmd. For subpaths, it includes
// the RelPath segments: <Container>/_launchers/<RelPath>/ide-menu.cmd.
//
// Returns filepath.Join(Container, "_launchers", RelPath, "ide-menu.cmd").
func (l *Layout) MenuLauncherPath() string {
	return filepath.Join(l.Container, "_launchers", l.RelPath, "ide-menu.cmd")
}

// HubName returns the base name of the main worktree.
//
// Returns filepath.Base(MainWorktree).
func (l *Layout) HubName() string {
	return filepath.Base(l.MainWorktree)
}

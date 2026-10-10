// hubgeom.go implements the hub-mode tellers that convert a resolved *lyxcwd.Location into each
// engine's own geometry struct: ReedGeometry, BurlerGeometry and ReconcileGeometry are its members.

package hubgeom

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// ReconcileGeometry builds the hubreconcile.Geometry for l: the hub's board dir and l's worktree root.
// It reports false, with the zero Geometry, when the hub has no board-level lyx dir, so a standalone repository never reconciles.
func ReconcileGeometry(l *lyxcwd.Location) (hubreconcile.Geometry, bool) {
	if !preflight.BoardLyxPresent(l) {
		return hubreconcile.Geometry{}, false
	}
	return hubreconcile.Geometry{BoardDir: fabricengine.BoardDir(l.HubPath), WorktreePath: l.WorktreePath()}, true
}

// ReedGeometry builds a reedengine.Geometry for l: the resolved Location's paths, read off its accessors and passed through untouched, plus the name prefix and parent reed forms strand names from.
// It performs no os.Getwd, no git discovery, and no path resolution of its own — internal/lyxcwd stays the sole owner of cwd resolution (the Cwd Resolution Invariant).
//
// NameShortname is the hub's recorded shortname; an absent record leaves it empty and is no error here,
// since reed refuses the spawn that needs a shortname while `reed status`, `down` and the watchdog keep working.
// The prime leaves NameSlug and ParentName empty;
// a task worktree sets NameSlug to its raw worktree name and ParentName to what ResolveParent returns, the orch name of the worktree the pair was created from.
// An unresolvable parent logs a warning and leaves ParentName empty, since the parent is an optional escalation channel.
// SpawnOrder is told as a lazy closure over l (spawnorder.go), so building the geometry reads and spawns nothing.
// Failing to tell the prime from a task worktree is the one error:
// telling a task worktree a prime's geometry would give its strands the prime's names for life.
func ReedGeometry(l *lyxcwd.Location) (reedengine.Geometry, error) {
	prime, err := isPrimeWorktree(l.WorktreePath())
	if err != nil {
		return reedengine.Geometry{}, err
	}
	return reedGeometry(l, prime), nil
}

// gitEntryName is the entry at every git worktree's root.
const gitEntryName = ".git"

// isPrimeWorktree reports whether the worktree at worktreeRoot is the hub's prime, the repository's main worktree.
// It reads the .git entry rather than spawning `git worktree list` as fabricengine.PrimeName does,
// so every hub wiring path stays spawn-free and the untagged wiring tests stay offline (the Test Tier Purity Invariant).
// The main worktree's .git is a directory, or a gitdir file pointing straight at its git directory;
// a linked worktree's .git is a gitdir file pointing into the common git directory's worktrees/ directory.
// A missing or unreadable entry is an error.
func isPrimeWorktree(worktreeRoot string) (bool, error) {
	entry := filepath.Join(worktreeRoot, gitEntryName)
	info, err := os.Stat(entry)
	if err != nil {
		return false, fmt.Errorf("hubgeom: cannot tell the prime from a task worktree: %w", err)
	}
	if info.IsDir() {
		return true, nil
	}
	data, err := os.ReadFile(entry)
	if err != nil {
		return false, fmt.Errorf("hubgeom: cannot tell the prime from a task worktree: %w", err)
	}
	gitDir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return false, fmt.Errorf("hubgeom: cannot tell the prime from a task worktree: %s is neither a directory nor a gitdir file", entry)
	}
	return filepath.Base(filepath.Dir(filepath.Clean(strings.TrimSpace(gitDir)))) != "worktrees", nil
}

// reedGeometry is ReedGeometry once the prime is told from a task worktree, split out so the unit test can tell either without a .git entry.
func reedGeometry(l *lyxcwd.Location, prime bool) reedengine.Geometry {
	shortname, _ := fabricengine.ReadShortname(fabricengine.BoardDir(l.HubPath))
	var slug, parent string
	if !prime {
		slug = l.WorktreeName
		parent = parentNameOrEmpty(l, "strands")
	}
	return reedengine.Geometry{
		SpawnOrder:         spawnOrder(l),
		SocketKey:          reedengine.ServerName(l.HubPath),
		SessionName:        reedengine.SessionName(l.WorktreePath()),
		AnchorPath:         l.AnchorPath(),
		PaneCwd:            l.AnchorPath(),
		WorktreeRoot:       l.WorktreePath(),
		LogsDir:            fabricengine.HubLogsDir(l.HubPath),
		WorktreeName:       l.WorktreeName,
		HubPath:            l.HubPath,
		NameShortname:      shortname,
		NameSlug:           slug,
		ParentName:         parent,
		DiscoverSignalPath: filepath.Join(fabricengine.HubScratchDir(l.HubPath), reedengine.DiscoverSignalFileName),
	}
}

// BurlerGeometry builds a burlerengine.Geometry for l: the resolved Location's paths, read off its
// accessors and passed through untouched.
// It performs no os.Getwd, no git discovery, and no path resolution of its own — internal/lyxcwd
// stays the sole owner of cwd resolution (the Cwd Resolution Invariant), and BurlerGeometry only
// reads what l's caller already resolved.
// WorktreeRoot is l.AnchorPath(), NOT l.WorktreePath(): a review segment's _lyx content is
// anchor-anchored, matching the commit seam that commits it, so telling burler the anchor path as
// its profile root is what keeps the two aligned. Converging or reverting the two would silently
// change behaviour in a subpath-anchored hub, where the anchor path and the worktree path diverge.
// standalonegeom.BurlerGeometry is the mode where WorktreeRoot and AnchorPath still legitimately
// diverge: standalone fills WorktreeRoot with the reviewed target directory, not the anchor path.
// RepoRoot is l.WorktreePath(), the directory holding PATTERN.md,
// which the anchor path is not in a subpath-anchored hub.
// ParentName is what ResolveParent returns; an unresolvable parent logs a warning and leaves it empty, as reedGeometry does.
func BurlerGeometry(l *lyxcwd.Location) burlerengine.Geometry {
	return burlerengine.Geometry{
		WorktreeRoot: l.AnchorPath(),
		AnchorPath:   l.AnchorPath(),
		RepoRoot:     l.WorktreePath(),
		ParentName:   parentNameOrEmpty(l, "burler prompts"),
	}
}

// hubgeom.go implements the hub-mode tellers that turn a resolved *lymcwd.Location into each engine's geometry struct.

package hubgeom

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// ReconcileGeometry builds the hubreconcile.Geometry for l: the hub's Bolt checkout and l's worktree root.
// It reports false, with the zero Geometry, when the hub has no board-level lym dir, so a standalone repository never reconciles.
func ReconcileGeometry(l *lymcwd.Location) (hubreconcile.Geometry, bool) {
	if !preflight.BoardLymPresent(l) {
		return hubreconcile.Geometry{}, false
	}
	return hubreconcile.Geometry{BoardDir: fabricengine.BoltDir(l.HubPath), WorktreePath: l.WorktreePath()}, true
}

// ReedGeometry builds a reedengine.Geometry for l, passing the resolved Location's paths through untouched.
// It also fills the shortname and parent name that strand names are formed from.
// It performs no os.Getwd, no git discovery and no path resolution of its own.
// The Cwd Resolution Invariant keeps internal/lymcwd the sole owner of cwd resolution.
//
// NameShortname is the hub's recorded shortname; an absent record leaves it empty and is no error here.
// Reed refuses only the spawn that needs a shortname, while `reed status`, `down` and the watchdog keep working.
// The prime leaves NameSlug and ParentName empty.
// A task worktree sets NameSlug to its raw worktree name.
// ParentName is the orch name of the worktree the pair was created from, as resolved.
// An unresolvable parent logs a warning and leaves ParentName empty, since the parent is an optional escalation channel.
// SpawnOrder is told as a lazy closure over l (spawnorder.go), so building the geometry reads and spawns nothing.
// PaneEnv is likewise a lazy closure:
// it loads the hub's env config module, with this machine's override, at each strand launch.
// Failing to tell the prime from a task worktree is the one error:
// telling a task worktree a prime's geometry would give its strands the prime's names for life.
func ReedGeometry(l *lymcwd.Location) (reedengine.Geometry, error) {
	prime, err := isPrimeWorktree(l.WorktreePath())
	if err != nil {
		return reedengine.Geometry{}, err
	}
	return reedGeometry(l, prime), nil
}

// gitEntryName is the entry at every git worktree's root.
const gitEntryName = ".git"

// isPrimeWorktree reports whether the worktree at worktreeRoot is the hub's prime, the repository's main worktree.
// It reads the .git entry rather than spawning `git worktree list`,
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

// reedGeometry builds the geometry of l once the caller has told whether l is the prime.
// It is split out so the unit test can pass either answer without a .git entry.
func reedGeometry(l *lymcwd.Location, prime bool) reedengine.Geometry {
	shortname, _ := fabricengine.ReadShortname(fabricengine.BoltDir(l.HubPath))
	var slug, parent string
	if !prime {
		slug = l.WorktreeName
		parent = parentNameOrEmpty(l, "strands")
	}
	return reedengine.Geometry{
		SpawnOrder:         spawnOrder(l),
		Buttons:            buttonContext(l, prime),
		PaneEnv:            paneEnv(l),
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
		PauseRecordPath:    pauseGeometry(l, prime).RecordPath,
		DiscoverSignalPath: filepath.Join(fabricengine.HubScratchDir(l.HubPath), reedengine.DiscoverSignalFileName),
	}
}

// paneEnv returns a closure that loads the variables of the hub's env config module for a strand pane in l's worktree.
// The module is loaded with this machine's override on each call.
func paneEnv(l *lymcwd.Location) func() (map[string]string, error) {
	return func() (map[string]string, error) {
		cfg, err := envconfig.LoadConfig(fabricengine.ConfigDirs(l))
		return cfg.Vars, err
	}
}

// BurlerGeometry builds a burlerengine.Geometry for l, passing the resolved Location's paths through untouched.
// It performs no os.Getwd, no git discovery and no path resolution of its own.
// The Cwd Resolution Invariant keeps internal/lymcwd the sole owner of cwd resolution.
// BurlerGeometry reads only what the caller already resolved.
// WorktreeRoot is l.AnchorPath(), not l.WorktreePath().
// A review segment's _lym content is anchor-anchored, matching the commit seam that commits it.
// Passing the anchor path as the profile root keeps the two aligned.
// Converging or reverting the two would silently change behaviour in a subpath-anchored hub.
// There the anchor path and the worktree path diverge.
// A standalone repository fills WorktreeRoot with the reviewed target directory, not the anchor path.
// RepoRoot is l.WorktreePath(), the directory holding PATTERN.md.
// The anchor path does not hold it in a subpath-anchored hub.
// ParentName is the resolved parent's name; an unresolvable parent logs a warning and leaves it empty.
// Shortname is the hub's recorded shortname, and Slug is the worktree name unless the worktree is the prime,
// so the names formed from them match the names reed gives the strands.
// Failing to tell the prime from a task worktree is the one error.
func BurlerGeometry(l *lymcwd.Location) (burlerengine.Geometry, error) {
	prime, err := isPrimeWorktree(l.WorktreePath())
	if err != nil {
		return burlerengine.Geometry{}, err
	}
	return burlerGeometry(l, prime), nil
}

// burlerGeometry builds the geometry of l once the caller has told whether l is the prime.
// It is split out so the unit test can pass either answer without a .git entry.
func burlerGeometry(l *lymcwd.Location, prime bool) burlerengine.Geometry {
	shortname, _ := fabricengine.ReadShortname(fabricengine.BoltDir(l.HubPath))
	var slug string
	if !prime {
		slug = l.WorktreeName
	}
	return burlerengine.Geometry{
		WorktreeRoot: l.AnchorPath(),
		AnchorPath:   l.AnchorPath(),
		RepoRoot:     l.WorktreePath(),
		ParentName:   parentNameOrEmpty(l, "burler prompts"),
		Shortname:    shortname,
		Slug:         slug,
		SpecsDir:     fabricengine.SpecsDir(l.HubPath),
	}
}

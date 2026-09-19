// paths.go declares the five prime-anchored lifecycle path constructors: BattenDir, StatusFile,
// RunLock, StatusLock, and PrimeRunLock. Every one of them is a plain filepath.Join onto the given
// *lyxcwd.Location's AnchorPath(), per the Cwd Resolution Invariant -- none of them calls os.Getwd
// or any git command.
//
// This whole tree is ephemeral, not durable, unlike loom's own status file (shedrun.StatusFile
// lives under _lyx and is fabric-synced): the lifecycle's state -- which task worktree is mid-create,
// which lock is held, what a run last observed -- is per-machine and per-attempt, never meant to be
// committed or shared between machines working the same hub. Per the Durable-vs-Ephemeral State
// Invariant, every never-tracked file lives under .lyx, so this package's whole tree sits there
// rather than under _lyx.

package battencli

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// battenDirName is the relative-path segment battencli joins onto lyxdirs.DotLyxDirName to
// scope every lifecycle-owned path under its own subdirectory.
// battencli is this segment's sole declarer.
const battenDirName = "lifecycle"

// BattenDir returns the path to the per-slug lifecycle directory: the prime *lyxcwd.Location's
// AnchorPath() joined with lyxdirs.DotLyxDirName, battenDirName, and slug.
// The .lyx segment comes from lyxdirs.DotLyxDirName rather than a literal, per the Lyxdirs
// Single-Declarer Invariant, exactly as shedrun.StatusLock already does.
func BattenDir(l *lyxcwd.Location, slug string) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, battenDirName, slug)
}

// StatusFile returns the path to a slug's persisted lifecycle status.json, under BattenDir(l,
// slug).
func StatusFile(l *lyxcwd.Location, slug string) string {
	return filepath.Join(BattenDir(l, slug), "status.json")
}

// RunLock returns the path to a slug's lifecycle run lock, under BattenDir(l, slug).
// It must never equal StatusLock(l, slug): shedengine.Shed's own validation rejects LockPath ==
// StatusLockPath outright, and a shared file would hang on the first persist rather than fail.
func RunLock(l *lyxcwd.Location, slug string) string {
	return filepath.Join(BattenDir(l, slug), "run.lock")
}

// StatusLock returns the path to the advisory lock guarding concurrent access to StatusFile(l,
// slug), under BattenDir(l, slug).
func StatusLock(l *lyxcwd.Location, slug string) string {
	return filepath.Join(BattenDir(l, slug), "status.json.lock")
}

// PrimeRunLock returns the path to the hub-scoped advisory lock that serialises every slug's
// WorktreeCreate and WorktreeTeardown rows against one another: the prime *lyxcwd.Location's
// AnchorPath() joined with lyxdirs.DotLyxDirName and battenDirName, one level above any single
// slug's own BattenDir.
// Unlike the four accessors above, PrimeRunLock takes no slug: the lock it names is shared across
// every task worktree this hub creates or tears down, not scoped to one.
func PrimeRunLock(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, battenDirName, "run.lock")
}

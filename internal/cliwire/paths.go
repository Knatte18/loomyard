// paths.go holds cliwire's pure path helpers.
// They produce no operator-facing message, so they need no Module descriptor to drive them.

package cliwire

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/standalonestate"
)

// gitDirName is the per-repository git administrative entry.
// It is a directory in an ordinary clone and a file in a linked worktree.
// It is a plain literal rather than an internal/lymdirs constant.
// lymdirs declares only loomyard's own "_lym" and ".lym" directories, never git's.
const gitDirName = ".git"

// ResolveToldDir makes one told directory flag absolute against cwd.
// An empty flagValue stays empty, so each mode can apply its own default.
// An absolute value is cleaned, and a relative one is joined onto cwd.
//
// Every path handed downstream must be absolute.
// A relative flag value does not fail.
// It silently names two directories.
// The CLI process resolves it against its own working directory, while the process it spawns runs in another.
// The default-versus-override check is a path equality that a relative spelling can never satisfy.
// Resolve once, at the wiring boundary, because that is the last point that knows which working
// directory the operator typed the flag from.
//
// PRECONDITION, enforced by convention rather than asserted:
// cwd must be non-empty and absolute whenever flagValue is a non-empty relative value.
// filepath.Join drops an empty element silently, so an empty cwd would return flagValue unresolved.
// Production callers take cwd from an already-resolved working directory, so the precondition holds.
func ResolveToldDir(cwd, flagValue string) string {
	if flagValue == "" {
		return ""
	}
	if filepath.IsAbs(flagValue) {
		return filepath.Clean(flagValue)
	}
	return filepath.Join(cwd, flagValue)
}

// pathContains reports whether inner is outer itself or a descendant of it.
// It uses filepath.Rel and a ".." prefix test.
// Both paths must already be absolute and cleaned.
func pathContains(outer, inner string) bool {
	// filepath.Rel is case-sensitive, but Windows paths are not.
	// Paths that differ only in case name one directory on Windows.
	// On Windows alone both paths are folded to lower case first.
	// The project's hosts are Linux, so this branch is never run live and is unverified.
	if runtime.GOOS == "windows" {
		outer, inner = strings.ToLower(outer), strings.ToLower(inner)
	}
	rel, err := filepath.Rel(outer, inner)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// NormalizeForContainment returns the spelling of path that a containment or equality test must use.
// It applies standalonestate.Normalize to the deepest existing ancestor of path, then rejoins the missing remainder.
//
// Plain Normalize falls back to Clean whenever filepath.EvalSymlinks fails.
// EvalSymlinks fails when any component is missing, which a not-yet-created state directory routinely is.
// Comparing an unresolved path with a resolved one can report disjoint.
// That is wrong for a path that reaches inside the other through a symlink.
func NormalizeForContainment(path string) string {
	existing := filepath.Clean(path)
	var missing []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return filepath.Clean(path)
		}
		missing = append([]string{filepath.Base(existing)}, missing...)
		existing = parent
	}
	return filepath.Join(append([]string{standalonestate.Normalize(existing)}, missing...)...)
}

// RepositoryRootOf returns the nearest repository root at or above dir.
// That is the closest ancestor, dir itself included, that carries a ".git" entry.
// It returns dir unchanged when no ancestor carries one.
//
// Nearest, never topmost: a submodule and a nested repository are each their own repository.
// Climbing past the first ".git" would re-target a standalone run at the superproject that contains it.
//
// It walks the filesystem rather than asking git.
// That keeps wiring free of process spawns.
// It lets the wiring truth table run from untagged tests under the Test Tier Purity Invariant.
// Turning a working directory into a Location belongs to internal/lymcwd, per the Cwd Resolution Invariant.
// This function only lifts an already-resolved path to the root of the tree it lives in.
//
// It uses os.Lstat, not os.Stat, and no directory-or-file test.
// A linked worktree records ".git" as a file, and a repository reached through a symlink is still a repository.
//
// PRECONDITION, enforced by convention rather than asserted: dir must be non-empty and absolute.
// filepath.Join drops an empty element silently, so an empty dir would search from the process's own working directory.
// Production callers derive dir from a working directory that lymcwd has already resolved.
//
// The walk honors GIT_CEILING_DIRECTORIES as git does.
// It never climbs into a listed directory, and a walk stopped there returns dir unchanged.
// dir itself is checked even when it is listed.
func RepositoryRootOf(dir string) string {
	return repositoryRootBelowCeilings(dir, gitCeilingDirectories(os.Getenv("GIT_CEILING_DIRECTORIES")))
}

// repositoryRootBelowCeilings returns the nearest repository root at or above dir, without climbing into a ceiling.
// When the walk would climb into a directory in ceilings, it stops and returns dir unchanged.
func repositoryRootBelowCeilings(dir string, ceilings []string) string {
	for candidate := dir; ; {
		if _, err := os.Lstat(filepath.Join(candidate, gitDirName)); err == nil {
			return candidate
		}
		parent := filepath.Dir(candidate)
		if parent == candidate || slices.Contains(ceilings, parent) {
			return dir
		}
		candidate = parent
	}
}

// gitCeilingDirectories parses a GIT_CEILING_DIRECTORIES value into the absolute directories it lists.
// Each is normalized as the walk's own input is, so a symlinked spelling still matches.
// Relative and empty entries are dropped, since git ignores them too.
func gitCeilingDirectories(value string) []string {
	var ceilings []string
	for _, entry := range filepath.SplitList(value) {
		if filepath.IsAbs(entry) {
			ceilings = append(ceilings, standalonestate.Normalize(entry))
		}
	}
	return ceilings
}

// SamePlanDir reports whether a told --plan-dir names the same directory as the mode's own default.
// Both arguments are absolute, so this is a path equality.
// A "." or trailing-separator spelling of the default counts as the default, not as an override.
//
// It compares through NormalizeForContainment rather than filepath.Clean alone.
// A plain Clean equality reports an override for a directory named through a symlinked state home or HOME.
func SamePlanDir(planDir, defaultPlanDir string) bool {
	return NormalizeForContainment(planDir) == NormalizeForContainment(defaultPlanDir)
}

// ResolvePlanDir resolves a told --plan-dir against defaultPlanDir, the mode's own default.
// An empty toldPlanDir returns (defaultPlanDir, false).
// A non-empty one returns itself as the resolved directory.
// overridden is true only when it names a directory other than the default.
//
// It is a package function, not a Module method.
// It cannot fail, produces no operator-facing message, and reads no descriptor field.
// A told value naming the default through a ".", a trailing separator, or a symlink is not an override.
func ResolvePlanDir(toldPlanDir, defaultPlanDir string) (planDir string, overridden bool) {
	if toldPlanDir == "" {
		return defaultPlanDir, false
	}
	return toldPlanDir, !SamePlanDir(toldPlanDir, defaultPlanDir)
}

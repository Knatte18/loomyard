// dirtiness.go holds the package's `git status --porcelain` probes: dirtiness checks and tracked dirty-path listings.
// worktreeDirty is the one primitive for the dirtiness check,
// so every call site shares its error wording and its spawn-failure path.
// Each call site declares its scope (dirtyScope), so a reader sees which files that site would lose.
// Worktree enumeration lives in worktreelist.go, not here.
//
// The probe stays package-private rather than moving to internal/gitrepo:
// most call sites are topology verbs with no Repo handle, and a gitrepo method would force one per probe.
package fabricengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// dirtyScope selects which files worktreeDirty considers when deciding whether a worktree is dirty.
type dirtyScope int

const (
	// scopeTracked considers only tracked files, `git status --porcelain --untracked-files=no`.
	// Untracked files are ignored.
	// Use it where the destructive action to be gated leaves untracked files alone.
	scopeTracked dirtyScope = iota
	// scopeAll considers tracked and untracked files alike, `git status --porcelain`.
	// Use it where the destructive action would take untracked files with it.
	scopeAll
)

// worktreeDirty runs `git status --porcelain` in dir, scoped by the caller's declared choice.
// It reports whether the worktree is dirty.
//
// A spawn failure and a nonzero exit both return one error.
// The error names what the probe was for and where,
// and it wraps the gitexec error, which already renders the command, the exit code, and stderr.
// Repeating the command here would bury git's stderr behind a duplicate.
//
// detail is the trimmed porcelain listing, returned even when err is nil.
// Callers that report the listing use it; the rest ignore it.
//
// pathspec, when given, limits the probe to the paths it matches, relative to dir.
func worktreeDirty(scope dirtyScope, dir string, pathspec ...string) (dirty bool, detail string, err error) {
	args := []string{"status", "--porcelain"}
	if scope == scopeTracked {
		args = append(args, "--untracked-files=no")
	}
	if len(pathspec) > 0 {
		args = append(append(args, "--"), pathspec...)
	}

	stdout, runErr := gitexec.Run(args, dir)
	if runErr != nil {
		return false, "", fmt.Errorf("check for uncommitted changes in %s: %w", dir, runErr)
	}

	trimmed := strings.TrimSpace(stdout)
	return trimmed != "", trimmed, nil
}

// trackedDirtyPaths returns the tracked uncommitted paths of the checkout at dir, as absolute paths under dir.
// It uses the tracked-only scope in the NUL-separated form, which does not quote paths,
// so a path with a space or a non-ASCII character comes back verbatim.
// A rename or copy entry names its destination and is followed by a record holding its source, which is skipped.
func trackedDirtyPaths(dir string) ([]string, error) {
	stdout, err := gitexec.Run([]string{"status", "--porcelain", "-z", "--untracked-files=no"}, dir)
	if err != nil {
		return nil, fmt.Errorf("check for uncommitted changes in %s: %w", dir, err)
	}
	var paths []string
	records := strings.Split(stdout, "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 4 {
			continue
		}
		paths = append(paths, filepath.Join(dir, filepath.FromSlash(record[3:])))
		if strings.ContainsAny(record[:2], "RC") {
			i++
		}
	}
	return paths, nil
}

// dirtiness.go holds the package's `git status --porcelain` probes: the dirtiness check and the tracked dirty-path listing.
//
// Every call site in this package used to hand-roll its own `git status --porcelain` invocation —
// eight of them, four passing `--untracked-files=no` and four not — with each site free to reinvent
// the scope, the error wording, and whether a spawn failure and a nonzero exit were distinguished.
// worktreeDirty replaces all eight: scope is the caller's declared choice (dirtyScope), not a
// property baked into the primitive, so a reader sees at each call site exactly which files that
// site cares about losing.
//
// git worktree list --porcelain in worktreelist.go is a different git subcommand entirely — it
// enumerates worktrees, not file status — and is outside this file's remit.
//
// This probe is deliberately NOT promoted into internal/gitrepo. The
// dirtiness-probe-stays-fabric-local decision in _mill/discussion.md records why: six of the eight
// call sites are Topology verbs, and Topology holds only a Config, no Repo handle at all — a
// gitrepo method would force constructing a Repo per probe solely to answer one question, at six
// sites that have none. Only pull.go's warpWorktreeDirty has a Repo handle. The probe stays here,
// package-private, with every consumer in-package.
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
	// scopeTracked considers only tracked files (`git status --porcelain --untracked-files=no`).
	// Untracked files are ignored, which is the right scope wherever the destructive action to be
	// gated (a reset, a forced worktree removal) leaves untracked files alone.
	scopeTracked dirtyScope = iota
	// scopeAll considers tracked and untracked files alike (`git status --porcelain`). This is the
	// right scope wherever the destructive action would take untracked files down with it.
	scopeAll
)

// worktreeDirty runs `git status --porcelain` in dir, scoped per the caller's declared choice, and
// reports whether the worktree is dirty.
//
// A spawn failure and a nonzero exit are both reported as a single consolidated error, carrying the
// trimmed stderr and the exit code in the message; callers that used to distinguish the two forms
// collapse onto the spawn-failure wording, with the exit-code detail arriving inside the wrapped
// error rather than as a second branch.
// The wrapper names what this probe was for and where, never the git command itself: since the
// gitexec split, *gitexec.GitError already renders `git status --porcelain …: exit <code>: <stderr>`,
// so repeating the command here would push git's own actionable stderr behind a duplicate of what
// the reader has already been told.
//
// detail is the trimmed stdout — the porcelain listing itself — returned even when err is nil, for
// the one caller (warpclean.go's dirtyReason) that surfaces it to its own caller; every other site
// ignores it.
func worktreeDirty(scope dirtyScope, dir string) (dirty bool, detail string, err error) {
	args := []string{"status", "--porcelain"}
	if scope == scopeTracked {
		args = append(args, "--untracked-files=no")
	}

	stdout, runErr := gitexec.Run(args, dir)
	if runErr != nil {
		return false, "", fmt.Errorf("check for uncommitted changes in %s: %w", dir, runErr)
	}

	trimmed := strings.TrimSpace(stdout)
	return trimmed != "", trimmed, nil
}

// trackedDirtyPaths returns the tracked uncommitted paths of the checkout at dir, as absolute paths under dir.
// It reads the same scope worktreeDirty(scopeTracked, dir) does, in the NUL-separated form that quotes nothing, so a path with a space or a non-ASCII character comes back verbatim.
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

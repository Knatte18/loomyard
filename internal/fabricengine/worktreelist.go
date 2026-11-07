// worktreelist.go lists a repository's worktrees for fabric's worktree lookups:
// in-process through dotgit, or from `git worktree list --porcelain` for a repository whose refs are not files.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// detachedBranch is the branch name recorded for a worktree whose HEAD is detached.
const detachedBranch = "(detached)"

// WorktreeEntry represents a single worktree of a repository, as `git worktree list` reports it.
type WorktreeEntry struct {
	Path     string `json:"path"`
	Head     string `json:"head"`
	Branch   string `json:"branch"`
	Main     bool   `json:"main"`
	Prunable bool   `json:"prunable"`
}

// List returns every worktree of the repository holding sourceDir, an absolute path anywhere inside a worktree.
// The main worktree comes first with Main=true; all others have Main=false.
// It reads the repository's files and starts no git process.
// A repository whose refs are not files is the exception: it lists through `git worktree list --porcelain`.
// A sourceDir that does not exist, a bare repository and a directory in no repository are errors.
func List(sourceDir string) ([]WorktreeEntry, error) {
	worktrees, err := dotgit.ListWorktrees(sourceDir)
	if errors.Is(err, dotgit.ErrRefStorage) {
		return listFromPorcelain(sourceDir)
	}
	if err != nil {
		return nil, fmt.Errorf("list git worktrees in %q: %w", sourceDir, err)
	}

	entries := make([]WorktreeEntry, len(worktrees))
	for i, worktree := range worktrees {
		branch := detachedBranch
		if worktree.Branch != "" {
			branch = strings.TrimPrefix(worktree.Branch, "refs/heads/")
		}
		entries[i] = WorktreeEntry{
			Path:     worktree.Path,
			Head:     worktree.Head,
			Branch:   branch,
			Main:     i == 0,
			Prunable: worktree.Prunable,
		}
	}
	return entries, nil
}

// listFromPorcelain lists the worktrees of the repository holding sourceDir through `git worktree list --porcelain`.
func listFromPorcelain(sourceDir string) ([]WorktreeEntry, error) {
	stdout, err := gitexec.Run([]string{"worktree", "list", "--porcelain"}, sourceDir)
	if err != nil {
		return nil, fmt.Errorf("list git worktrees in %q: %w", sourceDir, err)
	}
	return parseWorktreePorcelain(stdout)
}

// parseWorktreePorcelain parses the porcelain output from `git worktree list --porcelain`.
// The first block gets Main=true; all others get Main=false.
func parseWorktreePorcelain(out string) ([]WorktreeEntry, error) {
	blocks := strings.Split(out, "\n\n")
	var entries []WorktreeEntry
	firstBlock := true

	for _, block := range blocks {
		// Skip empty blocks (trailing blank lines produce an empty final block)
		if strings.TrimSpace(block) == "" {
			continue
		}

		lines := strings.Split(block, "\n")
		entry := WorktreeEntry{
			Main: firstBlock, // FIRST non-empty block is main
		}
		firstBlock = false

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			if strings.HasPrefix(line, "worktree ") {
				entry.Path = strings.TrimPrefix(line, "worktree ")
			} else if strings.HasPrefix(line, "HEAD ") {
				entry.Head = strings.TrimPrefix(line, "HEAD ")
			} else if strings.HasPrefix(line, "branch ") {
				branchRef := strings.TrimPrefix(line, "branch ")
				entry.Branch = strings.TrimPrefix(branchRef, "refs/heads/")
			} else if line == "detached" {
				entry.Branch = detachedBranch
			} else if strings.HasPrefix(line, "prunable") {
				// Real porcelain output emits "prunable <reason>" (a reason string follows the keyword), never a bare "prunable" line,
				// so an exact-equality match would silently never fire.
				entry.Prunable = true
			} else if line == "bare" {
				return nil, fmt.Errorf("bare repositories are not supported")
			}
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// matchParentBranch returns the path of the first live (non-Prunable) entry whose Branch equals parentBranch.
// The path is normalized via filepath.FromSlash.
// A Prunable entry is skipped before its branch is checked, so it never matches.
// A match on the caller's own branch is an ordinary match: refusing it is the caller's policy.
// Main plays no role in matching.
// Returns ("", false) when no live entry matches.
func matchParentBranch(entries []WorktreeEntry, parentBranch string) (path string, ok bool) {
	for _, entry := range entries {
		if entry.Prunable {
			continue
		}
		if entry.Branch == parentBranch {
			return filepath.FromSlash(entry.Path), true
		}
	}
	return "", false
}

// OpenParent opens the Fabric paired with parentBranch.
// It lists l's hub worktrees, finds the live one on parentBranch, resolves its path as a worktree root, and opens it.
// It performs no wiring and creates nothing.
// It returns a plain (unwrapped) error naming parentBranch when no live entry matches,
// so a caller can tell that case apart without unwrapping.
// Every other failure is wrapped with parentBranch, and with the matched path once there is one,
// so no error from the open or resolve steps escapes unwrapped.
func OpenParent(l *lymcwd.Location, parentBranch string) (*Fabric, error) {
	entries, err := List(l.AnchorPath())
	if err != nil {
		return nil, fmt.Errorf("fabricengine: open parent fabric for branch %q: %w", parentBranch, err)
	}

	matchPath, ok := matchParentBranch(entries, parentBranch)
	if !ok {
		return nil, fmt.Errorf("fabricengine: no live pair for parent branch %q", parentBranch)
	}

	// The matched path is a worktree root, not an acting cwd,
	// so it goes through ResolveWorktree: Resolve's strict cwd gate would reject it.
	parentLoc, err := lymcwd.ResolveWorktree(matchPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: open parent fabric for branch %q at %q: %w", parentBranch, matchPath, err)
	}

	f, err := Open(parentLoc)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: open parent fabric for branch %q at %q: %w", parentBranch, matchPath, err)
	}
	return f, nil
}

// PrimeName returns the base name of l's main worktree, the Main entry List returns.
// It reads the worktree list on every call, so a caller resolves the name when it needs it.
func PrimeName(l *lymcwd.Location) (string, error) {
	entries, err := List(l.AnchorPath())
	if err != nil {
		return "", fmt.Errorf("resolve main worktree: %w", err)
	}
	for _, entry := range entries {
		if entry.Main {
			// Normalize the porcelain path (git may emit forward slashes) before
			// taking its base name.
			prime := filepath.FromSlash(entry.Path)
			return filepath.Base(filepath.Clean(prime)), nil
		}
	}
	return "", fmt.Errorf("no main worktree found in %q", l.AnchorPath())
}

// IsPrimeWorktree reports whether l is the hub's prime worktree.
// A failure to find the main worktree is returned unchanged.
func IsPrimeWorktree(l *lymcwd.Location) (bool, error) {
	prime, err := PrimeName(l)
	if err != nil {
		return false, err
	}
	return prime == l.WorktreeName, nil
}

// RecordsRepoRoot returns the path of the weft prime worktree.
// It is the directory `git -C` targets for weft worktree add and remove.
func RecordsRepoRoot(l *lymcwd.Location) (string, error) {
	primeName, err := PrimeName(l)
	if err != nil {
		return "", err
	}
	return weftname.SiblingPath(l.HubPath, primeName), nil
}

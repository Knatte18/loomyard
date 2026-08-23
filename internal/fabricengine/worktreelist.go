// worktreelist.go parses `git worktree list --porcelain` into structured entries;
// it is the single porcelain parser shared across the codebase.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// WorktreeEntry represents a single git worktree in the output of `git worktree list`.
type WorktreeEntry struct {
	Path     string `json:"path"`
	Head     string `json:"head"`
	Branch   string `json:"branch"`
	Main     bool   `json:"main"`
	Prunable bool   `json:"prunable"`
}

// List returns a list of all git worktrees in the repository.
// The FIRST block in the porcelain output is marked as Main=true;
// all others have Main=false.
func List(sourceDir string) ([]WorktreeEntry, error) {
	stdout, err := gitexec.Run([]string{"worktree", "list", "--porcelain"}, sourceDir)
	if err != nil {
		return nil, fmt.Errorf("list git worktrees in %q: %w", sourceDir, err)
	}

	return parseWorktreePorcelain(stdout)
}

// parseWorktreePorcelain parses the porcelain output from `git worktree list --porcelain`.
// The FIRST block gets Main=true; all others get Main=false.
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
				entry.Branch = "(detached)"
			} else if strings.HasPrefix(line, "prunable") {
				// Real porcelain output emits "prunable <reason>" (a reason string
				// follows the keyword), never a bare "prunable" line, so an
				// exact-equality match would silently never fire.
				entry.Prunable = true
			} else if line == "bare" {
				return nil, fmt.Errorf("bare repositories are not supported")
			}
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// matchParentBranch returns the path of the first live (non-Prunable) entry in entries whose Branch
// equals parentBranch, with the path normalized via filepath.FromSlash (mirroring PrimeName's own
// normalization above).
// A Prunable entry is skipped before its branch is even checked — a prunable entry is never a match,
// per the prunable-entries-are-skipped-and-resolve-failures-name-the-branch design decision.
// It special-cases nothing else: a match whose Branch equals the caller's own acting worktree branch
// is an ordinary match, not an error (that refusal is loom policy, not fabric policy, and belongs to
// a future internal/loomcli caller, never here), and Main plays no role in matching.
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

// OpenParent resolves and opens the Fabric handle paired with parentBranch, the parent-fabric
// resolution chain: it lists l's hub worktrees, matches parentBranch against them via
// matchParentBranch, resolves the match through lyxcwd.ResolveWorktree, and opens it via Open.
// It performs no wiring and creates nothing — it is a pure read-and-open chain, exactly like the
// existing Open(l).
// It returns a plain (unwrapped) error naming parentBranch when no live entry matches, so a caller
// can turn "no live pair" into its own Stuck classification without unwrapping anything.
// Every other failure is wrapped with the branch (and, once matched, the resolved path) so neither
// Open's bare *ErrMissingPath nor ResolveWorktree's bare lyxcwd.ErrNotAGitRepo ever escapes unwrapped.
func OpenParent(l *lyxcwd.Location, parentBranch string) (*Fabric, error) {
	entries, err := List(l.AnchorPath())
	if err != nil {
		return nil, fmt.Errorf("fabricengine: open parent fabric for branch %q: %w", parentBranch, err)
	}

	matchPath, ok := matchParentBranch(entries, parentBranch)
	if !ok {
		return nil, fmt.Errorf("fabricengine: no live pair for parent branch %q", parentBranch)
	}

	// The matched path is a worktree root, not an acting cwd, so ResolveWorktree is used rather
	// than Resolve — Resolve's strict cwd gate would spuriously reject it here.
	parentLoc, err := lyxcwd.ResolveWorktree(matchPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: open parent fabric for branch %q at %q: %w", parentBranch, matchPath, err)
	}

	f, err := Open(parentLoc)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: open parent fabric for branch %q at %q: %w", parentBranch, matchPath, err)
	}
	return f, nil
}

// PrimeName resolves the base name of l's main worktree by scanning `git worktree list --porcelain`
// for the FIRST (Main) entry.
// It replaces lyxcwd's former per-Resolve prime scan: lyxcwd no longer performs this
// subprocess-backed lookup at all (see the Cwd Resolution Invariant), so every caller needing the
// prime's name now resolves it here, on demand.
func PrimeName(l *lyxcwd.Location) (string, error) {
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

// WeftRepoRoot returns the path to the weft prime worktree (the git -C target for weft worktree
// add/remove), resolved via PrimeName.
func WeftRepoRoot(l *lyxcwd.Location) (string, error) {
	primeName, err := PrimeName(l)
	if err != nil {
		return "", err
	}
	return weftname.SiblingPath(l.HubPath, primeName), nil
}

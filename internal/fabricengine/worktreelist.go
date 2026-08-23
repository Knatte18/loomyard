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

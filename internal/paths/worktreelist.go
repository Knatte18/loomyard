package paths

import (
	"fmt"
	"strings"

	"github.com/Knatte18/mhgo/internal/git"
)

// WorktreeEntry represents a single git worktree in the output of `git worktree list`.
type WorktreeEntry struct {
	Path   string `json:"path"`
	Head   string `json:"head"`
	Branch string `json:"branch"`
	Main   bool   `json:"main"`
}

// List returns a list of all git worktrees in the repository.
//
// The sourceDir is any worktree in the repository (usually the main checkout).
// Runs `git worktree list --porcelain` and parses the output. The FIRST block
// in the porcelain output is marked as Main=true; all subsequent blocks have Main=false.
//
// The porcelain format contains blocks separated by blank lines, each with:
//   - "worktree <path>" (the worktree path)
//   - "HEAD <sha>" (the current HEAD SHA)
//   - "branch refs/heads/<name>" (the branch, or "detached" for a detached HEAD)
//   - "bare" (only in bare repositories, which are rejected)
//
// Returns WorktreeEntry slice or an error if parsing or git execution fails.
func List(sourceDir string) ([]WorktreeEntry, error) {
	stdout, stderr, exitCode, err := git.RunGit([]string{"worktree", "list", "--porcelain"}, sourceDir)
	if err != nil {
		return nil, err
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("git worktree list failed: %s", stderr)
	}

	return parseWorktreePorcelain(stdout)
}

// parseWorktreePorcelain parses the porcelain output from `git worktree list --porcelain`.
//
// Format: blocks separated by blank lines, each containing:
//   - "worktree <path>"
//   - "HEAD <sha>"
//   - "branch refs/heads/<name>" or "detached"
//   - optionally "bare" (rejected as an error)
//
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
			} else if line == "bare" {
				return nil, fmt.Errorf("bare repositories are not supported")
			}
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

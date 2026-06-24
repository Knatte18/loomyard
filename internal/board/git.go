// git.go implements git plumbing for the board: fast-forward pull and push-with-rebase-retry.
// Pull and CommitPush wrap the git command-line interface with resilience logic.

package board

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/git"
)

// BoardPushError represents a fatal git push error
type BoardPushError string

func (e BoardPushError) Error() string {
	return string(e)
}

// Pull runs git pull --ff-only and returns whether the repo was updated
func Pull(boardPath string) (updated bool, err error) {
	stdout, stderr, exitCode, err := git.RunGit([]string{"pull", "--ff-only"}, boardPath)
	if err != nil {
		return false, fmt.Errorf("pull: %w", err)
	}
	if exitCode != 0 {
		return false, BoardPushError(fmt.Sprintf("pull failed: %s", stderr))
	}
	updated = !strings.Contains(stdout, "Already up to date.")
	return updated, nil
}

// CommitPush stages, commits, and pushes changes with rebase retry logic.
// skipPush, when true, commits locally but skips the push.
func CommitPush(boardPath string, relPaths []string, message string, skipPush bool) error {
	// Stage files
	args := append([]string{"add", "--"}, relPaths...)
	_, _, exitCode, err := git.RunGit(args, boardPath)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	if exitCode != 0 {
		return BoardPushError("add failed")
	}

	// Check for staged changes
	_, _, exitCode, err = git.RunGit([]string{"diff", "--cached", "--quiet"}, boardPath)
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}
	if exitCode == 0 {
		// No staged changes - idempotent
		return nil
	}
	if exitCode != 1 {
		return BoardPushError("diff check failed")
	}

	// Commit
	_, _, exitCode, err = git.RunGit([]string{"commit", "-m", message}, boardPath)
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	if exitCode != 0 {
		return BoardPushError("commit failed")
	}

	// Skip push if requested
	if skipPush {
		return nil
	}

	// Push with rebase retry
	for attempt := 0; attempt < 2; attempt++ {
		_, stderr, exitCode, err := git.RunGit([]string{"push"}, boardPath)
		if err != nil {
			return fmt.Errorf("push: %w", err)
		}
		if exitCode == 0 {
			return nil
		}

		// Check for non-fast-forward error
		if strings.Contains(stderr, "non-fast-forward") || strings.Contains(stderr, "rejected") {
			// Try rebase
			_, _, exitCode, err := git.RunGit([]string{"pull", "--rebase"}, boardPath)
			if err != nil {
				return fmt.Errorf("rebase: %w", err)
			}
			if exitCode != 0 {
				// Abort rebase on failure
				git.RunGit([]string{"rebase", "--abort"}, boardPath)
				return BoardPushError("rebase failed")
			}
			// Continue to next push attempt
			continue
		}

		// Other push error
		return BoardPushError(fmt.Sprintf("push failed: %s", stderr))
	}

	return BoardPushError("push still failing after rebase retry")
}

// git.go — the filesystem + git plumbing under the store.
//
// PathGuard rejects unsafe relative paths; AtomicWrite writes via temp-file +
// rename; Pull and CommitPush wrap git with fast-forward pull and push-with-
// rebase-retry. The low-level disk and remote layer.

package board

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BoardPushError represents a fatal git push error
type BoardPushError string

func (e BoardPushError) Error() string {
	return string(e)
}

// BoardPathError represents an invalid board path
type BoardPathError string

func (e BoardPathError) Error() string {
	return string(e)
}

// PathGuard validates a relative path for board operations
func PathGuard(relPath string) error {
	if relPath == "" {
		return BoardPathError("empty path")
	}

	// Check for absolute paths (both Windows and Unix styles)
	if filepath.IsAbs(relPath) || (len(relPath) > 0 && relPath[0] == '/') {
		return BoardPathError("absolute path not allowed")
	}

	// Check for Windows-style absolute paths on non-Windows systems
	if len(relPath) > 1 && relPath[1] == ':' {
		return BoardPathError("absolute path not allowed")
	}

	// Split by both separators to preserve ".." for validation (before cleaning would remove it)
	parts := strings.FieldsFunc(relPath, func(r rune) bool {
		return r == '\\' || r == '/'
	})
	for _, c := range parts {
		if c == ".." {
			return BoardPathError("parent directory reference not allowed")
		}
	}

	return nil
}

// AtomicWrite writes content to a file atomically using a temp file
func AtomicWrite(wikiPath, relPath, content string) error {
	if err := PathGuard(relPath); err != nil {
		return err
	}

	fullPath := filepath.Join(wikiPath, relPath)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // cleanup on error

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	// The rename is the atomic swap. Concurrent readers are excluded from this
	// instant by the swap lock (see store.Save / store.Load), so on Windows the
	// rename never loses a sharing-violation race against an open reader.
	if err := os.Rename(tmpPath, fullPath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// RunGit runs a git command and returns stdout, stderr, and exit code
func RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	hideProcWindow(cmd) // no console window flash on Windows

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	exitCode = cmd.ProcessState.ExitCode()

	// Only return err for execution failures, not for non-zero exit codes
	if err != nil && cmd.ProcessState == nil {
		return outBuf.String(), errBuf.String(), exitCode, err
	}
	return outBuf.String(), errBuf.String(), exitCode, nil
}

// Pull runs git pull --ff-only and returns whether the repo was updated
func Pull(wikiPath string) (updated bool, err error) {
	stdout, stderr, exitCode, err := RunGit([]string{"pull", "--ff-only"}, wikiPath)
	if err != nil {
		return false, fmt.Errorf("pull: %w", err)
	}
	if exitCode != 0 {
		return false, BoardPushError(fmt.Sprintf("pull failed: %s", stderr))
	}
	updated = !strings.Contains(stdout, "Already up to date.")
	return updated, nil
}

// CommitPush stages, commits, and pushes changes with rebase retry logic
func CommitPush(wikiPath string, relPaths []string, message string) error {
	// Stage files
	args := append([]string{"add", "--"}, relPaths...)
	_, _, exitCode, err := RunGit(args, wikiPath)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	if exitCode != 0 {
		return BoardPushError("add failed")
	}

	// Check for staged changes
	_, _, exitCode, err = RunGit([]string{"diff", "--cached", "--quiet"}, wikiPath)
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
	_, _, exitCode, err = RunGit([]string{"commit", "-m", message}, wikiPath)
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	if exitCode != 0 {
		return BoardPushError("commit failed")
	}

	// Skip push if env var set
	if os.Getenv("WIKI_SKIP_PUSH") == "1" {
		return nil
	}

	// Push with rebase retry
	for attempt := 0; attempt < 2; attempt++ {
		_, stderr, exitCode, err := RunGit([]string{"push"}, wikiPath)
		if err != nil {
			return fmt.Errorf("push: %w", err)
		}
		if exitCode == 0 {
			return nil
		}

		// Check for non-fast-forward error
		if strings.Contains(stderr, "non-fast-forward") || strings.Contains(stderr, "rejected") {
			// Try rebase
			_, _, exitCode, err := RunGit([]string{"pull", "--rebase"}, wikiPath)
			if err != nil {
				return fmt.Errorf("rebase: %w", err)
			}
			if exitCode != 0 {
				// Abort rebase on failure
				RunGit([]string{"rebase", "--abort"}, wikiPath)
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

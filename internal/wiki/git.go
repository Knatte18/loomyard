package wiki

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WikiPushError represents a fatal git push error
type WikiPushError string

func (e WikiPushError) Error() string {
	return string(e)
}

// WikiPathError represents an invalid wiki path
type WikiPathError string

func (e WikiPathError) Error() string {
	return string(e)
}

// PathGuard validates a relative path for wiki operations
func PathGuard(relPath string) error {
	if relPath == "" {
		return WikiPathError("empty path")
	}
	if filepath.IsAbs(relPath) {
		return WikiPathError("absolute path not allowed")
	}

	cleaned := filepath.Clean(relPath)
	components := strings.Split(cleaned, string(filepath.Separator))
	for _, c := range components {
		if c == ".." {
			return WikiPathError("parent directory reference not allowed")
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

	if err := os.Rename(tmpPath, fullPath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// RunGit runs a git command and returns stdout, stderr, and exit code
func RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd

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
		return false, WikiPushError(fmt.Sprintf("pull failed: %s", stderr))
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
		return WikiPushError("add failed")
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
		return WikiPushError("diff check failed")
	}

	// Commit
	_, _, exitCode, err = RunGit([]string{"commit", "-m", message}, wikiPath)
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	if exitCode != 0 {
		return WikiPushError("commit failed")
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
				return WikiPushError("rebase failed")
			}
			// Continue to next push attempt
			continue
		}

		// Other push error
		return WikiPushError(fmt.Sprintf("push failed: %s", stderr))
	}

	return WikiPushError("push still failing after rebase retry")
}

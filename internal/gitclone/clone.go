// clone.go implements the clone orchestration logic with strict-abort teardown.

package gitclone

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/git"
)

// removeAll is a testability seam for os.RemoveAll, allowing tests to inject errors.
var removeAll = os.RemoveAll

// cloneHub orchestrates the cloning of host, weft, and board repositories into a Hub directory.
//
// It takes cwd (current working directory), and three repository URLs: hostURL, weftURL, and
// boardURL (which may be empty to use a derived default). It returns the path to the created
// Hub directory, the resolved board URL (either explicit or derived), and any error encountered.
//
// The operation proceeds in phases:
//   1. Derive the host repo name; if derivation fails, return an error without cleanup.
//   2. Compute the Hub path as <cwd>/<name>-HUB.
//   3. Check if the Hub path exists; if so, return an error without removing it (we did not create it).
//   4. Create the Hub directory; if it fails, return the wrapped error (no teardown yet).
//   5. Clone host repo to <Hub>/<name>; on failure, teardown and return the error.
//   6. Clone weft repo to <Hub>/<name>-weft; on failure, teardown and return the error.
//   7. Resolve board URL: if boardURL is empty, use deriveBoardURL(weftURL); otherwise use boardURL.
//   8. Clone board repo to <Hub>/_board; on failure, teardown and return the error.
//   9. Return the Hub path, resolved board URL, and nil error.
//
// If any clone fails, teardownHub removes the entire Hub directory; if removal also fails,
// the error mentions both the clone failure and the residual Hub path.
func cloneHub(cwd, hostURL, weftURL, boardURL string) (hubPath, resolvedBoardURL string, err error) {
	// Normalize cwd to an absolute path
	cwd = filepath.Clean(cwd)

	// Step 1: Derive host repo name
	name := deriveHostName(hostURL)
	if name == "" {
		return "", "", fmt.Errorf("could not derive repo name from host URL %s", hostURL)
	}

	// Step 2: Compute Hub path
	hubPath = filepath.Join(cwd, name+hubSuffix)

	// Step 3: Check if Hub already exists
	if _, err := os.Stat(hubPath); err == nil {
		return "", "", fmt.Errorf("hub already exists at %s", hubPath)
	}

	// Step 4: Create Hub directory
	if err := os.MkdirAll(hubPath, 0o755); err != nil {
		return "", "", err
	}

	// Step 5: Clone host repo
	if err := cloneRepo(hostURL, filepath.Join(hubPath, name)); err != nil {
		return "", "", teardownHub(hubPath, err)
	}

	// Step 6: Clone weft repo
	if err := cloneRepo(weftURL, filepath.Join(hubPath, name+weftSuffix)); err != nil {
		return "", "", teardownHub(hubPath, err)
	}

	// Step 7: Resolve board URL
	board := boardURL
	if board == "" {
		board = deriveBoardURL(weftURL)
	}

	// Step 8: Clone board repo
	if err := cloneRepo(board, filepath.Join(hubPath, boardDirName)); err != nil {
		return "", "", teardownHub(hubPath, err)
	}

	// Step 9: Success
	return hubPath, board, nil
}

// cloneRepo clones a repository from url to dest.
//
// The clone is executed via git.RunGit with the parent directory of dest as the cwd,
// and the basename of dest as the destination argument. Paths are cleaned and normalized.
// Non-zero git exit returns an error wrapping the stderr output.
func cloneRepo(url, dest string) error {
	// Clean and normalize paths
	dest = filepath.Clean(dest)
	parentDir := filepath.Dir(dest)
	parentDir = filepath.Clean(parentDir)
	destName := filepath.Base(dest)

	// Verify the parent directory exists
	info, err := os.Stat(parentDir)
	if err != nil {
		return fmt.Errorf("parent directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("parent directory does not exist: %s is not a directory", parentDir)
	}

	// Convert paths to use forward slashes for git compatibility on Windows.
	// dest is split into parent+basename so git resolves cleanly on Windows;
	// the plan's full-absolute-dest variant is functionally equivalent.
	gitURL := filepath.ToSlash(url)
	gitDest := filepath.ToSlash(destName)

	stdout, stderr, exitCode, err := git.RunGit([]string{"clone", gitURL, gitDest}, parentDir)
	if err != nil {
		return fmt.Errorf("clone failed: %w", err)
	}

	if exitCode != 0 {
		return fmt.Errorf("clone failed: %s", stderr)
	}

	_ = stdout // stdout is not used; we only check for errors

	return nil
}

// teardownHub removes the Hub directory and returns an error combining the cause with
// information about the failed removal (if applicable).
//
// If removeAll succeeds, teardownHub returns cause unchanged. If removeAll fails,
// teardownHub returns an error combining cause with a message about the residual Hub.
func teardownHub(hubPath string, cause error) error {
	if err := removeAll(hubPath); err != nil {
		return fmt.Errorf("%w; residual hub left at %s; remove it manually before retrying", cause, hubPath)
	}
	return cause
}

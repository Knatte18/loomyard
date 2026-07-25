// clone.go implements the clone orchestration logic with strict-abort teardown.
//
// Adapted from warpengine's clone.go. The one behavioral delta: after the weft
// clone succeeds, the weft primary's freshly-cloned branch is renamed onto its
// WeftBranchName-suffixed pairing (e.g. "main" -> "main-weft") so weft:main is
// never claimed directly — every fabric-managed weft branch, including the
// primary's, carries the uniform "-weft" suffix.

package fabricengine

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// RemoveAll is an exported testability seam for os.RemoveAll, allowing tests to
// inject errors. It is fabric's OWN teardown seam, distinct from warpengine's:
// each module's clone orchestration and its differential test tear down through
// its own module's RemoveAll, never the other's.
var RemoveAll = os.RemoveAll

// CloneHub orchestrates the cloning of host, weft, and board repositories into a Hub directory.
//
// It takes cwd (current working directory), and three repository URLs: hostURL, weftURL, and
// boardURL (which may be empty to use a derived default). It returns the path to the created
// Hub directory, the resolved board URL (either explicit or derived), and any error encountered.
//
// The operation proceeds in phases:
//  1. Derive the host repo name; if derivation fails, return an error without cleanup.
//  2. Compute the Hub path as <cwd>/<name>-HUB.
//  3. Check if the Hub path exists; if so, return an error without removing it (we did not create it).
//  4. Create the Hub directory; if it fails, return the wrapped error (no teardown yet).
//  5. Clone host repo to <Hub>/<name>; on failure, teardown and return the error.
//  6. Clone weft repo to <Hub>/<name>-weft; on failure, teardown and return the error.
//     6b. Read the weft primary's checked-out branch and check out its
//     WeftBranchName-suffixed pairing at the same HEAD; on failure, teardown
//     and return the error.
//  7. Resolve board URL: if boardURL is empty, use deriveBoardURL(weftURL); otherwise use boardURL.
//  8. Clone board repo to <Hub>/_board; on failure, teardown and return the error.
//  9. Return the Hub path, resolved board URL, and nil error.
//
// If any clone fails, teardownHub removes the entire Hub directory; if removal also fails,
// the error mentions both the clone failure and the residual Hub path.
func CloneHub(cwd, hostURL, weftURL, boardURL string) (hubPath, resolvedBoardURL string, err error) {
	// Normalize cwd to an absolute path
	cwd = filepath.Clean(cwd)

	// Step 1: Derive host repo name
	name := DeriveHostName(hostURL)
	if name == "" {
		return "", "", fmt.Errorf("could not derive repo name from host URL %s", hostURL)
	}

	// Step 2: Compute Hub path
	hubPath = hubgeometry.HubPath(cwd, name)

	// Step 3: Check if Hub already exists
	if _, err := os.Stat(hubPath); err == nil {
		return "", "", fmt.Errorf("hub already exists at %s", hubPath)
	}

	// Step 4: Create Hub directory
	if err := os.MkdirAll(hubPath, 0o755); err != nil {
		return "", "", err
	}

	// Step 5: Clone host repo
	hostWorktreePath := filepath.Join(hubPath, name)
	if err := cloneRepo(hostURL, hostWorktreePath); err != nil {
		return "", "", teardownHub(hubPath, err)
	}

	// Install the post-checkout hook after the host worktree exists so drift
	// warnings fire on every subsequent git checkout within this repo.
	// Hook installation is non-fatal: a failure is logged but does not abort
	// the clone (the hook is belt-and-suspenders for usability, not correctness).
	if hookLayout, err := hubgeometry.Resolve(hostWorktreePath); err == nil {
		if hookErr := InstallPostCheckoutHook(hookLayout); hookErr != nil {
			log.Printf("fabric clone: post-checkout hook install (non-fatal): %v", hookErr)
		}
	} else {
		log.Printf("fabric clone: resolve layout for hook install (non-fatal): %v", err)
	}

	// Step 6: Clone weft repo
	weftPath := hubgeometry.WeftSiblingPath(hubPath, name)
	if err := cloneRepo(weftURL, weftPath); err != nil {
		return "", "", teardownHub(hubPath, err)
	}

	// Step 6b: Rename the weft primary's freshly-cloned branch onto its
	// WeftBranchName-suffixed pairing, so weft:<branch> is never claimed
	// directly under fabric's uniform branch scheme.
	if err := suffixWeftPrimaryBranch(weftPath); err != nil {
		return "", "", teardownHub(hubPath, err)
	}

	// Step 7: Resolve board URL
	board := boardURL
	if board == "" {
		board = deriveBoardURL(weftURL)
	}

	// Step 8: Clone board repo
	if err := cloneRepo(board, hubgeometry.BoardDir(hubPath)); err != nil {
		return "", "", teardownHub(hubPath, err)
	}

	// Step 9: Success
	return hubPath, board, nil
}

// suffixWeftPrimaryBranch reads the branch checked out at weftPath (the weft
// primary, immediately after clone) and checks out its WeftBranchName-suffixed
// pairing at the same HEAD (e.g. "main" -> "main-weft"). Returns an error if
// the weft primary is on a detached HEAD (no branch to read) or if either git
// call fails.
func suffixWeftPrimaryBranch(weftPath string) error {
	stdout, _, exitCode, err := gitexec.RunGit([]string{"branch", "--show-current"}, weftPath)
	if err != nil {
		return fmt.Errorf("resolve weft primary branch: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("git branch --show-current in weft primary failed (git exit %d)", exitCode)
	}
	hostBranch := strings.TrimSpace(stdout)
	if hostBranch == "" {
		return fmt.Errorf("weft primary at %s is on a detached HEAD after clone; cannot derive its weft branch", weftPath)
	}

	suffixedBranch := WeftBranchName(hostBranch)
	_, _, exitCode, err = gitexec.RunGit([]string{"checkout", "-b", suffixedBranch}, weftPath)
	if err != nil {
		return fmt.Errorf("create weft primary branch %q: %w", suffixedBranch, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("checkout -b %q in weft primary failed (git exit %d)", suffixedBranch, exitCode)
	}
	return nil
}

// cloneRepo clones a repository from url to dest.
//
// The clone is executed via gitexec.RunGit with the parent directory of dest as the cwd,
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

	stdout, _, exitCode, err := gitexec.RunGit([]string{"clone", gitURL, gitDest}, parentDir)
	if err != nil {
		return fmt.Errorf("clone failed: %w", err)
	}

	if exitCode != 0 {
		return fmt.Errorf("clone %q to %q failed (git exit %d)", url, dest, exitCode)
	}

	_ = stdout // stdout is not used; we only check for errors

	return nil
}

// teardownHub removes the Hub directory and returns an error combining the cause with
// information about the failed removal (if applicable).
//
// If RemoveAll succeeds, teardownHub returns cause unchanged. If RemoveAll fails,
// teardownHub returns an error combining cause with a message about the residual Hub.
func teardownHub(hubPath string, cause error) error {
	if err := RemoveAll(hubPath); err != nil {
		return fmt.Errorf("%w; residual hub left at %s; remove it manually before retrying", cause, hubPath)
	}
	return cause
}

// DeriveHostName extracts the host repository basename from a raw URL or file path.
//
// It trims any trailing slash or backslash, then takes the final path segment of rawURL
// after splitting on forward slash, backslash, and colon (for HTTPS URLs, file paths,
// and SCP-form URLs like git@github.com:user/repo.git).
// A single trailing .git extension is stripped if present. Returns an empty string if no
// basename can be extracted or if the URL contains no path segments.
//
// Examples:
//
//   - "https://github.com/u/repo.git" → "repo"
//   - "https://github.com/u/repo" → "repo"
//   - "git@github.com:u/repo.git" → "repo"
//   - "https://github.com/u/repo/" → "repo"
//   - "C:\path\to\repo.git" → "repo"
//   - "" → ""
func DeriveHostName(rawURL string) string {
	// Trim trailing slashes (both forward and back)
	rawURL = strings.TrimSuffix(rawURL, "/")
	rawURL = strings.TrimSuffix(rawURL, "\\")

	// Split on forward slash, backslash, and colon to handle HTTPS, file paths, and SCP forms
	var parts []string
	for _, seg := range strings.FieldsFunc(rawURL, func(r rune) bool { return r == '/' || r == '\\' || r == ':' }) {
		if seg != "" {
			parts = append(parts, seg)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	// Take the last segment and strip .git suffix
	name := parts[len(parts)-1]
	name = strings.TrimSuffix(name, ".git")

	return name
}

// deriveBoardURL derives the board repository URL from a weft repository URL.
//
// It strips a single trailing .git suffix from weftURL if present, then appends .wiki.git.
// This ensures that both "…/weft.git" and "…/weft" yield "…/weft.wiki.git".
//
// Examples:
//
//   - "https://github.com/u/weft.git" → "https://github.com/u/weft.wiki.git"
//   - "https://github.com/u/weft" → "https://github.com/u/weft.wiki.git"
func deriveBoardURL(weftURL string) string {
	weftURL = strings.TrimSuffix(weftURL, ".git")
	return weftURL + ".wiki.git"
}

// clone.go implements the clone orchestration logic with strict-abort teardown.
//
// After the weft clone succeeds, the weft primary is checked out onto its
// WeftBranchName-suffixed pairing (e.g. "main-weft" for a default branch
// "main") so weft:main is never claimed directly — every fabric-managed weft
// branch, including the primary's, carries the uniform "-weft" suffix. The
// suffixed branch is adopted from an existing origin/<branch>-weft when the
// remote already carries one (a re-clone of a hub with weft history) and
// created fresh only otherwise; the freshly-cloned default branch itself
// remains, unclaimed.

package fabricengine

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// RemoveAll is an exported testability seam for os.RemoveAll, allowing tests to
// inject errors into fabric's own clone-orchestration teardown path.
var RemoveAll = os.RemoveAll

// staleFabricAnchorName is the pre-rename lyx-anchor marker filename. It has
// no compatibility fallback read (lyxcwd.AnchorFileName does not read it);
// it exists here only so CloneHub can detect an old clone's leftover marker
// and hard-error with re-clone as the remedy, rather than silently
// re-anchoring at the wrong subpath.
const staleFabricAnchorName = ".fabric-anchor"

// CloneResult carries the resolved geometry CloneHub hands back to the caller
// once the git-level clone, board-worktree materialization, and anchor
// resolution are done. It is deliberately git/geometry-only — the CLI layer
// (internal/fabriccli) drives config materialization, weft:main commit, and
// junction wiring from these fields, because those calls route through
// internal/configsync, which fabricengine must never import (see the
// fabricengine → configsync → configreg → fabricengine cycle documented in
// this file's clone-does-everything batch scope).
type CloneResult struct {
	HubPath  string // HubPath is the created <name>-HUB container directory.
	Anchor   string // Anchor is the resolved lyx-anchor subpath (e.g. "backend" or ".").
	BoardDir string // BoardDir is the package-level BoardDir(HubPath) result, the weft:main checkout.
	WeftBase string // WeftBase is the weft-side directory paired with PrimeCwd.
	PrimeCwd string // PrimeCwd is the resolved prime host worktree path at Anchor.
}

// CloneHub orchestrates the cloning of host and weft repositories, then
// materializes <Hub>/_board as a second weft worktree, into a Hub directory.
// It clones + checks out + materializes the board worktree + resolves/records
// the lyx-anchor subpath, and returns the resolved geometry for the CLI layer
// to drive config materialization and wiring — CloneHub deliberately does NOT
// do that itself, to avoid the fabricengine → configsync import cycle.
//
// It takes cwd (current working directory), two repository URLs (hostURL and
// weftURL), and subpath (the lyx-anchor subpath to resolve; see the anchor
// resolution step below). It returns the resolved CloneResult and any error
// encountered.
//
// The operation proceeds in phases:
//  1. Derive the host repo name; if derivation fails, return an error without cleanup.
//  2. Compute the Hub path as <cwd>/<name>-HUB.
//  3. Check if the Hub path exists; if so, return an error without removing it (we did not create it).
//  4. Create the Hub directory; if it fails, return the wrapped error (no teardown yet).
//  5. Clone host repo to <Hub>/<name>; on failure, teardown and return the error.
//  6. Clone weft repo to <Hub>/<name>-weft; on failure, teardown and return the error.
//     6b. Read the weft primary's checked-out branch and check out its
//     WeftBranchName-suffixed pairing at the same HEAD, capturing the host
//     branch name it read; on failure, teardown and return the error.
//  7. Materialize <Hub>/_board as a second weft worktree via ensureBoardWorktree,
//     adopted onto the captured host branch if it already exists locally from
//     step 6's clone, freshly orphan-created otherwise; on failure, teardown
//     and return the error.
//  8. Resolve the lyx-anchor subpath adopt-or-create (adopt: read the marker
//     already committed on weft:main; create: validate subpath exists in the
//     host worktree, then write the marker to the board worktree on disk —
//     the CLI commits it), and return the resolved CloneResult.
//
// Any clone OR worktree-add failure triggers teardownHub, which removes the
// entire Hub directory; if removal also fails, the error mentions both the
// original failure and the residual Hub path.
func CloneHub(cwd, hostURL, weftURL, subpath string) (CloneResult, error) {
	// Normalize cwd to an absolute path
	cwd = filepath.Clean(cwd)

	// Step 1: Derive host repo name
	name := DeriveHostName(hostURL)
	if name == "" {
		return CloneResult{}, fmt.Errorf("could not derive repo name from host URL %s", hostURL)
	}

	// Step 2: Compute Hub path
	hubPath := HubPath(cwd, name)

	// Step 3: Check if Hub already exists
	if _, err := os.Stat(hubPath); err == nil {
		return CloneResult{}, fmt.Errorf("hub already exists at %s", hubPath)
	}

	// Step 4: Create Hub directory
	if err := os.MkdirAll(hubPath, 0o755); err != nil {
		return CloneResult{}, err
	}

	// Step 5: Clone host repo
	hostWorktreePath := filepath.Join(hubPath, name)
	if err := cloneRepo(hostURL, hostWorktreePath); err != nil {
		return CloneResult{}, teardownHub(hubPath, err)
	}

	// Install the post-checkout hook after the host worktree exists so drift
	// warnings fire on every subsequent git checkout within this repo.
	// Hook installation is non-fatal: a failure is logged but does not abort
	// the clone (the hook is belt-and-suspenders for usability, not correctness).
	//
	// hubPath and name are already in scope from steps 2 and 1; a direct
	// struct construction is a simplification, not a correctness fix, since
	// InstallPostCheckoutHook reads exactly one field (WorktreePath()) — it
	// needs a path, not a resolution. Step 3 above aborts the clone if the hub
	// already exists and step 4 creates it fresh, so at this point the hub is
	// provably empty, <hubPath>/_board cannot exist, and a Resolve call here
	// would always have succeeded — there is no failure path left to log.
	hookLocation := &lyxcwd.Location{HubPath: hubPath, WorktreeName: name}
	if hookErr := InstallPostCheckoutHook(hookLocation); hookErr != nil {
		log.Printf("fabric clone: post-checkout hook install (non-fatal): %v", hookErr)
	}

	// Step 6: Clone weft repo
	weftPath := weftname.SiblingPath(hubPath, name)
	if err := cloneRepo(weftURL, weftPath); err != nil {
		return CloneResult{}, teardownHub(hubPath, err)
	}

	// Step 6b: Rename the weft primary's freshly-cloned branch onto its
	// WeftBranchName-suffixed pairing, so weft:<branch> is never claimed
	// directly under fabric's uniform branch scheme. Capture hostBranch (the
	// branch read before the rename) so step 7's _board worktree-add can reuse
	// it directly, rather than re-reading git branch --show-current at weftPath
	// after the rename (which would incorrectly see the suffixed branch).
	hostBranch, err := suffixWeftPrimaryBranch(weftPath)
	if err != nil {
		return CloneResult{}, teardownHub(hubPath, err)
	}

	// Step 7: Materialize <Hub>/_board as a second weft worktree, checked out
	// on hostBranch — adopted if that branch already exists locally from
	// step 6's clone, freshly orphan-created otherwise (a genuinely empty
	// weft remote).
	boardDir := BoardDir(hubPath)
	if err := ensureBoardWorktree(weftPath, hostBranch, boardDir); err != nil {
		return CloneResult{}, teardownHub(hubPath, err)
	}

	// Step 8: Resolve the lyx-anchor subpath adopt-or-create, and write the
	// marker to the board worktree ON DISK. The CLI layer commits it onto
	// weft:main (config materialization and the commit both live in
	// internal/fabriccli to avoid the fabricengine → configsync import cycle).
	//
	// A leftover pre-rename .fabric-anchor with no .lyx-anchor beside it is a
	// hard error, not a silent fallback: without this check, an old clone
	// would fall through to the create path below and re-anchor at "."
	// (or the requested --subpath) even though a real anchor was already
	// recorded under the old name, silently resolving _lyx to the wrong place
	// for a subpath-anchored repo.
	if _, statErr := os.Stat(filepath.Join(boardDir, staleFabricAnchorName)); statErr == nil {
		if _, newErr := os.Stat(filepath.Join(boardDir, lyxcwd.AnchorFileName)); os.IsNotExist(newErr) {
			return CloneResult{}, teardownHub(hubPath, fmt.Errorf(
				"found stale %s marker with no %s beside it at %s; re-clone this hub to migrate to the renamed marker",
				staleFabricAnchorName, lyxcwd.AnchorFileName, boardDir))
		}
	}

	markerPath := filepath.Join(boardDir, lyxcwd.AnchorFileName)
	var anchor string
	if data, statErr := os.ReadFile(markerPath); statErr == nil {
		// Adopt path: ensureBoardWorktree checked out weft:main, which already
		// carries a committed marker from a prior clone — this is a re-clone.
		recorded := strings.TrimSpace(string(data))
		if requested := filepath.Clean(subpath); subpath != "" && requested != "." && requested != recorded {
			// A non-default requested subpath disagrees with the recorded
			// anchor: never silently re-anchor, the record is authoritative.
			return CloneResult{}, teardownHub(hubPath, fmt.Errorf(
				"requested --subpath %q does not match the recorded anchor %q for this hub", requested, recorded))
		}
		anchor = recorded
	} else {
		// Create path: first-ever clone of this weft, no marker committed yet.
		// Validate the requested subpath exists in the host worktree before
		// recording it, so a typo like "backedn" fails loudly instead of
		// silently anchoring to a directory that was never there.
		anchor = filepath.Clean(subpath)
		if subpath == "" {
			anchor = "."
		}
		if info, statErr := os.Stat(filepath.Join(hostWorktreePath, anchor)); statErr != nil || !info.IsDir() {
			return CloneResult{}, teardownHub(hubPath, fmt.Errorf("subpath %q does not exist in the cloned host repo", anchor))
		}
		if err := os.WriteFile(markerPath, []byte(anchor+"\n"), 0o644); err != nil {
			return CloneResult{}, teardownHub(hubPath, fmt.Errorf("write %s: %w", markerPath, err))
		}
	}

	// Resolve the prime layout now that the marker exists on disk, so
	// RelPath — and therefore WeftBase — reflects the resolved anchor.
	primeCwd := filepath.Join(hostWorktreePath, anchor)
	l, err := lyxcwd.Resolve(primeCwd)
	if err != nil {
		return CloneResult{}, teardownHub(hubPath, fmt.Errorf("resolve prime layout at %s: %w", primeCwd, err))
	}

	// Wire the operator-convenience _board junction as a named special case,
	// the same point pathspec junctions are wired at the CLI layer (see
	// internal/fabriccli/clone.go) — but here directly, since _board needs no
	// fabric.yaml load and CloneHub must not import configsync.
	if err := wireBoardLink(l, filepath.Base(hostWorktreePath)); err != nil {
		return CloneResult{}, teardownHub(hubPath, fmt.Errorf("wire board junction: %w", err))
	}

	weftBase := filepath.Join(WeftWorktree(l), l.AnchorRel)

	return CloneResult{
		HubPath:  hubPath,
		Anchor:   anchor,
		BoardDir: boardDir,
		WeftBase: weftBase,
		PrimeCwd: primeCwd,
	}, nil
}

// suffixWeftPrimaryBranch reads the branch checked out at weftPath (the weft
// primary, immediately after clone) and checks out its WeftBranchName-suffixed
// pairing, adopt-or-create style. When origin already carries the suffixed
// branch — a re-clone (fresh machine, `clone --reset`) of a hub that has synced
// weft history — that remote branch is adopted as a tracking local branch, so
// the fresh hub inherits the accumulated weft state (its _lyx content) and its
// first push can rebase-recover through the configured upstream instead of
// diverging permanently from an untracked fork. Only when the remote has no
// suffixed branch yet (a genuinely new hub) is the branch created fresh at the
// current HEAD. Returns an error if the weft primary is on a detached HEAD (no
// branch to read) or if any git call fails.
//
// Returns the host branch name it read (before the rename) so CloneHub's
// _board-worktree-add step can reuse it directly — re-reading
// git branch --show-current at weftPath after this function returns would
// incorrectly see the already-renamed <hostBranch>-weft, not hostBranch.
func suffixWeftPrimaryBranch(weftPath string) (hostBranch string, err error) {
	stdout, _, exitCode, err := gitexec.RunGit([]string{"branch", "--show-current"}, weftPath)
	if err != nil {
		return "", fmt.Errorf("resolve weft primary branch: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("git branch --show-current in weft primary failed (git exit %d)", exitCode)
	}
	hostBranch = strings.TrimSpace(stdout)
	if hostBranch == "" {
		return "", fmt.Errorf("weft primary at %s is on a detached HEAD after clone; cannot derive its weft branch", weftPath)
	}

	suffixedBranch := WeftBranchName(hostBranch)

	// Adopt path: the remote already carries the suffixed branch (this is a
	// re-clone of a hub with existing weft history). Starting the local branch
	// from origin/<suffixed> both checks out that history and configures the
	// upstream (git's default branch.autoSetupMerge for a remote-tracking start
	// point), which the create path below deliberately leaves to the first push.
	remoteRef := "refs/remotes/origin/" + suffixedBranch
	_, _, exitCode, err = gitexec.RunGit([]string{"rev-parse", "--verify", "--quiet", remoteRef}, weftPath)
	if err != nil {
		return "", fmt.Errorf("check for remote weft primary branch: %w", err)
	}
	checkoutArgs := []string{"checkout", "-b", suffixedBranch}
	if exitCode == 0 {
		checkoutArgs = append(checkoutArgs, "origin/"+suffixedBranch)
	}

	_, _, exitCode, err = gitexec.RunGit(checkoutArgs, weftPath)
	if err != nil {
		return "", fmt.Errorf("create weft primary branch %q: %w", suffixedBranch, err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("checkout -b %q in weft primary failed (git exit %d)", suffixedBranch, exitCode)
	}
	return hostBranch, nil
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

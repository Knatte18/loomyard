// hook.go installs the embedded post-checkout drift-warning hook into the repo's
// common hooks directory. The install is idempotent (sentinel-guarded) and
// non-clobbering: an existing user hook is chained rather than overwritten.

package warpengine

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// postCheckoutScript is the embedded POSIX sh post-checkout hook body.
// It warns when the host and weft worktree branches diverge after a git checkout.
//
//go:embed post-checkout.sh
var postCheckoutScript string

// hookSentinel is the unique marker written into every warp-managed hook file.
// Its presence on any line means the file already contains the warp hook body,
// making re-installation idempotent.
const hookSentinel = "WARP_SENTINEL: post-checkout drift warning"

// chainedHookPreamble is prepended when chaining around an existing user hook.
// It runs the prior hook first, captures its exit code, and runs the warp check
// regardless so the warning is always visible even if the user hook fails.
const chainedHookPreamble = `#!/bin/sh
# post-checkout — user hook chain wrapper written by warp.
# The original hook was moved to post-checkout.user and is invoked first.

SCRIPT_DIR="$(dirname "$0")"
"$SCRIPT_DIR/post-checkout.user" "$@"
_user_exit=$?

`

// InstallPostCheckoutHook installs the embedded post-checkout drift-warning
// hook into the repo's common hooks directory.
//
// The function resolves the common hooks directory via
// gitexec.RunGit("rev-parse --git-common-dir") on l.WorktreeRoot, then appends
// "hooks/post-checkout" to obtain the hook path.
//
// Idempotency: if the target file already contains hookSentinel, the function
// returns nil immediately without touching the file.
//
// Non-clobbering chaining: if the target exists and does not contain the warp
// sentinel, the existing file is renamed to post-checkout.user and a new
// wrapper is written that (a) invokes the user hook first, (b) runs the warp
// drift check. The wrapper itself contains the sentinel so repeated installs are
// idempotent.
//
// On platforms that support chmod (non-Windows), the file is marked executable.
// On Windows, git reads and executes the hook via its bundled bash regardless of
// the file mode, so the chmod is a no-op but harmless.
func InstallPostCheckoutHook(l *hubgeometry.Layout) error {
	// Resolve the common git directory so the hook lands in the shared .git
	// even when called from a linked worktree (where --git-dir differs).
	commonDirOut, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--git-common-dir"},
		l.WorktreeRoot,
	)
	if err != nil {
		return fmt.Errorf("resolve git common dir: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("rev-parse --git-common-dir failed with exit code %d", exitCode)
	}

	// git emits the path with forward slashes; convert to native separators.
	// When the path is relative (e.g. ".git" in a standard clone), resolve it
	// relative to the worktree root so the hook lands in the correct directory
	// regardless of the test process's working directory.
	commonDir := filepath.FromSlash(strings.TrimSpace(commonDirOut))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(l.WorktreeRoot, commonDir)
	}
	hookPath := filepath.Join(commonDir, "hooks", "post-checkout")

	// Ensure the hooks directory exists (it may not in a freshly-initialised bare repo).
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		return fmt.Errorf("mkdir hooks dir: %w", err)
	}

	// Check whether the warp sentinel is already present — idempotent re-install.
	existing, readErr := os.ReadFile(hookPath)
	if readErr == nil && strings.Contains(string(existing), hookSentinel) {
		// Already installed; nothing to do.
		return nil
	}

	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("read existing hook: %w", readErr)
	}

	if readErr == nil {
		// A user hook exists that does not contain the warp sentinel; chain it.
		if err := chainUserHook(hookPath, existing); err != nil {
			return fmt.Errorf("chain existing post-checkout hook: %w", err)
		}
		return nil
	}

	// No prior hook; write the embedded script directly.
	if err := writeHookFile(hookPath, postCheckoutScript); err != nil {
		return fmt.Errorf("write post-checkout hook: %w", err)
	}

	return nil
}

// chainUserHook backs up the existing hook to post-checkout.user and writes a
// wrapper script that invokes the user hook first, then runs the warp check.
//
// The wrapper is written atomically: the backup is created first; if any step
// fails after the backup, the original hook is restored so the repo is never
// left in a state where neither hook runs.
func chainUserHook(hookPath string, original []byte) error {
	userHookPath := hookPath + ".user"

	// Write the user hook backup first; refuse to clobber an existing backup
	// (a previous chaining attempt may have left one).
	if _, statErr := os.Lstat(userHookPath); statErr == nil {
		return fmt.Errorf("cannot chain: backup file already exists at %s", userHookPath)
	}

	if err := os.WriteFile(userHookPath, original, 0o755); err != nil {
		return fmt.Errorf("write user hook backup: %w", err)
	}

	// Build the chained wrapper: preamble + warp body (which carries the sentinel).
	chainContent := chainedHookPreamble + postCheckoutScript

	if err := writeHookFile(hookPath, chainContent); err != nil {
		// Restore the original on failure.
		_ = os.WriteFile(hookPath, original, 0o755)
		_ = os.Remove(userHookPath)
		return fmt.Errorf("write chained hook: %w", err)
	}

	return nil
}

// writeHookFile writes content to path and marks it executable.
// The file mode 0o755 satisfies git's hook execution requirement on POSIX;
// on Windows, git-for-Windows reads hooks via its bundled bash and ignores the
// file mode, but setting it here is harmless and future-proofs for WSL use.
func writeHookFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return fmt.Errorf("write hook file %s: %w", path, err)
	}
	return nil
}

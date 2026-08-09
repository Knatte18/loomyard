// hook.go installs the embedded post-checkout drift-warning hook into the repo's common hooks
// directory.
// The install is idempotent (sentinel-guarded) and non-clobbering: an existing user hook is chained
// rather than overwritten.
//
// The install sentinel is fabric's own ("FABRIC_SENTINEL: post-checkout drift warning"), so a
// fabric-installed hook can chain cleanly alongside any other hook already installed on the same
// repo.

package fabricengine

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// postCheckoutScript is the embedded POSIX sh post-checkout hook body.
// It warns when the warp and weft worktree branches diverge after a git checkout.
//
//go:embed post-checkout.sh
var postCheckoutScript string

// hookSentinel is the unique marker written into every fabric-managed hook file.
// Its presence on any line means the file already contains the fabric hook body,
// making re-installation idempotent, and it is what lets fabric chain cleanly
// alongside a post-checkout hook installed by anything else on the same repo
// rather than clobbering it.
const hookSentinel = "FABRIC_SENTINEL: post-checkout drift warning"

// chainedHookPreamble is prepended when chaining around an existing user hook.
// It runs the prior hook first, then runs the fabric check regardless of the
// user hook's outcome, so the drift warning is always visible even when the
// user hook fails. The user hook's exit code is deliberately not propagated:
// the fabric body ends in exit 0 because a drift warning must never hard-block
// a checkout.
// The executable test around the chained call preserves the operator's own
// intent: git ignores a non-executable hook, so a hook disabled with chmod -x
// must stay disabled once fabric has moved it aside, and invoking it anyway
// would print a "Permission denied" line on every checkout.
const chainedHookPreamble = `#!/bin/sh
# post-checkout — user hook chain wrapper written by fabric.
# The original hook was moved to post-checkout.user and is invoked first.

SCRIPT_DIR="$(dirname "$0")"
if [ -x "$SCRIPT_DIR/post-checkout.user" ]; then
    "$SCRIPT_DIR/post-checkout.user" "$@"
fi

`

// InstallPostCheckoutHook installs the embedded post-checkout drift-warning hook into the repo's
// common hooks directory.
// Idempotent if already installed;
// chains around any existing hook without clobbering.
// On POSIX platforms, marks the file executable;
// on Windows, git ignores the mode.
func InstallPostCheckoutHook(l *lyxcwd.Location) error {
	// Ask git for the hooks directory rather than composing <git-common-dir>/hooks: that composition
	// ignores core.hooksPath, so on a repo that sets it fabric wrote a hook into a directory git no
	// longer consults, reported success, and the drift warning simply never fired again.
	// --git-path hooks answers with the core.hooksPath directory when one is set and with the COMMON
	// gitdir's hooks/ otherwise, including when called from a linked worktree.
	hooksDirOut, stderr, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--git-path", "hooks"},
		l.WorktreePath(),
	)
	if err != nil {
		return fmt.Errorf("resolve git hooks dir: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("rev-parse --git-path hooks failed (git exit %d): %s", exitCode, strings.TrimSpace(stderr))
	}

	// git emits the path with forward slashes; convert to native separators.
	// When the path is relative (e.g. ".git/hooks" in a standard clone), resolve it
	// relative to the worktree root so the hook lands in the correct directory
	// regardless of the test process's working directory.
	hooksDir := filepath.FromSlash(strings.TrimSpace(hooksDirOut))
	if !filepath.IsAbs(hooksDir) {
		hooksDir = filepath.Join(l.WorktreePath(), hooksDir)
	}
	hookPath := filepath.Join(hooksDir, "post-checkout")

	// Ensure the hooks directory exists (it may not in a freshly-initialised bare repo).
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		return fmt.Errorf("mkdir hooks dir: %w", err)
	}

	// Check whether the fabric sentinel is already present — idempotent re-install.
	existing, readErr := os.ReadFile(hookPath)
	if readErr == nil && strings.Contains(string(existing), hookSentinel) {
		// Already installed; nothing to do.
		return nil
	}

	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("read existing hook: %w", readErr)
	}

	if readErr == nil {
		// A user hook exists that does not contain the fabric sentinel; chain it.
		if err := chainUserHook(hookPath, existing, existingMode(hookPath)); err != nil {
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

// existingMode returns the file mode already on path, or 0o755 when it cannot be read.
// It exists so chaining can preserve the operator's own hook mode on the backup instead of
// promoting it: a hook disabled with chmod -x must stay disabled after fabric moves it aside.
func existingMode(path string) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return 0o755
	}
	return info.Mode().Perm()
}

// chainUserHook backs up the existing hook to post-checkout.user and writes
// a wrapper that invokes both hooks. The backup is created first, and if
// writing the wrapper fails, the original is restored.
// originalMode is the mode the existing hook carried, preserved on the backup so the wrapper's
// executable test reproduces the operator's own enable/disable decision.
func chainUserHook(hookPath string, original []byte, originalMode os.FileMode) error {
	userHookPath := hookPath + ".user"

	// Write the user hook backup first; refuse to clobber an existing backup
	// (a previous chaining attempt may have left one).
	if _, statErr := os.Lstat(userHookPath); statErr == nil {
		return fmt.Errorf("cannot chain: backup file already exists at %s", userHookPath)
	}

	if err := os.WriteFile(userHookPath, original, originalMode); err != nil {
		return fmt.Errorf("write user hook backup: %w", err)
	}
	if err := os.Chmod(userHookPath, originalMode); err != nil {
		return fmt.Errorf("preserve user hook mode on backup: %w", err)
	}

	// Build the chained wrapper: preamble + fabric body (which carries the sentinel).
	chainContent := chainedHookPreamble + postCheckoutScript

	if err := writeHookFile(hookPath, chainContent); err != nil {
		// Restore the original on failure.
		_ = os.WriteFile(hookPath, original, originalMode)
		_ = os.Remove(userHookPath)
		return fmt.Errorf("write chained hook: %w", err)
	}

	return nil
}

// writeHookFile writes content to path and marks it executable.
// The file mode 0o755 satisfies git's hook execution requirement on POSIX;
// on Windows, git-for-Windows reads hooks via its bundled bash and ignores the
// file mode, but setting it here is harmless and future-proofs for WSL use.
//
// The explicit Chmod is what actually enforces the mode: os.WriteFile applies its perm argument only
// when it CREATES the file, so chaining around an existing non-executable user hook produced a
// wrapper git then refused to run, silently retiring both that hook and fabric's drift warning.
func writeHookFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return fmt.Errorf("write hook file %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return fmt.Errorf("mark hook file %s executable: %w", path, err)
	}
	return nil
}

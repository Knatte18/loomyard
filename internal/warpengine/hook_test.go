//go:build integration

// hook_test.go covers InstallPostCheckoutHook: idempotent install, non-clobbering
// chain of an existing user hook, correct weft-sibling resolution for prime and
// child worktrees, and an end-to-end checkout that fires the drift warning.

package warpengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/paths"
)

// resolveCommonHooksDir returns the common git hooks directory for the repo
// rooted at repoDir, mirroring the logic in InstallPostCheckoutHook.
// When git emits a relative path (e.g. ".git" for a standard clone) it is
// resolved relative to repoDir so the result is always an absolute path.
func resolveCommonHooksDir(t *testing.T, repoDir string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse --git-common-dir in %s: %v", repoDir, err)
	}
	commonDir := filepath.FromSlash(strings.TrimSpace(string(out)))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(repoDir, commonDir)
	}
	return filepath.Join(commonDir, "hooks")
}

// TestInstallPostCheckoutHook_Idempotent verifies that calling InstallPostCheckoutHook
// twice does not duplicate the script or alter the file content after the first install.
func TestInstallPostCheckoutHook_Idempotent(t *testing.T) {
	t.Parallel()

	f := lyxtest.CopyHostHub(t)
	l, err := paths.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("paths.Resolve(%q): %v", f.Hub, err)
	}

	if err := InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("first InstallPostCheckoutHook: %v", err)
	}

	hooksDir := resolveCommonHooksDir(t, f.Hub)
	hookPath := filepath.Join(hooksDir, "post-checkout")

	firstContent, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read hook after first install: %v", err)
	}

	// Second install must be a no-op.
	if err := InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("second InstallPostCheckoutHook: %v", err)
	}

	secondContent, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read hook after second install: %v", err)
	}

	if string(firstContent) != string(secondContent) {
		t.Errorf("hook content changed on re-install; first=%q second=%q", firstContent, secondContent)
	}

	// The sentinel must appear exactly once (no duplication on re-install).
	count := strings.Count(string(secondContent), hookSentinel)
	if count != 1 {
		t.Errorf("sentinel appears %d times after re-install; want exactly 1", count)
	}
}

// TestInstallPostCheckoutHook_ChainIdempotent verifies that when a user hook exists,
// InstallPostCheckoutHook backs it up to post-checkout.user, writes a chained wrapper
// that references the backup, and that a second install is a no-op (sentinel already present).
func TestInstallPostCheckoutHook_ChainIdempotent(t *testing.T) {
	t.Parallel()

	const userHookContent = "#!/bin/sh\necho user\n"

	f := lyxtest.CopyHostHub(t)
	l, err := paths.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("paths.Resolve(%q): %v", f.Hub, err)
	}

	// Plant a user hook.
	hooksDir := resolveCommonHooksDir(t, f.Hub)
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	hookPath := filepath.Join(hooksDir, "post-checkout")
	if err := os.WriteFile(hookPath, []byte(userHookContent), 0o755); err != nil {
		t.Fatalf("write user hook: %v", err)
	}

	// First install — chains the user hook.
	if err := InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("first InstallPostCheckoutHook: %v", err)
	}

	firstChainContent, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read first chain: %v", err)
	}

	// Verify the original user hook was backed up to post-checkout.user unchanged.
	userBackupPath := hookPath + ".user"
	backupContent, err := os.ReadFile(userBackupPath)
	if err != nil {
		t.Fatalf("read user hook backup: %v", err)
	}
	if string(backupContent) != userHookContent {
		t.Errorf("backup content = %q; want %q", backupContent, userHookContent)
	}

	// Verify the chained wrapper references the backup file.
	if !strings.Contains(string(firstChainContent), "post-checkout.user") {
		t.Errorf("chained hook does not reference post-checkout.user; content: %q", string(firstChainContent))
	}

	// Second install — must be idempotent (sentinel already present).
	if err := InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("second InstallPostCheckoutHook: %v", err)
	}

	secondChainContent, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read second chain: %v", err)
	}

	if string(firstChainContent) != string(secondChainContent) {
		t.Errorf("chained hook changed on re-install; first=%q second=%q", firstChainContent, secondChainContent)
	}
}

// TestInstallPostCheckoutHook_WeftResolution_Prime verifies that the hook script
// correctly resolves the <PrimeName>-weft sibling for a prime (main) worktree.
// A real git checkout is performed; the warning must fire when the host and weft
// branches diverge. This exercises the cwd-based worktree identification and the
// GIT_DIR unset logic in the embedded shell script.
func TestInstallPostCheckoutHook_WeftResolution_Prime(t *testing.T) {
	t.Parallel()

	f := lyxtest.CopyPairedLocal(t)
	l, err := paths.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("paths.Resolve(%q): %v", f.Hub, err)
	}

	// Install the hook in the shared repo.
	if err := InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("InstallPostCheckoutHook: %v", err)
	}

	// Create a branch in the host prime and switch to it, leaving weft on main.
	// This manufactures an in-sync state (host=hook-prime-test, weft=main) after
	// the checkout — divergence is confirmed on the next switch.
	lyxtest.MustRun(t, f.Hub, "git", "checkout", "-b", "hook-prime-test")

	// Switch the weft prime to a new diverging branch so the next host checkout fires.
	lyxtest.MustRun(t, f.WeftPrime, "git", "checkout", "-b", "hook-prime-weft-side")

	// Switch host back to main; now host=main, weft=hook-prime-weft-side → divergence.
	cmd := exec.Command("git", "checkout", "main")
	cmd.Dir = f.Hub
	out, _ := cmd.CombinedOutput()

	if !strings.Contains(string(out), "warp:") {
		t.Logf("hook output: %s", string(out))
		t.Error("expected warp drift warning for prime worktree; got none")
	}
}

// TestInstallPostCheckoutHook_WeftResolution_Child verifies that the hook script
// correctly resolves the <slug>-weft sibling for a child (non-prime) worktree.
// After Add creates the child pair, the hook is installed, weft is manually moved
// to a different branch, and a real git checkout in the child fires the warning.
//
// Note: git worktrees cannot check out a branch that is already checked out in
// another worktree. To trigger the hook without hitting that constraint, we create
// two extra branches in the child host and switch between them while the weft child
// stays on a third, non-overlapping branch — guaranteeing divergence.
func TestInstallPostCheckoutHook_WeftResolution_Child(t *testing.T) {
	t.Parallel()

	const slug = "hook-child-test"

	f := lyxtest.CopyPairedLocal(t)
	l, err := paths.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("paths.Resolve(%q): %v", f.Hub, err)
	}

	// Create a child worktree pair via Add; the child host is on branch <slug>.
	w := New(Config{})
	if _, err := w.Add(l, slug, AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Install the hook (affects the shared common .git/hooks).
	if err := InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("InstallPostCheckoutHook: %v", err)
	}

	childHost := l.WorktreePath(slug)
	childWeft := l.WeftWorktreePath(slug)

	// Create a second branch in the child host so we have two branches to switch
	// between (avoids the git constraint that a branch can only be checked out in
	// one worktree at a time).
	lyxtest.MustRun(t, childHost, "git", "checkout", "-b", "hook-host-alt")

	// Move the weft child to a different branch — this creates the divergence the
	// hook should detect. The weft branch is independent of the host's branches.
	lyxtest.MustRun(t, childWeft, "git", "checkout", "-b", "hook-weft-diverge")

	// Switch the child host back to the original slug branch; the hook fires and
	// must warn because childHost=<slug> while childWeft=hook-weft-diverge.
	cmd := exec.Command("git", "checkout", slug)
	cmd.Dir = childHost
	out, _ := cmd.CombinedOutput()

	// host=<slug>, weft=hook-weft-diverge → guaranteed branch divergence.
	if !strings.Contains(string(out), "warp:") {
		t.Logf("hook output: %s", string(out))
		t.Error("expected warp drift warning for child worktree; got none")
	}
}

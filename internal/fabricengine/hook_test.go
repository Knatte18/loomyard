//go:build integration

// hook_test.go covers InstallPostCheckoutHook: idempotent install, non-clobbering
// chain of an existing user hook, and correct weft-sibling branch resolution for
// prime and child worktrees under fabric's suffixed branch-naming scheme.
//
// fabric's weft branch is always the warp branch plus WeftBranchName's "-weft"
// suffix, never a literal warp/weft branch-name match, so each case additionally
// asserts the in-sync (correctly suffixed) state produces no warning before
// diverging it. The child-pair setup uses raw `git worktree add` rather than
// fabricengine's own Add verb, for test-fixture simplicity.

package fabricengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
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

// TestInstallPostCheckoutHook_Idempotent verifies that calling InstallPostCheckoutHook twice does
// not duplicate the script or alter the file content after the first install.
func TestInstallPostCheckoutHook_Idempotent(t *testing.T) {
	t.Parallel()

	f := gitkit.CopyWarpHub(t)
	l, err := lyxcwd.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%q): %v", f.Hub, err)
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
// InstallPostCheckoutHook backs it up to post-checkout.user, writes a chained wrapper that
// references the backup, and that a second install is a no-op (sentinel already present).
func TestInstallPostCheckoutHook_ChainIdempotent(t *testing.T) {
	t.Parallel()

	const userHookContent = "#!/bin/sh\necho user\n"

	f := gitkit.CopyWarpHub(t)
	l, err := lyxcwd.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%q): %v", f.Hub, err)
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

// TestInstallPostCheckoutHook_WeftResolution_Prime verifies that the hook script correctly resolves
// the <PrimeName>-weft sibling for a prime (main) worktree under fabric's suffixed branch scheme:
// the weft prime must sit on WeftBranchName(warpBranch) to be considered in sync, not on a literal
// warp-branch-name match.
// A real git checkout is performed for both the in-sync and diverged cases.
func TestInstallPostCheckoutHook_WeftResolution_Prime(t *testing.T) {
	t.Parallel()

	f := gitkit.CopyPairedLocal(t)
	l, err := lyxcwd.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%q): %v", f.Hub, err)
	}

	// Install the hook in the shared repo.
	if err := InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("InstallPostCheckoutHook: %v", err)
	}

	// Put the weft prime on the suffixed branch that pairs with warp "main"
	// under fabric's uniform scheme — this is the in-sync state.
	gitkit.MustRun(t, f.WeftPrime, "git", "checkout", "-b", WeftBranchName("main"))

	// Create a scratch branch in the warp prime so we have something to switch
	// away from and back to "main".
	gitkit.MustRun(t, f.Hub, "git", "checkout", "-b", "hook-prime-scratch")

	// Warp back to main while weft sits on "main-weft": in sync, no warning.
	cmd := exec.Command("git", "checkout", "main")
	cmd.Dir = f.Hub
	out, _ := cmd.CombinedOutput()
	if strings.Contains(string(out), "fabric:") {
		t.Errorf("unexpected fabric drift warning for in-sync suffixed prime state: %s", out)
	}

	// Diverge: move weft to a branch that is not the suffixed pairing.
	gitkit.MustRun(t, f.WeftPrime, "git", "checkout", "-b", "hook-prime-weft-side")

	// Switch warp away and back to fire the hook again with weft diverged.
	gitkit.MustRun(t, f.Hub, "git", "checkout", "hook-prime-scratch")
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = f.Hub
	out, _ = cmd.CombinedOutput()

	if !strings.Contains(string(out), "fabric:") {
		t.Logf("hook output: %s", string(out))
		t.Error("expected fabric drift warning for prime worktree; got none")
	}
}

// TestInstallPostCheckoutHook_WeftResolution_Child verifies that the hook script correctly resolves
// the <slug>-weft sibling for a child (non-prime) worktree under fabric's suffixed branch scheme.
// The child worktree pair is created directly via `git worktree add` (fabricengine's own Add verb
// lands in a later batch);
// the weft child's branch is WeftBranchName(slug).
//
// Note: git worktrees cannot check out a branch that is already checked out in another worktree.
// To trigger the hook without hitting that constraint, we create an extra branch in the child warp
// and switch between it and slug while the weft child stays on a third, non-overlapping branch for
// the diverged case.
func TestInstallPostCheckoutHook_WeftResolution_Child(t *testing.T) {
	t.Parallel()

	const slug = "hook-child-test"

	f := gitkit.CopyPairedLocal(t)
	l, err := lyxcwd.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%q): %v", f.Hub, err)
	}

	// Create a child worktree pair directly via git worktree add — fabricengine's
	// own Add verb lands in a later batch; this test only needs a warp/weft
	// worktree pair on disk to exercise the hook script itself.
	childWarp := WorktreePath(l, slug)
	gitkit.MustRun(t, f.Hub, "git", "worktree", "add", childWarp, "-b", slug)

	weftBranch := WeftBranchName(slug)
	childWeft := WeftWorktreePath(l, slug)
	gitkit.MustRun(t, f.WeftPrime, "git", "worktree", "add", childWeft, "-b", weftBranch)

	// Install the hook (affects the shared common .git/hooks for the warp repo,
	// which covers the warp prime and every warp child worktree).
	if err := InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("InstallPostCheckoutHook: %v", err)
	}

	// Create a second branch in the child warp so we have two branches to switch
	// between (avoids the git constraint that a branch can only be checked out in
	// one worktree at a time).
	gitkit.MustRun(t, childWarp, "git", "checkout", "-b", "hook-warp-alt")

	// Child warp back to slug while weft sits on its correctly-suffixed branch:
	// in sync, no warning.
	cmd := exec.Command("git", "checkout", slug)
	cmd.Dir = childWarp
	out, _ := cmd.CombinedOutput()
	if strings.Contains(string(out), "fabric:") {
		t.Errorf("unexpected fabric drift warning for in-sync suffixed child state: %s", out)
	}

	// Move the weft child to a different branch — this creates the divergence the
	// hook should detect. The weft branch is independent of the warp's branches.
	gitkit.MustRun(t, childWeft, "git", "checkout", "-b", "hook-weft-diverge")

	// Switch warp away and back to fire the hook again with weft diverged.
	gitkit.MustRun(t, childWarp, "git", "checkout", "hook-warp-alt")
	cmd = exec.Command("git", "checkout", slug)
	cmd.Dir = childWarp
	out, _ = cmd.CombinedOutput()

	if !strings.Contains(string(out), "fabric:") {
		t.Logf("hook output: %s", string(out))
		t.Error("expected fabric drift warning for child worktree; got none")
	}
}

// TestInstallPostCheckoutHook_ChainedWrapperIsExecutable pins the executable bit on the chained
// wrapper.
// os.WriteFile applies its perm argument only when it CREATES the file, so chaining around an
// existing non-executable user hook used to leave the wrapper non-executable, and git then ignored
// the hook entirely — silently retiring both the operator's hook and fabric's drift warning.
func TestInstallPostCheckoutHook_ChainedWrapperIsExecutable(t *testing.T) {
	t.Parallel()

	f := gitkit.CopyWarpHub(t)
	l, err := lyxcwd.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%q): %v", f.Hub, err)
	}

	hooksDir := resolveCommonHooksDir(t, l.WorktreePath())
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	hookPath := filepath.Join(hooksDir, "post-checkout")

	// A user hook the operator had DISABLED with chmod -x.
	const disabledUserHook = "#!/bin/sh\necho user hook\n"
	if err := os.WriteFile(hookPath, []byte(disabledUserHook), 0o644); err != nil {
		t.Fatalf("seed non-executable user hook: %v", err)
	}
	if err := os.Chmod(hookPath, 0o644); err != nil {
		t.Fatalf("chmod seeded user hook: %v", err)
	}

	if err := InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("InstallPostCheckoutHook: %v", err)
	}

	wrapperInfo, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("stat chained wrapper: %v", err)
	}
	if wrapperInfo.Mode().Perm()&0o111 == 0 {
		t.Errorf("chained wrapper mode = %04o; want an executable mode — git ignores a non-executable hook",
			wrapperInfo.Mode().Perm())
	}

	// The operator's own disable decision must survive: the backup keeps its non-executable mode,
	// and the wrapper guards the call with an executable test so nothing runs it anyway.
	backupInfo, err := os.Stat(hookPath + ".user")
	if err != nil {
		t.Fatalf("stat user hook backup: %v", err)
	}
	if backupInfo.Mode().Perm()&0o111 != 0 {
		t.Errorf("user hook backup mode = %04o; want the original non-executable mode preserved",
			backupInfo.Mode().Perm())
	}

	wrapper, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read chained wrapper: %v", err)
	}
	if !strings.Contains(string(wrapper), `if [ -x "$SCRIPT_DIR/post-checkout.user" ]; then`) {
		t.Errorf("chained wrapper does not guard the user hook with an executable test; content:\n%s", wrapper)
	}
}

// TestInstallPostCheckoutHook_HonoursCoreHooksPath pins that the hook lands where git will actually
// look for it.
// Composing <git-common-dir>/hooks ignores core.hooksPath, so on a repo that sets it fabric wrote
// the hook into a directory git no longer consults and reported success.
func TestInstallPostCheckoutHook_HonoursCoreHooksPath(t *testing.T) {
	t.Parallel()

	f := gitkit.CopyWarpHub(t)
	l, err := lyxcwd.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%q): %v", f.Hub, err)
	}

	customHooksDir := filepath.Join(t.TempDir(), "custom-hooks")
	if err := os.MkdirAll(customHooksDir, 0o755); err != nil {
		t.Fatalf("mkdir custom hooks dir: %v", err)
	}
	cmd := exec.Command("git", "config", "core.hooksPath", customHooksDir)
	cmd.Dir = l.WorktreePath()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config core.hooksPath: %v (%s)", err, out)
	}

	if err := InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("InstallPostCheckoutHook: %v", err)
	}

	installed, err := os.ReadFile(filepath.Join(customHooksDir, "post-checkout"))
	if err != nil {
		t.Fatalf("hook not installed into core.hooksPath dir %q: %v", customHooksDir, err)
	}
	if !strings.Contains(string(installed), hookSentinel) {
		t.Errorf("hook installed into core.hooksPath dir lacks the fabric sentinel; content:\n%s", installed)
	}
}

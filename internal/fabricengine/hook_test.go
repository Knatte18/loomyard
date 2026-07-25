//go:build integration

// hook_test.go covers InstallPostCheckoutHook: idempotent install, non-clobbering
// chain of an existing user hook, and correct weft-sibling branch resolution for
// prime and child worktrees under fabric's suffixed branch-naming scheme.
//
// Adapted from warpengine's hook_test.go. The weft-resolution cases differ from
// warp's: fabric's weft branch is always the host branch plus WeftBranchName's
// "-weft" suffix, never a literal host/weft branch-name match, so each case
// additionally asserts the in-sync (correctly suffixed) state produces no
// warning before diverging it. The child-pair setup uses raw `git worktree add`
// rather than fabricengine's own Add verb, which lands in a later batch.

package fabricengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
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
	l, err := hubgeometry.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(%q): %v", f.Hub, err)
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
	l, err := hubgeometry.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(%q): %v", f.Hub, err)
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
// correctly resolves the <PrimeName>-weft sibling for a prime (main) worktree
// under fabric's suffixed branch scheme: the weft prime must sit on
// WeftBranchName(hostBranch) to be considered in sync, not on a literal
// host-branch-name match. A real git checkout is performed for both the
// in-sync and diverged cases.
func TestInstallPostCheckoutHook_WeftResolution_Prime(t *testing.T) {
	t.Parallel()

	f := lyxtest.CopyPairedLocal(t)
	l, err := hubgeometry.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(%q): %v", f.Hub, err)
	}

	// Install the hook in the shared repo.
	if err := InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("InstallPostCheckoutHook: %v", err)
	}

	// Put the weft prime on the suffixed branch that pairs with host "main"
	// under fabric's uniform scheme — this is the in-sync state.
	lyxtest.MustRun(t, f.WeftPrime, "git", "checkout", "-b", WeftBranchName("main"))

	// Create a scratch branch in the host prime so we have something to switch
	// away from and back to "main".
	lyxtest.MustRun(t, f.Hub, "git", "checkout", "-b", "hook-prime-scratch")

	// Host back to main while weft sits on "main-weft": in sync, no warning.
	cmd := exec.Command("git", "checkout", "main")
	cmd.Dir = f.Hub
	out, _ := cmd.CombinedOutput()
	if strings.Contains(string(out), "fabric:") {
		t.Errorf("unexpected fabric drift warning for in-sync suffixed prime state: %s", out)
	}

	// Diverge: move weft to a branch that is not the suffixed pairing.
	lyxtest.MustRun(t, f.WeftPrime, "git", "checkout", "-b", "hook-prime-weft-side")

	// Switch host away and back to fire the hook again with weft diverged.
	lyxtest.MustRun(t, f.Hub, "git", "checkout", "hook-prime-scratch")
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = f.Hub
	out, _ = cmd.CombinedOutput()

	if !strings.Contains(string(out), "fabric:") {
		t.Logf("hook output: %s", string(out))
		t.Error("expected fabric drift warning for prime worktree; got none")
	}
}

// TestInstallPostCheckoutHook_WeftResolution_Child verifies that the hook script
// correctly resolves the <slug>-weft sibling for a child (non-prime) worktree
// under fabric's suffixed branch scheme. The child worktree pair is created
// directly via `git worktree add` (fabricengine's own Add verb lands in a later
// batch); the weft child's branch is WeftBranchName(slug).
//
// Note: git worktrees cannot check out a branch that is already checked out in
// another worktree. To trigger the hook without hitting that constraint, we create
// an extra branch in the child host and switch between it and slug while the weft
// child stays on a third, non-overlapping branch for the diverged case.
func TestInstallPostCheckoutHook_WeftResolution_Child(t *testing.T) {
	t.Parallel()

	const slug = "hook-child-test"

	f := lyxtest.CopyPairedLocal(t)
	l, err := hubgeometry.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(%q): %v", f.Hub, err)
	}

	// Create a child worktree pair directly via git worktree add — fabricengine's
	// own Add verb lands in a later batch; this test only needs a host/weft
	// worktree pair on disk to exercise the hook script itself.
	childHost := l.WorktreePath(slug)
	lyxtest.MustRun(t, f.Hub, "git", "worktree", "add", childHost, "-b", slug)

	weftBranch := WeftBranchName(slug)
	childWeft := l.WeftWorktreePath(slug)
	lyxtest.MustRun(t, f.WeftPrime, "git", "worktree", "add", childWeft, "-b", weftBranch)

	// Install the hook (affects the shared common .git/hooks for the host repo,
	// which covers the host prime and every host child worktree).
	if err := InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("InstallPostCheckoutHook: %v", err)
	}

	// Create a second branch in the child host so we have two branches to switch
	// between (avoids the git constraint that a branch can only be checked out in
	// one worktree at a time).
	lyxtest.MustRun(t, childHost, "git", "checkout", "-b", "hook-host-alt")

	// Child host back to slug while weft sits on its correctly-suffixed branch:
	// in sync, no warning.
	cmd := exec.Command("git", "checkout", slug)
	cmd.Dir = childHost
	out, _ := cmd.CombinedOutput()
	if strings.Contains(string(out), "fabric:") {
		t.Errorf("unexpected fabric drift warning for in-sync suffixed child state: %s", out)
	}

	// Move the weft child to a different branch — this creates the divergence the
	// hook should detect. The weft branch is independent of the host's branches.
	lyxtest.MustRun(t, childWeft, "git", "checkout", "-b", "hook-weft-diverge")

	// Switch host away and back to fire the hook again with weft diverged.
	lyxtest.MustRun(t, childHost, "git", "checkout", "hook-host-alt")
	cmd = exec.Command("git", "checkout", slug)
	cmd.Dir = childHost
	out, _ = cmd.CombinedOutput()

	if !strings.Contains(string(out), "fabric:") {
		t.Logf("hook output: %s", string(out))
		t.Error("expected fabric drift warning for child worktree; got none")
	}
}

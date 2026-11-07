// hook_test.go covers InstallPostCheckoutHook.
// It checks that a re-install is idempotent, that an existing user hook is chained without being clobbered,
// and that the weft-sibling branch resolves for prime and child worktrees under fabric's suffixed naming scheme.
//
// The weft branch is the warp branch plus RecordsBranchName's "-weft" suffix; it is never matched literally.
// Each case first asserts that the in-sync state, with the weft branch correctly suffixed, produces no warning.
// It then diverges the state and asserts the warning.
// The child-pair setup runs raw `git worktree add` rather than fabricengine's own Add verb, for fixture simplicity.

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// resolveCommonHooksDir returns the common git hooks directory of the repo at repoDir, as an absolute path.
// git reports a relative path, such as ".git" for a standard clone, so the result is resolved against repoDir.
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

// TestInstallPostCheckoutHook_Idempotent checks that a second install leaves the hook file unchanged.
// It must not duplicate the script.
//
//testtiming:keep a second InstallPostCheckoutHook call leaving the script content unchanged and unduplicated; it installs the post-checkout hook script that git runs on checkout, so coverage of its blocks cannot stand in for the assertion
func TestInstallPostCheckoutHook_Idempotent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	hub := h.PrimeWorktree()

	if err := fabricengine.InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("first InstallPostCheckoutHook: %v", err)
	}

	hooksDir := resolveCommonHooksDir(t, hub)
	hookPath := filepath.Join(hooksDir, "post-checkout")

	firstContent, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read hook after first install: %v", err)
	}

	// Second install must be a no-op.
	if err := fabricengine.InstallPostCheckoutHook(l); err != nil {
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
	count := strings.Count(string(secondContent), fabricengine.HookSentinelForTest)
	if count != 1 {
		t.Errorf("sentinel appears %d times after re-install; want exactly 1", count)
	}
}

// TestInstallPostCheckoutHook_ChainIdempotent verifies the chaining of an existing user hook.
// InstallPostCheckoutHook backs the user hook up to post-checkout.user.
// It then writes a wrapper that references the backup, and a second install leaves the wrapper unchanged.
//
//testtiming:keep an existing user hook being backed up to post-checkout.user, chained and left alone by a second install; it installs the post-checkout hook script that git runs on checkout, so coverage of its blocks cannot stand in for the assertion
func TestInstallPostCheckoutHook_ChainIdempotent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const userHookContent = "#!/bin/sh\necho user\n"

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	hub := h.PrimeWorktree()

	// Plant a user hook.
	hooksDir := resolveCommonHooksDir(t, hub)
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	hookPath := filepath.Join(hooksDir, "post-checkout")
	if err := os.WriteFile(hookPath, []byte(userHookContent), 0o755); err != nil {
		t.Fatalf("write user hook: %v", err)
	}

	// First install — chains the user hook.
	if err := fabricengine.InstallPostCheckoutHook(l); err != nil {
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
	if err := fabricengine.InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("second InstallPostCheckoutHook: %v", err)
	}

	secondChainContent, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read second chain: %v", err)
	}

	if string(firstChainContent) != string(secondChainContent) {
		t.Errorf("chained hook changed on re-install; first=%q second=%q", firstChainContent, secondChainContent)
	}

	// A branch checkout fires the wrapper, which must find and run the executable user hook beside it.
	cmd := exec.Command("git", "checkout", "-b", "hook-chain-probe")
	cmd.Dir = hub
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git checkout -b hook-chain-probe: %v (%s)", err, out)
	}
	if !strings.Contains(string(out), "user\n") {
		t.Errorf("checkout output = %q; want the chained user hook's output", out)
	}
}

// TestInstallPostCheckoutHook_WeftResolution_Prime verifies that the hook resolves the prime worktree's weft sibling.
// Under the suffixed branch scheme, the weft prime is in sync when it sits on RecordsBranchName(warpBranch).
// A literal warp-branch-name match does not count.
// A real git checkout is performed for both the in-sync and diverged cases.
//
//testtiming:keep the installed hook resolving the <PrimeName>-weft sibling for a prime worktree under the suffixed branch scheme, in sync and diverged; it installs the post-checkout hook script that git runs on checkout, so coverage of its blocks cannot stand in for the assertion
func TestInstallPostCheckoutHook_WeftResolution_Prime(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	hub := h.PrimeWorktree()
	weftPrime := h.PrimeRecords()

	// Install the hook in the shared repo.
	if err := fabricengine.InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("InstallPostCheckoutHook: %v", err)
	}

	// A real hub's weft prime is wired onto RecordsBranchName("main") by CloneAndWire, so it starts in sync.
	// Confirm that invariant instead of reconstructing it by hand.
	if branch := gitkit.CurrentBranch(t, weftPrime); branch != fabricengine.RecordsBranchName("main") {
		t.Fatalf("weft prime branch = %q; want %q (already wired by CloneAndWire)", branch, fabricengine.RecordsBranchName("main"))
	}

	// Create a scratch branch in the warp prime to switch away from and back to "main".
	gitkit.MustRun(t, hub, "git", "checkout", "-b", "hook-prime-scratch")

	// Warp back to main while weft sits on "main-weft": in sync, no warning.
	cmd := exec.Command("git", "checkout", "main")
	cmd.Dir = hub
	out, _ := cmd.CombinedOutput()
	if strings.Contains(string(out), "fabric:") {
		t.Errorf("unexpected fabric drift warning for in-sync suffixed prime state: %s", out)
	}

	// Diverge: move weft to a branch that is not the suffixed pairing.
	gitkit.MustRun(t, weftPrime, "git", "checkout", "-b", "hook-prime-weft-side")

	// Switch warp away and back to fire the hook again with weft diverged.
	gitkit.MustRun(t, hub, "git", "checkout", "hook-prime-scratch")
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = hub
	out, _ = cmd.CombinedOutput()

	if !strings.Contains(string(out), "fabric:") {
		t.Logf("hook output: %s", string(out))
		t.Error("expected fabric drift warning for prime worktree; got none")
	}
}

// TestInstallPostCheckoutHook_WeftResolution_Child verifies that the hook resolves the child's weft sibling.
// The test creates the child worktree pair with `git worktree add`.
// The weft child's branch is RecordsBranchName(slug).
//
// Git worktrees cannot check out a branch that another worktree already has checked out.
// The test works around that by switching the child warp between slug and an extra branch.
// The weft child stays on its own branch, and the diverged case moves it to another branch.
//
//testtiming:keep the installed hook resolving the <slug>-weft sibling for a child worktree, in sync and diverged; it installs the post-checkout hook script that git runs on checkout, so coverage of its blocks cannot stand in for the assertion
func TestInstallPostCheckoutHook_WeftResolution_Child(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "hook-child-test"

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	hub := h.PrimeWorktree()
	weftPrime := h.PrimeRecords()

	// Create the child worktree pair with git worktree add.
	// The hook script only needs the pair on disk to run.
	childWarp := h.PairCodeWorktree(slug)
	gitkit.MustRun(t, hub, "git", "worktree", "add", childWarp, "-b", slug)

	weftBranch := fabricengine.RecordsBranchName(slug)
	childWeft := h.PairRecordsSibling(slug)
	gitkit.MustRun(t, weftPrime, "git", "worktree", "add", childWeft, "-b", weftBranch)

	// Install the hook in the common .git/hooks of the warp repo.
	// That covers the warp prime and every warp child worktree.
	if err := fabricengine.InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("InstallPostCheckoutHook: %v", err)
	}

	// Create a second branch in the child warp to switch between.
	// A branch can be checked out in only one worktree at a time, so the switch needs two branches.
	gitkit.MustRun(t, childWarp, "git", "checkout", "-b", "hook-warp-alt")

	// Child warp back to slug while weft sits on its correctly-suffixed branch:
	// in sync, no warning.
	cmd := exec.Command("git", "checkout", slug)
	cmd.Dir = childWarp
	out, _ := cmd.CombinedOutput()
	if strings.Contains(string(out), "fabric:") {
		t.Errorf("unexpected fabric drift warning for in-sync suffixed child state: %s", out)
	}

	// Move the weft child to a different branch, which creates the divergence the hook should detect.
	// The weft branch is independent of the warp's branches.
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

// TestInstallPostCheckoutHook_ChainedWrapperIsExecutable pins the executable bit on the chained wrapper.
// os.WriteFile applies its perm argument only when it creates the file.
// Chaining around an existing non-executable user hook must still leave the wrapper executable:
// otherwise git ignores the hook, and both the user's hook and fabric's drift warning stop running.
//
//testtiming:keep the chained wrapper carrying the executable bit so git runs both hooks; it installs the post-checkout hook script that git runs on checkout, so coverage of its blocks cannot stand in for the assertion
func TestInstallPostCheckoutHook_ChainedWrapperIsExecutable(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	hooksDir := resolveCommonHooksDir(t, l.WorktreePath())
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	hookPath := filepath.Join(hooksDir, "post-checkout")

	// A user hook the operator disabled with chmod -x.
	const disabledUserHook = "#!/bin/sh\necho user hook\n"
	if err := os.WriteFile(hookPath, []byte(disabledUserHook), 0o644); err != nil {
		t.Fatalf("seed non-executable user hook: %v", err)
	}
	if err := os.Chmod(hookPath, 0o644); err != nil {
		t.Fatalf("chmod seeded user hook: %v", err)
	}

	if err := fabricengine.InstallPostCheckoutHook(l); err != nil {
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

// TestInstallPostCheckoutHook_HonoursCoreHooksPath checks that the hook lands where git looks for it.
// Composing <git-common-dir>/hooks ignores core.hooksPath.
// On a repo that sets it, the hook lands in a directory git does not consult, and fabric reports success.
//
//testtiming:keep the hook landing where core.hooksPath points; it installs the post-checkout hook script that git runs on checkout, so coverage of its blocks cannot stand in for the assertion
func TestInstallPostCheckoutHook_HonoursCoreHooksPath(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	customHooksDir := filepath.Join(t.TempDir(), "custom-hooks")
	if err := os.MkdirAll(customHooksDir, 0o755); err != nil {
		t.Fatalf("mkdir custom hooks dir: %v", err)
	}
	cmd := exec.Command("git", "config", "core.hooksPath", customHooksDir)
	cmd.Dir = l.WorktreePath()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config core.hooksPath: %v (%s)", err, out)
	}

	if err := fabricengine.InstallPostCheckoutHook(l); err != nil {
		t.Fatalf("InstallPostCheckoutHook: %v", err)
	}

	installed, err := os.ReadFile(filepath.Join(customHooksDir, "post-checkout"))
	if err != nil {
		t.Fatalf("hook not installed into core.hooksPath dir %q: %v", customHooksDir, err)
	}
	if !strings.Contains(string(installed), fabricengine.HookSentinelForTest) {
		t.Errorf("hook installed into core.hooksPath dir lacks the fabric sentinel; content:\n%s", installed)
	}
}

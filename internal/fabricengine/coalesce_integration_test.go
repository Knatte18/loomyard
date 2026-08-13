//go:build integration

// coalesce_integration_test.go — integration coverage for CoalescePushBothAt
// against real warp+weft repos with bare origins: the two-sided-advance case
// (including the no-warp-root-.gitrepo-push.lock assertion), and the
// diverged-warp-remote case that must return nil (not an error) without
// spinning. Package fabricengine (internal), reusing this package's existing
// fixture helpers (newPlainWarpRepo, currentSHA, newFabric from
// index_integration_test.go; seedFabricConfig/writeWarpFile from
// commit_integration_test.go) plus gitkit for bare-remote setup.

package fabricengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// addWarpBareRemote creates a bare git repository at <dir>/warp-remote.git,
// adds it to warpPath as "origin", and pushes -u to establish upstream
// tracking on "main" — the state CoalescePushBothAt's warp side needs before
// it has anything to push against. Returns the bare repo's path.
func addWarpBareRemote(t *testing.T, dir, warpPath string) string {
	t.Helper()

	bare := filepath.Join(dir, "warp-remote.git")
	gitkit.MustRun(t, dir, "git", "init", "--bare", "-b", "main", bare)
	gitkit.MustRun(t, warpPath, "git", "remote", "add", "origin", filepath.ToSlash(bare))
	gitkit.MustRun(t, warpPath, "git", "push", "-u", "origin", "main")
	return bare
}

// commitPlain writes name inside dir with content and commits it directly,
// with no trailer — the weft-side counterpart of commitWarp for tests that
// only need a plain, unpushed commit and do not care about Fabric.Commit's
// trailer/correspondence machinery. Returns the new HEAD SHA.
func commitPlain(t *testing.T, dir, name, content string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, dir, "git", "add", name)
	gitkit.MustRun(t, dir, "git", "commit", "-q", "-m", content)
	return currentSHA(t, dir)
}

// bareBranchSHA returns the SHA that branch points to inside the bare repo at
// bareDir.
func bareBranchSHA(t *testing.T, bareDir, branch string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", branch)
	cmd.Dir = bareDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s in %s: %v", branch, bareDir, err)
	}
	return strings.TrimSpace(string(out))
}

// runWithDeadline runs fn in a goroutine and fails the test if it has not
// returned within the given deadline — the promptness assertion the
// diverged-remote test needs: a spinning CoalescePushBothAt call would never
// reach the select's success branch.
func runWithDeadline(t *testing.T, deadline time.Duration, fn func() error) error {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(deadline):
		t.Fatalf("call did not return within %s; want it to complete promptly (no spin)", deadline)
		return nil
	}
}

// TestCoalescePushBothAt_AdvancesBothSidesAndLeavesNoWarpRootLock covers the straightforward
// two-sided case: an unpushed commit on each side,
// and the no-warp-root-gitrepo-push-lock Shared Decision's assertion that the warp-via-fabric path
// never leaves a .gitrepo-push.lock at the warp worktree root.
func TestCoalescePushBothAt_AdvancesBothSidesAndLeavesNoWarpRootLock(t *testing.T) {
	fixtures := t.TempDir()

	warpPath := newPlainWarpRepo(t)
	warpBare := addWarpBareRemote(t, fixtures, warpPath)
	warpSHA := commitPlain(t, warpPath, "warp-file.txt", "warp change")

	weftFixture := gitkit.CopyWeft(t)
	weftSHA := commitPlain(t, weftFixture.WeftPath, "weft-file.txt", "weft change")

	if _, err := CoalescePushBothAt(warpPath, weftFixture.WeftPath, SyncOptions{}); err != nil {
		t.Fatalf("CoalescePushBothAt() error = %v; want nil", err)
	}

	if got := bareBranchSHA(t, warpBare, "main"); got != warpSHA {
		t.Errorf("warp bare main = %q; want it advanced to local HEAD %q", got, warpSHA)
	}
	if got := bareBranchSHA(t, weftFixture.Bare, "main"); got != weftSHA {
		t.Errorf("weft bare main = %q; want it advanced to local HEAD %q", got, weftSHA)
	}

	if _, err := os.Stat(filepath.Join(warpPath, gitrepo.PushLockFileName)); !os.IsNotExist(err) {
		t.Errorf("%s exists at warp worktree root after CoalescePushBothAt(); want it absent (rebase-free, lock-free-per-side push)", gitrepo.PushLockFileName)
	}
}

// TestCoalescePushBothAt_DivergedWarpRemote_ReturnsNilWithoutSpinning covers the rejection path: a
// second warp clone pushes a commit the original checkout lacks, so the warp side's push is a
// non-fast-forward rejection.
// CoalescePushBothAt must return nil (not an error), leave the warp bare's HEAD unadvanced by this
// call, mutate no warp working-tree file, and return promptly rather than spinning.
func TestCoalescePushBothAt_DivergedWarpRemote_ReturnsNilWithoutSpinning(t *testing.T) {
	fixtures := t.TempDir()

	warpPath := newPlainWarpRepo(t)
	warpBare := addWarpBareRemote(t, fixtures, warpPath)

	// A second clone advances the bare remote past what warpPath's checkout
	// has, setting up the divergence: warpPath's next push will be rejected.
	warpClone2 := filepath.Join(fixtures, "warp-clone-2")
	gitkit.MustRun(t, fixtures, "git", "clone", warpBare, warpClone2)
	gitkit.MustRun(t, warpClone2, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, warpClone2, "git", "config", "user.name", "Test")
	commitPlain(t, warpClone2, "other.txt", "from second clone")
	gitkit.MustRun(t, warpClone2, "git", "push")

	// warpPath now commits locally too, without ever having fetched the
	// second clone's commit — its next push is a genuine non-fast-forward.
	commitPlain(t, warpPath, "warp-file.txt", "warp change that will be rejected")

	weftFixture := gitkit.CopyWeft(t)

	bareHeadBefore := bareBranchSHA(t, warpBare, "main")
	wantWarpFileContent, err := os.ReadFile(filepath.Join(warpPath, "warp-file.txt"))
	if err != nil {
		t.Fatalf("read warp-file.txt before call: %v", err)
	}

	callErr := runWithDeadline(t, 30*time.Second, func() error {
		_, err := CoalescePushBothAt(warpPath, weftFixture.WeftPath, SyncOptions{})
		return err
	})
	if callErr != nil {
		t.Fatalf("CoalescePushBothAt() error = %v; want nil (a rejected push is not a genuine failure)", callErr)
	}

	if got := bareBranchSHA(t, warpBare, "main"); got != bareHeadBefore {
		t.Errorf("warp bare main = %q; want it unadvanced at %q (rejected push must not partially land)", got, bareHeadBefore)
	}
	gotWarpFileContent, err := os.ReadFile(filepath.Join(warpPath, "warp-file.txt"))
	if err != nil {
		t.Fatalf("read warp-file.txt after call: %v", err)
	}
	if string(gotWarpFileContent) != string(wantWarpFileContent) {
		t.Errorf("warp-file.txt content after call = %q; want unchanged %q (no pull --rebase ever runs)", gotWarpFileContent, wantWarpFileContent)
	}
	for _, rebaseDir := range []string{"rebase-merge", "rebase-apply"} {
		if _, statErr := os.Stat(filepath.Join(warpPath, ".git", rebaseDir)); statErr == nil {
			t.Errorf(".git/%s exists after CoalescePushBothAt(); want no rebase ever attempted", rebaseDir)
		}
	}
}

// TestCoalescePushBothAt_EmptyWarpPath_PushesWeftFromUnrelatedCwd covers the weft-only "lyx fabric
// sync" production path (warpPath == "", the shape spawnPush/SpawnDetachedPush always uses per
// fabriccli/spawn.go): with the detached child's cwd left at some unrelated, non-git directory
// (nothing overrides cmd.Dir), CoalescePushBothAt must still push the weft side and return nil —
// not open (or fail to open) a git checkout at the inherited cwd for the absent warp side.
// Before the headOrEmpty("") short-circuit fix, this call errored (aborting the loop before the
// weft push ever ran) because gitrepo.New("").CurrentSHA() resolves "" to the process's cwd via
// filepath.Abs and found no .git there.
func TestCoalescePushBothAt_EmptyWarpPath_PushesWeftFromUnrelatedCwd(t *testing.T) {
	unrelatedCwd := t.TempDir()

	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(unrelatedCwd); err != nil {
		t.Fatalf("Chdir(%s): %v", unrelatedCwd, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origCwd); err != nil {
			t.Fatalf("restore Chdir(%s): %v", origCwd, err)
		}
	})

	weftFixture := gitkit.CopyWeft(t)
	weftSHA := commitPlain(t, weftFixture.WeftPath, "weft-file.txt", "weft change, no warp")

	if _, err := CoalescePushBothAt("", weftFixture.WeftPath, SyncOptions{}); err != nil {
		t.Fatalf("CoalescePushBothAt(\"\", ...) error = %v; want nil (empty warpPath must be a true no-op, not a cwd-relative git open)", err)
	}

	if got := bareBranchSHA(t, weftFixture.Bare, "main"); got != weftSHA {
		t.Errorf("weft bare main = %q; want it advanced to local HEAD %q", got, weftSHA)
	}
}

// coalesce_integration_test.go holds integration tests for CoalescePushBothAt.
// They run against real warp and weft repos with bare origins.
// They cover a two-sided advance, which leaves no push lock at the warp root.
// They also cover a diverged warp remote: CoalescePushBothAt must return nil without spinning.
// The file is in package fabricengine_test:
// it drives the warp/bare fixture through the ForTest shims in export_test.go and through gitkit for bare-remote setup.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// addWarpBareRemote creates a bare git repository at <dir>/warp-remote.git,
// adds it to warpPath as "origin", and pushes -u to set upstream tracking on "main".
// The upstream lets a later plain `git push` from warpPath reach the bare repository.
// It returns the bare repo's path.
func addWarpBareRemote(t *testing.T, dir, warpPath string) string {
	t.Helper()

	bare := filepath.Join(dir, "warp-remote.git")
	gitkit.MustRun(t, dir, "git", "init", "--bare", "-b", "main", bare)
	gitkit.MustRun(t, warpPath, "git", "remote", "add", "origin", filepath.ToSlash(bare))
	gitkit.MustRun(t, warpPath, "git", "push", "-u", "origin", "main")
	return bare
}

// runWithDeadline runs fn in a goroutine and fails the test if it has not returned within deadline.
// A spinning call never returns, so the deadline is what makes such a hang fail the test promptly.
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

// TestCoalescePushBothAt_AdvancesBothSidesAndLeavesNoWarpRootLock covers an unpushed commit on each side.
// It also asserts that the warp-via-fabric path never leaves a push lock file at the warp worktree root.
//
//testtiming:keep both sides advancing and no push lock being left at the warp root; coverage of its blocks by other tests does not show an assertion of this
func TestCoalescePushBothAt_AdvancesBothSidesAndLeavesNoWarpRootLock(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	warpBare := addWarpBareRemote(t, fixtures, warpPath)
	warpSHA := gitkit.CommitFile(t, warpPath, "warp-file.txt", "warp change", "warp change")

	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	weftSHA := gitkit.CommitFile(t, weftFixture.PrimeRecords(), "weft-file.txt", "weft change", "weft change")

	if _, err := fabricengine.CoalescePushBothAt(warpPath, weftFixture.PrimeRecords(), fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("fabricengine.CoalescePushBothAt() error = %v; want nil", err)
	}

	if got := fabricengine.BareBranchSHAForTest(t, warpBare, "main"); got != warpSHA {
		t.Errorf("warp bare main = %q; want it advanced to local HEAD %q", got, warpSHA)
	}
	if got := fabricengine.BareBranchSHAForTest(t, weftFixture.RecordsBare, fabricengine.RecordsBranchName("main")); got != weftSHA {
		t.Errorf("weft bare main = %q; want it advanced to local HEAD %q", got, weftSHA)
	}

	if _, err := os.Stat(filepath.Join(warpPath, gitrepo.PushLockFileName)); !os.IsNotExist(err) {
		t.Errorf("%s exists at warp worktree root after fabricengine.CoalescePushBothAt(); want it absent (rebase-free, lock-free-per-side push)", gitrepo.PushLockFileName)
	}
}

// TestCoalescePushBothAt_DivergedWarpRemote_ReturnsNilWithoutSpinning covers the rejection path.
// A second warp clone pushes a commit that the original checkout lacks,
// so the warp side's push is a non-fast-forward rejection.
// CoalescePushBothAt must return nil and leave the warp bare's HEAD unadvanced by this call.
// It must mutate no warp working-tree file and return promptly rather than spinning.
func TestCoalescePushBothAt_DivergedWarpRemote_ReturnsNilWithoutSpinning(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	warpBare := addWarpBareRemote(t, fixtures, warpPath)

	// A second clone advances the bare remote past warpPath's checkout.
	// The next push from warpPath is then rejected.
	warpClone2 := filepath.Join(fixtures, "warp-clone-2")
	gitkit.MustRun(t, fixtures, "git", "clone", warpBare, warpClone2)
	gitkit.Git(t, warpClone2, "config", "user.email", "test@test.com")
	gitkit.Git(t, warpClone2, "config", "user.name", "Test")
	gitkit.CommitFile(t, warpClone2, "other.txt", "from second clone", "from second clone")
	gitkit.MustRun(t, warpClone2, "git", "push")

	// warpPath commits locally too, without fetching the second clone's commit,
	// so its next push is a non-fast-forward.
	gitkit.CommitFile(t, warpPath, "warp-file.txt", "warp change that will be rejected", "warp change that will be rejected")

	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	bareHeadBefore := fabricengine.BareBranchSHAForTest(t, warpBare, "main")
	wantWarpFileContent, err := os.ReadFile(filepath.Join(warpPath, "warp-file.txt"))
	if err != nil {
		t.Fatalf("read warp-file.txt before call: %v", err)
	}

	callErr := runWithDeadline(t, 30*time.Second, func() error {
		_, err := fabricengine.CoalescePushBothAt(warpPath, weftFixture.PrimeRecords(), fabricengine.SyncOptions{})
		return err
	})
	if callErr != nil {
		t.Fatalf("fabricengine.CoalescePushBothAt() error = %v; want nil (a rejected push is not a genuine failure)", callErr)
	}
	records, err := fabricengine.ListFailedPushes(weftFixture.Location.HubPath)
	if err != nil || len(records) != 1 || records[0].Repo != "code" || records[0].Branch != "main" || records[0].Class != fabricengine.PushFailureNotFastForward {
		t.Errorf("ListFailedPushes() = %+v, %v; want the one not-fast-forward code record of branch main", records, err)
	}

	if got := fabricengine.BareBranchSHAForTest(t, warpBare, "main"); got != bareHeadBefore {
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
			t.Errorf(".git/%s exists after fabricengine.CoalescePushBothAt(); want no rebase ever attempted", rebaseDir)
		}
	}
}

// TestCoalescePushBothAt_EmptyWarpPath_PushesWeftFromUnrelatedCwd covers the weft-only detached push.
// The warp path is empty, the shape a caller of SpawnDetachedPush gets by passing no warp path.
// CoalescePushBothAt must still push the weft side and return nil when the child's cwd is a non-git directory;
// it must not open a git checkout at the inherited cwd for the absent warp side.
// gitrepo.New("").CurrentSHA() resolves "" to the process cwd through filepath.Abs, where no .git exists.
func TestCoalescePushBothAt_EmptyWarpPath_PushesWeftFromUnrelatedCwd(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: os.Chdir changes the process-global cwd.
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

	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	weftSHA := gitkit.CommitFile(t, weftFixture.PrimeRecords(), "weft-file.txt", "weft change, no warp", "weft change, no warp")

	if _, err := fabricengine.CoalescePushBothAt("", weftFixture.PrimeRecords(), fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("fabricengine.CoalescePushBothAt(\"\", ...) error = %v; want nil (empty warpPath must be a true no-op, not a cwd-relative git open)", err)
	}

	if got := fabricengine.BareBranchSHAForTest(t, weftFixture.RecordsBare, fabricengine.RecordsBranchName("main")); got != weftSHA {
		t.Errorf("weft bare main = %q; want it advanced to local HEAD %q", got, weftSHA)
	}
}

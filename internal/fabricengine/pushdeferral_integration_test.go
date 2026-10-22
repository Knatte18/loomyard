//go:build integration

// pushdeferral_integration_test.go covers a detached push child whose first gated push meets a covering pause, over plain code and records repos with local bare origins.

package fabricengine

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestCoalescePushRequest_CoveredCallDefers covers a detached child whose first gated push meets a covering pause:
// the exit runs once at that push, both pending flags are back on disk, origin holds neither commit and no failed push is recorded.
// Serial: it installs the process's network gate and swaps the deferral's exit.
func TestCoalescePushRequest_CoveredCallDefers(t *testing.T) {
	// withBareOrigin gives a plain repo a bare origin holding its main and returns the origin's path.
	withBareOrigin := func(repo string) string {
		bare := newBoltBareRemote(t, t.TempDir())
		gitkit.MustRun(t, repo, "git", "remote", "add", "origin", filepath.ToSlash(bare))
		gitkit.MustRun(t, repo, "git", "push", "-u", "origin", "main")
		return bare
	}
	warpPath := newPlainWarpRepo(t)
	warpBare := withBareOrigin(warpPath)
	weftPath := newPlainWarpRepo(t)
	weftBare := withBareOrigin(weftPath)
	warpBefore := BareBranchSHAForTest(t, warpBare, "main")
	weftBefore := BareBranchSHAForTest(t, weftBare, "main")
	gitkit.CommitFile(t, warpPath, "warp-file.txt", "warp change", "warp change")
	gitkit.CommitFile(t, weftPath, "weft-file.txt", "weft change", "weft change")

	HoldHubPauseForTest(t, "")
	exits := 0
	previousExit := exitDeferred
	exitDeferred = func() { exits++; runtime.Goexit() }
	t.Cleanup(func() { exitDeferred = previousExit })

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		CoalescePushRequest(PushSpawn{WarpPath: warpPath, WeftPath: weftPath}, SyncOptions{}) //nolint:errcheck
	}()
	<-finished

	if exits != 1 {
		t.Errorf("deferral exits = %d; want 1, at the first gated push", exits)
	}
	lockDir := RecordsLockDirPath(weftPath)
	for _, side := range []string{pushSideCode, pushSideRecords} {
		if _, err := os.Stat(pendingFlagPath(lockDir, side)); err != nil {
			t.Errorf("pending flag of %s: %v; want it back on disk", side, err)
		}
	}
	if got := BareBranchSHAForTest(t, warpBare, "main"); got != warpBefore {
		t.Errorf("warp bare main = %q; want it unchanged at %q", got, warpBefore)
	}
	if got := BareBranchSHAForTest(t, weftBare, "main"); got != weftBefore {
		t.Errorf("weft bare main = %q; want it unchanged at %q", got, weftBefore)
	}
	if failed, err := ListFailedPushes(filepath.Dir(weftPath)); err != nil || len(failed) != 0 {
		t.Errorf("ListFailedPushes() = (%+v, %v); want none", failed, err)
	}
}

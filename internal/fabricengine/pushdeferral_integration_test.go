// pushdeferral_integration_test.go covers the order of a detached push child's two sides.
// It also covers a child that meets a covering pause, and a push a pause holds before it takes the push lock.
// It runs over plain code and records repos with local bare origins.

package fabricengine

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// withBareOrigin gives a plain repo a bare origin holding its main and returns the origin's path.
func withBareOrigin(t *testing.T, repo string) string {
	t.Helper()
	bare := newBoltBareRemote(t, t.TempDir())
	gitkit.MustRun(t, repo, "git", "remote", "add", "origin", filepath.ToSlash(bare))
	gitkit.MustRun(t, repo, "git", "push", "-u", "origin", "main")
	return bare
}

// newChangedPair returns a code and a records repo, each with a bare origin and one unpushed commit.
func newChangedPair(t *testing.T) (warpPath, warpBare, weftPath, weftBare string) {
	t.Helper()
	warpPath = newPlainWarpRepo(t)
	warpBare = withBareOrigin(t, warpPath)
	weftPath = newPlainWarpRepo(t)
	weftBare = withBareOrigin(t, weftPath)
	gitkit.CommitFile(t, warpPath, "warp-file.txt", "warp change", "warp change")
	gitkit.CommitFile(t, weftPath, "weft-file.txt", "weft change", "weft change")
	return warpPath, warpBare, weftPath, weftBare
}

// pushLockFree reports whether another process could take the push lock of the records repo at weftPath now.
// It takes the lock by one try and releases it at once.
func pushLockFree(t *testing.T, weftPath string) bool {
	t.Helper()
	path := filepath.Join(RecordsLockDirPath(weftPath), weftPushLockFile)
	held, acquired, err := lock.TryAcquireWriteLock(path)
	if err != nil {
		t.Errorf("TryAcquireWriteLock(%s) error = %v", path, err)
		return false
	}
	if acquired {
		_ = held.Release()
	}
	return acquired
}

// TestCoalescePushRequest_CoveredCallDefers covers a detached child over a code and a records repo with bare origins.
// A covered child defers at the pause gate, before it takes the push lock, with the lock free.
// The exit runs once, and both pending flags are back on disk.
// Origin holds neither commit, and no failed push is recorded.
// An uncovered child pushes the records side before the code side.
// Serial: the covered row installs the process's network gate and swaps the deferral's exit.
func TestCoalescePushRequest_CoveredCallDefers(t *testing.T) {
	tiergate.Require(t, 1)

	t.Run("CoveredCallDefersBeforeThePushLock", func(t *testing.T) {
		warpPath, warpBare, weftPath, weftBare := newChangedPair(t)
		warpBefore := BareBranchSHAForTest(t, warpBare, "main")
		weftBefore := BareBranchSHAForTest(t, weftBare, "main")

		HoldHubPauseForTest(t, "")
		exits := 0
		lockFreeAtExit := false
		previousExit := exitDeferred
		exitDeferred = func() {
			exits++
			lockFreeAtExit = pushLockFree(t, weftPath)
			runtime.Goexit()
		}
		t.Cleanup(func() { exitDeferred = previousExit })

		finished := make(chan struct{})
		go func() {
			defer close(finished)
			CoalescePushRequest(PushSpawn{WarpPath: warpPath, WeftPath: weftPath}, SyncOptions{}, MachineNames{}) //nolint:errcheck
		}()
		<-finished

		if exits != 1 || !lockFreeAtExit {
			t.Errorf("deferral exits = %d with the push lock free %v; want 1, at the gate before the lock is taken", exits, lockFreeAtExit)
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
	})

	t.Run("AMissedAcquireLeavesNoAdmissionBehind", func(t *testing.T) {
		warpPath, _, weftPath, _ := newChangedPair(t)
		lockDir, err := ensureWeftLockDirAt(weftPath)
		if err != nil {
			t.Fatalf("ensureWeftLockDirAt(%s): %v", weftPath, err)
		}
		holder, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftPushLockFile))
		if err != nil {
			t.Fatalf("AcquireWriteLock: %v", err)
		}
		defer func() { _ = holder.Release() }()
		cover, waits := PauseGateForTest(t, nil)

		if _, err := CoalescePushRequest(PushSpawn{WarpPath: warpPath, WeftPath: weftPath}, SyncOptions{}, MachineNames{}); err != nil {
			t.Fatalf("CoalescePushRequest() with the push lock held error = %v; want nil", err)
		}
		cover()
		_, done := pauserecord.Network("probe")
		done()

		if *waits != 1 {
			t.Errorf("gate waits of a network call after the missed acquire = %d; want 1, so no admission was left standing", *waits)
		}
	})

	t.Run("UncoveredCallPushesTheRecordsSideBeforeTheCodeSide", func(t *testing.T) {
		warpPath, warpBare, weftPath, weftBare := newChangedPair(t)
		order := filepath.Join(t.TempDir(), "push-order.log")
		for side, bare := range map[string]string{"records": weftBare, "code": warpBare} {
			hook := "#!/bin/sh\necho " + side + " >> '" + filepath.ToSlash(order) + "'\n"
			if err := os.WriteFile(filepath.Join(bare, "hooks", "pre-receive"), []byte(hook), 0o755); err != nil {
				t.Fatalf("write the %s pre-receive hook: %v", side, err)
			}
		}

		if _, err := CoalescePushRequest(PushSpawn{WarpPath: warpPath, WeftPath: weftPath}, SyncOptions{}, MachineNames{}); err != nil {
			t.Fatalf("CoalescePushRequest() error = %v; want nil", err)
		}

		logged, err := os.ReadFile(order)
		if err != nil {
			t.Fatalf("read the push order: %v", err)
		}
		if got, want := strings.Fields(string(logged)), []string{"records", "code"}; !slices.Equal(got, want) {
			t.Errorf("origins received pushes in the order %q; want %q", got, want)
		}
	})
}

// TestCoalescePushBothAt_HeldByAPauseWithoutThePushLock covers a push a pause holds.
// It runs over a code and a records repo with bare origins.
// While the pause holds the call, the push lock is free, so another pair's push takes it meanwhile.
// Once the pause lifts, the call takes the lock and pushes both sides.
// Serial: it installs the process's network gate.
func TestCoalescePushBothAt_HeldByAPauseWithoutThePushLock(t *testing.T) {
	tiergate.Require(t, 1)
	warpPath, warpBare, weftPath, weftBare := newChangedPair(t)
	cover, waits := PauseGateForTest(t, func() {
		if !pushLockFree(t, weftPath) {
			t.Errorf("the push lock was held while the pause held the call; want it free for another pair's push")
		}
	})
	cover()

	if _, err := CoalescePushBothAt(warpPath, weftPath, SyncOptions{}); err != nil {
		t.Fatalf("CoalescePushBothAt() error = %v; want nil", err)
	}

	if *waits != 1 {
		t.Errorf("gate waits = %d; want 1, before the push lock", *waits)
	}
	for _, side := range []struct{ repo, bare string }{{warpPath, warpBare}, {weftPath, weftBare}} {
		head, err := headOrEmpty(side.repo)
		if err != nil {
			t.Fatalf("headOrEmpty(%s): %v", side.repo, err)
		}
		if got := BareBranchSHAForTest(t, side.bare, "main"); got != head {
			t.Errorf("bare main of %s = %q; want the pushed head %q", side.repo, got, head)
		}
	}
}

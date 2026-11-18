// pushdeferral_integration_test.go covers the order of a detached push child's two sides,
// and a child whose first gated push meets a covering pause;
// it runs over plain code and records repos with local bare origins.

package fabricengine

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCoalescePushRequest_CoveredCallDefers covers a detached child over a code and a records repo with bare origins.
// A child whose first gated push meets a covering pause defers at the records side's push, the first side pushed.
// The exit runs once, and both pending flags are back on disk.
// Origin holds neither commit, and no failed push is recorded.
// An uncovered child pushes the records side before the code side.
// Serial: the covered row installs the process's network gate and swaps the deferral's exit.
func TestCoalescePushRequest_CoveredCallDefers(t *testing.T) {
	tiergate.Require(t, 1)
	// withBareOrigin gives a plain repo a bare origin holding its main and returns the origin's path.
	withBareOrigin := func(t *testing.T, repo string) string {
		bare := newBoltBareRemote(t, t.TempDir())
		gitkit.MustRun(t, repo, "git", "remote", "add", "origin", filepath.ToSlash(bare))
		gitkit.MustRun(t, repo, "git", "push", "-u", "origin", "main")
		return bare
	}
	// newChangedPair returns a code and a records repo, each with a bare origin and one unpushed commit.
	newChangedPair := func(t *testing.T) (warpPath, warpBare, weftPath, weftBare string) {
		warpPath = newPlainWarpRepo(t)
		warpBare = withBareOrigin(t, warpPath)
		weftPath = newPlainWarpRepo(t)
		weftBare = withBareOrigin(t, weftPath)
		gitkit.CommitFile(t, warpPath, "warp-file.txt", "warp change", "warp change")
		gitkit.CommitFile(t, weftPath, "weft-file.txt", "weft change", "weft change")
		return warpPath, warpBare, weftPath, weftBare
	}

	t.Run("CoveredCallDefersAtTheRecordsPush", func(t *testing.T) {
		warpPath, warpBare, weftPath, weftBare := newChangedPair(t)
		warpBefore := BareBranchSHAForTest(t, warpBare, "main")
		weftBefore := BareBranchSHAForTest(t, weftBare, "main")

		HoldHubPauseForTest(t, "")
		exits := 0
		previousExit := exitDeferred
		exitDeferred = func() { exits++; runtime.Goexit() }
		t.Cleanup(func() { exitDeferred = previousExit })

		finished := make(chan struct{})
		go func() {
			defer close(finished)
			CoalescePushRequest(PushSpawn{WarpPath: warpPath, WeftPath: weftPath}, SyncOptions{}, MachineNames{}) //nolint:errcheck
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

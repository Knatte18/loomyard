//go:build tier0

// teardownlock_test.go covers DetachedPush's exit under a pause, which needs no git and no real hub:
// the worktree paths it is given do not exist, so a child that goes on past the pause check stops at the gone-worktree check.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestDetachedPush_CoveredChildExitsBeforeItsFlags covers a covered child that was not spawned under its pair's parent lock:
// it logs the deferral and returns before it takes the teardown lock, and records no failed push.
// A parent-locked child skips the covered check, which the teardown lock file it then writes shows.
// Serial: HoldHubPauseForTest installs the process's network gate.
func TestDetachedPush_CoveredChildExitsBeforeItsFlags(t *testing.T) {
	const slug = "pair"

	tests := []struct {
		name         string
		parentLocked bool
		wantLockFile bool
	}{
		{"CoveredChildExits", false, false},
		{"ParentLockedChildSkipsTheCheck", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := t.TempDir()
			HoldHubPauseForTest(t, slug)
			logged := logcapture.CaptureVerbose(t)

			_, err := DetachedPush(hub, PushSpawn{
				WarpPath:     filepath.Join(hub, slug),
				WeftPath:     filepath.Join(hub, slug+"-weft"),
				PauseSlug:    slug,
				TeardownSlug: slug,
				ParentLocked: tt.parentLocked,
			})
			if err != nil {
				t.Fatalf("DetachedPush() error = %v; want nil", err)
			}

			if _, err := os.Stat(TeardownLockPath(hub, slug)); (err == nil) != tt.wantLockFile {
				t.Errorf("teardown lock file present = %v; want %v", err == nil, tt.wantLockFile)
			}
			failed, err := ListFailedPushes(hub)
			if err != nil || len(failed) != 0 {
				t.Errorf("ListFailedPushes() = (%+v, %v); want none", failed, err)
			}
			if got := strings.Contains(logged.String(), "a pause covers the push"); got == tt.parentLocked {
				t.Errorf("deferral logged = %v; want %v", got, !tt.parentLocked)
			}
		})
	}
}

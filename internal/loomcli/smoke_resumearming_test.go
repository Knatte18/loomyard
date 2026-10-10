//go:build tmux

// smoke_resumearming_test.go pins the arming half of `loom resume` after a hub reconcile, on a hub whose reed config is good and whose tmux key is registered with the kit.
// The reconcile half, which refuses at reed's config load before any server could start, is TestLoomResumeReconcilesTheHubConfigBeforeArming in the integration tier.

package loomcli

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeom"
)

// TestLoomResumeArmsAfterReconcile asserts `loom resume` on a hub whose build has no stamp reconciles the hub config and then arms past the reed config load.
// The stamp exists afterwards, and the verb reaches the run's own state: it refuses over the missing driver with the way forward, never over a config.
// Resume brings no reed session up, so the registered hub key holds the kit's session-less server and no session.
func TestLoomResumeArmsAfterReconcile(t *testing.T) {
	tmuxBinaryPath(t)
	exe := sharedLyxBinary(t)
	_, loc, worktree, slug := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)
	seedGoDriverRun(t, loc)
	seedAndCommitStatus(t, loc, slug)
	commitRetiredBatcherKey(t, loc)

	out, exit, err := runLoomCLINoFatal(exe, worktree, 60*time.Second, "loom", "resume")
	if err != nil {
		t.Fatalf("loom resume: %v; output: %s", err, out)
	}

	if _, err := os.Stat(hubStampPath(loc)); err != nil {
		t.Errorf("hub build stamp after loom resume: %v; want it written", err)
	}
	if exit != 1 || !strings.Contains(out, "no live driver") || !strings.Contains(out, "lyx loom start") {
		t.Errorf("loom resume exit = %d, output %s; want the no-live-driver refusal naming lyx loom start", exit, out)
	}
	reedGeom, err := hubgeom.ReedGeometry(loc)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	if _, statusErr := probeReedEngine(t, loc).Status(); statusErr == nil {
		t.Errorf("reed session is up on key %q after loom resume; want none: resume never brings reed up", reedGeom.SocketKey)
	}
}

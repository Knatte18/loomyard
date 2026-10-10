//go:build tmux

// smoke_resumearming_test.go pins the half of `loom resume` past the reed config load after a hub reconcile, on a hub whose reed config is good and whose tmux key is registered with the kit.
// The reconcile half, which refuses at reed's config load before any server could start, is TestLoomResumeReconcilesTheHubConfigBeforeArming in the integration tier.

package loomcli

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
)

// TestLoomResumeReconcilesThenRefusesWithoutLiveDriver asserts `loom resume` on a hub whose build has no stamp reconciles the hub config and then gets past the reed config load.
// The stamp exists afterwards, and the verb reaches the run's own state: it refuses over the missing driver with the way forward, never over a config.
// Resume brings no reed session up, so reed reports no session on the registered hub key.
func TestLoomResumeReconcilesThenRefusesWithoutLiveDriver(t *testing.T) {
	tmuxBinaryPath(t)
	exe := lyxbin.Build(t)
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
	if _, statusErr := probeReedEngine(t, loc).Status(); !errors.Is(statusErr, reedengine.ErrNoSession) {
		t.Errorf("reed status on key %q after loom resume = %v; want reed's no-session refusal: resume never brings reed up", reedGeom.SocketKey, statusErr)
	}
}

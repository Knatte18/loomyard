//go:build smoke

// smoke_operatorstrand_test.go covers the two live-substrate properties this batch introduces
// against a real wired hub and a real tmux session: the operator strand's own lifecycle (add,
// re-entrant no-op, dead-entry relaunch, and --no-attach's own absence), and the watchdog spawn's
// gate position -- that it fires even under --no-attach, the one thing card 13's own Tier 1 file
// cannot reach through the real RunE (see its own doc comment) because ensureStatusStrand needs a
// live tmux server.
//
// It reuses this package's existing smoke fixtures throughout: buildLyxBinary, newWiredPairFixture,
// registerBootstrapTeardown, probeReedEngine, statusStrandCount, and tmuxBinaryPath, rather than
// building a second rig. Like its siblings it drives the real built cmd/lyx binary as a subprocess,
// never RunCLI in-process, per this package's smoke suite doc comment: `lyx loom start` spawns its
// detached driver via os.Executable(), which an in-process call would resolve to the test binary.

package loomcli

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// operatorStrand returns the tracked strand named operatorStrandDisplayName from eng's own Status(),
// and whether one was found -- the same findStatusStrand helper bootstrap.go already exports within
// this package, applied to the operator strand's own name instead of the status strand's.
func operatorStrand(t *testing.T, eng *reedengine.Engine) (reedengine.StrandStatus, bool) {
	t.Helper()
	status, err := eng.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	return findStatusStrand(status.Strands, operatorStrandDisplayName)
}

// (1) after a bootstrap, the strand table contains a strand under operatorStrandDisplayName and it
// is Live.
func TestSmokeOperatorStrand_BootstrapAddsALiveOperatorStrand(t *testing.T) {
	tmuxBinaryPath(t)
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start")
	if err != nil {
		t.Fatalf("loom start: %v; output: %s", err, stdout)
	}

	eng := probeReedEngine(t, loc)
	strand, found := operatorStrand(t, eng)
	if !found {
		t.Fatalf("no strand named %q after bootstrap", operatorStrandDisplayName)
	}
	if !strand.Live {
		t.Errorf("operator strand %+v is not Live after bootstrap", strand)
	}
}

// (2) a second bootstrap in the same worktree leaves exactly one operator strand -- the IfAbsent
// no-op path -- and leaves the Selvage pane's own binding untouched by the operator strand's add.
func TestSmokeOperatorStrand_SecondBootstrapIsANoOp(t *testing.T) {
	tmuxBinaryPath(t)
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	firstOut, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start")
	if err != nil {
		t.Fatalf("first loom start: %v; output: %s", err, firstOut)
	}

	eng := probeReedEngine(t, loc)
	dotLyxDir := filepath.Join(loc.AnchorPath(), lyxdirs.DotLyxDirName)
	before, err := reedengine.LoadState(dotLyxDir)
	if err != nil {
		t.Fatalf("LoadState before second bootstrap: %v", err)
	}

	secondOut, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start")
	if err != nil {
		t.Fatalf("second loom start: %v; output: %s", err, secondOut)
	}

	if count := statusStrandCount(t, eng, operatorStrandDisplayName); count != 1 {
		t.Errorf("operator strands after two bootstraps = %d; want exactly 1 -- IfAbsent must no-op, not stack a second pane", count)
	}

	after, err := reedengine.LoadState(dotLyxDir)
	if err != nil {
		t.Fatalf("LoadState after second bootstrap: %v", err)
	}
	if after.SelvagePaneID != before.SelvagePaneID {
		t.Errorf("SelvagePaneID changed from %q to %q across the second bootstrap; want the operator strand's add to leave it untouched -- the operator strand must never become the Selvage pane", before.SelvagePaneID, after.SelvagePaneID)
	}
}

// (3) a worktree whose reed server was killed and re-booted -- the dead-entry case
// resolveStatusStrandAction's own doc comment describes -- gets the operator strand relaunched,
// present and Live again, neither duplicated nor permanently lost.
func TestSmokeOperatorStrand_RelaunchesAfterReedServerRestart(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	firstOut, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start")
	if err != nil {
		t.Fatalf("first loom start: %v; output: %s", err, firstOut)
	}

	eng := probeReedEngine(t, loc)
	socket, session := eng.Socket(), eng.SessionName()
	if err := exec.Command(tmuxPath, "-L", socket, "kill-server").Run(); err != nil {
		t.Fatalf("kill-server: %v", err)
	}
	waitTmuxSessionGone(t, tmuxPath, socket, session)

	secondOut, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start")
	if err != nil {
		t.Fatalf("second (post-restart) loom start: %v; output: %s", err, secondOut)
	}

	strand, found := operatorStrand(t, eng)
	if !found {
		t.Fatalf("no strand named %q after the reed server restart and a re-bootstrap", operatorStrandDisplayName)
	}
	if !strand.Live {
		t.Errorf("operator strand %+v is not Live after the reed server restart and a re-bootstrap", strand)
	}
	if count := statusStrandCount(t, eng, operatorStrandDisplayName); count != 1 {
		t.Errorf("operator strands after the restart-and-relaunch = %d; want exactly 1 -- neither duplicated nor permanently lost", count)
	}
}

// waitTmuxSessionGone blocks until `tmux -L socket has-session -t session` exits non-zero, or fails
// the test after a timeout -- tmux's kill-server is asynchronous, so a caller that simulates a crash
// must wait for the server to actually die before exercising recovery, mirroring
// internal/reedcli/smoke_test.go's own waitServerGone.
func waitTmuxSessionGone(t *testing.T, tmuxPath, socket, session string) {
	t.Helper()
	const timeout = 30 * time.Second
	deadline := time.Now().Add(timeout)
	for {
		if err := exec.Command(tmuxPath, "-L", socket, "has-session", "-t", session).Run(); err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("tmux server still up %s after kill-server (socket %s)", timeout, socket)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// (4) a `lyx loom start --no-attach` bootstrap adds no strand under operatorStrandDisplayName at
// all: the operator strand's add sits inside start.go's own mustAttach gate, since an invocation
// that hands no terminal over has no operator to give a pane to.
func TestSmokeOperatorStrand_NoAttachAddsNoOperatorStrand(t *testing.T) {
	tmuxBinaryPath(t)
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start", "--no-attach")
	if err != nil {
		t.Fatalf("loom start --no-attach: %v; output: %s", err, stdout)
	}

	eng := probeReedEngine(t, loc)
	if count := statusStrandCount(t, eng, operatorStrandDisplayName); count != 0 {
		t.Errorf("operator strands after --no-attach = %d; want 0 -- the add sits inside mustAttach's gate", count)
	}
}

// (5) a `lyx loom start --no-attach` bootstrap still leaves a live per-hub watchdog daemon for the
// fixture hub, found by the same argv-signature scan card 14 adds to the smoke teardown. This is
// what proves the watchdog call sits outside the mustAttach gate -- the single thing a later edit is
// most likely to get wrong, and the property card 12's own code comment claims -- which is the one
// property card 13's own Tier 1 file structurally cannot reach (see its own doc comment).
func TestSmokeWatchdog_NoAttachStillSpawnsTheDaemon(t *testing.T) {
	tmuxBinaryPath(t)
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start", "--no-attach")
	if err != nil {
		t.Fatalf("loom start --no-attach: %v; output: %s", err, stdout)
	}

	if pids := findWatchdogPIDs(loc.HubPath); len(pids) == 0 {
		t.Errorf("no watchdog daemon found for hub %s after --no-attach; want the spawn to fire outside the mustAttach gate", loc.HubPath)
	}
}

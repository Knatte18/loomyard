//go:build smoke

// smoke_starttail_test.go covers the two live-substrate properties of `lyx loom start`'s tail
// against a real wired hub and a real tmux session: the attach tail's strand set -- on the unseeded
// fixture, which resolveSeedDriver seeds llm, it adds neither a loom-operator strand nor a status
// strand -- and the watchdog spawn's gate position -- that it fires even under --no-attach, the one
// thing start_watchdog_test.go's Tier 1 file cannot reach through the real RunE (see its own doc
// comment) because reed Up needs a live tmux server.
//
// It reuses this package's existing smoke fixtures throughout: buildLyxBinary, newWiredPairFixture,
// registerBootstrapTeardown, probeReedEngine, statusStrandCount, and tmuxBinaryPath, rather than
// building a second rig. Like its siblings it drives the real built cmd/lyx binary as a subprocess,
// never RunCLI in-process, per this package's smoke suite doc comment: `lyx loom start` spawns its
// detached driver via os.Executable(), which an in-process call would resolve to the test binary.

package loomcli

import (
	"testing"
	"time"
)

// TestSmokeStart_AttachTailAddsNoOperatorStrand pins that an attached `lyx loom start` leaves no
// strand named "loom-operator" and no status strand: the fixture is unseeded, so resolveSeedDriver
// seeds it llm, and an llm-driven start carries no status strand. The smoke run has no TTY, so the
// attach itself exits non-zero after the tail ran; the strand table is what is asserted. The
// literal is spelled inline as a guard against the removed operator strand coming back: Selvage is
// the operator's terminal, not a strand.
func TestSmokeStart_AttachTailAddsNoOperatorStrand(t *testing.T) {
	tmuxBinaryPath(t)
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	// Without a TTY the attach exits non-zero, which surfaces only as the ignored exit code; err is
	// set only on a timeout or a failed launch, and either means the tail may never have run.
	stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start")
	if err != nil {
		t.Fatalf("loom start: %v; output: %s", err, stdout)
	}

	eng := probeReedEngine(t, loc)
	if count := statusStrandCount(t, eng, "loom-operator"); count != 0 {
		t.Errorf("loom-operator strands after an attached start = %d; want 0 -- Selvage is the operator's terminal, not a strand", count)
	}
	if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 0 {
		t.Errorf("status strands after an attached llm-driven start = %d; want 0", count)
	}
}

// TestSmokeWatchdog_NoAttachStillSpawnsTheDaemon pins that a `lyx loom start --no-attach`
// bootstrap still leaves a live per-hub watchdog daemon for the fixture hub, found by the same
// argv-signature scan the smoke teardown uses. This is what proves the watchdog call sits outside
// the mustAttach gate -- the single thing a later edit is most likely to get wrong -- which is the
// one property start_watchdog_test.go structurally cannot reach (see its own doc comment).
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

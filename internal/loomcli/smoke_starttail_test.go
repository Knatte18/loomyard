// smoke_starttail_test.go pins the live-substrate properties of `lym loom start`'s tail.
// It runs against a real wired hub and a real tmux session.
// The verb never attaches or switches a tmux client.
// The strand set it leaves is unchanged by that.
// On the unseeded fixture, resolveSeedDriver seeds llm, so the set is one status strand and no loom-operator strand.
//
// It also pins the watchdog spawn's gate position: it fires on every start.
// `start_watchdog_test.go` cannot reach that position through the real RunE, because reed Up needs a live tmux server.
//
// The smoke tests in this package reuse its existing fixtures rather than building a second rig.
// They drive the real built cmd/lym binary as a subprocess and never call RunCLI in-process.
// `lym loom start` spawns its detached driver via os.Executable().
// An in-process call would resolve that to the test binary.

package loomcli

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// reedAttachedClients returns the tmux clients attached to the reed session on reed's own server.
func reedAttachedClients(t *testing.T, tmuxPath string, eng *reedengine.Engine) []string {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", eng.Socket(), "list-clients", "-t", "="+eng.SessionName(), "-F", "#{client_name}").Output()
	if err != nil {
		t.Fatalf("list-clients on reed's server: %v", err)
	}
	return strings.Fields(string(out))
}

// TestSmokeStart_NeverAttachesAndAddsNoOperatorStrand pins that `lym loom start` exits 0 with an ok envelope.
// It attaches no client, with or without --no-attach and with or without $TMUX naming reed's own server.
// It leaves no strand named "loom-operator" and exactly one status strand.
// The fixture is unseeded, so resolveSeedDriver seeds it llm, and an llm-driven start ensures the status strand.
// The literal is spelled inline as a guard against the removed operator strand coming back.
// Selvage is the operator's terminal, not a strand.
// The smoke run has no TTY,
// so a start that still attached would exit non-zero.
func TestSmokeStart_NeverAttachesAndAddsNoOperatorStrand(t *testing.T) {
	tiergate.Require(t, 2)
	tmuxPath := tmuxBinaryPath(t)
	exe := lymbin.Build(t)

	// A stub provider lets the llm-driven start's driver launch reach readiness,
	// so the verb reaches its success envelope.
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	stubPath := writeStubDriverScript(t)
	hubforge.SeedConfig(t, h, map[string]string{
		"claude":  claudeConfigWithBinary(stubPath),
		"loom":    fastDeadlineLoomConfig(),
		"reed":    reedengine.ConfigTemplate(),
		"shuttle": driverShuttleConfig(t),
		"webster": websterengine.ConfigTemplate(),
	})
	const slug = "loom-smoke-start-task"
	hubforge.AddPair(t, h, slug)
	worktree := h.PairCodeWorktree(slug)
	loc, err := lymcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lymcwd.Resolve(%s): %v", worktree, err)
	}
	registerBootstrapTeardown(t, loc, worktree)

	startOK := func(label string, args ...string) {
		t.Helper()
		stdout, exit, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, 30*time.Second, append([]string{"loom", "start"}, args...)...)
		if err != nil {
			t.Fatalf("%s: %v; output: %s", label, err, stdout)
		}
		if exit != 0 || !strings.Contains(stdout, `"ok":true`) {
			t.Fatalf("%s exited %d; want 0 with an ok envelope -- output: %s", label, exit, stdout)
		}
	}

	startOK("loom start")

	eng := probeReedEngine(t, loc)
	if count := statusStrandCount(t, eng, "loom-operator"); count != 0 {
		t.Errorf("loom-operator strands after start = %d; want 0 -- Selvage is the operator's terminal, not a strand", count)
	}
	if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
		t.Errorf("status strands after an llm-driven start = %d; want 1", count)
	}

	// $TMUX names reed's own server, the case that would switch the caller's client.
	tmuxEnv, err := exec.Command(tmuxPath, "-L", eng.Socket(), "display-message", "-p", "-t", "="+eng.SessionName(), "#{socket_path},#{pid},0").Output()
	if err != nil {
		t.Fatalf("read reed's server identity: %v", err)
	}
	clientsBefore := reedAttachedClients(t, tmuxPath, eng)
	t.Setenv("TMUX", strings.TrimSpace(string(tmuxEnv)))
	startOK("loom start inside reed's tmux server")
	startOK("loom start --no-attach inside reed's tmux server", "--no-attach")

	if clientsAfter := reedAttachedClients(t, tmuxPath, eng); strings.Join(clientsAfter, ",") != strings.Join(clientsBefore, ",") {
		t.Errorf("reed session's attached clients = %v after start inside its server; want %v unchanged", clientsAfter, clientsBefore)
	}
}

// TestSmokeWatchdog_NoAttachStillSpawnsTheDaemon pins the watchdog spawn after `lym loom start --no-attach`.
// It checks for a live per-hub watchdog daemon for the fixture hub.
// The check finds the daemon by the argv-signature scan the smoke teardown uses.
// The test proves the watchdog spawn fires on every start, not only on an attaching one.
// A later edit is most likely to break that, and hermetic tests cannot reach it.
func TestSmokeWatchdog_NoAttachStillSpawnsTheDaemon(t *testing.T) {
	tiergate.Require(t, 2)
	tmuxBinaryPath(t)
	exe := lymbin.Build(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	// Registered before the teardown, so it runs after the teardown's kills:
	// a git child of a killed process can still be writing into the hub when the temp dir is removed.
	waitForFixtureProcessesToExit(t)
	registerBootstrapTeardown(t, loc, worktree)

	stdout, _, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, 30*time.Second, "loom", "start", "--no-attach")
	if err != nil {
		t.Fatalf("loom start --no-attach: %v; output: %s", err, stdout)
	}

	if pids := findWatchdogPIDs(loc.HubPath); len(pids) == 0 {
		t.Errorf("no watchdog daemon found for hub %s after --no-attach; want the watchdog spawn to fire on every start", loc.HubPath)
	}
}

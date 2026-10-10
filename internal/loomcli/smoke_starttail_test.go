//go:build tmux

// smoke_starttail_test.go covers the two live-substrate properties of `lyx loom start`'s tail against a real wired hub and a real tmux session:
// that the verb never attaches or switches a tmux client, and that the strand set it leaves -- on the unseeded fixture, which resolveSeedDriver seeds llm, neither a loom-operator strand nor a status strand -- is unchanged by that.
// It also pins the watchdog spawn's gate position -- that it fires on every start, the one thing start_watchdog_test.go's Tier 1 file cannot reach through the real RunE (see its own doc comment) because reed Up needs a live tmux server.
//
// It reuses this package's existing smoke fixtures throughout: lyxbin.Build, newWiredPairFixture,
// registerBootstrapTeardown, probeReedEngine, statusStrandCount, and tmuxBinaryPath, rather than
// building a second rig. Like its siblings it drives the real built cmd/lyx binary as a subprocess,
// never RunCLI in-process, per this package's smoke suite doc comment: `lyx loom start` spawns its
// detached driver via os.Executable(), which an in-process call would resolve to the test binary.

package loomcli

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
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

// TestSmokeStart_NeverAttachesAndAddsNoOperatorStrand pins that `lyx loom start` exits 0 with an ok envelope and attaches no client, with or without --no-attach and with or without $TMUX naming reed's own server,
// and that it leaves no strand named "loom-operator" and no status strand:
// the fixture is unseeded, so resolveSeedDriver seeds it llm,
// and an llm-driven start carries no status strand.
// The literal is spelled inline as a guard against the removed operator strand coming back: Selvage is the operator's terminal, not a strand.
// The smoke run has no TTY,
// so a start that still attached would exit non-zero.
func TestSmokeStart_NeverAttachesAndAddsNoOperatorStrand(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)
	exe := lyxbin.Build(t)

	// A stub provider lets the llm-driven start's driver launch reach readiness,
	// so the verb reaches its success envelope.
	h := hubforge.NewHub(t, ".")
	hubforge.SeedConfig(t, h, map[string]string{
		"loom":    fastDeadlineLoomConfig(),
		"reed":    reedengine.ConfigTemplate(),
		"shuttle": driverShuttleConfig(t, writeStubDriverScript(t)),
		"webster": websterengine.ConfigTemplate(),
	})
	const slug = "loom-smoke-start-task"
	hubforge.AddPair(t, h, slug)
	worktree := h.PairCodeWorktree(slug)
	loc, err := lyxcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", worktree, err)
	}
	registerBootstrapTeardown(t, loc, worktree)

	startOK := func(label string, args ...string) {
		t.Helper()
		stdout, exit, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, append([]string{"loom", "start"}, args...)...)
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
	if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 0 {
		t.Errorf("status strands after an llm-driven start = %d; want 0", count)
	}

	// $TMUX names reed's own server, the case that used to switch the caller's client.
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

// TestSmokeWatchdog_NoAttachStillSpawnsTheDaemon pins that a `lyx loom start --no-attach` bootstrap leaves a live per-hub watchdog daemon for the fixture hub, found by the same argv-signature scan the smoke teardown uses.
// This is what proves the watchdog spawn fires on every start, not only on an attaching one -- the single thing a later edit is most likely to get wrong -- which is the one property start_watchdog_test.go structurally cannot reach (see its own doc comment).
func TestSmokeWatchdog_NoAttachStillSpawnsTheDaemon(t *testing.T) {
	tmuxBinaryPath(t)
	exe := lyxbin.Build(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start", "--no-attach")
	if err != nil {
		t.Fatalf("loom start --no-attach: %v; output: %s", err, stdout)
	}

	if pids := findWatchdogPIDs(loc.HubPath); len(pids) == 0 {
		t.Errorf("no watchdog daemon found for hub %s after --no-attach; want the watchdog spawn to fire on every start", loc.HubPath)
	}
}

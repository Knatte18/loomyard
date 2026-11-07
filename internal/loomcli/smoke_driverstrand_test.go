// smoke_driverstrand_test.go covers the one live-substrate property no Tier 1 test can reach:
// A real reed session is driven through three successive "loom start" bootstraps of an llm-seeded worktree.
// It must end with exactly one strand under driverStrandDisplayName every time, never two.
// mustSpawnDriver and resolveDriverStrandAction are already pinned at Tier 1 with a synthetic strand table.
// Only a real tmux server and a real spawned pane can prove two things:
// a do-not-spawn verdict leaves reed holding one strand rather than two,
// and a dead pane's corpse is removed before a relaunch rather than left beside a second, live one.
// Reed's own add has no upsert semantics to reconcile either case for us.
// The same session also pins that an llm-seeded start leaves exactly one status strand beside its driver strand.
// A start re-arms the session's follow state and selects the live driver strand's window; a resume does not.
//
// The file drives the real built cmd/lym binary as a subprocess, never RunCLI in-process.
// "lym loom start" spawns its llm driver through a real shuttle run against a real tmux pane.
// Only a genuine subprocess boundary keeps this test's own binary out of that pane's launch line.
//
// Each strand's launch line is typed into a running plain shell pane with tmux send-keys.
// The line is not the split-window's trailing command; launchStrandLocked in reedengine's spawn.go types it in.
// So the stub provider finishing leaves the pane's shell alive, and the pane stays live indefinitely.
// A dead pane therefore has to be simulated by killing it.
// The case that needs one runs "tmux kill-pane" on the driver's own pane, never waiting for the stub script to finish.
//
// The provider behind every spawn in this file is a stubbed shell script, never a real `claude`.
// A real `claude` spawns a live, billed session bounded only by the task at hand.
// That is why the sandbox suite does not script this path at all.
// The test writes the stub itself and points the shuttle config's claude key at it.
// It never re-execs this test binary.
// The Live-Substrate Spawn Observability invariant forbids re-executing os.Executable() under go test.
//
// testmain_test.go arms the hermetic git test environment (gitkit.HermeticGitEnv()) for the whole test binary.
// It covers untagged files too, so this file needs no TestMain of its own.
//
// The test also proves that shuttle's blocking Start succeeds against a real reed pane.
// Each "loom start --no-attach" below blocks inside StartDriver's call to Start.
// That call returns once the provider is ready, or it reports the run as a refusal naming shuttle's ErrNotStarted.
// Asserting each invocation's exit code makes a readiness regression surface as that refusal, not a silent pass.
package loomcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// stubRecordSessionStart is the shell a stubbed claude runs on its own arguments before it draws its first box.
// It appends claudeengine's startup SessionStart payload to the events file named by the --settings recording hooks.
// A real launch's SessionStart hook does the same.
// Shuttle starts a claude run only once its events file holds a session start beside the ready pane.
const stubRecordSessionStart = `record_session_start() {
  while [ $# -gt 0 ]; do
    if [ "$1" = --settings ]; then
      printf '%s\n' '` + claudeengine.SessionStartupPayloadFixture + `' >> "$(grep -o "[^']*events\.jsonl" "$2" | head -1)"
    fi
    shift
  done
}
record_session_start "$@"
`

// writeStubDriverScript writes a POSIX shell script that stands in for the claude binary this file's spawns launch.
// The script records a session start through stubRecordSessionStart.
// It ignores every other argument that the claude engine appends to its launch line.
// It then prints claudeengine's idle-input-box fixture and sleeps for a long, harmless duration.
// The fixture is required: without it, shuttle's startup step never observes readiness,
// and no verified send finds the session idle.
// Every "loom start --no-attach" below would then refuse instead of succeeding.
// claudeengine owns the fixture text and how a provider's panes are classified.
// This package needs only a realistic stand-in, never those classification details.
// The driver spec names no skill, so the prompt pointer rides the launch line.
// The one typed line is the `/color` command shuttle types for the driver's colored segment.
// The script prints the fixture again after reading that line, so the session reads idle, and starts the sleep.
// The script never exits on its own: the third case kills the pane directly (see the file-level doc comment),
// so the sleep only needs to outlast the whole test.
func writeStubDriverScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub-claude.sh")
	script := `#!/bin/sh
` + stubRecordSessionStart + `box() { printf '%s\n' '` + claudeengine.IdleInputBoxFixture + `'; }
box
IFS= read -r line
box
sleep 3600
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub driver script: %v", err)
	}
	return path
}

// driverShuttleConfig returns shuttle's shipped config template with startup_timeout_s lowered from 90 to 10.
// A readiness regression is shuttle's startup step never observing the ready marker.
// The lowered window makes that regression surface as a refusal, not as a timeout.
// The refusal lands inside the per-invocation timeout runLoomCLINoFatal enforces, so that timeout does not mask it.
// The substring is asserted present before it is replaced,
// so a template drift fails here rather than silently keeping the 90-second window.
func driverShuttleConfig(t *testing.T) string {
	t.Helper()
	cfg := shuttleengine.ConfigTemplate()
	const timeoutKey = "startup_timeout_s: 90"
	if !strings.Contains(cfg, timeoutKey) {
		t.Fatalf("shuttle config template drift: %q not found", timeoutKey)
	}
	return strings.Replace(cfg, timeoutKey, "startup_timeout_s: 10", 1)
}

// driverStrand returns the tracked driver strand from eng's Status(), and whether one was found.
// It fails the test if Status errors, and it delegates the lookup to findDriverStrand.
func driverStrand(t *testing.T, eng *reedengine.Engine) (reedengine.StrandStatus, bool) {
	t.Helper()
	status, err := eng.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	return findDriverStrand(status.Strands)
}

// waitDriverStrandDead blocks until eng reports a strand named driverStrandDisplayName that is
// present but no longer live, a tracked strand whose pane has died.
// It fails the test if that state is not reached within timeout.
func waitDriverStrandDead(t *testing.T, eng *reedengine.Engine, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if strand, found := driverStrand(t, eng); found && !strand.Live {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("driver strand %q never went dead within %s", driverStrandDisplayName, timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestSmokeDriverStrand_ReentrantAcrossThreeBootstraps drives three successive bootstraps.
// Each one runs "loom start --no-attach" against one llm-seeded worktree.
// It asserts the driver strand count after each bootstrap, in order.
//
// A first bootstrap leaves exactly one driver strand.
// A second bootstrap, while the first driver's pane is still alive, leaves exactly one again.
// No Tier 1 test can prove this re-entrancy.
// The do-not-spawn verdict and reed actually holding one strand are two different facts.
// A third bootstrap, run promptly after the pane's own kill, leaves exactly one again.
// It must not leave a corpse beside a live pane.
// The count is what separates corpse removal from a second add, since reed's add has no upsert.
//
// After each bootstrap it also asserts exactly one status strand exists.
// An llm-seeded start ensures that status strand beside its driver strand.
func TestSmokeDriverStrand_ReentrantAcrossThreeBootstraps(t *testing.T) {
	tiergate.Require(t, 2)
	tmuxPath := tmuxBinaryPath(t)
	exe := lymbin.Build(t)

	stubPath := writeStubDriverScript(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.SeedConfig(t, h, map[string]string{
		"claude":  claudeConfigWithBinary(stubPath),
		"loom":    fastDeadlineLoomConfig(),
		"reed":    reedengine.ConfigTemplate(),
		"shuttle": driverShuttleConfig(t),
		"webster": websterengine.ConfigTemplate(),
	})
	const slug = "loom-smoke-driver-task"
	hubforge.AddPair(t, h, slug)
	worktree := h.PairCodeWorktree(slug)

	loc, err := lymcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lymcwd.Resolve(%s): %v", worktree, err)
	}
	registerBootstrapTeardown(t, loc, worktree)
	seedLLMDriver(t, loc)

	eng := probeReedEngine(t, loc)

	// (1) a first bootstrap leaves exactly one strand under the driver name, live.
	firstOut, firstExit, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, 30*time.Second, "loom", "start", "--no-attach")
	if err != nil {
		t.Fatalf("first loom start: %v; output: %s", err, firstOut)
	}
	if firstExit != 0 {
		t.Fatalf("first loom start exited %d; want 0 -- output: %s", firstExit, firstOut)
	}
	strand, found := driverStrand(t, eng)
	if !found || !strand.Live {
		t.Fatalf("driver strand after the first bootstrap = (found=%v live=%v); want a live strand", found, found && strand.Live)
	}
	if count := driverStrandCount(t, eng); count != 1 {
		t.Fatalf("driver strands after the first bootstrap = %d; want exactly 1", count)
	}
	if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
		t.Fatalf("status strands after the first bootstrap = %d; want exactly 1 -- an llm-seeded start ensures the status strand", count)
	}

	// (2) a second bootstrap while the first driver's pane is still alive must leave exactly one
	// strand and must not touch the live pane the first bootstrap already spawned.
	// Follow goes off first: a start that enters a run with a live driver is the path a parked run's resume takes,
	// and that path re-arms follow.
	setFollow(t, eng, "off")
	secondOut, secondExit, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, 30*time.Second, "loom", "start", "--no-attach")
	if err != nil {
		t.Fatalf("second loom start: %v; output: %s", err, secondOut)
	}
	if secondExit != 0 {
		t.Fatalf("second loom start exited %d; want 0 -- output: %s", secondExit, secondOut)
	}
	if count := driverStrandCount(t, eng); count != 1 {
		t.Fatalf("driver strands after the second bootstrap = %d; want exactly 1 -- a do-not-spawn verdict must leave reed holding one strand, not two", count)
	}
	if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
		t.Fatalf("status strands after the second bootstrap = %d; want exactly 1 -- an llm-seeded start ensures the status strand", count)
	}
	strand, found = driverStrand(t, eng)
	if !found || !strand.Live {
		t.Fatalf("driver strand after the second bootstrap = (found=%v live=%v); want it still live and untouched by the second bootstrap's own no-op", found, found && strand.Live)
	}
	if got := readFollow(t, eng); got != "on" {
		t.Errorf("@lym_follow after the second bootstrap = %q; want on", got)
	}
	if got, want := windowID(t, eng, "="+eng.SessionName()+":"), windowID(t, eng, strand.PaneID); got != want {
		t.Errorf("current window after the second bootstrap = %s; want the live driver strand's window %s, the newest live agent", got, want)
	}

	// Kill the driver's own pane directly, before the third bootstrap.
	// Each strand's launch line is typed into a running shell via send-keys (see the file-level doc comment),
	// so the stub process finishing leaves the shell alive and the pane reported live indefinitely.
	// Only killing the pane produces the dead-but-tracked corpse that the third bootstrap must remove.
	if err := exec.Command(tmuxPath, "-L", eng.Socket(), "kill-pane", "-t", strand.PaneID).Run(); err != nil {
		t.Fatalf("kill-pane %s: %v", strand.PaneID, err)
	}
	waitDriverStrandDead(t, eng, 10*time.Second)

	// (3) a third bootstrap, run promptly after the pane's own kill, removes the dead entry and relaunches.
	// Exactly one strand remains, never a corpse plus a live pane beside it.
	// The relaunch lands within one second of the corpse's removal on purpose.
	// That is the case the random suffix in driverReportPath exists for:
	// Two attempts composing a report path in the same clock second would otherwise collide.
	// Spec.validate would then refuse the second.
	thirdOut, thirdExit, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, 30*time.Second, "loom", "start", "--no-attach")
	if err != nil {
		t.Fatalf("third loom start: %v; output: %s", err, thirdOut)
	}
	if thirdExit != 0 {
		t.Fatalf("third loom start exited %d; want 0 -- output: %s", thirdExit, thirdOut)
	}
	if count := driverStrandCount(t, eng); count != 1 {
		t.Fatalf("driver strands after the third bootstrap = %d; want exactly 1 -- corpse removal must replace the dead entry, never add a second one beside it", count)
	}
	if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
		t.Fatalf("status strands after the third bootstrap = %d; want exactly 1 -- an llm-seeded start ensures the status strand", count)
	}
}

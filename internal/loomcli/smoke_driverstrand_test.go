//go:build tmux

// smoke_driverstrand_test.go covers the one live-substrate property no Tier 1 test can reach: that a
// real reed session, driven through three successive "loom start" bootstraps of an llm-seeded
// worktree, ends up with exactly one strand under driverStrandDisplayName every time -- never two.
// mustSpawnDriver and resolveDriverStrandAction are already pinned at Tier 1 with a synthetic strand
// table; what only a real tmux server and a real spawned pane can prove is that a do-not-spawn
// verdict really leaves reed holding one strand rather than two, and that a dead pane's corpse is
// removed before a relaunch rather than left beside a second, live one -- reed's own add has no
// upsert semantics to reconcile either case for us.
// It also pins, in the same session, that an llm-seeded start ensures one status strand while its driver strand still spawns.
//
// Like this package's other smoke tests it drives the real built cmd/lyx binary as a subprocess,
// never RunCLI in-process (see smoke_test.go's own header): "lyx loom start" spawns its llm driver
// through a real shuttle run against a real tmux pane, and only a genuine subprocess boundary keeps
// this test's own binary out of that pane's launch line.
//
// Every strand's own launch line is typed into an already-running plain shell pane via tmux send-keys
// (reedengine's own launchStrandLocked, spawn.go), not run as the split-window's own trailing
// command, so the stub provider process finishing its own work leaves the pane's shell alive and the
// pane itself reported live indefinitely -- so a dead pane can only be simulated by killing tmux
// outright rather than by waiting on a spawned command to exit. This file's own third case does that
// at the single-pane grain: it kills the driver's own pane directly via "tmux kill-pane", never by
// waiting for the stub script to finish on its own.
//
// The provider binary behind every one of this file's spawns is a stubbed shell script, never a real
// `claude`: per this batch's own scope note, exercising the real thing spawns a live, billed Claude
// session bounded only by the task at hand, which is why the sandbox suite deliberately does not
// script this path at all (batch 8 records that disposition in prose). The stub is a script the test
// itself writes and points the shuttle config's own claude key at -- never a re-exec of this test
// binary -- per the Live-Substrate Spawn Observability invariant's clause against re-executing
// os.Executable() under go test.
//
// This package's own testmain_test.go already arms the hermetic git test environment
// (gitkit.HermeticGitEnv()) for the whole test binary, untagged files included, so this file needs no
// TestMain of its own.
//
// This test also proves shuttle's own blocking Start succeeds against a real reed pane: every "loom
// start --no-attach" below now blocks inside StartDriver's call to Start, which returns only once the
// provider is ready (or reports the run as a refusal, naming shuttle's ErrNotStarted), and its
// assertions below on the invocation's own exit code catch a readiness regression as that refusal
// rather than letting it pass silently.
package loomcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// writeStubDriverScript writes a POSIX shell script standing in for the claude binary this file's
// spawns launch: it ignores every argument the claude engine's own launch line appends, prints
// claudeengine's own idle-input-box fixture, answers skill loads, then sleeps for a long, harmless duration.
// The fixture is required under the readiness signal this file now drives: without it shuttle's own startup step would never observe readiness and no verified send would find the session idle,
// so every "loom start --no-attach" below would refuse instead of succeeding.
// The fixture text, and how a provider's panes are classified, is claudeengine's own concern (see claudeengine.IdleInputBoxFixture);
// this package only needs a realistic stand-in, never the classification details behind it.
// The script prints the fixture again after each line it reads, so the session reads idle before the next send.
// The driver spec names skills,
// and shuttle types one skill-load message before the prompt pointer, waiting until the turn ends;
// a leading `/color` line, which shuttle types for the driver's colored segment, is skipped.
// The script answers the load message by appending a Stop event with no transcript to the events.jsonl beside the `--settings` file,
// so shuttle confirms the load unverified at once instead of waiting out the skill-load timeout,
// and it starts the sleep at the next line, the pointer.
// The script never needs to
// exit on its own -- this file's own third case kills its pane directly (see the file-level doc
// comment) -- so the sleep only needs to outlast the whole test, never to be observed finishing.
func writeStubDriverScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub-claude.sh")
	script := `#!/bin/sh
settings=
while [ $# -gt 0 ]; do
  if [ "$1" = "--settings" ]; then settings=$2; fi
  shift
done
box() { printf '%s\n' '` + claudeengine.IdleInputBoxFixture + `'; }
box
IFS= read -r line
box
case $line in /color*) IFS= read -r line; box ;; esac
printf '%s\n' '{"hook_event_name":"Stop","last_assistant_message":"ok"}' >> "$(dirname "$settings")/events.jsonl"
IFS= read -r line
box
sleep 3600
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub driver script: %v", err)
	}
	return path
}

// driverShuttleConfig returns shuttle's shipped config template with the claude key pointed at
// stubPath and startup_timeout_s lowered from 90 to 10. Unlike smoke_test.go's own
// providerlessShuttleConfig, this file's whole point is proving reed's own strand bookkeeping across a
// REAL spawned pane, so the provider path must resolve to a real (if stubbed) executable rather than a
// deliberately-broken one. The lowered window keeps a readiness regression -- shuttle's own startup
// step never observing the ready marker -- surfacing as a refusal envelope inside the 30s per-invocation timeout
// runLoomCLINoFatal enforces, rather than that timeout itself firing and masking the real failure. Both
// substring replacements are asserted present before being applied, so a template drift (the shipped
// claude key or the shipped startup_timeout_s value changing shape) fails loudly here rather than
// silently leaving the 90s window in place.
func driverShuttleConfig(t *testing.T, stubPath string) string {
	t.Helper()
	cfg := shuttleengine.ConfigTemplate()
	const claudeKey = "claude: ${env:LYX_SHUTTLE_CLAUDE:-}"
	const timeoutKey = "startup_timeout_s: 90"
	if !strings.Contains(cfg, claudeKey) {
		t.Fatalf("shuttle config template drift: %q not found", claudeKey)
	}
	if !strings.Contains(cfg, timeoutKey) {
		t.Fatalf("shuttle config template drift: %q not found", timeoutKey)
	}
	cfg = strings.Replace(cfg, claudeKey, "claude: "+stubPath, 1)
	cfg = strings.Replace(cfg, timeoutKey, "startup_timeout_s: 10", 1)
	return cfg
}

// driverStrand returns the tracked driver strand from eng's own Status(), and whether one was found,
// through the findDriverStrand helper bootstrap.go uses.
func driverStrand(t *testing.T, eng *reedengine.Engine) (reedengine.StrandStatus, bool) {
	t.Helper()
	status, err := eng.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	return findDriverStrand(status.Strands)
}

// waitDriverStrandDead blocks until eng reports a strand named driverStrandDisplayName that is
// present but no longer live -- the corpse resolveDriverStrandAction's own dead case matches -- or
// fails the test after timeout.
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

// TestSmokeDriverStrand_ReentrantAcrossThreeBootstraps drives three successive "loom start
// --no-attach" bootstraps against one llm-seeded worktree and asserts the driver strand count after
// each, in order.
//
// A first bootstrap leaves exactly one. A second, while the first driver's pane is still alive,
// leaves exactly one again -- the re-entrancy property no Tier 1 test can prove, since the predicate
// saying do-not-spawn and reed actually holding one strand are two different facts. A third, run
// promptly after the pane's own kill, leaves exactly one once more and never a corpse plus a live
// pane beside it -- the count is what distinguishes corpse removal from a second add, a distinction
// reed's own upsert-less add cannot make for us.
//
// After each bootstrap it also asserts exactly one status strand exists: this is the one llm-seeded start whose driver strand really spawns,
// so it pins that an llm-seeded start ensures the status strand while its driver strand still spawns.
func TestSmokeDriverStrand_ReentrantAcrossThreeBootstraps(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)
	exe := sharedLyxBinary(t)

	stubPath := writeStubDriverScript(t)

	h := hubforge.NewHub(t, ".")
	hubforge.SeedConfig(t, h, map[string]string{
		"loom":    fastDeadlineLoomConfig(),
		"reed":    reedengine.ConfigTemplate(),
		"shuttle": driverShuttleConfig(t, stubPath),
		"webster": websterengine.ConfigTemplate(),
	})
	const slug = "loom-smoke-driver-task"
	hubforge.AddPair(t, h, slug)
	worktree := h.PairCodeWorktree(slug)

	loc, err := lyxcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", worktree, err)
	}
	registerBootstrapTeardown(t, loc, worktree)
	seedLLMDriver(t, loc)

	eng := probeReedEngine(t, loc)

	// (1) a first bootstrap leaves exactly one strand under the driver name, live.
	firstOut, firstExit, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start", "--no-attach")
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
		t.Fatalf("status strands after the first bootstrap = %d; want exactly 1 -- an llm-seeded start keeps the status strand too", count)
	}

	// (2) a second bootstrap while the first driver's pane is still alive must leave exactly one
	// strand and must not touch the live pane the first bootstrap already spawned.
	secondOut, secondExit, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start", "--no-attach")
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
		t.Fatalf("status strands after the second bootstrap = %d; want exactly 1 -- an llm-seeded start keeps the status strand too", count)
	}
	strand, found = driverStrand(t, eng)
	if !found || !strand.Live {
		t.Fatalf("driver strand after the second bootstrap = (found=%v live=%v); want it still live and untouched by the second bootstrap's own no-op", found, found && strand.Live)
	}

	// Kill the driver's own pane directly, before the third bootstrap: every strand's launch line is
	// typed into an already-running shell via send-keys (see the file-level doc comment), so the
	// stub's own process finishing leaves the pane's shell alive and the pane reported live forever --
	// only killing the pane itself produces the dead-but-tracked corpse resolveDriverStrandAction's
	// own third case exists to remove.
	if err := exec.Command(tmuxPath, "-L", eng.Socket(), "kill-pane", "-t", strand.PaneID).Run(); err != nil {
		t.Fatalf("kill-pane %s: %v", strand.PaneID, err)
	}
	waitDriverStrandDead(t, eng, 10*time.Second)

	// (3) a third bootstrap, run promptly after the pane's own kill, removes the dead entry and
	// relaunches: exactly one strand again, never a corpse plus a live pane beside it. Landing this
	// relaunch inside one second of the corpse's own removal is deliberate: it is the case
	// driverReportPath's own random suffix exists for, since two attempts composing a report path in
	// the same clock second would otherwise collide and Spec.validate would refuse the second one
	// outright.
	thirdOut, thirdExit, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start", "--no-attach")
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
		t.Fatalf("status strands after the third bootstrap = %d; want exactly 1 -- an llm-seeded start keeps the status strand too", count)
	}
}

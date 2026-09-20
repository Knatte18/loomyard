//go:build smoke

// smoke_driverstrand_test.go covers the one live-substrate property no Tier 1 test can reach: that a
// real reed session, driven through three successive "loom start" bootstraps of an llm-seeded
// worktree, ends up with exactly one strand under driverStrandDisplayName every time -- never two.
// mustSpawnDriver and resolveDriverStrandAction are already pinned at Tier 1 with a synthetic strand
// table; what only a real tmux server and a real spawned pane can prove is that a do-not-spawn
// verdict really leaves reed holding one strand rather than two, and that a dead pane's corpse is
// removed before a relaunch rather than left beside a second, live one -- reed's own add has no
// upsert semantics to reconcile either case for us.
//
// Like this package's other smoke tests it drives the real built cmd/lyx binary as a subprocess,
// never RunCLI in-process (see smoke_test.go's own header): "lyx loom start" spawns its llm driver
// through a real shuttle run against a real tmux pane, and only a genuine subprocess boundary keeps
// this test's own binary out of that pane's launch line.
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
package loomcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// driverStrandStubSettleDelay is the fixed delay writeStubDriverScript's generated script sleeps
// through before its first marker check, on every invocation. It exists so the pane it runs in is
// guaranteed to be observed live at least once, even in this file's third case, where the marker is
// already present the instant the pane is spawned and the script would otherwise exit before
// awaitDriverPane's own poll ever caught it alive -- a race, not a hypothetical, since a script with
// no delay at all checks and exits inside the same scheduling quantum tmux uses to report the pane
// live in the first place.
const driverStrandStubSettleDelay = 300 * time.Millisecond

// writeStubDriverScript writes a POSIX shell script standing in for the claude binary this file's
// spawns launch: it ignores its stdin (the prompt claudeengine's own launch line feeds via shell
// redirection) and every flag argument that line appends (--session-id, --settings, and the rest),
// sleeps driverStrandStubSettleDelay, then polls for exitMarker's presence every 100ms and exits 0 as
// soon as it appears.
//
// exitMarker's path is baked into the script's own text rather than read from an environment
// variable, because tmux's own server is not guaranteed to forward this test process's environment
// into the pane it spawns.
func writeStubDriverScript(t *testing.T, exitMarker string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub-claude.sh")
	script := fmt.Sprintf(`#!/bin/sh
sleep %s
while [ ! -f '%s' ]; do
  sleep 0.1
done
exit 0
`, driverStrandStubSettleDelay, exitMarker)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub driver script: %v", err)
	}
	return path
}

// driverShuttleConfig returns shuttle's shipped config template with the claude key pointed at
// stubPath. Unlike smoke_test.go's own providerlessShuttleConfig, this file's whole point is proving
// reed's own strand bookkeeping across a REAL spawned pane, so the provider path must resolve to a
// real (if stubbed) executable rather than a deliberately-broken one.
func driverShuttleConfig(stubPath string) string {
	cfg := shuttleengine.ConfigTemplate()
	return strings.Replace(cfg, "claude: ${env:LYX_SHUTTLE_CLAUDE:-}", "claude: "+stubPath, 1)
}

// seedLLMDriver writes loc's own self-run seed directly to disk with driver=llm, uncommitted --
// mirroring smoke_test.go's own seedAndCommitStatus and
// TestSmokeBootstrap_OriginRecordSelfHealsAfterCrashBetweenWriteAndCommit's shape of driving a
// production primitive directly rather than through a CLI subprocess.
//
// The written seed's Params must agree byte-for-byte with the one seedAndCommitBootstrap's own step
// 1b (loomSeedFor) writes on the first "loom start" against it, since WriteSeed refuses a disagreeing
// existing seed outright rather than silently accepting the later write: recorded.ParentBranch is
// read from the origin record newWiredPairFixture-style callers already committed via
// hubforge.AddPair, so this seed's own params.parent matches what the bootstrap itself would compute.
func seedLLMDriver(t *testing.T, loc *lyxcwd.Location) {
	t.Helper()
	recorded, found, err := fabricengine.ReadOrigin(loc)
	if err != nil || !found {
		t.Fatalf("ReadOrigin before seeding the llm driver: found=%v err=%v", found, err)
	}
	seed := shedrun.Seed{
		Recipe: shedrun.RecipeLoom,
		Driver: shedrun.DriverLLM,
		Params: map[string]string{"parent": recorded.ParentBranch},
	}
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, seed); err != nil {
		t.Fatalf("WriteSeed(llm driver): %v", err)
	}
}

// driverStrand returns the tracked strand named driverStrandDisplayName from eng's own Status(), and
// whether one was found -- the same findStatusStrand helper bootstrap.go already exports within this
// package, applied to the driver strand's own name instead of the status or operator strand's.
func driverStrand(t *testing.T, eng *reedengine.Engine) (reedengine.StrandStatus, bool) {
	t.Helper()
	status, err := eng.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	return findStatusStrand(status.Strands, driverStrandDisplayName)
}

// waitDriverStrandLive blocks until eng reports a live strand named driverStrandDisplayName, or fails
// the test after timeout.
func waitDriverStrandLive(t *testing.T, eng *reedengine.Engine, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if strand, found := driverStrand(t, eng); found && strand.Live {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("driver strand %q never became live within %s", driverStrandDisplayName, timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
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
// promptly after the stub's pane has exited, leaves exactly one once more and never a corpse plus a
// live pane beside it -- the count is what distinguishes corpse removal from a second add, a
// distinction reed's own upsert-less add cannot make for us.
func TestSmokeDriverStrand_ReentrantAcrossThreeBootstraps(t *testing.T) {
	tmuxBinaryPath(t)
	exe := buildLyxBinary(t)

	exitMarker := filepath.Join(t.TempDir(), "driver-exit-marker")
	stubPath := writeStubDriverScript(t, exitMarker)

	h := hubforge.NewHub(t, ".")
	hubforge.SeedConfig(t, h, map[string]string{
		"loom":    fastDeadlineLoomConfig(),
		"reed":    reedengine.ConfigTemplate(),
		"shuttle": driverShuttleConfig(stubPath),
		"webster": websterengine.ConfigTemplate(),
	})
	const slug = "loom-smoke-driver-task"
	hubforge.AddPair(t, h, slug)
	worktree := h.PairWarpWorktree(slug)

	loc, err := lyxcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", worktree, err)
	}
	registerBootstrapTeardown(t, loc, worktree)
	seedLLMDriver(t, loc)

	eng := probeReedEngine(t, loc)

	// (1) a first bootstrap leaves exactly one strand under the driver name, live.
	firstOut, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start", "--no-attach")
	if err != nil {
		t.Fatalf("first loom start: %v; output: %s", err, firstOut)
	}
	waitDriverStrandLive(t, eng, 10*time.Second)
	if count := statusStrandCount(t, eng, driverStrandDisplayName); count != 1 {
		t.Fatalf("driver strands after the first bootstrap = %d; want exactly 1", count)
	}

	// (2) a second bootstrap while the first driver's pane is still alive must leave exactly one
	// strand and must not touch the live pane the first bootstrap already spawned.
	secondOut, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start", "--no-attach")
	if err != nil {
		t.Fatalf("second loom start: %v; output: %s", err, secondOut)
	}
	if count := statusStrandCount(t, eng, driverStrandDisplayName); count != 1 {
		t.Fatalf("driver strands after the second bootstrap = %d; want exactly 1 -- a do-not-spawn verdict must leave reed holding one strand, not two", count)
	}
	if strand, found := driverStrand(t, eng); !found || !strand.Live {
		t.Fatalf("driver strand after the second bootstrap = (found=%v live=%v); want it still live and untouched by the second bootstrap's own no-op", found, found && strand.Live)
	}

	// Signal the stub to exit and wait for reed to observe the pane as dead before the third
	// bootstrap: the corpse-removal path this case exists to exercise only fires against a genuinely
	// dead pane, and running the third bootstrap against a still-live one would only repeat the
	// second case's own assertion.
	if err := os.WriteFile(exitMarker, []byte("go\n"), 0o644); err != nil {
		t.Fatalf("write exit marker: %v", err)
	}
	waitDriverStrandDead(t, eng, 10*time.Second)

	// (3) a third bootstrap, run promptly after the pane's own exit, removes the dead entry and
	// relaunches: exactly one strand again, never a corpse plus a live pane beside it. Landing this
	// relaunch inside one second of the corpse's own removal is deliberate: it is the case
	// driverReportPath's own random suffix exists for, since two attempts composing a report path in
	// the same clock second would otherwise collide and Spec.validate would refuse the second one
	// outright.
	thirdOut, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start", "--no-attach")
	if err != nil {
		t.Fatalf("third loom start: %v; output: %s", err, thirdOut)
	}
	if count := statusStrandCount(t, eng, driverStrandDisplayName); count != 1 {
		t.Fatalf("driver strands after the third bootstrap = %d; want exactly 1 -- corpse removal must replace the dead entry, never add a second one beside it", count)
	}
}

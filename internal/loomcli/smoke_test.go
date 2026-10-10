//go:build tmux

// smoke_test.go is the tagged smoke suite for the session bootstrap: the one thing nothing else in
// this package can catch, because it needs a real tmux server, a real detached driver process, and
// real advisory-lock interaction, not a hermetic fixture. It follows internal/reedcli's own smoke
// suite conventions (see its smoke_test.go/smoke_lifecycle_test.go) -- a real wired hub via
// hubforge.NewHub, a real tmux binary resolved from PATH or an override env var, skipping outright
// when the multiplexer is absent -- adapted for loom's own shape: every verb here goes through the
// REAL BUILT cmd/lyx binary as a genuine subprocess, never RunCLI in-process, because
// "lyx loom start" spawns its detached driver via os.Executable(), which resolves to whatever binary
// is currently running -- an in-process RunCLI call would resolve that to the test binary itself, not
// a binary "loom run" knows how to dispatch.
//
// This suite is the regression home for the two bugs this task's own design rounds found before any
// code existed: the cleanliness-ordering blocker (loom's own seed dirtying the records worktree and failing
// loom's own first precondition row -- smoke_prebootstrap_integration_test.go's TestLoomPreBootstrapPair) and the
// double-spawn window (the run lock being taken by the child long after the spawn call returns --
// TestSmokeBootstrapLifecycle's concurrent-bootstrap step).
//
// A note on driver-liveness timing: loom's own producer table (contracts/recipes/loom-recipe.yaml) backs every row with a real producer -- no row reports Done unconditionally.
// A freshly-bootstrapped driver against a pair with no discussion or plan artifacts yet still bounces at Discussion-Write's own gate a bounded number of times (its "gates" entry's attempts budget) and then blocks, well before reaching any later row -- a lifecycle that can still complete in well under a second.
// Tests here that assert "a driver process exists" treat that as a best-effort observation (logged, not failed, when the driver has already run to completion by check time) and lean on the STATUS FILE's own history -- durable regardless of whether the driver process itself is still alive -- for the assertions that must hold unconditionally.
package loomcli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// tmuxBinaryPath returns the tmux binary path from the environment or resolved via PATH, skipping
// the calling test when it is absent so a -tags=tmux run never hard-fails on a machine without the
// tool -- mirroring internal/reedcli's own tmuxBinaryPath.
func tmuxBinaryPath(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("LYX_LOOM_TMUX"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	path, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not found on PATH; set LYX_LOOM_TMUX to override")
	}
	return path
}

// registerBootstrapTeardown registers a cleanup that kills any surviving driver process for worktree
// and the per-hub watchdog daemon card 12 now spawns for it, then tears the reed substrate down, so
// a failed assertion never leaves a live tmux server, a detached driver, or a watchdog daemon running
// past the test.
//
// The watchdog reap is unconditional inventory, not a branch: out of process the smoke tier builds
// and runs a real binary, so testing.Testing() is false there and the spawn genuinely fires against
// this fixture's own hub, which this test then tears down. LYX_REED_WATCHDOG does not suppress it
// either -- that key reaches the worktree-level watch goroutine, never the process start.
func registerBootstrapTeardown(t *testing.T, loc *lyxcwd.Location, worktree string) {
	t.Helper()
	reedGeom, err := hubgeom.ReedGeometry(loc)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	// Registered before the first boot, so reed's boot finds the kit's hermetic server on the hub's key; the kill and socket-file removal run after the teardown below.
	tmuxkit.KillOnCleanup(t, tmuxBinaryPath(t), reedGeom.SocketKey)
	t.Cleanup(func() {
		for _, pid := range findDriverPIDs(worktree) {
			_ = proc.KillPID(pid)
		}
		for _, pid := range findWatchdogPIDs(loc.HubPath) {
			_ = proc.KillPID(pid)
		}
		eng := probeReedEngine(t, loc)
		_, _ = eng.Down()
	})
}

// probeReedEngine builds a standalone *reedengine.Engine against loc, independent of any loomCLI
// receiver, so a test can query reed's own Status()/Down() without going through the loom CLI seam.
func probeReedEngine(t *testing.T, loc *lyxcwd.Location) *reedengine.Engine {
	t.Helper()
	reedCfg, err := reedengine.LoadConfig(loc.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("load reed config: %v", err)
	}
	reedGeom, err := hubgeom.ReedGeometry(loc)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	return reedengine.New(reedCfg, reedGeom)
}

// waitRunLockFree polls until loc's run lock is observably free -- released by a driver that has run
// to completion on its own, or by one this test already killed -- or fails the test after timeout.
func waitRunLockFree(t *testing.T, loc *lyxcwd.Location, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		fl, free, err := lock.TryAcquireWriteLock(shedrun.RunLock(loc, shedrun.SelfRunID))
		if err != nil {
			t.Fatalf("probe run lock: %v", err)
		}
		if free {
			_ = fl.Release()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("run lock still held after %s", timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitForCurrentProducer polls loc's status file until it records want as the current producer with
// the machine still running, or fails the test after timeout.
//
// It exists because "kill the detached driver, then assert the row it was on" is a race unless the
// row is ESTABLISHED first, and a killed-at-an-arbitrary-moment driver lands on whichever row it
// happened to reach. Killing without this wait made
// TestSmokeRunStandalone's first step fail intermittently whenever the kill
// landed while the driver was still on Loom-Preflight: the follow-up drive then legitimately
// completed that row and advanced to the next one, so both the current_producer-unchanged and the
// history-unchanged assertions reported a routing bug that was not there (crucible round
// opus5-high-r7, F5 -- reproduced on the pre-round tree, so it predates that round's own changes).
//
// The poll interval is deliberately short relative to the window it is catching: with the fixture's
// startup_timeout_s of 2, Discussion-Write is the current producer for roughly two seconds before
// its providerless launch is classified, which a 25ms poll cannot miss.
func waitForCurrentProducer(t *testing.T, loc *lyxcwd.Location, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for {
		st, found, err := state.ReadJSONStrict[shedengine.Status](shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID))
		if err == nil && found {
			last = string(st.CurrentProducer)
			if st.CurrentProducer == want && st.State == shedengine.StateRunning {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("status file never recorded current_producer %q while running within %s (last seen %q); the driver never reached the row this test kills it on", want, timeout, last)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// poisonStatusFile adds an unrecognised top-level field to loc's already-seeded status file and
// commits the change records-side. This is the whole rig behind the driver-failure regression cases: the
// lenient json.Unmarshal shedengine.persist and loomshed.Seed both read through
// (internal/state.UpdateJSON) tolerates an unknown field, so ordinary re-entrant seeding stays
// unaffected, while the strict decoder Shed.Run's own read gate uses (state.ReadJSONStrict,
// DisallowUnknownFields) rejects it outright, and does so BEFORE the loop ever calls a producer or
// performs a persist -- an environment condition the production code already honours, not a change to
// it.
func poisonStatusFile(t *testing.T, loc *lyxcwd.Location) {
	t.Helper()
	statusPath := shedrun.StatusFile(loc, shedrun.SelfRunID)

	raw, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status file: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal status file: %v", err)
	}
	fields["__smoke_unknown_field__"] = true
	poisoned, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		t.Fatalf("marshal poisoned status: %v", err)
	}
	if err := os.WriteFile(statusPath, poisoned, 0o644); err != nil {
		t.Fatalf("write poisoned status: %v", err)
	}

	rec := fabricengine.NewMutations("")
	if _, _, err := fabricengine.CommitRecordsPaths(rec, fabricengine.RecordsWorktree(loc), loc.AnchorRel, []string{smokeStatusRel(loc)}, "smoke: poison status file for driver-failure rig", fabricengine.EnvSyncOptions()); err != nil {
		t.Fatalf("commit poisoned status: %v", err)
	}
}

// poisonStatusFileMalformed overwrites loc's already-seeded status file with genuinely malformed
// JSON (not merely an unknown field) and commits it records-side. It is the second poison shape the
// crash-recovery design promises never looks like bootstrap's own gate: unlike the unknown-field
// shape poisonStatusFile writes, malformed JSON does not decode even leniently, so before crucible
// round fable5-high-r5's F3 fix it made loomshed.Seed (and therefore `lyx loom start`) refuse on the
// envelope before ever spawning a driver. loomshed.Seed now maps a decode failure to ErrSeedExists,
// so both poison shapes reach the same "a driver that died is a run that finished" bootstrap path.
func poisonStatusFileMalformed(t *testing.T, loc *lyxcwd.Location) {
	t.Helper()
	statusPath := shedrun.StatusFile(loc, shedrun.SelfRunID)
	if err := os.WriteFile(statusPath, []byte(`{ "current_producer": "Discussion-Write", "state": "run`), 0o644); err != nil {
		t.Fatalf("write malformed status: %v", err)
	}
	rec := fabricengine.NewMutations("")
	if _, _, err := fabricengine.CommitRecordsPaths(rec, fabricengine.RecordsWorktree(loc), loc.AnchorRel, []string{smokeStatusRel(loc)}, "smoke: malformed status file for driver-failure rig", fabricengine.EnvSyncOptions()); err != nil {
		t.Fatalf("commit malformed status: %v", err)
	}
}

// statusStrandCount returns how many of eng's tracked strands agentname.Matches accepts for name,
// the matcher production finds a strand by, since production records a strand under a full agent name.
func statusStrandCount(t *testing.T, eng *reedengine.Engine, name string) int {
	t.Helper()
	return countStrands(t, eng, func(strandName string) bool { return agentname.Matches(strandName, name) })
}

// driverStrandCount returns how many of eng's tracked strands loomengine.IsDriverStrand accepts,
// the matcher production finds the driver by, so a driver under the legacy literal counts too.
func driverStrandCount(t *testing.T, eng *reedengine.Engine) int {
	t.Helper()
	return countStrands(t, eng, loomengine.IsDriverStrand)
}

func countStrands(t *testing.T, eng *reedengine.Engine, accept func(strandName string) bool) int {
	t.Helper()
	status, err := eng.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	count := 0
	for _, s := range status.Strands {
		if accept(s.Name) {
			count++
		}
	}
	return count
}

// TestSmokeRunStandalone drives the standalone run verb over one bootstrapped pair, in this order:
// the verb advancing the machine from an existing seed, then the same verb failing on a poisoned status file.
// The second step builds on the status file the first leaves behind.
func TestSmokeRunStandalone(t *testing.T) {
	tmuxBinaryPath(t)
	exe := sharedLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	// "loom run" calls reed.Up() before the phase machine runs, so the second step brings a real tmux
	// server up even though it never reaches a producer.
	registerBootstrapTeardown(t, loc, worktree)
	seedGoDriverRun(t, loc)

	// The run verb standalone with no tmux at all, on a pair the fixture seeded by running the
	// bootstrap once and then killing the driver, advances the machine and records that advance in the
	// status file.
	t.Run("advances the machine from an existing seed", func(t *testing.T) {
		stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start")
		if err != nil {
			t.Fatalf("bootstrap: %v; output: %s", err, stdout)
		}

		// Establish WHICH row the driver is killed on before killing it. Every assertion below is about
		// the follow-up drive re-entering that same row, and a driver killed at an arbitrary moment lands
		// on whichever row it happened to reach -- see waitForCurrentProducer for the intermittent
		// failure that made this explicit.
		waitForCurrentProducer(t, loc, "Discussion-Write", 30*time.Second)

		for _, pid := range findDriverPIDs(worktree) {
			_ = proc.KillPID(pid)
		}
		waitRunLockFree(t, loc, 20*time.Second)

		before, foundBefore, err := state.ReadJSONStrict[shedengine.Status](shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID))
		if err != nil || !foundBefore {
			t.Fatalf("read status file before standalone drive: found=%v err=%v", foundBefore, err)
		}
		if before.State != shedengine.StateRunning {
			t.Fatalf("status state before standalone drive = %q; want %q -- the killed driver must have left its row in flight", before.State, shedengine.StateRunning)
		}
		if before.CurrentProducer != "Discussion-Write" {
			t.Fatalf("current_producer before standalone drive = %q; want %q -- the kill landed on a different row than the one waited for, so the attribution assertions below would be racing rather than testing", before.CurrentProducer, "Discussion-Write")
		}

		// The timeout is generous and the exit code is deliberately not asserted. What this case is
		// about is the VERB advancing the machine standalone, and in a fixture with no provider the row
		// it advances into ends in a launch failure -- a legitimate non-zero exit that says the driver
		// did its job and the (absent) agent did not. Asserting exit 0 instead made this test depend on
		// a real provider session completing, which is how it came to spawn one, block on it, and time
		// out.
		//
		// The durable assertion is the status file's own state/error transition, NOT history length: a
		// producer call that returns an error reaches no verdict, and shedengine's own appendHistory
		// (see its doc comment, and internal/shedengine/run_routing_test.go's TestRun_ProducerError)
		// deliberately records no history entry for it -- current_producer, state, and error carry the
		// failure instead. A prior version of this test asserted history growth here and passed only by
		// accident, because the OLD, buggy shuttleengine Wait took the full run/discussion timeout
		// (~61s) to classify the dead pane, long enough that the assertion was never reached before this
		// test's own timeout in CI; once Wait's started-gating fix let the dead pane classify fast
		// (~startup_timeout_s), the same "no history entry" outcome surfaced immediately and revealed
		// the stale assertion.
		driveOut, driveCode, err := runLoomCLINoFatal(exe, worktree, 3*time.Minute, "loom", "run")
		if err != nil {
			t.Fatalf("loom run: %v; output: %s", err, driveOut)
		}
		t.Logf("loom run exited %d: %s", driveCode, strings.TrimSpace(driveOut))

		after, foundAfter, err := state.ReadJSONStrict[shedengine.Status](shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID))
		if err != nil || !foundAfter {
			t.Fatalf("read status file after standalone drive: found=%v err=%v", foundAfter, err)
		}
		if after.State != shedengine.StateFailed {
			t.Errorf("status state after standalone drive = %q; want %q -- the machine must advance from running to a durably recorded failure", after.State, shedengine.StateFailed)
		}
		if after.Error == "" {
			t.Errorf("status error after standalone drive is empty; want the shuttle failure's own text recorded")
		}
		if after.CurrentProducer != before.CurrentProducer {
			t.Errorf("current_producer changed from %q to %q; want it unchanged -- the failure is attributed to the same row, not routed onward", before.CurrentProducer, after.CurrentProducer)
		}
		if len(after.History) != len(before.History) {
			t.Errorf("history length changed from %d to %d; want unchanged -- a producer call that reached no verdict records no history entry", len(before.History), len(after.History))
		}
	})

	// A driver rigged to fail before its first persist leaves a non-empty driver log naming the
	// failure. See poisonStatusFile's doc comment for the rig itself: an unknown field state.UpdateJSON
	// tolerates but state.ReadJSONStrict rejects, so Shed.Run's own read gate fails before the loop ever
	// calls a producer -- before any persist can happen.
	t.Run("failure before the first persist leaves a non-empty driver log", func(t *testing.T) {
		poisonStatusFile(t, loc)
		poisonedBytes, err := os.ReadFile(shedrun.StatusFile(loc, shedrun.SelfRunID))
		if err != nil {
			t.Fatalf("read poisoned status file: %v", err)
		}

		logPath := filepath.Join(t.TempDir(), "driver.log")
		logFile, err := os.Create(logPath)
		if err != nil {
			t.Fatalf("create driver log: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "loom", "run")
		cmd.Dir = worktree
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		runErr := cmd.Run()
		_ = logFile.Close()
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatalf("loom run against a poisoned status file timed out")
		}
		if runErr == nil {
			t.Fatalf("loom run against a poisoned status file succeeded; want a failure before any persist")
		}

		content, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read driver log: %v", err)
		}
		if strings.TrimSpace(string(content)) == "" {
			t.Fatalf("driver log is empty; want it to name the failure")
		}
		if !strings.Contains(strings.ToLower(string(content)), "decode") {
			t.Errorf("driver log = %q; want it to name the decode failure", content)
		}

		after, err := os.ReadFile(shedrun.StatusFile(loc, shedrun.SelfRunID))
		if err != nil {
			t.Fatalf("read status file after the failed drive: %v", err)
		}
		if string(after) != string(poisonedBytes) {
			t.Errorf("status file changed after the failed drive; want it untouched -- a persist must never have happened")
		}
	})
}

// recordsPathspecStatus returns dir's `git status --porcelain` output scoped to relPath, empty when
// relPath is fully committed and unchanged.
func recordsPathspecStatus(t *testing.T, dir, relPath string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain", "--", relPath).Output()
	if err != nil {
		t.Fatalf("git -C %s status --porcelain -- %s: %v", dir, relPath, err)
	}
	return strings.TrimSpace(string(out))
}

// TestSmokeBootstrapLifecycle drives "lyx loom start" over one wired, go-seeded pair, in this order, each step building on the state the one before left:
// two concurrent first bootstraps, the strand, driver and status file they leave, a second bootstrap, a crash-healed origin record, then the two rigged died-driver bootstraps.
// The steps that rig a died driver kill any surviving driver first and clear the driver log, so each asserts on its own driver's log.
func TestSmokeBootstrapLifecycle(t *testing.T) {
	tmuxBinaryPath(t)
	exe := sharedLyxBinary(t)
	_, loc, worktree, slug := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)
	seedGoDriverRun(t, loc)

	killDriversAndClearLog := func(t *testing.T) {
		t.Helper()
		for _, pid := range findDriverPIDs(worktree) {
			_ = proc.KillPID(pid)
		}
		waitRunLockFree(t, loc, 20*time.Second)
		if err := os.Remove(loomengine.LoomDriverLog(loc)); err != nil && !os.IsNotExist(err) {
			t.Fatalf("clear driver log: %v", err)
		}
	}

	// The spawn handshake -- two bootstrap invocations started concurrently produce exactly one
	// driver process and no already-running refusal in the driver log. This is a named regression
	// guard: the pre-fix defect was the run lock being taken by the child long after the spawn call
	// returned, which raced two concurrent "lyx loom start" invocations into each believing it must spawn
	// its own driver.
	t.Run("concurrent first bootstraps yield one driver", func(t *testing.T) {
		// A background sampler polls the live driver-pid count for the whole duration both "loom start"
		// invocations are in flight, tracking the maximum ever observed -- a post-hoc single check after
		// both return cannot prove a double-spawn never happened transiently, since either or both
		// drivers may already have finished their own bounded bounce loop by the time a caller checks
		// only at the end. This is the mechanism assertion the card requires, not merely a happy outcome.
		stopSampling := make(chan struct{})
		var sampleWG sync.WaitGroup
		var maxConcurrentDrivers int
		sampleWG.Add(1)
		go func() {
			defer sampleWG.Done()
			for {
				select {
				case <-stopSampling:
					return
				default:
				}
				if n := len(findDriverPIDs(worktree)); n > maxConcurrentDrivers {
					maxConcurrentDrivers = n
				}
				time.Sleep(2 * time.Millisecond)
			}
		}()

		const runners = 2
		outputs := make([]string, runners)
		runErrs := make([]error, runners)
		var wg sync.WaitGroup
		for i := 0; i < runners; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				outputs[i], _, runErrs[i] = runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start")
			}(i)
		}
		wg.Wait()
		close(stopSampling)
		sampleWG.Wait()

		for i, err := range runErrs {
			if err != nil {
				t.Fatalf("concurrent run #%d: %v; output: %s", i, err, outputs[i])
			}
		}

		if maxConcurrentDrivers > 1 {
			t.Errorf("observed %d driver processes alive at once during the concurrent bootstrap; want at most 1 -- the bootstrap lock must serialize the spawn decision", maxConcurrentDrivers)
		}

		logContent, _ := os.ReadFile(loomengine.LoomDriverLog(loc))
		if strings.Contains(strings.ToLower(string(logContent)), "already running") {
			t.Errorf("driver log names an already-running refusal: %s", logContent)
		}
	})

	// The bootstrap in a real wired hub brings the tmux session up, leaves exactly one status
	// strand present under its fixed name, leaves a detached driver process alive, and leaves the status
	// file seeded.
	// This step asserts on what the concurrent bootstraps above left, and no overall envelope:
	// a fast-finishing driver (loom's own bounded bounce loop can finish inside a single poll gap)
	// can make the handshake itself report a refusal even though every one of this step's own assertions already succeeded.
	// See the file-level doc comment.
	t.Run("bootstrap leaves one status strand, a driver and a seeded status file", func(t *testing.T) {
		eng := probeReedEngine(t, loc)
		if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
			t.Errorf("status strands named %q = %d; want exactly 1", statusStrandDisplayName, count)
		}

		if pids := findDriverPIDs(worktree); len(pids) == 0 {
			// See the file-level doc comment's note on driver-liveness timing: loom's own bounded
			// bounce loop can finish before this check runs. Logged rather than failed, since the
			// durable status-file assertion below is what actually pins "the driver ran".
			t.Logf("no detached driver process observed for %s; loom's bounded bounce loop may already have finished", worktree)
		} else if !proc.IsAlive(pids[0]) {
			t.Errorf("driver pid %d found but not alive", pids[0])
		}

		st, found, err := state.ReadJSONStrict[shedengine.Status](shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID))
		if err != nil || !found {
			t.Fatalf("read status file after bootstrap: found=%v err=%v", found, err)
		}
		if st.CurrentProducer == "" {
			t.Errorf("seeded status file has an empty current_producer")
		}
	})

	// A second bootstrap invocation while the first driver holds the run lock leaves still exactly
	// one strand and still exactly one driver process, spawns nothing new, and does not surface the
	// seed-exists refusal as a failure.
	// The overall envelope is not asserted ok:true, for the same fast-driver-race reason the file-level doc comment states:
	// what this step guards is that the SEED-EXISTS sentinel specifically is never what surfaces as a failure,
	// which is asserted directly against the error text below rather than against ok:false's mere presence.
	t.Run("second invocation does not spawn a second driver", func(t *testing.T) {
		firstPIDs := findDriverPIDs(worktree)

		stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start")
		if err != nil {
			t.Fatalf("second loom start: %v; output: %s", err, stdout)
		}
		if strings.Contains(stdout, `"ok":false`) && strings.Contains(strings.ToLower(stdout), "already exists") {
			t.Errorf("second bootstrap surfaced the seed-exists sentinel as a failure (must be tolerated, not surfaced): %s", stdout)
		}

		secondPIDs := findDriverPIDs(worktree)
		if len(firstPIDs) == 1 && len(secondPIDs) == 1 {
			if secondPIDs[0] != firstPIDs[0] {
				t.Errorf("driver pid after the second bootstrap = %d; want the same pid %d the first bootstrap spawned (no second driver)", secondPIDs[0], firstPIDs[0])
			}
		} else if len(secondPIDs) > 1 {
			t.Errorf("driver pids after two bootstraps = %v; want at most 1 alive at once", secondPIDs)
		} else {
			t.Logf("driver pid observation was inconclusive (first=%v second=%v); loom's bounded bounce loop may already have finished -- see the file-level doc comment", firstPIDs, secondPIDs)
		}

		eng := probeReedEngine(t, loc)
		if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
			t.Errorf("status strands named %q after two bootstraps = %d; want exactly 1", statusStrandDisplayName, count)
		}
	})

	// The regression guard for the origin-record self-healing gap the holistic review found: a
	// legacy pair whose provenance record was written to disk (step 1 of "loom start") but never committed
	// records-side (step 3), because the process died in between. The very next "loom start" must find the
	// record already present on disk with a matching value -- resolveParentBranch's ordinary re-run row,
	// requesting no write of its own -- and still commit the still-untracked record, exactly as the status
	// file already self-heals, rather than leaving it stranded as an untracked file that permanently fails
	// fabricengine.Clean's own first Preflight precondition row.
	t.Run("origin record self-heals after a crash between write and commit", func(t *testing.T) {
		recordsDir := fabricengine.RecordsWorktree(loc)
		originRel := filepath.Join(loc.AnchorRel, fabricengine.OriginRecordRel())

		// Roll the pair back to a legacy shape: no origin record tracked at all, as if the pair had been
		// created before the record existed.
		gitkit.Git(t, recordsDir, "rm", "-q", "--", originRel)
		gitkit.Git(t, recordsDir, "commit", "-m", "smoke: simulate legacy pair with no origin record")

		// Simulate the crash: write the record straight to disk through the same production primitive
		// step 1 itself uses, but never commit it -- the exact state a process death between steps 1 and
		// 3 would leave behind.
		rec := fabricengine.NewMutations("")
		if err := fabricengine.WriteOrigin(rec, loc, slug, fabricengine.Origin{ParentBranch: "main"}); err != nil {
			t.Fatalf("WriteOrigin (simulated crash write): %v", err)
		}

		if status := recordsPathspecStatus(t, recordsDir, originRel); status == "" {
			t.Fatalf("origin record status before the healing run = clean; want the simulated crash to leave it uncommitted")
		}

		// The next "loom start" needs no --parent: ReadOrigin finds the just-written record on disk with a
		// matching value, so resolveParentBranch reports write == false -- the exact row the finding says
		// used to strand the record forever.
		stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start")
		if err != nil {
			t.Fatalf("healing loom start: %v; output: %s", err, stdout)
		}

		// The origin record specifically must now be committed and clean. The overall records worktree is not
		// asserted clean here: once a driver actually runs, its own persists rewrite the status file's
		// working-tree content on every phase transition without ever committing those rewrites, which
		// legitimately dirties the records worktree again for a reason that has nothing to do with the
		// origin-record healing this step exists to pin.
		if status := recordsPathspecStatus(t, recordsDir, originRel); status != "" {
			t.Errorf("origin record status after the healing run = %q; want it committed and clean", status)
		}
	})

	// The handshake's died-child disposition -- a driver rigged to die immediately does NOT make the bootstrap refuse.
	// dispositionForHandshake maps awaitRunLockChildDied onto proceed, and deliberately so:
	// a child already gone before the handshake's first poll is a driver that RAN AND FINISHED,
	// and refusing there would tell an operator the bootstrap broke when in fact their task halted, while withholding the success envelope.
	// Only awaitRunLockDeadline -- a child still alive after the whole attempt budget, never having taken the lock -- is a genuine refusal,
	// and a dying driver cannot produce that.
	//
	// What it pins is the disposition that actually ships, plus the two properties that make it safe:
	// the driver log carries the real reason the run halted, and the bootstrap lock is released rather
	// than stranded.
	//
	// The rig is the poisoned-status-file mechanism poisonStatusFile documents, applied after the earlier
	// steps' genuine bootstraps so the pair already has a live reed substrate and the run lock has been
	// observed free again. The bootstrap's own steps 1-4 use only the lenient read paths (ReadOrigin,
	// Seed, CommitRecordsPaths) that tolerate the poisoned field, so only the spawned CHILD's strict read
	// gate ever sees the failure.
	t.Run("died driver proceeds to the success envelope and logs why", func(t *testing.T) {
		killDriversAndClearLog(t)
		poisonStatusFile(t, loc)

		stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start")
		if err != nil {
			t.Fatalf("rigged bootstrap: %v; output: %s", err, stdout)
		}

		// Proceeded to step 7 rather than refusing: the success envelope is the evidence.
		if !strings.Contains(stdout, `"ok":true`) {
			t.Errorf("rigged bootstrap output = %q; want the ok envelope, not a refusal before it", stdout)
		}
		if strings.Contains(stdout, `"ok":false`) {
			t.Errorf("rigged bootstrap emitted a refusal envelope: %s -- a driver that died is a run that finished, not a broken bootstrap", stdout)
		}

		// The rig is confirmed by the driver log's own content: the poisoned read gate's decode failure,
		// not an ordinary completed run. Without this the step would pass against any fast-exiting
		// driver and prove nothing about the rig.
		wantLog := loomengine.LoomDriverLog(loc)
		driverLog, err := os.ReadFile(wantLog)
		if err != nil {
			t.Fatalf("read driver log %s: %v", wantLog, err)
		}
		if !strings.Contains(strings.ToLower(string(driverLog)), "decode") {
			t.Errorf("driver log = %q; want it to name the poisoned status file's decode failure -- the reason the run halted must survive in the log even though the bootstrap proceeded", driverLog)
		}

		fl, free, err := lock.TryAcquireWriteLock(loomengine.LoomBootstrapLock(loc))
		if err != nil {
			t.Fatalf("probe bootstrap lock: %v", err)
		}
		if !free {
			t.Errorf("bootstrap lock still held after the bootstrap; want it released")
		} else {
			_ = fl.Release()
		}
	})

	// `lyx loom start` against a MALFORMED-JSON status file proceeds to the success envelope exactly as the unknown-field shape does, rather than refusing on the envelope at the Seed step.
	// This is a regression guard for a crucible finding, and the composed CLI-verb half its unit tests (loomshed.TestSeed_RefusesUndecodableFileAsExists, state.TestCorruptFile) cannot see:
	// only the real `lyx loom start` binary exercises the whole Seed -> VerifySeedOwnership -> commit -> spawn -> handshake chain against a poisoned records-committed status file.
	//
	// Before the fix, loomshed.Seed returned the raw decode error (not ErrSeedExists) for malformed
	// JSON, so step 2 of `lyx loom start` refused on the envelope before ever spawning a driver — the very
	// "poisoned status file looks like bootstrap's own gate" state the crash-recovery design forbids.
	// After it, Seed maps the decode failure to ErrSeedExists, the bootstrap tolerates it, and the
	// spawned driver's own Shed.Run step-1 read gate diagnoses the decode failure in the driver log,
	// exactly as the died-driver step pins for the unknown-field shape.
	t.Run("malformed status proceeds to the success envelope and logs why", func(t *testing.T) {
		killDriversAndClearLog(t)
		poisonStatusFileMalformed(t, loc)

		stdout, _, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", "start")
		if err != nil {
			t.Fatalf("malformed-status bootstrap: %v; output: %s", err, stdout)
		}

		// Proceeded to step 7 rather than refusing at step 2's Seed: the success envelope is the evidence.
		if !strings.Contains(stdout, `"ok":true`) {
			t.Errorf("malformed-status bootstrap output = %q; want the ok envelope, not a Seed refusal before it", stdout)
		}
		if strings.Contains(stdout, `"ok":false`) {
			t.Errorf("malformed-status bootstrap emitted a refusal envelope: %s -- a malformed status file must defer to the driver's own read gate, not refuse at the Seed step", stdout)
		}

		// The driver log carries the decode failure from Shed.Run's own step-1 read gate.
		driverLog, err := os.ReadFile(loomengine.LoomDriverLog(loc))
		if err != nil {
			t.Fatalf("read driver log: %v", err)
		}
		if !strings.Contains(strings.ToLower(string(driverLog)), "decode") {
			t.Errorf("driver log = %q; want it to name the malformed status file's decode failure", driverLog)
		}
	})
}

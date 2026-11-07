// smoke_test.go is the tagged smoke suite for the session bootstrap.
// It covers what the hermetic tests cannot reach.
// It needs a real tmux server, a real detached driver process, and real advisory-lock interaction.
// It follows the smoke suite conventions of internal/reedcli.
// It wires a real hub with hubforge.NewHub and resolves a real tmux binary from PATH or an override env var.
// It skips the suite when the multiplexer is absent.
// Every verb here runs the built cmd/lym binary as a genuine subprocess, never RunCLI in-process.
// "lym loom start" spawns its detached driver via os.Executable(), which resolves to the running binary,
// so an in-process call would resolve it to the test binary, not a binary "loom drive" knows how to dispatch.
//
// The suite guards two regressions.
// The first is the cleanliness ordering: loom's own seed dirtied the records worktree.
// That failed loom's first precondition row, which TestLoomPreBootstrapPair pins.
// The second is the double-spawn window: the child took the run lock long after the spawn call returned.
// TestSmokeBootstrapLifecycle's concurrent-bootstrap step pins it.
//
// Driver liveness is timing-dependent.
// The producer table in contracts/recipes/tapestry-recipe.yaml backs every row with a real producer,
// so no row reports Done unconditionally.
// A freshly bootstrapped driver against a pair with no discussion or plan artifacts bounces at Discussion-Write's gate.
// It does so a bounded number of times, set by the attempts budget of its "gates" entry.
// It then blocks well before any later row, so the lifecycle completes in well under a second.
// Tests that assert a driver process exists treat that as a best-effort observation:
// they log it rather than fail when the driver has already run to completion by check time.
// Assertions that must always hold read the status file's history instead.
// That history stays durable whether or not the driver process is still alive.
package loomcli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// tmuxBinaryPath returns the tmux binary path from LYM_LOOM_TMUX or from PATH.
// It skips the calling test when tmux is absent,
// so a -tags=tmux run does not fail on a machine without the tool.
func tmuxBinaryPath(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("LYM_LOOM_TMUX"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	path, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not found on PATH; set LYM_LOOM_TMUX to override")
	}
	return path
}

// registerBootstrapTeardown registers a cleanup that kills any surviving driver process for worktree
// and the per-hub watchdog daemon, then tears the reed substrate down.
// A failed assertion then never leaves a live tmux server, a detached driver, or a watchdog daemon behind.
//
// The watchdog reap is unconditional, not a branch.
// The smoke tier builds and runs a real binary, so testing.Testing() is false there,
// and the spawn fires against this fixture's own hub, which this test then tears down.
// LYM_REED_WATCHDOG does not suppress the spawn:
// that key reaches the worktree-level watch goroutine, never the process start.
func registerBootstrapTeardown(t *testing.T, loc *lymcwd.Location, worktree string) {
	t.Helper()
	reedGeom, err := hubgeom.ReedGeometry(loc)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	// Registered before the first boot, so reed's boot finds the kit's hermetic server on the hub's key.
	// The kill and socket-file removal run after the teardown below.
	tmuxkit.KillOnCleanup(t, tmuxBinaryPath(t), reedGeom.SocketKey)
	t.Cleanup(func() {
		killGoDriver(worktree)
		for _, pid := range findWatchdogPIDs(loc.HubPath) {
			_ = proc.KillPID(pid)
		}
		eng := probeReedEngine(t, loc)
		_, _ = eng.Down()
	})
}

// killGoDriver kills the Go driver of worktree first, so it steps no more.
// It then kills the loop and step processes the driver spawned, which outlive it.
func killGoDriver(worktree string) {
	for _, pid := range findDriverPIDs(worktree) {
		_ = proc.KillPID(pid)
	}
	for _, pid := range findStepPIDs(worktree) {
		_ = proc.KillPID(pid)
	}
}

// findStepPIDs returns the pids of every live process whose current working directory is worktree.
// Such a process has an adjacent "loom" followed by "step" pair in its argv:
// the detached loop, or the step child the Go driver's --until-stop waiter spawns.
// Linux only; returns nil on any other GOOS.
func findStepPIDs(worktree string) []int {
	if runtime.GOOS != "linux" {
		return nil
	}
	var pids []int
	for _, pid := range tmuxkit.Pids() {
		cwd, ok := tmuxkit.ProcCwd(pid)
		if !ok || cwd != worktree {
			continue
		}
		argv, ok := tmuxkit.ProcArgv(pid)
		if !ok {
			continue
		}
		for i := 0; i+1 < len(argv); i++ {
			if argv[i] == "loom" && argv[i+1] == "step" {
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids
}

// probeReedEngine builds a standalone *reedengine.Engine for loc,
// so a test can query reed's own Status and Down without going through the loom CLI.
func probeReedEngine(t *testing.T, loc *lymcwd.Location) *reedengine.Engine {
	t.Helper()
	reedCfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: loc.AnchorPath()}, "reed")
	if err != nil {
		t.Fatalf("load reed config: %v", err)
	}
	reedGeom, err := hubgeom.ReedGeometry(loc)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	return reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
}

// waitRunLockFree polls until loc's run and driver locks are both observably free, or fails the test after timeout.
// A lock is released once its driver has run to completion, or once this test has killed it.
func waitRunLockFree(t *testing.T, loc *lymcwd.Location, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		runFree := lockFree(t, shedrun.RunLock(loc, shedrun.SelfRunID))
		driverFree := lockFree(t, shedrun.DriverLock(loc, shedrun.SelfRunID))
		if runFree && driverFree {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("run lock free = %v and driver lock free = %v after %s", runFree, driverFree, timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitRecordsIndexLockGone polls until the records worktree's git index.lock is absent, or fails the test after timeout.
// A killed driver can leave a git child it started holding the lock for a moment after the driver's own locks are free.
func waitRecordsIndexLockGone(t *testing.T, loc *lymcwd.Location, timeout time.Duration) {
	t.Helper()
	recordsDir := fabricengine.RecordsWorktree(loc)
	out, err := exec.Command("git", "-C", recordsDir, "rev-parse", "--git-path", "index.lock").Output()
	if err != nil {
		t.Fatalf("git -C %s rev-parse --git-path index.lock: %v", recordsDir, err)
	}
	lockPath := strings.TrimSpace(string(out))
	if !filepath.IsAbs(lockPath) {
		lockPath = filepath.Join(recordsDir, lockPath)
	}
	deadline := time.Now().Add(timeout)
	for {
		_, err := os.Stat(lockPath)
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			t.Fatalf("stat %s: %v", lockPath, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("records index lock %s still present after %s", lockPath, timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// lockFree reports whether the lock at path is free, probing it without keeping it.
// A lock whose directory does not exist yet has never been taken, so it reads free.
func lockFree(t *testing.T, path string) bool {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the lock directory for %s: %v", path, err)
	}
	fl, free, err := lock.TryAcquireWriteLock(path)
	if err != nil {
		t.Fatalf("probe lock %s: %v", path, err)
	}
	if free {
		_ = fl.Release()
	}
	return free
}

// waitForCurrentProducer polls loc's status file until it records want as the current producer with
// the machine still running, or fails the test after timeout.
//
// A test that kills the detached driver must first wait until the row it means to kill is established.
// A driver killed at an arbitrary moment lands on whichever row it happened to reach,
// so an assertion about the row it was on races.
//
// The poll interval is short relative to that window.
// The fixture's startup_timeout_s is 2.
// Discussion-Write is the current producer for roughly two seconds before its providerless launch is classified.
// A 25ms poll cannot miss that window.
func waitForCurrentProducer(t *testing.T, loc *lymcwd.Location, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for {
		st, found, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID))
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

// readDriverLogContaining polls loc's driver log until its lowercased content contains want or 20 seconds pass.
// It returns the content of the last read.
// The bootstrap can return while the driver is still between taking the run lock and writing its failure,
// so a single read can see an empty log.
func readDriverLogContaining(loc *lymcwd.Location, want string) []byte {
	logPath := loomengine.LoomDriverLog(loc)
	deadline := time.Now().Add(20 * time.Second)
	for {
		content, _ := os.ReadFile(logPath)
		if strings.Contains(strings.ToLower(string(content)), want) || time.Now().After(deadline) {
			return content
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// poisonStatusFile rewrites the state of loc's already-seeded status file to an unrecognised value.
// It then commits the change records-side, and the driver-failure regression cases rely on this poisoned state.
// The file still decodes, so loomshed.Seed's UpdateJSON takes its found branch.
// Ordinary re-entrant seeding stays unaffected.
// Shed.Run's read gate refuses the unrecognised state before the loop calls a producer or persists.
// The rig changes no production code: production already refuses this state.
func poisonStatusFile(t *testing.T, loc *lymcwd.Location) {
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
	fields["state"] = "poisoned"
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

// poisonStatusFileMalformed overwrites loc's already-seeded status file with malformed JSON and commits it records-side.
// Unlike the unrecognised state poisonStatusFile writes, malformed JSON does not decode at all.
// loomshed.Seed maps a decode failure to ErrSeedExists, so both poison shapes take the same bootstrap path.
// On that path a driver that died is a run that finished.
func poisonStatusFileMalformed(t *testing.T, loc *lymcwd.Location) {
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

// statusStrandCount returns how many of eng's tracked strands agentname.Matches accepts for name.
// It uses the matcher production finds a strand by, since production records a strand under a full agent name.
func statusStrandCount(t *testing.T, eng *reedengine.Engine, name string) int {
	t.Helper()
	return countStrands(t, eng, func(strandName string) bool { return agentname.Matches(strandName, name) })
}

// driverStrandCount returns how many of eng's tracked strands loomengine.IsDriverStrand accepts.
// It uses the matcher production finds the driver by, so a driver under the legacy literal counts too.
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
	tiergate.Require(t, 2)
	tmuxBinaryPath(t)
	exe := lymbin.Build(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	// "tapestry run" calls reed.Up() before the phase machine runs, so the second step brings a real tmux
	// server up even though it never reaches a producer.
	registerBootstrapTeardown(t, loc, worktree)
	seedGoDriverRun(t, loc)

	// The run verb, standalone with no tmux at all, advances the machine.
	// It runs on a pair that the fixture seeded by running the bootstrap once and then killing the driver;
	// the advance is recorded in the status file.
	t.Run("advances the machine from an existing seed", func(t *testing.T) {
		stdout, _, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, 30*time.Second, "loom", "start")
		if err != nil {
			t.Fatalf("bootstrap: %v; output: %s", err, stdout)
		}

		// Establish which row the driver is killed on before killing it.
		// Every assertion below is about the follow-up drive re-entering that same row.
		// A driver killed at an arbitrary moment lands on whichever row it reached.
		// waitForCurrentProducer waits for that row first.
		waitForCurrentProducer(t, loc, "Discussion-Write", 30*time.Second)

		killGoDriver(worktree)
		waitRunLockFree(t, loc, 20*time.Second)

		before, foundBefore, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID))
		if err != nil || !foundBefore {
			t.Fatalf("read status file before standalone drive: found=%v err=%v", foundBefore, err)
		}
		if before.State != shedengine.StateRunning {
			t.Fatalf("status state before standalone drive = %q; want %q -- the killed driver must have left its row in flight", before.State, shedengine.StateRunning)
		}
		if before.CurrentProducer != "Discussion-Write" {
			t.Fatalf("current_producer before standalone drive = %q; want %q -- the kill landed on a different row than the one waited for, so the attribution assertions below would be racing rather than testing", before.CurrentProducer, "Discussion-Write")
		}

		// The timeout is generous and the exit code is not asserted.
		// This case is about the verb advancing the machine standalone.
		// In a fixture with no provider, the row it advances into ends in a launch failure.
		// That non-zero exit says the driver did its job and the absent agent did not.
		// Asserting exit 0 would make the test depend on a real provider session completing.
		//
		// The durable assertion is the status file's own state/error transition, not history length.
		// A producer call that returns an error reaches no verdict.
		// shedengine's appendHistory records no history entry for it.
		// The failure is carried by current_producer, state, and error instead.
		// TestRun_ProducerError in internal/shedengine/run_routing_test.go covers this case.
		driveOut, driveCode, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, 3*time.Minute, "loom", "run")
		if err != nil {
			t.Fatalf("tapestry run: %v; output: %s", err, driveOut)
		}
		t.Logf("tapestry run exited %d: %s", driveCode, strings.TrimSpace(driveOut))

		after, foundAfter, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID))
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

	// A driver rigged to fail before its first persist leaves a non-empty driver log naming the failure.
	// See poisonStatusFile's doc comment for the rig:
	// an unrecognised state that decodes, but that Shed.Run's own read gate refuses.
	// The run fails before the loop calls a producer, so before any persist can happen.
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
			t.Fatalf("tapestry run against a poisoned status file timed out")
		}
		if runErr == nil {
			t.Fatalf("tapestry run against a poisoned status file succeeded; want a failure before any persist")
		}

		content, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read driver log: %v", err)
		}
		if strings.TrimSpace(string(content)) == "" {
			t.Fatalf("driver log is empty; want it to name the failure")
		}
		if !strings.Contains(strings.ToLower(string(content)), "invalid state") {
			t.Errorf("driver log = %q; want it to name the invalid state", content)
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

// TestSmokeBootstrapLifecycle drives "lym loom start" over one wired, go-seeded pair.
// The steps run in order, each building on the state the one before left:
// two concurrent first bootstraps, which leave a strand, a driver and a status file;
// a second bootstrap; a crash-healed origin record; then the two rigged died-driver bootstraps.
// The steps that rig a died driver kill any surviving driver first and clear the driver log,
// so each asserts on its own driver's log.
func TestSmokeBootstrapLifecycle(t *testing.T) {
	tiergate.Require(t, 2)
	tmuxBinaryPath(t)
	exe := lymbin.Build(t)
	_, loc, worktree, slug := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)
	seedGoDriverRun(t, loc)

	killDriversAndClearLog := func(t *testing.T) {
		t.Helper()
		killGoDriver(worktree)
		waitRunLockFree(t, loc, 20*time.Second)
		waitRecordsIndexLockGone(t, loc, 5*time.Second)
		if err := os.Remove(loomengine.LoomDriverLog(loc)); err != nil && !os.IsNotExist(err) {
			t.Fatalf("clear driver log: %v", err)
		}
	}

	// bootstrapUnlessSeeded runs a genuine "loom start" when no earlier step has seeded the run,
	// so a rigged step selected alone with -run still finds the seeded status file and run directory its rig builds on.
	bootstrapUnlessSeeded := func(t *testing.T) {
		t.Helper()
		_, err := os.Stat(shedrun.StatusFile(loc, shedrun.SelfRunID))
		if err == nil {
			return
		}
		if !os.IsNotExist(err) {
			t.Fatalf("stat status file: %v", err)
		}
		if stdout, _, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, 30*time.Second, "loom", "start"); err != nil {
			t.Fatalf("seeding loom start: %v; output: %s", err, stdout)
		}
	}

	// Two bootstrap invocations started concurrently produce exactly one driver process,
	// and the driver log holds no already-running refusal.
	// This guards against the run lock being taken by the child long after the spawn call returns.
	// That race lets two concurrent "lym loom start" invocations each decide to spawn their own driver.
	t.Run("concurrent first bootstraps yield one driver", func(t *testing.T) {
		// A background sampler tracks the maximum live driver-pid count.
		// It polls that count for the whole time both "loom start" invocations are in flight.
		// A single check after both return cannot prove that no double-spawn happened transiently.
		// Either driver may already have finished its bounded bounce loop by then.
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
				outputs[i], _, runErrs[i] = runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, 30*time.Second, "loom", "start")
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
		if lowered := strings.ToLower(string(logContent)); strings.Contains(lowered, "already running") || strings.Contains(lowered, "already drives") {
			t.Errorf("driver log names an already-running or already-driven refusal: %s", logContent)
		}
	})

	// The bootstrap in a real wired hub brings the tmux session up.
	// It leaves exactly one status strand under its fixed name, a live detached driver process, and the status file seeded.
	// This step asserts on what the concurrent bootstraps above left, not on an overall envelope.
	// A fast-finishing driver can make the handshake itself report a refusal even though every assertion here succeeds.
	t.Run("bootstrap leaves one status strand, a driver and a seeded status file", func(t *testing.T) {
		eng := probeReedEngine(t, loc)
		if count := statusStrandCount(t, eng, statusStrandDisplayName); count != 1 {
			t.Errorf("status strands named %q = %d; want exactly 1", statusStrandDisplayName, count)
		}

		if pids := findDriverPIDs(worktree); len(pids) == 0 {
			// loom's bounded bounce loop can finish before this check runs, so a missing driver is logged rather than failed.
			// The durable status-file assertion below is what pins that the driver ran.
			t.Logf("no detached driver process observed for %s; loom's bounded bounce loop may already have finished", worktree)
		} else if !proc.IsAlive(pids[0]) {
			t.Errorf("driver pid %d found but not alive", pids[0])
		}

		st, found, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID))
		if err != nil || !found {
			t.Fatalf("read status file after bootstrap: found=%v err=%v", found, err)
		}
		// current_producer is empty until the driver's first step; after it, the field names a row.
		// The seed is therefore pinned by its state alone.
		if st.State == "" {
			t.Errorf("seeded status file has an empty state")
		}
	})

	// A second bootstrap invocation made while the first driver holds the run lock spawns nothing new.
	// It leaves exactly one strand and one driver process, and it does not surface the seed-exists refusal as a failure.
	// The envelope is not asserted ok:true, since a fast-finishing driver can make the handshake report a refusal.
	// What this step guards is that the seed-exists sentinel never surfaces as a failure;
	// the error text below asserts it.
	t.Run("second invocation does not spawn a second driver", func(t *testing.T) {
		firstPIDs := findDriverPIDs(worktree)

		stdout, _, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, 30*time.Second, "loom", "start")
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

	// The origin record self-heals after a crash between its write and its commit.
	// The pair has the record written to disk by step 1 of "loom start" but not committed by step 3.
	// The process died in between.
	// The next "loom start" finds the record already on disk with a matching value and needs no write of its own,
	// and it still commits the untracked record, as the status file already self-heals.
	// Left untracked, the record would permanently fail `fabricengine.Clean`'s first Preflight precondition row.
	t.Run("origin record self-heals after a crash between write and commit", func(t *testing.T) {
		// The previous step leaves the first bootstrap's driver alive.
		// It commits into this same records worktree, so it must be gone before the step edits the index.
		killDriversAndClearLog(t)

		recordsDir := fabricengine.RecordsWorktree(loc)
		originRel := filepath.Join(loc.AnchorRel, fabricengine.OriginRecordRel())

		// Roll the pair back to a legacy shape: no origin record tracked at all, as if the pair had been
		// created before the record existed.
		gitkit.Git(t, recordsDir, "rm", "-q", "--", originRel)
		gitkit.Git(t, recordsDir, "commit", "-m", "smoke: simulate legacy pair with no origin record")

		// Simulate the crash: write the record to disk through the production primitive step 1 uses, without committing it.
		// That is the state a process death between steps 1 and 3 leaves behind.
		rec := fabricengine.NewMutations("")
		if err := fabricengine.WriteOrigin(rec, loc, slug, fabricengine.Origin{ParentBranch: "main"}); err != nil {
			t.Fatalf("WriteOrigin (simulated crash write): %v", err)
		}

		if status := recordsPathspecStatus(t, recordsDir, originRel); status == "" {
			t.Fatalf("origin record status before the healing run = clean; want the simulated crash to leave it uncommitted")
		}

		// The next `loom start` needs no --parent: ReadOrigin finds the record on disk with a matching value.
		// Therefore resolveParentBranch reports write == false.
		// The assertion below checks that this run commits the record.
		stdout, _, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, 30*time.Second, "loom", "start")
		if err != nil {
			t.Fatalf("healing loom start: %v; output: %s", err, stdout)
		}

		// The origin record must be committed and clean.
		// The whole records worktree is not asserted clean.
		// A running driver rewrites the status file on every phase transition without committing those rewrites,
		// so the worktree goes dirty for a reason unrelated to the origin-record healing this step pins.
		if status := recordsPathspecStatus(t, recordsDir, originRel); status != "" {
			t.Errorf("origin record status after the healing run = %q; want it committed and clean", status)
		}
	})

	// A driver that dies immediately does not make the bootstrap refuse.
	// dispositionForHandshake maps awaitRunLockChildDied onto proceed.
	// A child already gone at the handshake's first poll means the driver ran and finished.
	// Refusing there would tell an operator the bootstrap broke, though their task had halted,
	// and it would withhold the success envelope.
	// Only awaitRunLockDeadline is a genuine refusal:
	// a child still alive after the whole attempt budget, never having taken the lock.
	// A dying driver cannot produce that.
	//
	// The test pins the shipped disposition and its two safety properties:
	// the driver log carries the real reason the run halted, and the bootstrap lock is released rather than stranded.
	//
	// The rig is the poisoned-status-file mechanism that poisonStatusFile documents.
	// It applies after the earlier steps' genuine bootstraps.
	// The pair then has a live reed substrate, and the run lock has been observed free again.
	// The bootstrap's steps 1-4 (ReadOrigin, Seed, CommitRecordsPaths) tolerate the unrecognised state,
	// and the spawned child's read gate alone sees the failure.
	t.Run("died driver proceeds to the success envelope and logs why", func(t *testing.T) {
		bootstrapUnlessSeeded(t)
		killDriversAndClearLog(t)
		poisonStatusFile(t, loc)

		stdout, _, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, 30*time.Second, "loom", "start")
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

		// The rig is confirmed by the driver log's own content:
		// the read gate's invalid-state refusal, not an ordinary completed run.
		// Without this the step would pass against any fast-exiting driver and prove nothing about the rig.
		// The poisoned state is not running, so the handshake reports halted on its first poll,
		// and the bootstrap can return while the driver is still starting; the log is polled for rather than read once.
		driverLog := readDriverLogContaining(loc, "invalid state")
		if !strings.Contains(strings.ToLower(string(driverLog)), "invalid state") {
			t.Errorf("driver log = %q; want it to name the poisoned status file's invalid state -- the reason the run halted must survive in the log even though the bootstrap proceeded", driverLog)
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

	// `lym loom start` against a malformed-JSON status file proceeds to the success envelope.
	// It does so as the unrecognised-state shape does, rather than refusing at the Seed step.
	// The unit tests cannot see the composed CLI-verb path.
	// They cover loomshed.TestSeed_RefusesUndecodableFileAsExists and state.TestCorruptFile.
	// Only the real `lym loom start` binary runs the Seed -> VerifySeedOwnership -> commit -> spawn -> handshake chain.
	// It must run that chain against a poisoned records-committed status file.
	//
	// Seed must map a malformed-JSON decode failure to ErrSeedExists.
	// Otherwise step 2 of `lym loom start` refuses on the envelope before it spawns a driver.
	// A poisoned status file must not look like the bootstrap's own gate, which the crash-recovery design forbids.
	// Once it does, the bootstrap tolerates the error,
	// and the spawned driver's Shed.Run step-1 read gate diagnoses the decode failure in the driver log.
	// The died-driver step pins that for the unrecognised-state shape.
	t.Run("malformed status proceeds to the success envelope and logs why", func(t *testing.T) {
		bootstrapUnlessSeeded(t)
		killDriversAndClearLog(t)
		poisonStatusFileMalformed(t, loc)

		stdout, _, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, 30*time.Second, "loom", "start")
		if err != nil {
			t.Fatalf("malformed-status bootstrap: %v; output: %s", err, stdout)
		}

		// The bootstrap got past the Seed step rather than refusing there, so the success envelope is the evidence.
		if !strings.Contains(stdout, `"ok":true`) {
			t.Errorf("malformed-status bootstrap output = %q; want the ok envelope, not a Seed refusal before it", stdout)
		}
		if strings.Contains(stdout, `"ok":false`) {
			t.Errorf("malformed-status bootstrap emitted a refusal envelope: %s -- a malformed status file must defer to the driver's own read gate, not refuse at the Seed step", stdout)
		}

		// The driver log carries the decode failure from Shed.Run's read gate.
		driverLog := readDriverLogContaining(loc, "decode")
		if !strings.Contains(strings.ToLower(string(driverLog)), "decode") {
			t.Errorf("driver log = %q; want it to name the malformed status file's decode failure", driverLog)
		}
	})
}

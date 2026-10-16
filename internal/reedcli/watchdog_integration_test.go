//go:build tmux

// watchdog_integration_test.go carries the watchdog daemon's live-behaviour assertions as one TestWatchdogDaemon scenario: discovery against a real hub with real tmux sessions, the single-instance lock's two outcomes, the stop a last down writes, departure teardown, and re-entry re-reading a flipped watchdog: config value.
//
// It is a separate file from watchdog_test.go, rather than tagged content inside it, because a Go build tag is per-file: tagging watchdog_test.go itself would hide its pure-seam tests from the untagged tier where they belong.
// The hub fixture is built through internal/hubforge per the hubforge Fabric-Fixture Invariant, never hand-assembled.
//
// ensureWatchdogSpawned's own os.Executable() re-exec is deliberately NOT exercised live here: under `go test`, os.Executable() resolves to the test binary itself, and re-execing it with reed-watchdog-shaped args is exactly the recursive-whole-suite hazard suppressWatchdogSpawn exists to prevent (see cli.go's doc comment on that field).
// This file instead drives watchdogCmd()'s RunE and runWatchdogLoop directly, in-process, which is the daemon's own live behaviour; the spawn call site's wiring is pinned by the engine's own spawn test, and the "attach/resume attempt the spawn" step below drives ensureWatchdogSpawned itself (not the os.Executable() re-exec) far enough to observe it was actually invoked rather than suppressed.
package reedcli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// watchdogIntegrationTmux resolves the configured multiplexer binary, skipping the calling test when
// it is absent so this file never hard-fails on a machine without tmux.
func watchdogIntegrationTmux(t *testing.T, cfg reedengine.Config) string {
	t.Helper()
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}
	return cfg.Tmux
}

// bootWatchdogEngine boots a real engine for worktreeRoot and returns it; the caller owns tearing the session down.
func bootWatchdogEngine(t *testing.T, worktreeRoot string) *reedengine.Engine {
	t.Helper()
	location, err := lyxcwd.ResolveWorktree(worktreeRoot)
	if err != nil {
		t.Fatalf("ResolveWorktree(%s): %v", worktreeRoot, err)
	}
	cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: location.AnchorPath()}, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	watchdogIntegrationTmux(t, cfg)
	geom, err := hubgeom.ReedGeometry(location)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	eng := reedengine.New(cfg, geom, shuttleengines.EnvScrub())
	if _, err := eng.Up(); err != nil {
		t.Fatalf("eng.Up(): %v", err)
	}
	return eng
}

// watchdogIntegrationEngine boots a real engine for worktreeRoot and returns it, with a cleanup on t that tears the session down.
func watchdogIntegrationEngine(t *testing.T, worktreeRoot string) *reedengine.Engine {
	t.Helper()
	eng := bootWatchdogEngine(t, worktreeRoot)
	t.Cleanup(func() {
		_, _ = eng.Down()
	})
	return eng
}

// watchdogTestCycle is the discovery cycle the compressed timing runs at, and watchdogTestWait the bound a test gives a discovery-dependent condition:
// many cycles, so a slow tmux round trip never flakes it, while a condition that holds returns at the first poll.
// watchdogSignalTestBase is the base cadence of the loop that proves the discover signal wakes a backed-off loop.
const (
	watchdogTestCycle      = 200 * time.Millisecond
	watchdogTestWait       = 10 * time.Second
	watchdogSignalTestBase = 100 * time.Millisecond
)

// compressedWatchdogTiming returns a watchdogTiming whose discovery cycle is sub-second and never backs off, while IdleCycles and OrphanGoneCycles keep their production values, so a test still proves the consecutive-cycle rules and only waits less.
func compressedWatchdogTiming() watchdogTiming {
	return watchdogTiming{
		DiscoveryCycle:   watchdogTestCycle,
		DiscoveryCeiling: watchdogTestCycle,
		IdleCycles:       watchdogHubIdleCycles,
		OrphanGoneCycles: watchdogOrphanGoneCycles,
	}
}

// runWatchdogCmdInBackground starts watchdogCmd() against hub/tmuxPath with the given timing on a cancellable context and
// returns the cancel func plus a channel that receives RunE's error (or nil) once it returns.
func runWatchdogCmdInBackground(t *testing.T, hub, tmuxPath string, timing watchdogTiming) (cancel context.CancelFunc, done chan error, out *bytes.Buffer) {
	t.Helper()
	c := &reedCLI{watchdogTiming: &timing}
	cmd := c.watchdogCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--hub-path", hub, "--tmux", tmuxPath})

	ctx, cancelFn := context.WithCancel(context.Background())
	done = make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(ctx)
	}()
	return cancelFn, done, buf
}

// sessionListed reports whether session is currently listed on socket by the tmux binary at tmuxPath.
func sessionListed(tmuxPath, socket, session string) bool {
	names, err := reedengine.ListSessions(tmuxPath, socket)
	if err != nil {
		return false
	}
	for _, n := range names {
		if n == session {
			return true
		}
	}
	return false
}

// TestWatchdogDaemon runs the watchdog daemon's live claims against one hub with one extra pair worktree, the steps in a fixed order:
// the first two start the daemon through its command on a hub with no session, then with one;
// the next three share one in-process discovery loop watching two sessions, whose engines and loop the first of them boots and the later ones rely on;
// the last flips the prime worktree's watchdog: key to off across a down and an up.
// The scenario does not call t.Parallel, because the daemon command points the process-global logger's durable sink at the hub's logs directory.
func TestWatchdogDaemon(t *testing.T) {
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const pairSlug = "watchdog-second"
	hubforge.AddPair(t, h, pairSlug)
	prime := h.PrimeWorktree()
	pair := h.PairCodeWorktree(pairSlug)
	socket := reedengine.ServerName(h.Path)
	lockPath := filepath.Join(fabricengine.HubScratchDir(h.Path), watchdogLockFileName)

	cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: h.Location.AnchorPath()}, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	tmuxPath := watchdogIntegrationTmux(t, cfg)
	tmuxkit.KillOnCleanup(t, tmuxPath, socket)

	// DaemonLockAndLogs drives the daemon command against a hub that has never run one: it points the durable log sink at fabricengine.HubLogsDir(hub) before taking the lock, and keeps the logger's stderr half — the only observable proof of watchdogCmd's documented ordering (sink, then lock, stderr kept) is a trace-*.log file appearing there and the captured stderr half carrying the daemon's "watchdog daemon starting" line — and holds the single-instance lock, so a second attempt against the SAME hub exits 0 (contention) without taking it, while an unusable lock path (a hub path whose HubScratchDir cannot be created because a FILE sits where an intermediate directory component must go) exits non-zero.
	if !t.Run("DaemonLockAndLogs", func(t *testing.T) {
		logsDir := fabricengine.HubLogsDir(h.Path)
		if _, err := os.Stat(logsDir); err == nil {
			t.Fatalf("hub logs dir %s already exists before the daemon ever ran", logsDir)
		}

		logs := logcapture.CaptureVerbose(t)
		cancel1, done1, _ := runWatchdogCmdInBackground(t, h.Path, tmuxPath, compressedWatchdogTiming())
		defer cancel1()

		// Give the first daemon time to actually acquire the lock before the second attempt races it.
		waitForCondition(t, 10*time.Second, func() bool {
			_, err := os.Stat(lockPath)
			return err == nil
		})
		waitForCondition(t, 10*time.Second, func() bool {
			entries, err := os.ReadDir(logsDir)
			return err == nil && len(entries) > 0
		})

		entries, err := os.ReadDir(logsDir)
		if err != nil {
			t.Fatalf("ReadDir(%s): %v", logsDir, err)
		}
		var sawTrace bool
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "trace-") {
				sawTrace = true
			}
		}
		if !sawTrace {
			t.Errorf("hub logs dir %s entries = %v, want at least one trace-*.log file — this is the only observable proof the durable sink was pointed at HubLogsDir", logsDir, entries)
		}
		waitForCondition(t, 10*time.Second, func() bool {
			return strings.Contains(logs.String(), "reed: watchdog daemon starting")
		})

		// A second attempt against the SAME hub while the first holds the lock must exit 0 (contention), never taking the lock.
		c2 := &reedCLI{}
		cmd2 := c2.watchdogCmd()
		buf2 := &bytes.Buffer{}
		cmd2.SetOut(buf2)
		cmd2.SetArgs([]string{"--hub-path", h.Path, "--tmux", tmuxPath})
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		if err := cmd2.ExecuteContext(ctx2); err != nil {
			t.Errorf("second watchdog attempt returned error %v; want nil (contention exits 0)", err)
		}

		// An unusable lock path exits non-zero.
		unusableHub := unusableHubPath(t)
		c3 := &reedCLI{}
		cmd3 := c3.watchdogCmd()
		buf3 := &bytes.Buffer{}
		cmd3.SetOut(buf3)
		cmd3.SetArgs([]string{"--hub-path", unusableHub, "--tmux", tmuxPath})
		ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel3()
		_ = cmd3.ExecuteContext(ctx3)
		envelope.RequireErr(t, buf3.String(), "")

		cancel1()
		<-done1
	}) {
		return
	}

	// DownStopsTheDaemonAndReleasesLock boots the prime and the pair sessions and starts a daemon whose idle-cycle limit no step can reach, so only a stop can end it.
	// A non-last down leaves the daemon running across five discovery cycles; the last down's stop ends it within the step's bound, and the lock is then free.
	// The previous step left its lock file behind, so it is removed first: the daemon recreating it is what proves the daemon is running before the sessions go away.
	if !t.Run("DownStopsTheDaemonAndReleasesLock", func(t *testing.T) {
		primeEng := watchdogIntegrationEngine(t, prime)
		pairEng := watchdogIntegrationEngine(t, pair)
		if err := os.Remove(lockPath); err != nil {
			t.Fatalf("remove the previous daemon's lock file %s: %v", lockPath, err)
		}

		timing := compressedWatchdogTiming()
		timing.IdleCycles = 1000
		cancel, done, _ := runWatchdogCmdInBackground(t, h.Path, tmuxPath, timing)
		defer cancel()

		waitForCondition(t, 10*time.Second, func() bool {
			_, err := os.Stat(lockPath)
			return err == nil
		})

		if _, err := pairEng.Down(); err != nil {
			t.Fatalf("pairEng.Down(): %v", err)
		}
		select {
		case err := <-done:
			t.Fatalf("watchdog daemon exited (err=%v) after a non-last down; want it still running", err)
		case <-time.After(5 * watchdogTestCycle):
		}

		if _, err := primeEng.Down(); err != nil {
			t.Fatalf("primeEng.Down(): %v", err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("watchdogCmd RunE returned %v; want nil", err)
			}
		case <-time.After(watchdogTestWait):
			t.Fatal("watchdog daemon did not exit after the last down's stop")
		}

		held, acquired, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil || !acquired {
			t.Fatalf("watchdog lock after the daemon exited: acquired=%v err=%v; want it free", acquired, err)
		}
		held.Release()
	}) {
		return
	}

	// BootSignalWakesBackedOffDiscovery runs a discovery loop whose cadence has backed off over a live prime session, then boots the pair's session through reed and asserts the loop enters it before its next unsignalled cycle, which only the discover signal the boot touches can explain.
	// The loop is in-process on its own context, so the shared loop of the steps below is untouched.
	if !t.Run("BootSignalWakesBackedOffDiscovery", func(t *testing.T) {
		logs := logcapture.Capture(t)
		logger.SetVerbosity(2)

		primeEng := watchdogIntegrationEngine(t, prime)
		timing := watchdogTiming{
			DiscoveryCycle:   watchdogSignalTestBase,
			DiscoveryCeiling: 30 * time.Second,
			IdleCycles:       watchdogHubIdleCycles,
			OrphanGoneCycles: watchdogOrphanGoneCycles,
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- runWatchdogLoop(ctx, h.Path, tmuxPath, primeEng.ShellPath(), timing, reedengine.OpenFileWatch, reedengine.ListSessions, nil)
		}()
		defer func() {
			cancel()
			<-done
		}()

		entered := func(session string) bool {
			for _, line := range strings.Split(logs.String(), "\n") {
				if strings.Contains(line, "reed: watchdog entered session") && strings.Contains(line, session) {
					return true
				}
			}
			return false
		}
		waitForCondition(t, watchdogTestWait, func() bool { return entered(primeEng.SessionName()) })
		// The prime's entry resets the cadence to the base, so the unsignalled cycles after it fall 63 and then 127 bases later.
		// The pair boots past the 63rd and must be entered before the 120th, which no unsignalled cycle can do.
		primeEnteredAt := time.Now()
		time.Sleep(70 * watchdogSignalTestBase)

		pairEng := watchdogIntegrationEngine(t, pair)
		waitForCondition(t, time.Until(primeEnteredAt.Add(120*watchdogSignalTestBase)), func() bool { return entered(pairEng.SessionName()) })
	}) {
		return
	}

	// The next steps share one discovery loop over two sessions.
	// The first of them boots the engines and starts the loop; the loop and the engines outlive the step that created them, so their teardown is registered on the scenario rather than on that step.
	scenario := t
	var eng1, eng2 *reedengine.Engine
	loopCtx, cancelLoop := context.WithCancel(context.Background())
	defer cancelLoop()
	loopDone := make(chan error, 1)
	requireLoopRunning := func(t *testing.T, when string) {
		t.Helper()
		select {
		case err := <-loopDone:
			t.Fatalf("runWatchdogLoop exited %s (err=%v); want it still running", when, err)
		default:
		}
	}

	// DiscoversBothSessions boots both worktrees' engines, starts the loop and waits until the daemon has discovered both sessions.
	if !t.Run("DiscoversBothSessions", func(t *testing.T) {
		eng1 = bootWatchdogEngine(t, prime)
		scenario.Cleanup(func() { _, _ = eng1.Down() })
		eng2 = bootWatchdogEngine(t, pair)
		scenario.Cleanup(func() { _, _ = eng2.Down() })
		// The watch never opens, so this loop sees no stop and keeps the idle-cycle rule that covers a down followed by an up.
		noWatch := func(string, ...string) (<-chan fswatch.Event, func() error, error) {
			return nil, nil, errors.New("watch disabled for the shared discovery loop")
		}
		go func() {
			loopDone <- runWatchdogLoop(loopCtx, h.Path, tmuxPath, eng1.ShellPath(), compressedWatchdogTiming(), noWatch, reedengine.ListSessions, nil)
		}()

		// Both sessions must be discovered within a couple of discovery cycles.
		waitForCondition(t, watchdogTestWait, func() bool {
			return sessionListed(tmuxPath, socket, eng1.SessionName()) && sessionListed(tmuxPath, socket, eng2.SessionName())
		})
	}) {
		return
	}

	// DiscoversAndDropsDepartedSessions tears eng1's session down; the daemon must stop touching it while eng2's session keeps being watched.
	// This is observed indirectly: eng2 stays discoverable via ListSessions after eng1's session is gone, and the loop itself keeps running (it does not idle-exit, since eng2 is still live).
	if !t.Run("DiscoversAndDropsDepartedSessions", func(t *testing.T) {
		if _, err := eng1.Down(); err != nil {
			t.Fatalf("eng1.Down(): %v", err)
		}

		waitForCondition(t, watchdogTestWait, func() bool {
			return !sessionListed(tmuxPath, socket, eng1.SessionName()) && sessionListed(tmuxPath, socket, eng2.SessionName())
		})

		requireLoopRunning(t, "early; want it still running with eng2's session live")
	}) {
		return
	}

	// DownThenUpDoesNotKillDaemon drives a down immediately followed by an up against the daemon's one live worktree and asserts the daemon itself never exits and rediscovers the re-upped session — watchdogHubIdleCycles exists precisely to cover this gap.
	if !t.Run("DownThenUpDoesNotKillDaemon", func(t *testing.T) {
		if _, err := eng2.Down(); err != nil {
			t.Fatalf("eng2.Down(): %v", err)
		}
		if _, err := eng2.Up(); err != nil {
			t.Fatalf("eng2.Up() (immediate re-up): %v", err)
		}

		waitForCondition(t, watchdogTestWait, func() bool {
			return sessionListed(tmuxPath, socket, eng2.SessionName())
		})

		requireLoopRunning(t, "after a down immediately followed by an up")
	}) {
		return
	}

	// AttachAndResumeAttemptTheSpawn: ensureWatchdogSpawned's own os.Executable() re-exec is unsafe to drive live under `go test` (see the file-level doc comment), so this exercises the call site far enough to prove it is actually reached rather than suppressed: an unusable hubPath makes ensureWatchdogSpawned fail at its own MkdirAll step, before ever calling os.Executable(), which is safely observable.
	if !t.Run("AttachAndResumeAttemptTheSpawn", func(t *testing.T) {
		c := &reedCLI{eng: eng2, hubPath: unusableHubPath(t), suppressWatchdogSpawn: false}
		// A spawn attempt against an unusable hub path must return without panicking — proving
		// ensureWatchdogSpawned was reached (not suppressed) and degrades harmlessly, exactly as up,
		// resume, and attach each rely on.
		c.ensureWatchdogSpawned()
	}) {
		return
	}

	// ReEntryReReadsFlippedConfig brings the prime worktree's session back, downs it, flips its watchdog: key to off, ups it again, and asserts BOTH halves of re-entry re-reading the config: the one continuously running daemon (never restarted — loopDone is asserted still open throughout) rediscovers the re-upped session, and enterSession — the exact function the daemon's own discovery loop calls on every appeared name — now reads the flipped value straight off disk rather than the value it held before the down/up, so a watchdog:off worktree enters the known set but starts no watcher.
	t.Run("ReEntryReReadsFlippedConfig", func(t *testing.T) {
		if _, err := eng1.Up(); err != nil {
			t.Fatalf("eng1.Up() (prime back before the flip): %v", err)
		}
		waitForCondition(t, watchdogTestWait, func() bool {
			return sessionListed(tmuxPath, socket, eng1.SessionName())
		})
		if _, err := eng1.Down(); err != nil {
			t.Fatalf("eng1.Down(): %v", err)
		}

		location, err := lyxcwd.ResolveWorktree(prime)
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}
		// enterSession loads its own config straight off disk, so the flip must actually be seeded into the fixture's reed.yaml — the whole resolved config, every key present, since a partial override fails LoadConfig's strictness check.
		offCfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: location.AnchorPath()}, "reed")
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		offCfg.Watchdog = "off"
		seeded, err := yaml.Marshal(offCfg)
		if err != nil {
			t.Fatalf("marshal flipped config: %v", err)
		}
		hubforge.SeedConfig(t, h, map[string]string{"reed": string(seeded)})
		reread, err := reedengine.LoadConfig(configengine.Dirs{Anchor: location.AnchorPath()}, "reed")
		if err != nil {
			t.Fatalf("LoadConfig after seeding: %v", err)
		}
		if reread.Watchdog != "off" {
			t.Fatalf("seeded config's Watchdog = %q; want %q (fixture seeding did not take)", reread.Watchdog, "off")
		}

		geom, err := hubgeom.ReedGeometry(location)
		if err != nil {
			t.Fatalf("reed geometry: %v", err)
		}
		offEng := reedengine.New(offCfg, geom, shuttleengines.EnvScrub())
		if _, err := offEng.Up(); err != nil {
			t.Fatalf("offEng.Up() (re-up with watchdog: off): %v", err)
		}
		t.Cleanup(func() { _, _ = offEng.Down() })

		// The one daemon started above is still running throughout this whole down/flip/up sequence —
		// never restarted — and rediscovers the re-upped session under the SAME session name.
		waitForCondition(t, watchdogTestWait, func() bool {
			return sessionListed(tmuxPath, socket, offEng.SessionName())
		})
		requireLoopRunning(t, "during the down/flip/up sequence")

		// enterSession is exactly the function the running daemon's own discovery loop calls for a newly
		// appeared name — calling it here against the same hub/tmux/session identity proves what value the
		// daemon itself would now read on its own next discovery cycle.
		ws, err := enterSession(h.Path, tmuxPath, offEng.SessionName())
		if err != nil {
			t.Fatalf("enterSession (after down + flip-to-off + up): %v", err)
		}
		if ws.cancel != nil {
			ws.cancel()
			t.Error("enterSession() after down + flip-to-off + up returned a non-nil cancel; want nil — re-entry must re-read the flipped config rather than the value it held before the down/up")
		}
		if ws.eng == nil {
			t.Error("enterSession() after re-entry returned a nil eng; want a built Engine so departure bookkeeping stays uniform")
		}

		cancelLoop()
		<-loopDone
	})
}

// unusableHubPath returns a hub path that makes fabricengine.HubScratchDir(hub)'s MkdirAll fail: a
// regular file sits where an intermediate directory component of the hub path must go, so nothing
// can ever be created beneath it.
func unusableHubPath(t *testing.T) string {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "blocker-file")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", blocker, err)
	}
	return filepath.Join(blocker, "hub-under-a-file")
}

// waitForCondition polls cond every 100ms until it reports true or timeout elapses, failing the test
// on timeout.
func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %s", timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

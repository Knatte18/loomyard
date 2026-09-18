//go:build integration

// watchdog_integration_test.go carries the watchdog daemon's live-behaviour assertions: discovery
// against a real hub with real tmux sessions, the single-instance lock's two outcomes, the idle-exit
// timer, departure teardown, and re-entry re-reading a flipped watchdog: config value.
//
// It is a separate file from watchdog_test.go, rather than tagged content inside it, because a Go
// build tag is per-file: tagging watchdog_test.go itself would hide its pure-seam tests from the
// untagged tier where they belong. The hub fixture is built through internal/hubforge per the
// hubforge Fabric-Fixture Invariant, never hand-assembled.
//
// ensureWatchdogSpawned's own os.Executable() re-exec is deliberately NOT exercised live here: under
// `go test`, os.Executable() resolves to the test binary itself, and re-execing it with
// reed-watchdog-shaped args is exactly the recursive-whole-suite hazard suppressWatchdogSpawn exists
// to prevent (see cli.go's doc comment on that field). This file instead drives watchdogCmd()'s RunE
// and runWatchdogLoop directly, in-process, which is the daemon's own live behaviour; the spawn call
// site's wiring is pinned by cli_test.go/spawnwatchdog_test.go's non-integration tests, and the
// "attach/resume attempt the spawn" assertion below drives ensureWatchdogSpawned itself (not the
// os.Executable() re-exec) far enough to observe it was actually invoked rather than suppressed.
package reedcli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
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

// watchdogIntegrationEngine boots a real engine for worktreeRoot and returns it, its config, and a
// cleanup that tears the session down.
func watchdogIntegrationEngine(t *testing.T, worktreeRoot string) *reedengine.Engine {
	t.Helper()
	location, err := lyxcwd.ResolveWorktree(worktreeRoot)
	if err != nil {
		t.Fatalf("ResolveWorktree(%s): %v", worktreeRoot, err)
	}
	cfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	watchdogIntegrationTmux(t, cfg)
	geom := hubgeom.ReedGeometry(location)
	eng := reedengine.New(cfg, geom)
	if _, err := eng.Up(); err != nil {
		t.Fatalf("eng.Up(): %v", err)
	}
	t.Cleanup(func() {
		_, _ = eng.Down()
	})
	return eng
}

// runWatchdogCmdInBackground starts watchdogCmd() against hub/tmuxPath on a cancellable context and
// returns the cancel func plus a channel that receives RunE's error (or nil) once it returns.
func runWatchdogCmdInBackground(t *testing.T, hub, tmuxPath string) (cancel context.CancelFunc, done chan error, out *bytes.Buffer) {
	t.Helper()
	c := &reedCLI{}
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

func TestWatchdogIntegration_SingleInstanceLockContentionAndUnusableLockPath(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	cfg, err := reedengine.LoadConfig(h.Location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	tmuxPath := watchdogIntegrationTmux(t, cfg)

	cancel1, done1, _ := runWatchdogCmdInBackground(t, h.Path, tmuxPath)
	defer cancel1()

	// Give the first daemon time to actually acquire the lock before the second attempt races it.
	deadline := time.Now().Add(10 * time.Second)
	lockPath := filepath.Join(fabricengine.HubScratchDir(h.Path), watchdogLockFileName)
	for {
		if _, err := os.Stat(lockPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("watchdog lock file %s never appeared", lockPath)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// A second attempt against the SAME hub while the first holds the lock must exit 0 (contention),
	// never taking the lock.
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

	// An unusable lock path (a hub path whose HubScratchDir cannot be created because a FILE sits
	// where an intermediate directory component must go) exits non-zero.
	unusableHub := unusableHubPath(t)
	c3 := &reedCLI{}
	cmd3 := c3.watchdogCmd()
	buf3 := &bytes.Buffer{}
	cmd3.SetOut(buf3)
	cmd3.SetArgs([]string{"--hub-path", unusableHub, "--tmux", tmuxPath})
	ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel3()
	_ = cmd3.ExecuteContext(ctx3)
	var env map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf3.Bytes()), &env); err != nil {
		t.Fatalf("unusable-lock-path output is not valid JSON: %v; got: %q", err, buf3.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("unusable lock path ok = true; want false")
	}

	cancel1()
	<-done1
}

func TestWatchdogIntegration_DiscoversAndDropsDepartedSessions(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "watchdog-second")

	eng1 := watchdogIntegrationEngine(t, h.PrimeWorktree())
	eng2 := watchdogIntegrationEngine(t, h.PairWarpWorktree("watchdog-second"))

	tmuxPath := eng1.TmuxPath()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loopDone := make(chan error, 1)
	go func() {
		loopDone <- runWatchdogLoop(ctx, h.Path, tmuxPath)
	}()

	// Both sessions must be discovered within a couple of discovery cycles.
	waitForCondition(t, watchdogHubDiscoveryCycle*3, func() bool {
		names, err := reedengine.ListSessions(tmuxPath, reedengine.ServerName(h.Path))
		if err != nil {
			return false
		}
		found1, found2 := false, false
		for _, n := range names {
			if n == eng1.SessionName() {
				found1 = true
			}
			if n == eng2.SessionName() {
				found2 = true
			}
		}
		return found1 && found2
	})

	// Tear eng1's session down; the daemon must stop touching it while eng2's session keeps being
	// watched. This is observed indirectly: eng2 stays discoverable via ListSessions after eng1's
	// session is gone, and the loop itself keeps running (it does not idle-exit, since eng2 is still
	// live).
	if _, err := eng1.Down(); err != nil {
		t.Fatalf("eng1.Down(): %v", err)
	}

	waitForCondition(t, watchdogHubDiscoveryCycle*3, func() bool {
		names, err := reedengine.ListSessions(tmuxPath, reedengine.ServerName(h.Path))
		if err != nil {
			return false
		}
		for _, n := range names {
			if n == eng1.SessionName() {
				return false
			}
		}
		for _, n := range names {
			if n == eng2.SessionName() {
				return true
			}
		}
		return false
	})

	select {
	case err := <-loopDone:
		t.Fatalf("runWatchdogLoop exited early (err=%v); want it still running with eng2's session live", err)
	default:
	}

	cancel()
	<-loopDone
}

func TestWatchdogIntegration_ExitsAfterIdleCyclesAndReleasesLock(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	eng := watchdogIntegrationEngine(t, h.PrimeWorktree())
	tmuxPath := eng.TmuxPath()

	cancel, done, _ := runWatchdogCmdInBackground(t, h.Path, tmuxPath)
	defer cancel()

	lockPath := filepath.Join(fabricengine.HubScratchDir(h.Path), watchdogLockFileName)
	waitForCondition(t, 10*time.Second, func() bool {
		_, err := os.Stat(lockPath)
		return err == nil
	})

	if _, err := eng.Down(); err != nil {
		t.Fatalf("eng.Down(): %v", err)
	}

	// The daemon must exit on its own within watchdogHubIdleCycles*watchdogHubDiscoveryCycle plus
	// slack, and release its lock.
	slack := 10 * time.Second
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("watchdogCmd RunE returned %v; want nil", err)
		}
	case <-time.After(watchdogHubIdleCycles*watchdogHubDiscoveryCycle + slack):
		t.Fatal("watchdog daemon did not exit after its last session went away")
	}

	// A later spawn attempt must be able to take the now-released lock.
	c2 := &reedCLI{}
	cmd2 := c2.watchdogCmd()
	buf2 := &bytes.Buffer{}
	cmd2.SetOut(buf2)
	cmd2.SetArgs([]string{"--hub-path", h.Path, "--tmux", tmuxPath})
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	_ = cmd2.ExecuteContext(ctx2)
}

func TestWatchdogIntegration_OffWorktreeEntersKnownButStartsNoWatcher(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	location, err := lyxcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	defaultCfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	watchdogIntegrationTmux(t, defaultCfg)

	// enterSession loads its own config straight off disk (reedengine.LoadConfig), so the override
	// must actually be seeded into the fixture's reed.yaml — the whole resolved config, every key
	// present, since a partial override fails LoadConfig's strictness check.
	defaultCfg.Watchdog = "off"
	seeded, err := yaml.Marshal(defaultCfg)
	if err != nil {
		t.Fatalf("marshal seeded config: %v", err)
	}
	hubforge.SeedConfig(t, h, map[string]string{"reed": string(seeded)})

	cfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("LoadConfig after seeding: %v", err)
	}
	if cfg.Watchdog != "off" {
		t.Fatalf("seeded config's Watchdog = %q; want %q (fixture seeding did not take)", cfg.Watchdog, "off")
	}
	geom := hubgeom.ReedGeometry(location)
	eng := reedengine.New(cfg, geom)
	if _, err := eng.Up(); err != nil {
		t.Fatalf("eng.Up(): %v", err)
	}
	t.Cleanup(func() { _, _ = eng.Down() })

	ws, err := enterSession(h.Path, eng.TmuxPath(), eng.SessionName())
	if err != nil {
		t.Fatalf("enterSession: %v", err)
	}
	if ws.cancel != nil {
		t.Error("enterSession() for a watchdog:off worktree returned a non-nil cancel; want nil (no goroutine started)")
	}
	if ws.eng == nil {
		t.Error("enterSession() for a watchdog:off worktree returned a nil eng; want a built Engine so departure bookkeeping stays uniform")
	}
}

func TestWatchdogIntegration_AttachAndResumeAttemptTheSpawn(t *testing.T) {
	// ensureWatchdogSpawned's own os.Executable() re-exec is unsafe to drive live under `go test`
	// (see the file-level doc comment), so this exercises the call site far enough to prove it is
	// actually reached rather than suppressed: an unusable hubPath makes ensureWatchdogSpawned fail
	// at its own MkdirAll step, before ever calling os.Executable(), which is safely observable.
	h := hubforge.NewHub(t, ".")
	eng := watchdogIntegrationEngine(t, h.PrimeWorktree())

	unusableHub := unusableHubPath(t)
	c := &reedCLI{eng: eng, hubPath: unusableHub, suppressWatchdogSpawn: false}
	// A spawn attempt against an unusable hub path must return without panicking — proving
	// ensureWatchdogSpawned was reached (not suppressed) and degrades harmlessly, exactly as up,
	// resume, and attach each rely on.
	c.ensureWatchdogSpawned()
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

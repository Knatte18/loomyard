// ensuresession_integration_test.go pins EnsureSession's booted return value and AddStrand's attribution log line.
// Both assertions run against a real, running multiplexer server.
// Both need that server: booted separates "nothing was there to attach to" from "something already was".
// Only a has-session and list-panes round trip can tell those apart.
// No part of the file runs without a server, so it stays in the integration tier.

package reedengine

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// scrubProbeEnv is set by the cold-boot test and named in its engine's scrub.
// That makes the scrub's stripping of it observable.
const scrubProbeEnv = "LYM_REED_SCRUB_PROBE"

// newColdScratchEngine builds an *Engine rooted at a fresh t.TempDir(), against the LoadConfig-resolved cfg.
// It runs on its own scratch socket, derived from the temp dir's hash.
// A leaked scratch server therefore can never collide with a real hub server or another test's socket.
// It self-skips when the configured multiplexer binary is absent.
// It registers a t.Cleanup that tears down the engine's sessions and server on every path, including a failed run.
func newColdScratchEngine(t *testing.T) *Engine {
	t.Helper()
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(configengine.Dirs{Anchor: tmpDir}, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	hub := filepath.Dir(tmpDir)
	geom := Geometry{
		SocketKey:     ServerName(hub),
		SessionName:   SessionName(tmpDir),
		AnchorPath:    tmpDir,
		PaneCwd:       tmpDir,
		WorktreeRoot:  tmpDir,
		LogsDir:       filepath.Join(hub, "logs"),
		WorktreeName:  filepath.Base(tmpDir),
		HubPath:       hub,
		NameShortname: "tc",
		NameSlug:      "tslug",

		DiscoverSignalPath: filepath.Join(hub, DiscoverSignalFileName),
	}
	e := New(cfg, geom, EnvScrub{Names: append([]string{scrubProbeEnv}, testScrub.Names...), Prefixes: testScrub.Prefixes})

	// Registered before the Down cleanup so it runs after it.
	// The kill-server guards against a leaked scratch server when a failed test never reaches a clean teardown,
	// and it removes the server's socket file.
	tmuxkit.KillOnCleanup(t, cfg.Tmux, geom.SocketKey)
	t.Cleanup(func() {
		// Best-effort: Down may already have nothing left to tear down.
		// The test may have torn it down itself, or never booted on a failure path.
		_, _ = e.Down()
	})

	return e
}

// TestEnsureSession_BootedTrueOnColdSessionFalseOnWarm pins the booted return value of EnsureSession.
// It reports true against a worktree whose session has never been created.
// It reports false when called again against the live session it just created.
// It sets an environment variable, process-global state, so it does not run in parallel.
func TestEnsureSession_BootedTrueOnColdSessionFalseOnWarm(t *testing.T) {
	tiergate.Require(t, 2)
	t.Setenv(scrubProbeEnv, "1")
	e := newColdScratchEngine(t)

	// The test kit pre-starts a session-less hermetic server on this key;
	// the cold boot must reuse it, not wait out the stale-holder grace and replace it.
	// A boot that waited out the grace would find the holder still there and replace it,
	// so an unchanged server pid proves the reuse however loaded the machine.
	serverPID := func() string {
		t.Helper()
		out, err := e.tmux.output("display-message", "-p", "#{pid}")
		if err != nil {
			t.Fatalf("display-message #{pid}: %v", err)
		}
		return strings.TrimSpace(string(out))
	}
	pidBefore := serverPID()

	booted, err := e.EnsureSession()
	if err != nil {
		t.Fatalf("EnsureSession (cold) = %v, want a nil error", err)
	}
	if !booted {
		t.Errorf("EnsureSession (cold) booted = false, want true — nothing usable existed to attach to")
	}
	if pidAfter := serverPID(); pidAfter != pidBefore {
		t.Errorf("server pid after the cold boot = %s, want the pre-started server's %s", pidAfter, pidBefore)
	}
	signal := e.geom.DiscoverSignalPath
	if _, err := os.Stat(signal); err != nil {
		t.Fatalf("discover signal after a cold boot: %v, want the file at the told path", err)
	}
	if err := os.Remove(signal); err != nil {
		t.Fatalf("remove discover signal: %v", err)
	}

	booted, err = e.EnsureSession()
	if err != nil {
		t.Fatalf("EnsureSession (warm) = %v, want a nil error", err)
	}
	if booted {
		t.Errorf("EnsureSession (warm) booted = true, want false — the session it just created is already usable")
	}

	st, err := LoadState(e.stateDir())
	if err != nil {
		t.Fatalf("LoadState after the cold boot: %v", err)
	}
	if !slices.Contains(st.StrippedEnv, scrubProbeEnv) {
		t.Errorf("StrippedEnv = %v, want it to hold %s", st.StrippedEnv, scrubProbeEnv)
	}
	for _, key := range st.StrippedEnv {
		if !e.scrub.matches(key) {
			t.Errorf("StrippedEnv holds %q, which is no name or prefix of the told scrub", key)
		}
	}
	if _, err := os.Stat(signal); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("discover signal after a warm ensure: stat error = %v, want it absent — an attach touches nothing", err)
	}
}

// TestAddStrand_LogsAttributionOnlyOnColdBoot pins the attribution log line AddStrand emits.
// It captures with logcapture.CaptureVerbose, which sets both the logger output and its verbosity;
// neither alone captures at Info.
// A cold AddStrand must emit the attribution line,
// and a second AddStrand against the now-live session must not.
// It captures the process-global logger, so it does not run in parallel.
func TestAddStrand_LogsAttributionOnlyOnColdBoot(t *testing.T) {
	tiergate.Require(t, 2)
	e := newColdScratchEngine(t)
	logs := logcapture.CaptureVerbose(t)

	const attributionLine = "reed: add self-healed a cold worktree's session"

	if _, err := e.AddStrand(AddSpec{Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorOwnWindow}}); err != nil {
		t.Fatalf("AddStrand (cold) = %v, want a nil error", err)
	}
	if !strings.Contains(logs.String(), attributionLine) {
		t.Errorf("log output after a cold AddStrand = %q; want it to contain %q", logs.String(), attributionLine)
	}

	logs.Reset()
	if _, err := e.AddStrand(AddSpec{Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorOwnWindow}}); err != nil {
		t.Fatalf("AddStrand (warm) = %v, want a nil error", err)
	}
	if strings.Contains(logs.String(), attributionLine) {
		t.Errorf("log output after a warm AddStrand = %q; want it NOT to contain %q — the session was already up before this call", logs.String(), attributionLine)
	}
}

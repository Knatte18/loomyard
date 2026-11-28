// smoke_helpers_test.go builds what both smoke tiers share.
// It builds a hub with a live reed session and a strand added through `lym reed add`;
// it also runs the creel daemon in process on the real socket and polls panes.

package creelcli

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// smokePaneWindow is how long a smoke test waits for a pane to show a line before it reads it as absent.
const smokePaneWindow = 60 * time.Second

// smokeHub is a hub whose reed session is up, with the creel geometry of that hub.
type smokeHub struct {
	hub      *hubforge.Hub
	cwd      string
	reed     *reedengine.Engine
	geometry creelengine.Geometry
}

// newSmokeHub builds a hub, boots its reed session on a hermetic tmux server and tears both down when the test ends.
// It sets the strand name in the environment to empty, so every creel verb the test runs is EXTERNAL.
func newSmokeHub(t *testing.T) *smokeHub {
	t.Helper()
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not on PATH")
	}
	t.Setenv(agentname.StrandNameEnv, "")
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	location := hub.Location
	reedGeometry, err := hubgeom.ReedGeometry(location)
	if err != nil {
		t.Fatalf("ReedGeometry() = %v", err)
	}
	// Registered before the first boot, so reed's boot finds the kit's hermetic server on the hub's key.
	tmuxkit.KillOnCleanup(t, tmuxPath, reedGeometry.SocketKey)
	reedConfig, err := reedengine.LoadConfig(fabricengine.ConfigDirs(location), "reed")
	if err != nil {
		t.Fatalf("LoadConfig(reed) = %v", err)
	}
	smoke := &smokeHub{hub: hub, cwd: hub.PrimeWorktree(), reed: reedengine.New(reedConfig, reedGeometry, shuttleengines.EnvScrub())}

	if _, err := smoke.reed.Up(); err != nil {
		t.Fatalf("reed Up() = %v", err)
	}
	t.Cleanup(func() {
		smoke.reed.Down()
	})

	root, err := lymtemp.ProcessRoot()
	if err != nil {
		t.Fatalf("ProcessRoot() = %v", err)
	}
	smoke.geometry, err = hubgeom.CreelGeometry(hub.Path, root.Path())
	if err != nil {
		t.Fatalf("CreelGeometry() = %v", err)
	}
	return smoke
}

// addStrand adds a strand running command with no provider tag, as `lym reed add --cmd <command> --role <role>` does.
// It returns the strand's name and guid.
func (s *smokeHub) addStrand(t *testing.T, role, command string) (name, guid string) {
	t.Helper()
	return s.addProviderStrand(t, role, command, "")
}

// addProviderStrand adds a strand running command through the smoke hub's reed engine, with the provider tag set.
// It returns the strand's name and guid.
func (s *smokeHub) addProviderStrand(t *testing.T, role, command, provider string) (name, guid string) {
	t.Helper()
	strand, err := s.reed.AddStrand(reedengine.AddSpec{Role: role, Cmd: command, Provider: provider, Display: render.Display{Anchor: render.AnchorOwnWindow}})
	if err != nil {
		t.Fatalf("AddStrand(%q, provider %q) = %v", command, provider, err)
	}
	return strand.Name, strand.GUID
}

// startDaemon runs the creel daemon in process on the hub's real socket and stops it when the test ends.
// The hub delivery takes its engines from enginesFor.
func (s *smokeHub) startDaemon(t *testing.T, enginesFor func(shuttleengine.Config, configengine.Dirs) (shuttleengine.EngineSet, error)) {
	t.Helper()
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- creelengine.Run(ctx, creelengine.Options{
			Geometry:  s.geometry,
			Delivery:  newHubDelivery(s.hub.Path, enginesFor, reedengine.Outside{}),
			Clock:     realClock{},
			ParentPID: proc.ParentPID,
			GOOS:      runtime.GOOS,
			Ready:     func() { close(ready) },
		})
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("the daemon ended before it listened: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("the daemon did not listen")
	}
}

// creel runs one creel verb in the hub and returns its exit code and printed envelope.
func (s *smokeHub) creel(args ...string) (int, string) {
	var out bytes.Buffer
	code := clihelp.ExecuteIn(Command(), s.cwd, &out, args)
	return code, out.String()
}

// lastPane returns the strand's pane as it reads now, for a failure message.
func (s *smokeHub) lastPane(t *testing.T, guid string) string {
	t.Helper()
	capture, err := s.reed.CapturePane(guid)
	if err != nil {
		t.Fatalf("CapturePane(%s) = %v", guid, err)
	}
	return capture
}

// awaitPane polls the strand's pane once a second until it shows want or smokePaneWindow passes.
// It returns the last capture.
// The pane has no event to wait on, so it polls on a tick under a real-time deadline.
func (s *smokeHub) awaitPane(t *testing.T, guid, want string) (string, bool) {
	t.Helper()
	deadline := time.Now().Add(smokePaneWindow)
	for {
		capture, err := s.reed.CapturePane(guid)
		if err != nil {
			t.Fatalf("CapturePane(%s) = %v", guid, err)
		}
		if strings.Contains(capture, want) {
			return capture, true
		}
		if time.Now().After(deadline) {
			return capture, false
		}
		time.Sleep(time.Second)
	}
}

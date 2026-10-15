//go:build tmux || llm

// smoke_helpers_test.go builds what both smoke tiers share: a hub with a live reed session, a strand added through `lyx reed add`, the creel daemon run in process on the real socket, and pane polling.

package creelcli

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxtemp"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/reedengine"
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

	var up bytes.Buffer
	if code := reedcli.RunCLIIn(smoke.cwd, &up, []string{"up"}); code != 0 {
		t.Fatalf("lyx reed up = %d; output: %s", code, up.String())
	}
	t.Cleanup(func() {
		var down bytes.Buffer
		reedcli.RunCLIIn(smoke.cwd, &down, []string{"down"})
	})

	root, err := lyxtemp.ProcessRoot()
	if err != nil {
		t.Fatalf("ProcessRoot() = %v", err)
	}
	smoke.geometry, err = hubgeom.CreelGeometry(hub.Path, root.Path())
	if err != nil {
		t.Fatalf("CreelGeometry() = %v", err)
	}
	return smoke
}

// addStrand runs `lyx reed add --cmd <command> --role <role>` in the hub and returns the strand's name and guid.
func (s *smokeHub) addStrand(t *testing.T, role, command string) (name, guid string) {
	t.Helper()
	var out bytes.Buffer
	if code := reedcli.RunCLIIn(s.cwd, &out, []string{"add", "--cmd", command, "--role", role}); code != 0 {
		t.Fatalf("lyx reed add --cmd %q = %d; output: %s", command, code, out.String())
	}
	var added struct {
		Name string `json:"name"`
		GUID string `json:"guid"`
	}
	if err := json.Unmarshal(out.Bytes(), &added); err != nil || added.GUID == "" {
		t.Fatalf("lyx reed add printed %q; want the strand's name and guid: %v", out.String(), err)
	}
	return added.Name, added.GUID
}

// startDaemon runs the creel daemon in process on the hub's real socket, with the hub delivery over enginesFor, and stops it when the test ends.
func (s *smokeHub) startDaemon(t *testing.T, enginesFor func(shuttleengine.Config, configengine.Dirs) (shuttleengine.EngineSet, error)) {
	t.Helper()
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- creelengine.Run(ctx, creelengine.Options{
			Geometry:  s.geometry,
			Delivery:  newHubDelivery(s.hub.Path, enginesFor),
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
	code := RunCLIIn(s.cwd, &out, args)
	return code, out.String()
}

// awaitPane polls the strand's pane once a second until it shows want or smokePaneWindow passes, and returns the last capture.
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

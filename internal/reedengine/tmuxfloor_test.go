//go:build tier0 && !windows

// tmuxfloor_test.go pins the tmux floor check: the runner's refusal, the per-path cache, the retry of a probe that could not run, the allow-set-title answer and the server spawn's refusal.

package reedengine

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/toolfloor"
)

// countingProbe answers every probe with line and counts the paths it was asked about.
type countingProbe struct {
	line  string
	paths []string
}

func (p *countingProbe) probe(tmuxPath string) (string, error) {
	p.paths = append(p.paths, tmuxPath)
	return p.line, nil
}

func requireFloorError(t *testing.T, what string, err error) {
	t.Helper()
	var floorErr *toolfloor.FloorError
	if !errors.As(err, &floorErr) {
		t.Errorf("%s error = %v; want a *toolfloor.FloorError", what, err)
	}
}

// TestTmuxFloor swaps the process-global version probe and its cache, so it and its rows are not parallel.
// Go starts the package's parallel tests only after its serial ones finish, so none of them sees an injected line.
func TestTmuxFloor(t *testing.T) {
	t.Run("RunnerRefusesBeforeRunningTmux", func(t *testing.T) {
		var cmd TmuxCmd
		fake := installFakeTmuxOn(t, &cmd)
		cmd.floorLine = "tmux 3.3a"
		requireFloorError(t, "run", cmd.run("list-sessions"))
		requireFloorError(t, "runStdin", cmd.runStdin([]byte("x"), "load-buffer", "-"))
		_, err := cmd.output("list-sessions")
		requireFloorError(t, "output", err)
		if got := len(fake.Calls()); got != 0 {
			t.Errorf("fake saw %d calls; want 0", got)
		}
	})

	t.Run("HookRunnerWithoutFloorLineRunsNoProbe", func(t *testing.T) {
		probe := &countingProbe{line: "tmux 3.3a"}
		setTmuxVersionProbeForTest(t, probe.probe)
		var cmd TmuxCmd
		fake := installFakeTmuxOn(t, &cmd)
		if err := cmd.run("list-sessions"); err != nil {
			t.Fatalf("run() = %v; want nil", err)
		}
		if len(probe.paths) != 0 {
			t.Errorf("probes = %v; want none", probe.paths)
		}
		if got := fake.Count("list-sessions"); got != 1 {
			t.Errorf("fake list-sessions calls = %d; want 1", got)
		}
	})

	t.Run("HookRunnerWithUnparseableFloorLineIsAdmitted", func(t *testing.T) {
		var cmd TmuxCmd
		fake := installFakeTmuxOn(t, &cmd)
		cmd.floorLine = "tmux master"
		if err := cmd.run("list-sessions"); err != nil {
			t.Fatalf("run() = %v; want nil", err)
		}
		if got := fake.Count("list-sessions"); got != 1 {
			t.Errorf("fake list-sessions calls = %d; want 1", got)
		}
	})

	t.Run("ProductionRunnerRefusesFromTheProcessCache", func(t *testing.T) {
		probe := &countingProbe{line: "tmux 3.3a"}
		setTmuxVersionProbeForTest(t, probe.probe)
		noBinary := filepath.Join(t.TempDir(), "no-such-tmux")
		cmd := NewTmuxCmd(noBinary, "sock")
		requireFloorError(t, "run", cmd.run("list-sessions"))
		requireFloorError(t, "runStdin", cmd.runStdin([]byte("x"), "load-buffer", "-"))
		_, err := cmd.output("list-sessions")
		requireFloorError(t, "output", err)
		requireFloorError(t, "CheckTmuxFloor", CheckTmuxFloor(noBinary))
	})

	t.Run("ProbeRunsOncePerResolvedPath", func(t *testing.T) {
		probe := &countingProbe{line: "tmux 3.4"}
		setTmuxVersionProbeForTest(t, probe.probe)
		other := filepath.Join(t.TempDir(), "other-tmux")
		for _, path := range []string{"sh", "sh"} {
			if err := CheckTmuxFloor(path); err != nil {
				t.Fatalf("CheckTmuxFloor(%q) = %v; want nil", path, err)
			}
		}
		if len(probe.paths) != 1 {
			t.Fatalf("probes after two calls on one path = %d; want 1", len(probe.paths))
		}
		resolved, err := exec.LookPath("sh")
		if err != nil {
			t.Fatalf("resolve sh: %v", err)
		}
		if err := CheckTmuxFloor(resolved); err != nil {
			t.Fatalf("CheckTmuxFloor(%q) = %v; want nil", resolved, err)
		}
		if len(probe.paths) != 1 {
			t.Errorf("probes after the resolved spelling = %d; want 1", len(probe.paths))
		}
		if err := CheckTmuxFloor(other); err != nil {
			t.Fatalf("CheckTmuxFloor(%q) = %v; want nil", other, err)
		}
		if len(probe.paths) != 2 {
			t.Errorf("probes after a second path = %d; want 2", len(probe.paths))
		}
	})

	t.Run("ProbeThatCouldNotRunIsRetried", func(t *testing.T) {
		scripted := errors.New("scripted exec failure")
		probes := 0
		setTmuxVersionProbeForTest(t, func(string) (string, error) {
			probes++
			if probes == 1 {
				return "", scripted
			}
			return "tmux 3.4", nil
		})
		if err := CheckTmuxFloor("sh"); !errors.Is(err, scripted) {
			t.Errorf("first CheckTmuxFloor = %v; want the scripted error", err)
		}
		if err := CheckTmuxFloor("sh"); err != nil {
			t.Errorf("second CheckTmuxFloor = %v; want nil", err)
		}
		if probes != 2 {
			t.Errorf("probes = %d; want 2", probes)
		}
	})

	t.Run("UnparseableLineIsAdmittedWithOneWarning", func(t *testing.T) {
		logs := logcapture.Capture(t)
		probe := &countingProbe{line: "tmux next-3.6"}
		setTmuxVersionProbeForTest(t, probe.probe)
		for i := 0; i < 2; i++ {
			if err := CheckTmuxFloor("sh"); err != nil {
				t.Fatalf("CheckTmuxFloor call %d = %v; want nil", i+1, err)
			}
		}
		if len(probe.paths) != 1 {
			t.Errorf("probes = %d; want 1", len(probe.paths))
		}
		if got := strings.Count(logs.String(), "tmux next-3.6"); got != 1 {
			t.Errorf("warnings naming the line = %d; want 1\n%s", got, logs.String())
		}
	})

	t.Run("AllowSetTitleAnswerComesFromTheOneProbe", func(t *testing.T) {
		tests := []struct {
			line        string
			wantLacks   bool
			wantInfoLog int
		}{
			{line: "tmux 3.4", wantLacks: true, wantInfoLog: 1},
			{line: "tmux 3.5", wantLacks: false, wantInfoLog: 0},
			{line: "tmux next-3.6", wantLacks: false, wantInfoLog: 0},
		}
		for _, tt := range tests {
			logs := logcapture.CaptureVerbose(t)
			probe := &countingProbe{line: tt.line}
			setTmuxVersionProbeForTest(t, probe.probe)
			cmd := NewTmuxCmd("sh", "sock")
			if err := CheckTmuxFloor("sh"); err != nil {
				t.Fatalf("CheckTmuxFloor on %q = %v; want nil", tt.line, err)
			}
			for i := 0; i < 2; i++ {
				if got := cmd.lacksAllowSetTitle(); got != tt.wantLacks {
					t.Errorf("lacksAllowSetTitle() on %q call %d = %v; want %v", tt.line, i+1, got, tt.wantLacks)
				}
			}
			if len(probe.paths) != 1 {
				t.Errorf("probes on %q = %d; want 1", tt.line, len(probe.paths))
			}
			if got := strings.Count(logs.String(), "no allow-set-title"); got != tt.wantInfoLog {
				t.Errorf("fallback notes on %q = %d; want %d\n%s", tt.line, got, tt.wantInfoLog, logs.String())
			}
		}
	})

	t.Run("ServerSpawnRefusesBeforeRunningTmux", func(t *testing.T) {
		setTmuxVersionProbeForTest(t, (&countingProbe{line: "tmux 3.3a"}).probe)
		e, _ := reviveTestEngine(t)
		e.geom.SpawnOrder = toldSpawnOrder()
		_, _, err := e.ensureServerAndSessionLocked()
		requireFloorError(t, "ensureServerAndSessionLocked", err)
	})
}

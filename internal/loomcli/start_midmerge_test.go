// start_midmerge_test.go pins runDriverSpawnAndWait's mid-merge check: it refuses a spawn or a resume over an unfinished merge, and leaves a live working driver alone.
// Fakes only, no git.

package loomcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// fakeMidMerge records its calls and answers a canned state or error.
type fakeMidMerge struct {
	calls int
	state fabricengine.MidMergeState
	err   error
}

func (f *fakeMidMerge) probe(*lyxcwd.Location) (fabricengine.MidMergeState, error) {
	f.calls++
	return f.state, f.err
}

type midMergeEnvelope struct {
	OK        bool      `json:"ok"`
	Error     string    `json:"error"`
	Kind      string    `json:"kind"`
	Conflicts *[]string `json:"conflicts"`
}

func decodeMidMergeEnvelope(t *testing.T, out string) midMergeEnvelope {
	t.Helper()
	var env midMergeEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out, err)
	}
	return env
}

// assertMergeRefusal asserts out is a merge_in_progress refusal carrying want as its conflicts list.
func assertMergeRefusal(t *testing.T, out string, want []string) midMergeEnvelope {
	t.Helper()
	env := decodeMidMergeEnvelope(t, out)
	if env.OK {
		t.Fatalf("envelope ok = true; want a refusal (%q)", out)
	}
	if env.Kind != shedrun.StartMergeInProgressKind {
		t.Errorf("kind = %q; want %q", env.Kind, shedrun.StartMergeInProgressKind)
	}
	if env.Conflicts == nil {
		t.Fatalf("conflicts missing from %q", out)
	}
	if strings.Join(*env.Conflicts, "|") != strings.Join(want, "|") {
		t.Errorf("conflicts = %v; want %v", *env.Conflicts, want)
	}
	return env
}

// assertNothingPutToWork asserts a refusal neither started a driver, typed a resume line, nor opened the go arm's driver log.
func assertNothingPutToWork(t *testing.T, c *loomCLI, starter *fakeDriverStarter, sender *fakeDriverSender) {
	t.Helper()
	if starter.called {
		t.Error("the starter was called")
	}
	if len(sender.texts) != 0 {
		t.Errorf("sender sent %d line(s); want none", len(sender.texts))
	}
	if _, err := os.Stat(loomengine.LoomDriverLog(c.location)); !os.IsNotExist(err) {
		t.Errorf("driver log exists after a refusal (stat err %v)", err)
	}
}

func TestRunDriverSpawnAndWait_MidMerge_SpawnRefusals(t *testing.T) {
	fabricMsg := []string{"lyx fabric merge-stage", "lyx fabric merge --continue", "lyx fabric merge --abort"}
	tests := []struct {
		name      string
		driver    string
		state     fabricengine.MidMergeState
		want      []string
		wantIn    []string
		wantNotIn []string
		// staleMark writes a park marker before the call, which a refusal must leave on disk.
		staleMark bool
		// parkedLive replaces the receiver with one whose live driver is parked, its park marker on disk.
		parkedLive bool
	}{
		{name: "llm fabric-parked with conflicts", driver: shedrun.DriverLLM, state: fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked, Conflicts: []string{"a.go", "b/c.go"}}, want: []string{"a.go", "b/c.go"}, wantIn: fabricMsg},
		{name: "go arm fabric-parked", driver: shedrun.DriverGo, state: fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked, Conflicts: []string{"a.go"}}, want: []string{"a.go"}, wantIn: fabricMsg},
		{name: "parked with no conflicts left", driver: shedrun.DriverLLM, state: fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked}, want: []string{}, wantIn: fabricMsg},
		{name: "foreign", driver: shedrun.DriverLLM, state: fabricengine.MidMergeState{Kind: fabricengine.MidMergeForeign, Conflicts: []string{"x"}}, want: []string{"x"}, wantIn: []string{"git"}, wantNotIn: []string{"merge-stage"}},
		{name: "stale marker kept", driver: shedrun.DriverLLM, state: fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked, Conflicts: []string{"a"}}, want: []string{"a"}, wantIn: fabricMsg, staleMark: true},
		{name: "parked live driver refused", driver: shedrun.DriverLLM, state: fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked, Conflicts: []string{"a.go"}}, want: []string{"a.go"}, wantIn: fabricMsg, parkedLive: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			starter := &fakeDriverStarter{}
			sender := &fakeDriverSender{}
			var c *loomCLI
			var lockPath string
			marker := ""
			if tc.parkedLive {
				c, lockPath, marker = newResumeBranchReceiver(t, starter, &fakeDriverPaneProbeFull{strandsFn: parkedStrands(true)}, sender)
			} else {
				c, lockPath = newTestSpawnAndWaitReceiver(t, starter, &fakeDriverPaneProbeFull{strandsFn: noStrands})
				c.driverSender = sender
				c.frictionDir = t.TempDir()
				writeTestRunState(t, c, shedengine.StateRunning)
				marker = shedrun.ParkMarker(c.location, shedrun.ResolveRunID(c.location, c.runID))
				if tc.staleMark {
					if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(marker, nil, 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			fake := &fakeMidMerge{state: tc.state}
			c.midMerge = fake.probe

			var out bytes.Buffer
			bl := acquireTestBootstrapLock(t, lockPath)
			ok := c.runDriverSpawnAndWait(context.Background(), &out, tc.driver, bl, noopLockHeld)

			if ok {
				t.Fatalf("runDriverSpawnAndWait() = true; want a refusal")
			}
			env := assertMergeRefusal(t, out.String(), tc.want)
			if tc.want != nil && len(tc.want) == 0 && !strings.Contains(out.String(), `"conflicts":[]`) {
				t.Errorf("raw envelope %q lacks an empty conflicts list", out.String())
			}
			for _, s := range tc.wantIn {
				if !strings.Contains(env.Error, s) {
					t.Errorf("message %q lacks %q", env.Error, s)
				}
			}
			for _, s := range tc.wantNotIn {
				if strings.Contains(env.Error, s) {
					t.Errorf("message %q contains %q", env.Error, s)
				}
			}
			assertNothingPutToWork(t, c, starter, sender)
			if _, found := voucherOnDisk(t, c); found {
				t.Error("a merge refusal wrote a handoff voucher")
			}
			if (tc.staleMark || tc.parkedLive) && !markerExists(marker) {
				t.Error("park marker removed by a refusal")
			}
			assertBootstrapLockReleased(t, lockPath)
		})
	}
}

//testtiming:keep pins a live working driver, running or halted at hand-back, never reaching the mid-merge probe; the covering test never counts probe calls
func TestRunDriverSpawnAndWait_MidMerge_LiveWorkingDriverNotProbed(t *testing.T) {
	t.Run("running", func(t *testing.T) {
		c, lockPath, marker := newResumeBranchReceiver(t, &fakeDriverStarter{}, &fakeDriverPaneProbeFull{strandsFn: parkedStrands(true)}, &fakeDriverSender{})
		_ = os.Remove(marker)
		writeTestRunState(t, c, shedengine.StateRunning)
		fake := &fakeMidMerge{state: fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked}}
		c.midMerge = fake.probe

		bl := acquireTestBootstrapLock(t, lockPath)
		ok := c.runDriverSpawnAndWait(context.Background(), &bytes.Buffer{}, shedrun.DriverLLM, bl, noopLockHeld)

		if !ok {
			t.Fatal("runDriverSpawnAndWait() = false; want true")
		}
		if fake.calls != 0 {
			t.Errorf("probe calls = %d; want 0", fake.calls)
		}
		_ = bl.Release()
	})
	t.Run("halted at hand-back", func(t *testing.T) {
		c, lockPath, marker := newResumeBranchReceiver(t, &fakeDriverStarter{}, &fakeDriverPaneProbeFull{strandsFn: parkedStrands(true)}, &fakeDriverSender{})
		_ = os.Remove(marker)
		writeTestRunState(t, c, shedengine.StateAwaiting)
		fake := &fakeMidMerge{state: fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked}}
		c.midMerge = fake.probe

		var out bytes.Buffer
		bl := acquireTestBootstrapLock(t, lockPath)
		ok := c.runDriverSpawnAndWait(context.Background(), &out, shedrun.DriverLLM, bl, noopLockHeld)

		if ok {
			t.Fatal("runDriverSpawnAndWait() = true; want a refusal")
		}
		if fake.calls != 0 {
			t.Errorf("probe calls = %d; want 0", fake.calls)
		}
		if env := decodeMidMergeEnvelope(t, out.String()); env.Kind != shedrun.StartNotParkedKind {
			t.Errorf("kind = %q; want %q", env.Kind, shedrun.StartNotParkedKind)
		}
		assertBootstrapLockReleased(t, lockPath)
	})
}

func TestRunDriverSpawnAndWait_MidMerge_ProbeErrorRefuses(t *testing.T) {
	tests := []struct {
		name  string
		probe func(*lyxcwd.Location) (fabricengine.MidMergeState, error)
		// wantIn is a substring of the error message, when the probe's failure is canned.
		wantIn string
	}{
		{name: "probe error names the failure", probe: (&fakeMidMerge{err: errors.New("boom")}).probe, wantIn: "boom"},
		{name: "real probe on a non-pair", probe: fabricengine.MidMerge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			starter := &fakeDriverStarter{}
			sender := &fakeDriverSender{}
			c, lockPath := newTestSpawnAndWaitReceiver(t, starter, &fakeDriverPaneProbeFull{strandsFn: noStrands})
			c.driverSender = sender
			c.midMerge = tt.probe

			var out bytes.Buffer
			bl := acquireTestBootstrapLock(t, lockPath)
			ok := c.runDriverSpawnAndWait(context.Background(), &out, shedrun.DriverLLM, bl, noopLockHeld)

			if ok {
				t.Fatal("runDriverSpawnAndWait() = true; want a refusal")
			}
			env := decodeMidMergeEnvelope(t, out.String())
			if env.OK || env.Kind == shedrun.StartMergeInProgressKind || !strings.Contains(env.Error, tt.wantIn) {
				t.Errorf("envelope = %+v; want a plain error containing %q", env, tt.wantIn)
			}
			assertNothingPutToWork(t, c, starter, sender)
			assertBootstrapLockReleased(t, lockPath)
		})
	}
}

//testtiming:keep pins the mid-merge probe running exactly once on a clean pair before the starter is reached; its covering test uses a probe that counts nothing
func TestRunDriverSpawnAndWait_MidMerge_CleanPairProceeds(t *testing.T) {
	starter := &fakeDriverStarter{handle: stubDriverHandle{guid: "g-new", runDir: "/run/dir"}}
	c, lockPath := newTestSpawnAndWaitReceiver(t, starter, &fakeDriverPaneProbeFull{strandsFn: noStrands})
	fake := &fakeMidMerge{state: fabricengine.MidMergeState{Kind: fabricengine.MidMergeNone}}
	c.midMerge = fake.probe

	bl := acquireTestBootstrapLock(t, lockPath)
	c.runDriverSpawnAndWait(context.Background(), &bytes.Buffer{}, shedrun.DriverLLM, bl, noopLockHeld)

	if fake.calls != 1 {
		t.Errorf("probe calls = %d; want 1", fake.calls)
	}
	if !starter.called {
		t.Error("the starter was not reached on a clean pair")
	}
	_ = bl.Release()
}

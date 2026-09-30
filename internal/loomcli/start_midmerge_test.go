// start_midmerge_test.go pins runDriverSpawnAndWait's mid-merge check: it refuses a spawn or a resume over an unfinished merge, and leaves a live working driver alone.
// Fakes only, no git.
package loomcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
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

func TestRunDriverSpawnAndWait_MidMerge_SpawnRefusals(t *testing.T) {
	fabricMsg := []string{"lyx fabric merge-stage", "lyx fabric merge --continue", "lyx fabric merge --abort"}
	tests := []struct {
		name      string
		driver    string
		state     fabricengine.MidMergeState
		want      []string
		wantIn    []string
		wantNotIn []string
		staleMark bool
	}{
		{"llm fabric-parked with conflicts", shedrun.DriverLLM, fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked, Conflicts: []string{"a.go", "b/c.go"}}, []string{"a.go", "b/c.go"}, fabricMsg, nil, false},
		{"go arm fabric-parked", shedrun.DriverGo, fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked, Conflicts: []string{"a.go"}}, []string{"a.go"}, fabricMsg, nil, false},
		{"parked with no conflicts left", shedrun.DriverLLM, fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked}, []string{}, fabricMsg, nil, false},
		{"foreign", shedrun.DriverLLM, fabricengine.MidMergeState{Kind: fabricengine.MidMergeForeign, Conflicts: []string{"x"}}, []string{"x"}, []string{"git"}, []string{"merge-stage"}, false},
		{"stale marker kept", shedrun.DriverLLM, fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked, Conflicts: []string{"a"}}, []string{"a"}, fabricMsg, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			starter := &fakeDriverStarter{}
			c, lockPath := newTestSpawnAndWaitReceiver(t, starter, &fakeDriverPaneProbeFull{strandsFn: noStrands})
			fake := &fakeMidMerge{state: tc.state}
			c.midMerge = fake.probe
			marker := shedrun.ParkMarker(c.location, shedrun.ResolveRunID(c.location, c.runID))
			if tc.staleMark {
				if err := os.MkdirAll(dirOf(marker), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(marker, nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}

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
			if starter.called {
				t.Error("the starter was called")
			}
			if _, err := os.Stat(loomengine.LoomDriverLog(c.location)); !os.IsNotExist(err) {
				t.Errorf("driver log exists after a refusal (stat err %v)", err)
			}
			if tc.staleMark && !markerExists(marker) {
				t.Error("park marker removed by a refusal")
			}
			assertBootstrapLockReleased(t, lockPath)
		})
	}
}

func dirOf(p string) string {
	i := strings.LastIndexAny(p, `/\`)
	return p[:i]
}

func TestRunDriverSpawnAndWait_MidMerge_ParkedLiveDriverRefused(t *testing.T) {
	starter := &fakeDriverStarter{}
	sender := &fakeDriverSender{}
	c, lockPath, marker := newResumeBranchReceiver(t, starter, &fakeDriverPaneProbeFull{strandsFn: parkedStrands(true)}, sender)
	fake := &fakeMidMerge{state: fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked, Conflicts: []string{"a.go"}}}
	c.midMerge = fake.probe

	var out bytes.Buffer
	bl := acquireTestBootstrapLock(t, lockPath)
	ok := c.runDriverSpawnAndWait(context.Background(), &out, shedrun.DriverLLM, bl, noopLockHeld)

	if ok {
		t.Fatal("runDriverSpawnAndWait() = true; want a refusal")
	}
	assertMergeRefusal(t, out.String(), []string{"a.go"})
	if len(sender.texts) != 0 || starter.called {
		t.Errorf("sent %d line(s), starter called = %v; want neither", len(sender.texts), starter.called)
	}
	if !markerExists(marker) {
		t.Error("park marker removed by a refusal")
	}
	assertBootstrapLockReleased(t, lockPath)
}

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
	starter := &fakeDriverStarter{}
	c, lockPath := newTestSpawnAndWaitReceiver(t, starter, &fakeDriverPaneProbeFull{strandsFn: noStrands})
	fake := &fakeMidMerge{err: errors.New("boom")}
	c.midMerge = fake.probe

	var out bytes.Buffer
	bl := acquireTestBootstrapLock(t, lockPath)
	ok := c.runDriverSpawnAndWait(context.Background(), &out, shedrun.DriverLLM, bl, noopLockHeld)

	if ok {
		t.Fatal("runDriverSpawnAndWait() = true; want a refusal")
	}
	env := decodeMidMergeEnvelope(t, out.String())
	if env.OK || env.Kind == shedrun.StartMergeInProgressKind || !strings.Contains(env.Error, "boom") {
		t.Errorf("envelope = %+v; want a plain error naming boom", env)
	}
	if starter.called {
		t.Error("the starter was called")
	}
	assertBootstrapLockReleased(t, lockPath)
}

func TestRunDriverSpawnAndWait_MidMerge_RealProbeOnNonPairRefuses(t *testing.T) {
	starter := &fakeDriverStarter{}
	c, lockPath := newTestSpawnAndWaitReceiver(t, starter, &fakeDriverPaneProbeFull{strandsFn: noStrands})
	c.midMerge = fabricengine.MidMerge

	var out bytes.Buffer
	bl := acquireTestBootstrapLock(t, lockPath)
	ok := c.runDriverSpawnAndWait(context.Background(), &out, shedrun.DriverLLM, bl, noopLockHeld)

	if ok {
		t.Fatal("runDriverSpawnAndWait() = true; want a refusal")
	}
	env := decodeMidMergeEnvelope(t, out.String())
	if env.OK || env.Kind == shedrun.StartMergeInProgressKind {
		t.Errorf("envelope = %+v; want a plain probe error", env)
	}
	if starter.called {
		t.Error("the starter was called")
	}
	assertBootstrapLockReleased(t, lockPath)
}

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

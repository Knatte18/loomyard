// resume_test.go pins `lyx loom resume` outcome by outcome over fakes for the driver directory, the merge probe and the driver sender, with real status, run-lock and park-marker files in a temp directory.
// It also pins that the verb takes the bootstrap lock before reading anything.

package loomcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// fakeDriverDirectory answers DriverRow with a canned row.
type fakeDriverDirectory struct {
	row   reedengine.DirectoryRow
	found bool
}

func (f fakeDriverDirectory) DriverRow() (reedengine.DirectoryRow, bool, error) {
	return f.row, f.found, nil
}

// resumeEnvelope is the envelope fields the verb's outcomes are asserted on.
type resumeEnvelope struct {
	OK         bool      `json:"ok"`
	Error      string    `json:"error"`
	Kind       string    `json:"kind"`
	Message    string    `json:"message"`
	StopReport string    `json:"stop_report"`
	Conflicts  *[]string `json:"conflicts"`
}

// newResumeVerbReceiver builds a receiver for the resume verb over the shared temp-directory fixture, with the given driver strand and a clean merge probe.
// c.reed stays nil, so a call into reed would panic: the verb must touch only its seams.
func newResumeVerbReceiver(t *testing.T, sender *fakeDriverSender, directory fakeDriverDirectory) (*loomCLI, *fakeDriverStarter, *fakeDriverPaneProbeFull) {
	t.Helper()
	starter := &fakeDriverStarter{}
	probe := &fakeDriverPaneProbeFull{strandsFn: noStrands}
	c, _ := newTestSpawnAndWaitReceiver(t, starter, probe)
	c.driverSender = sender
	c.driverResumeWait = func() {}
	c.driverDirectory = directory
	return c, starter, probe
}

// invokeResume runs the verb and returns its decoded envelope and exit code, reporting a failure as an error so a goroutine can call it.
func invokeResume(c *loomCLI) (resumeEnvelope, int, error) {
	ctx, exit := clihelp.NewExitContext(context.Background())
	var out bytes.Buffer
	cmd := c.resumeCmd()
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		return resumeEnvelope{}, 0, err
	}
	var env resumeEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		return resumeEnvelope{}, 0, fmt.Errorf("decode envelope %q: %w", out.String(), err)
	}
	return env, exit.Code(), nil
}

// runResume runs the verb and returns its decoded envelope and exit code.
func runResume(t *testing.T, c *loomCLI) (resumeEnvelope, int) {
	t.Helper()
	env, code, err := invokeResume(c)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	return env, code
}

func holdRunLock(t *testing.T, c *loomCLI) {
	t.Helper()
	held, err := lock.AcquireWriteLock(c.shedPaths.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = held.Release() })
}

func putParkMarker(t *testing.T, c *loomCLI) string {
	t.Helper()
	marker := shedrun.ParkMarker(c.location, shedrun.ResolveRunID(c.location, c.runID))
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return marker
}

// TestResumeVerb reaches every outcome of `lyx loom resume` and asserts for each the envelope, the exit code, that nothing was spawned, added or removed,
// that a resume line was typed only when a parked driver was woken, and that no message names `lyx loom start` as the retry of resume.
func TestResumeVerb(t *testing.T) {
	t.Parallel()

	liveDriver := fakeDriverDirectory{found: true, row: reedengine.DirectoryRow{Name: driverStrandDisplayName, GUID: "g-drv", Live: true}}
	deadDriver := fakeDriverDirectory{found: true, row: reedengine.DirectoryRow{Name: driverStrandDisplayName, GUID: "g-drv"}}
	retiringDriver := fakeDriverDirectory{found: true, row: reedengine.DirectoryRow{Name: driverStrandDisplayName, GUID: "g-drv", Live: true, Retiring: true}}
	noDriver := fakeDriverDirectory{}
	mergeParked := fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked, Conflicts: []string{"a.go"}}

	tests := []struct {
		name string
		// runState is the persisted run state;
		// empty writes no status file.
		runState  shedengine.State
		directory fakeDriverDirectory
		runLock   bool
		marker    bool
		// noScratch points the lock paths at a directory that does not exist after the status file is written, as on a pair re-created from its branch.
		noScratch bool
		merge     fabricengine.MidMergeState
		wantOK    bool
		// wantKind is the refusal's envelope kind;
		// empty means none.
		wantKind string
		// wantIn holds substrings of the envelope's message or error.
		wantIn []string
		// wantSent is whether the resume line was typed.
		wantSent bool
	}{
		{name: "no status file", wantIn: []string{`"lyx loom start"`, "task worktree"}},
		{name: "done is a no-op success", runState: shedengine.StateDone, directory: noDriver, wantOK: true, wantIn: []string{"done"}},
		{name: "awaiting is refused", runState: shedengine.StateAwaiting, directory: liveDriver, marker: true, wantIn: []string{"lyx loom approve", "lyx loom reject", "lyx batten run"}},
		{name: "running with a live driver is a no-op", runState: shedengine.StateRunning, directory: liveDriver, wantOK: true, wantIn: []string{"already running"}},
		{name: "running with the run lock held is a no-op", runState: shedengine.StateRunning, directory: noDriver, runLock: true, wantOK: true, wantIn: []string{"already running"}},
		{name: "running with the scratch directory absent answers from the run's state", runState: shedengine.StateRunning, directory: liveDriver, noScratch: true, wantOK: true, wantIn: []string{"already running"}},
		{name: "running with no live driver is refused", runState: shedengine.StateRunning, directory: noDriver, wantIn: []string{"no live driver", `"lyx loom start"`}},
		{name: "halted with the run lock held is refused", runState: shedengine.StateBlocked, directory: liveDriver, runLock: true, marker: true, wantKind: shedrun.StartNotParkedKind, wantIn: []string{"post-run work", `"lyx loom resume"`}},
		{name: "halted live driver without a marker is refused", runState: shedengine.StatePaused, directory: liveDriver, wantKind: shedrun.StartNotParkedKind, wantIn: []string{"has not parked yet", "`lyx loom resume`"}},
		{name: "halted live driver without a marker is not probed for a merge", runState: shedengine.StateFailed, directory: liveDriver, merge: mergeParked, wantKind: shedrun.StartNotParkedKind, wantIn: []string{"has not parked yet"}},
		{name: "blocked parked driver is woken", runState: shedengine.StateBlocked, directory: liveDriver, marker: true, wantOK: true, wantSent: true, wantIn: []string{"woken"}},
		{name: "paused parked driver is woken", runState: shedengine.StatePaused, directory: liveDriver, marker: true, wantOK: true, wantSent: true, wantIn: []string{"woken"}},
		{name: "failed parked driver is woken", runState: shedengine.StateFailed, directory: liveDriver, marker: true, wantOK: true, wantSent: true, wantIn: []string{"woken"}},
		{name: "parked driver over an unfinished merge is refused", runState: shedengine.StateBlocked, directory: liveDriver, marker: true, merge: mergeParked, wantKind: shedrun.StartMergeInProgressKind, wantIn: []string{`"lyx loom resume"`, "lyx fabric merge"}},
		{name: "no live driver over an unfinished merge is refused", runState: shedengine.StateBlocked, directory: deadDriver, merge: mergeParked, wantKind: shedrun.StartMergeInProgressKind, wantIn: []string{`"lyx loom resume"`}},
		{name: "dead driver strand names batten then resume, or start", runState: shedengine.StateBlocked, directory: deadDriver, wantIn: []string{"dead", "lyx batten run <slug>", `"lyx loom resume" again`, `"lyx loom start"`}},
		{name: "retiring driver strand names start", runState: shedengine.StateBlocked, directory: retiringDriver, marker: true, wantIn: []string{"retiring", `"lyx loom start"`}},
		{name: "no driver strand names start", runState: shedengine.StateBlocked, directory: noDriver, wantIn: []string{"no driver strand", `"lyx loom start"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &fakeDriverSender{}
			c, starter, probe := newResumeVerbReceiver(t, sender, tt.directory)
			c.midMerge = func(*lyxcwd.Location) (fabricengine.MidMergeState, error) { return tt.merge, nil }
			if tt.runState != "" {
				writeTestRunState(t, c, tt.runState)
			}
			if tt.noScratch {
				absentScratch := filepath.Join(filepath.Dir(c.shedPaths.StatusPath), "absent-scratch")
				c.shedPaths.LockPath = filepath.Join(absentScratch, "run.lock")
				c.shedPaths.StatusLockPath = filepath.Join(absentScratch, "status.json.lock")
			}
			if tt.runLock {
				holdRunLock(t, c)
			}
			marker := ""
			if tt.marker {
				marker = putParkMarker(t, c)
			}

			env, code := runResume(t, c)

			if env.OK != tt.wantOK || (code == 0) != tt.wantOK {
				t.Fatalf("envelope ok = %v, exit code = %d; want ok = %v", env.OK, code, tt.wantOK)
			}
			if env.Kind != tt.wantKind {
				t.Errorf("kind = %q; want %q", env.Kind, tt.wantKind)
			}
			message := env.Message + env.Error
			for _, want := range tt.wantIn {
				if !strings.Contains(message, want) {
					t.Errorf("message %q lacks %q", message, want)
				}
			}
			for _, startRetry := range []string{"retry `lyx loom start`", `re-run "lyx loom start"`, `run "lyx loom start" again`} {
				if strings.Contains(message, startRetry) {
					t.Errorf("message %q names start as the retry of resume (%s)", message, startRetry)
				}
			}
			if tt.wantKind == shedrun.StartMergeInProgressKind && (env.Conflicts == nil || len(*env.Conflicts) == 0) {
				t.Errorf("merge refusal carries no conflicts: %+v", env)
			}
			if starter.called || probe.removeCalled {
				t.Errorf("the verb spawned (%v) or removed (%v) a strand", starter.called, probe.removeCalled)
			}
			if sent := len(sender.texts) > 0; sent != tt.wantSent {
				t.Errorf("resume line typed = %v, want %v", sent, tt.wantSent)
			}
			if tt.wantSent {
				if env.StopReport == "" || !strings.Contains(sender.texts[0], env.StopReport) {
					t.Errorf("stop_report %q is not the path the typed line %q names", env.StopReport, sender.texts)
				}
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Errorf("park marker still on disk after the wake (stat err %v)", err)
				}
			} else if tt.marker {
				if _, err := os.Stat(marker); err != nil {
					t.Errorf("park marker removed by a refused resume: %v", err)
				}
			}
			assertBootstrapLockReleased(t, loomengine.LoomBootstrapLock(c.location))
		})
	}
}

// TestResumeVerb_FailedDeliveryNamesResume asserts a resume line that cannot be delivered is refused naming `lyx loom status` and a re-run of `lyx loom resume`, and leaves the marker.
func TestResumeVerb_FailedDeliveryNamesResume(t *testing.T) {
	t.Parallel()

	sender := &fakeDriverSender{repeatErr: notReady()}
	directory := fakeDriverDirectory{found: true, row: reedengine.DirectoryRow{Name: driverStrandDisplayName, GUID: "g-drv", Live: true}}
	c, _, _ := newResumeVerbReceiver(t, sender, directory)
	c.midMerge = func(*lyxcwd.Location) (fabricengine.MidMergeState, error) {
		return fabricengine.MidMergeState{Kind: fabricengine.MidMergeNone}, nil
	}
	writeTestRunState(t, c, shedengine.StateBlocked)
	marker := putParkMarker(t, c)

	env, code := runResume(t, c)

	if env.OK || code == 0 {
		t.Fatalf("envelope ok = %v, exit code = %d; want a refusal", env.OK, code)
	}
	for _, want := range []string{`"lyx loom status"`, `"lyx loom resume" again`} {
		if !strings.Contains(env.Error, want) {
			t.Errorf("error %q lacks %q", env.Error, want)
		}
	}
	if strings.Contains(env.Error, `"lyx loom start"`) {
		t.Errorf("error %q names start as the retry of resume", env.Error)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("park marker removed by a failed delivery: %v", err)
	}
}

// TestResumeVerb_BlocksOnTheBootstrapLockThenReadsState asserts the verb waits for a held bootstrap lock and reads the run's state only after it is released:
// the status file is written while the lock is held,
// and the verb must report that state, not the absence it would have found reading first.
func TestResumeVerb_BlocksOnTheBootstrapLockThenReadsState(t *testing.T) {
	t.Parallel()

	c, _, _ := newResumeVerbReceiver(t, &fakeDriverSender{}, fakeDriverDirectory{})
	lockPath := loomengine.LoomBootstrapLock(c.location)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	held := acquireTestBootstrapLock(t, lockPath)

	type result struct {
		env  resumeEnvelope
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		env, code, err := invokeResume(c)
		done <- result{env, code, err}
	}()

	writeTestRunState(t, c, shedengine.StateDone)
	select {
	case <-done:
		t.Fatal("resume returned while the bootstrap lock was held")
	default:
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !got.env.OK || got.code != 0 || !strings.Contains(got.env.Message, "done") {
		t.Errorf("resume = %+v, exit %d; want the done no-op read after the release", got.env, got.code)
	}
}

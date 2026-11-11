// caller_test.go drives the `lym adopt` call over fake seams.
// It covers the verb's branches, the here call, the discard and the worker's arguments.

package adoptcli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// callerFakes holds what the fake caller seams were asked.
type callerFakes struct {
	spawned [][]string
	caps    []string
	pulls   int
}

// stepsThrough returns the step records of every adopt step up to and including last.
func stepsThrough(last string) []adoptrecord.StepRecord {
	var steps []adoptrecord.StepRecord
	for _, step := range adoptrecord.Steps {
		steps = append(steps, adoptrecord.StepRecord{Name: step})
		if step == last {
			break
		}
	}
	return steps
}

func TestRunAdopt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const spawnPID = 777
	alphaRequest := request{Slugs: []string{"alpha"}}

	// write replaces the world's adopt record with one of alpha at pid in state.
	write := func(t *testing.T, w *world, pid int, state string) {
		t.Helper()
		record := adoptrecord.Record{
			Request:  alphaRequest.recorded(),
			Subjects: []string{"alpha"},
			PID:      pid,
			Start:    time.Unix(6000, 0),
			State:    state,
			InFlight: adoptrecord.StepOwner,
		}
		switch state {
		case adoptrecord.StateDone:
			record.Envelope = map[string]any{"prime_recorded": true}
		case adoptrecord.StateStopped:
			record.Error, record.WayForward = "the owner write broke", "re-run `lym adopt alpha`"
		}
		if err := adoptrecord.Write(w.dir, record); err != nil {
			t.Fatalf("Write(adopt record) = %v", err)
		}
	}

	// rootRecord is an adopt of root at pid 55 that ended the steps through last and stands in state.
	rootRecord := func(state, last string) adoptrecord.Record {
		return adoptrecord.Record{
			Request: adoptrecord.Request{Root: true}, Subjects: []string{"root"}, PID: 55, Start: time.Unix(6000, 0),
			State: state, Steps: stepsThrough(last), InFlight: adoptrecord.StepDeploy,
		}
	}
	// wantSpawn checks that the one spawn ran the worker with args.
	wantSpawn := func(args ...string) func(t *testing.T, fakes *callerFakes) {
		return func(t *testing.T, fakes *callerFakes) {
			want := append([]string{"adopt", "worker"}, args...)
			if len(fakes.spawned) != 1 || !slices.Equal(fakes.spawned[0], want) {
				t.Errorf("spawned %v, want one spawn of %v", fakes.spawned, want)
			}
		}
	}

	tests := []struct {
		name    string
		strand  string
		req     request
		discard bool
		// here passes --here from the worktree named worktree; pullSkip is the detail of a Bolt pull that could not run.
		here      bool
		worktree  string
		pullSkip  string
		pullErr   error
		wantPulls int
		// wantNoRecord expects the call to leave no adopt record.
		wantNoRecord bool
		setup        func(t *testing.T, w *world)
		// afterSpawn stands for the worker the fake spawn starts; afterSleep acts at each wake of the follow, counting from one.
		afterSpawn func(t *testing.T, w *world)
		afterSleep func(t *testing.T, w *world, wake int)
		wantExit   int
		wantSpawns int
		wantOut    []string
		check      func(t *testing.T, fakes *callerFakes)
	}{
		{
			name: "DiscardWithASubjectIsRefusedNamingDiscardAlone", req: alphaRequest, discard: true,
			wantExit: 1, wantOut: []string{"lym adopt --discard"},
		},
		{
			name: "HereInAPrimeThisMachineOwnsEndsOwnedHereWithNoWorkerOrRecord", here: true, worktree: "prime",
			wantPulls: 1, wantNoRecord: true, wantOut: []string{`"outcome":"owned_here"`, `"subject":"root"`, `"ok":true`},
		},
		{
			name: "HereOwnedHereBesideAnotherSubjectsUnfinishedRecordEndsOwnedHere", here: true, worktree: "prime",
			setup:     func(t *testing.T, w *world) { write(t, w, 55, adoptrecord.StateStopped) },
			wantPulls: 1, wantOut: []string{`"outcome":"owned_here"`},
		},
		{
			name: "HereReadsTheLocalBoltWhenThePullCannotReachOrigin", here: true, worktree: "prime", pullSkip: "fetching origin failed",
			wantPulls: 1, wantOut: []string{`"outcome":"owned_here"`, "fetching origin failed"},
		},
		{
			name: "HereInAForeignOwnedPrimeAdoptsRoot", here: true, worktree: "prime",
			setup:      func(t *testing.T, w *world) { w.owners.Prime = there },
			afterSpawn: func(t *testing.T, w *world) { write(t, w, spawnPID, adoptrecord.StateDone) },
			wantPulls:  1, wantSpawns: 1, wantOut: []string{`"ok":true`}, check: wantSpawn("root"),
		},
		{
			name: "HereInAForeignOwnedBeamAdoptsTheBeam", here: true, worktree: "alpha",
			afterSpawn: func(t *testing.T, w *world) { write(t, w, spawnPID, adoptrecord.StateDone) },
			wantPulls:  1, wantSpawns: 1, wantOut: []string{`"ok":true`}, check: wantSpawn("alpha"),
		},
		{
			name: "HereAfterAWorkerKilledAtDeployResumesTheRecordWithoutAPull", here: true, worktree: "prime",
			setup: func(t *testing.T, w *world) {
				writeAdoptRecord(t, w.dir, rootRecord(adoptrecord.StateRunning, adoptrecord.StepOwner))
			},
			afterSpawn: func(t *testing.T, w *world) { write(t, w, spawnPID, adoptrecord.StateDone) },
			wantSpawns: 1, wantOut: []string{`"ok":true`}, check: wantSpawn("root"),
		},
		{
			name: "HereInABeamWithNoOwnerOfItsOwnFollowsTheRunningRootAdopt", here: true, worktree: "gamma",
			setup: func(t *testing.T, w *world) {
				w.owners.Entries["gamma"] = boardengine.OwnerEntry{IsProject: true, Beam: true}
				w.workerAlive = true
				writeAdoptRecord(t, w.dir, rootRecord(adoptrecord.StateRunning, adoptrecord.StepPause))
			},
			afterSleep: func(t *testing.T, w *world, _ int) { write(t, w, 55, adoptrecord.StateDone) },
			wantOut:    []string{`"ok":true`},
		},
		{
			name: "HereOnASubjectPastCheckBesideAHeldBackProjectEndsOwnedHere", here: true, worktree: "prime",
			setup: func(t *testing.T, w *world) {
				record := rootRecord(adoptrecord.StateStopped, adoptrecord.StepCheck)
				record.Request.Slugs, record.Subjects = []string{"alpha"}, []string{"alpha", "root"}
				record.InFlight, record.Inputs = adoptrecord.StepDone, adoptrecord.Inputs{PrimeRecorded: true, HeldBack: []string{"alpha"}}
				writeAdoptRecord(t, w.dir, record)
			},
			wantPulls: 1, wantOut: []string{`"outcome":"owned_here"`},
		},
		{
			name: "HereInATaskWorktreeDoesNothing", here: true, worktree: "idle",
			wantNoRecord: true, wantOut: []string{`"outcome":"no_subject"`, `"ok":true`},
		},
		{
			name: "HereWithASubjectIsRefused", here: true, worktree: "prime", req: alphaRequest,
			wantExit: 1, wantOut: []string{"--here"},
		},
		{
			name: "HereWithRootIsRefused", here: true, worktree: "prime", req: request{Root: true},
			wantExit: 1, wantOut: []string{"--here"},
		},
		{
			name: "HereWithAllIsRefused", here: true, worktree: "prime", req: request{All: true},
			wantExit: 1, wantOut: []string{"--here"},
		},
		{
			name: "HereWithDiscardIsRefusedNamingDiscardAlone", here: true, discard: true, worktree: "prime",
			wantExit: 1, wantOut: []string{"lym adopt --discard"},
		},
		{
			name: "HereWhenThePullFailsIsRefusedWithNoWorkerOrRecord", here: true, worktree: "prime", pullErr: errors.New("the board clone is broken"),
			wantPulls: 1, wantNoRecord: true, wantExit: 1, wantOut: []string{"pull Bolt", "the board clone is broken"},
		},
		{
			name: "HereResumingARecordCarriesTheCallsNoOrchToTheWorker", here: true, worktree: "prime", req: request{NoOrch: true},
			setup: func(t *testing.T, w *world) {
				writeAdoptRecord(t, w.dir, rootRecord(adoptrecord.StateRunning, adoptrecord.StepOwner))
			},
			afterSpawn: func(t *testing.T, w *world) { write(t, w, spawnPID, adoptrecord.StateDone) },
			wantSpawns: 1, wantOut: []string{`"ok":true`}, check: wantSpawn("root", "--no-orch"),
		},
		{
			name: "LiveRecordOfTheSameSubjectsIsFollowedWithoutSpawning", req: alphaRequest,
			setup:      func(t *testing.T, w *world) { w.workerAlive = true; write(t, w, 55, adoptrecord.StateRunning) },
			afterSleep: func(t *testing.T, w *world, _ int) { write(t, w, 55, adoptrecord.StateDone) },
			wantOut:    []string{`"prime_recorded":true`, `"ok":true`},
		},
		{
			name: "DoneEndsWithExitZeroAndTheEnvelope", req: alphaRequest,
			afterSpawn: func(t *testing.T, w *world) { write(t, w, spawnPID, adoptrecord.StateDone) },
			wantSpawns: 1, wantOut: []string{`"prime_recorded":true`, `"ok":true`},
		},
		{
			name: "StoppedEndsNonZeroNamingTheStopAndItsWayForward", req: alphaRequest,
			afterSpawn: func(t *testing.T, w *world) { write(t, w, spawnPID, adoptrecord.StateStopped) },
			wantExit:   1, wantSpawns: 1, wantOut: []string{"the owner write broke", "re-run `lym adopt alpha`", `"ok":false`},
		},
		{
			name: "InterruptedEndsNonZeroNamingTheRerunAndDiscard", req: alphaRequest,
			afterSpawn: func(t *testing.T, w *world) { write(t, w, spawnPID, adoptrecord.StateRunning) },
			wantExit:   1, wantSpawns: 1, wantOut: []string{"lym adopt alpha", "lym adopt --discard"},
		},
		{
			name: "UnfinishedRecordOfOtherSubjectsIsRefusedWithoutSpawning", req: request{Slugs: []string{"beta"}},
			setup:    func(t *testing.T, w *world) { write(t, w, 55, adoptrecord.StateStopped) },
			wantExit: 1, wantOut: []string{"alpha", "lym adopt alpha", "lym adopt --discard"},
		},
		{
			name: "OrchStrandSpawnsTheWorkerWithNoOrch", req: alphaRequest, strand: "kn:orch",
			afterSpawn: func(t *testing.T, w *world) { write(t, w, spawnPID, adoptrecord.StateDone) },
			wantSpawns: 1, wantOut: []string{`"ok":true`},
			check: func(t *testing.T, fakes *callerFakes) { requireNoOrchArg(t, fakes, true) },
		},
		{
			name: "NumberedOrchStrandSpawnsTheWorkerWithNoOrch", req: alphaRequest, strand: "kn:orch-2",
			afterSpawn: func(t *testing.T, w *world) { write(t, w, spawnPID, adoptrecord.StateDone) },
			wantSpawns: 1, wantOut: []string{`"ok":true`},
			check: func(t *testing.T, fakes *callerFakes) { requireNoOrchArg(t, fakes, true) },
		},
		{
			name: "ImplementerStrandSpawnsTheWorkerWithoutNoOrch", req: alphaRequest, strand: "kn:implementer-1",
			afterSpawn: func(t *testing.T, w *world) { write(t, w, spawnPID, adoptrecord.StateDone) },
			wantSpawns: 1, wantOut: []string{`"ok":true`},
			check: func(t *testing.T, fakes *callerFakes) { requireNoOrchArg(t, fakes, false) },
		},
		{
			name: "EmptyStrandSpawnsTheWorkerWithoutNoOrch", req: alphaRequest,
			afterSpawn: func(t *testing.T, w *world) { write(t, w, spawnPID, adoptrecord.StateDone) },
			wantSpawns: 1, wantOut: []string{`"ok":true`},
			check: func(t *testing.T, fakes *callerFakes) { requireNoOrchArg(t, fakes, false) },
		},
		{
			name: "UnparsableStrandSpawnsTheWorkerWithoutNoOrch", req: alphaRequest, strand: "not a name",
			afterSpawn: func(t *testing.T, w *world) { write(t, w, spawnPID, adoptrecord.StateDone) },
			wantSpawns: 1, wantOut: []string{`"ok":true`},
			check: func(t *testing.T, fakes *callerFakes) { requireNoOrchArg(t, fakes, false) },
		},
		{
			name: "EmptyStrandFollowsUncappedAndNeverAsksKeepWarm", req: alphaRequest,
			afterSpawn: func(t *testing.T, w *world) { w.workerAlive = true; write(t, w, spawnPID, adoptrecord.StateRunning) },
			afterSleep: func(t *testing.T, w *world, wake int) {
				if wake == 40 {
					write(t, w, spawnPID, adoptrecord.StateDone)
				}
			},
			wantSpawns: 1, wantOut: []string{`"ok":true`},
			check: func(t *testing.T, fakes *callerFakes) {
				if len(fakes.caps) != 0 {
					t.Errorf("KeepWarm asked for %q, want no ask for an empty strand", fakes.caps)
				}
			},
		},
		{
			name: "AgentStrandFollowsToTheKeepWarmCapAndExitsNonZeroStillRunning", req: alphaRequest, strand: "kn:implementer-1",
			afterSpawn: func(t *testing.T, w *world) { w.workerAlive = true; write(t, w, spawnPID, adoptrecord.StateRunning) },
			wantExit:   1, wantSpawns: 1, wantOut: []string{"still running", "lym beam status"},
			check: func(t *testing.T, fakes *callerFakes) {
				if !slices.Equal(fakes.caps, []string{"kn:implementer-1"}) {
					t.Errorf("KeepWarm asked for %q, want the agent strand once", fakes.caps)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld()
			w.dir = t.TempDir()
			if tt.setup != nil {
				tt.setup(t, w)
			}
			fakes := &callerFakes{}
			now := time.Unix(5000, 0)
			wakes := 0
			deps := callerDeps{
				HubPath:  w.dir,
				Strand:   tt.strand,
				Worktree: tt.worktree,
				PullBolt: func() (string, error) {
					fakes.pulls++
					return tt.pullSkip, tt.pullErr
				},
				Steps:      w.deps(),
				ReadRecord: func() (adoptrecord.Record, bool, error) { return adoptrecord.Read(w.dir) },
				Alive:      func(int, string) bool { return w.workerAlive },
				Now:        func() time.Time { return now },
				Sleep: func(wait time.Duration) {
					now = now.Add(wait)
					wakes++
					if tt.afterSleep != nil {
						tt.afterSleep(t, w, wakes)
					}
				},
				KeepWarm: func(strand string) time.Duration {
					fakes.caps = append(fakes.caps, strand)
					return 3 * time.Second
				},
				Discard: func() (bool, error) {
					t.Errorf("Discard ran, want it only for a bare --discard")
					return false, nil
				},
				Spawn: func(args []string) (int, <-chan struct{}, error) {
					fakes.spawned = append(fakes.spawned, args)
					if tt.afterSpawn != nil {
						tt.afterSpawn(t, w)
					}
					return spawnPID, make(chan struct{}), nil
				},
				Stderr: io.Discard,
			}

			var out bytes.Buffer
			exit := runAdopt(deps, tt.req, callFlags{Discard: tt.discard, Here: tt.here}, &out)

			if (exit != 0) != (tt.wantExit != 0) || len(fakes.spawned) != tt.wantSpawns {
				t.Errorf("runAdopt = exit %d after %d spawns, want exit %d after %d; output %s", exit, len(fakes.spawned), tt.wantExit, tt.wantSpawns, out.String())
			}
			if fakes.pulls != tt.wantPulls {
				t.Errorf("runAdopt pulled Bolt %d times, want %d", fakes.pulls, tt.wantPulls)
			}
			if _, found, err := adoptrecord.Read(w.dir); tt.wantNoRecord && (found || err != nil) {
				t.Errorf("adopt record after the call: found %v, error %v; want none", found, err)
			}
			for _, text := range tt.wantOut {
				if !strings.Contains(out.String(), text) {
					t.Errorf("output = %s, want it to contain %q", out.String(), text)
				}
			}
			if tt.check != nil {
				tt.check(t, fakes)
			}
		})
	}
}

// requireNoOrchArg fails the test unless the one spawn's arguments carry --no-orch exactly when want is set.
func requireNoOrchArg(t *testing.T, fakes *callerFakes, want bool) {
	t.Helper()
	if len(fakes.spawned) != 1 || slices.Contains(fakes.spawned[0], "--no-orch") != want {
		t.Errorf("spawned %v, want one spawn whose arguments carry --no-orch: %v", fakes.spawned, want)
	}
}

func TestDiscard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	unfinished := func(state string) adoptrecord.Record {
		return adoptrecord.Record{Request: adoptrecord.Request{Slugs: []string{"alpha"}}, Subjects: []string{"alpha"}, PID: 1, State: state}
	}
	tests := []struct {
		name string
		// seed writes the record under test into the hub, returning whether the discard should remove it.
		seed        func(t *testing.T, hub string)
		holdLock    bool
		wantRemoved bool
		wantErr     []string
		wantLeft    bool
	}{
		{
			name:        "RemovesAnInterruptedRecord",
			seed:        func(t *testing.T, hub string) { writeAdoptRecord(t, hub, unfinished(adoptrecord.StateRunning)) },
			wantRemoved: true,
		},
		{
			name:        "RemovesAStoppedRecord",
			seed:        func(t *testing.T, hub string) { writeAdoptRecord(t, hub, unfinished(adoptrecord.StateStopped)) },
			wantRemoved: true,
		},
		{
			name: "RemovesARecordThatDoesNotDecode",
			seed: func(t *testing.T, hub string) {
				if err := os.MkdirAll(fabricengine.AdoptScratchDir(hub), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(adoptrecord.Path(hub), []byte("{not json"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantRemoved: true,
		},
		{
			name:     "RefusesWhileAnotherHolderHoldsTheLockAndLeavesTheRecord",
			seed:     func(t *testing.T, hub string) { writeAdoptRecord(t, hub, unfinished(adoptrecord.StateRunning)) },
			holdLock: true, wantErr: []string{"lym beam status"}, wantLeft: true,
		},
		{
			name:     "LeavesADoneRecord",
			seed:     func(t *testing.T, hub string) { writeAdoptRecord(t, hub, unfinished(adoptrecord.StateDone)) },
			wantLeft: true,
		},
		{name: "ReportsNothingRemovedWithNoRecord"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := t.TempDir()
			if tt.seed != nil {
				tt.seed(t, hub)
			}
			if tt.holdLock {
				release, acquired, err := tryAdoptLock(hub)
				if err != nil || !acquired {
					t.Fatalf("tryAdoptLock = %v, %v; want the lock", acquired, err)
				}
				t.Cleanup(func() { _ = release() })
			}

			removed, err := discard(hub)

			for _, text := range tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), text) {
					t.Errorf("discard error = %v, want it to contain %q", err, text)
				}
			}
			if len(tt.wantErr) == 0 && err != nil {
				t.Errorf("discard error = %v, want none", err)
			}
			_, statErr := os.Stat(adoptrecord.Path(hub))
			if removed != tt.wantRemoved || (statErr == nil) != tt.wantLeft {
				t.Errorf("discard removed = %v, record left = %v; want removed %v, left %v", removed, statErr == nil, tt.wantRemoved, tt.wantLeft)
			}
		})
	}
}

// writeAdoptRecord writes record as the adopt record of hub.
func writeAdoptRecord(t *testing.T, hub string, record adoptrecord.Record) {
	t.Helper()
	if err := adoptrecord.Write(hub, record); err != nil {
		t.Fatalf("Write(adopt record) = %v", err)
	}
}

func TestWorkerArgs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		req  adoptrecord.Request
	}{
		{"NamedSlugs", adoptrecord.Request{Slugs: []string{"alpha", "beta"}}},
		{"RootAlone", adoptrecord.Request{Root: true}},
		{"AllWithKeepLocal", adoptrecord.Request{All: true, KeepLocal: true}},
		{"SlugWithDropLocalAndNoOrch", adoptrecord.Request{Slugs: []string{"alpha"}, DropLocal: true, NoOrch: true}},
		{"SlugAndRootWithKeepLocal", adoptrecord.Request{Slugs: []string{"alpha"}, Root: true, KeepLocal: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args := workerArgs(tt.req)
			if len(args) < 2 || args[0] != "adopt" || args[1] != "worker" {
				t.Fatalf("workerArgs(%+v) = %v, want it to open with adopt worker", tt.req, args)
			}
			cmd := workerCmd()
			if err := cmd.ParseFlags(args[2:]); err != nil {
				t.Fatalf("worker flags rejected %v: %v", args[2:], err)
			}
			flag := func(name string) bool {
				value, err := cmd.Flags().GetBool(name)
				if err != nil {
					t.Fatalf("GetBool(%s) = %v", name, err)
				}
				return value
			}
			got := normalizedRequest(request{
				Slugs:     cmd.Flags().Args(),
				All:       flag("all"),
				KeepLocal: flag("keep-local"),
				DropLocal: flag("drop-local"),
				NoOrch:    flag("no-orch"),
			})
			want := normalizedRequest(requestOf(tt.req))
			if got.recorded().Command() != want.recorded().Command() {
				t.Errorf("worker parsed %q from %v, want %q", got.recorded().Command(), args, want.recorded().Command())
			}
		})
	}
}

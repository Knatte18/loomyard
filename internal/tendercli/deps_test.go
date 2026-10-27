//go:build tier0

// deps_test.go pins the readings shuttlePair derives from a run's session state: running turn, and background work left by an ended turn, and the pending reading of tender's orch revive injects.
// It spawns nothing and touches neither reed nor the runner, so it stays out of the integration-tagged cli_test.go.

package tendercli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/tenderengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

// signalEngine is a shuttlefake.Engine that parses session signals: it returns its signals whatever the data.
// Its process always reads alive and no turn reads interrupted, so the state read depends on the signals alone.
type signalEngine struct {
	*shuttlefake.Engine
	signals []shuttleengine.SessionSignal
}

func (e *signalEngine) ParseSessionSignals(data []byte) ([]shuttleengine.SessionSignal, int) {
	return e.signals, len(data)
}

func (e *signalEngine) ProcessLiveness(string) shuttleengine.Liveness {
	return shuttleengine.LivenessAlive
}

func (e *signalEngine) TurnStartInterrupt(shuttleengine.SessionSignal) (time.Time, bool) {
	return time.Time{}, false
}

func TestShuttlePairReadings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		signals        []shuttleengine.SessionSignal
		wantBusy       bool
		wantBackground bool
		wantLabels     []string
	}{
		{
			name:     "busy turn is running and not background",
			signals:  []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalTurnStart}},
			wantBusy: true,
		},
		{
			name: "busy background is not running and lists the tasks",
			signals: []shuttleengine.SessionSignal{
				{Kind: shuttleengine.SessionSignalTurnStart},
				{Kind: shuttleengine.SessionSignalTurnEnd, Outstanding: []shuttleengine.BackgroundTask{{Label: "sleep 600"}, {ID: "bsh2"}}},
			},
			wantBackground: true,
			wantLabels:     []string{"sleep 600", "bsh2"},
		},
		{
			name: "an ended turn is neither",
			signals: []shuttleengine.SessionSignal{
				{Kind: shuttleengine.SessionSignalTurnStart},
				{Kind: shuttleengine.SessionSignalTurnEnd},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
			if err := os.WriteFile(eventsPath, []byte("x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			seedRunning := filepath.Join(runDir, "run-1")
			if err := os.MkdirAll(seedRunning, 0o755); err != nil {
				t.Fatal(err)
			}
			record := fmt.Sprintf(`{"strandGuid":"g-a","sessionId":"s-1","outcome":"running","eventsPath":%q}`, eventsPath)
			if err := os.WriteFile(filepath.Join(seedRunning, "run.json"), []byte(record), 0o644); err != nil {
				t.Fatal(err)
			}
			pair := shuttlePair{
				location: &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "p1"},
				cfg:      shuttleengine.Config{RunDir: runDir},
				engines:  shuttlefake.EngineSet(&signalEngine{Engine: &shuttlefake.Engine{}, signals: tt.signals}),
			}

			busy, err := pair.Busy("g-a")
			if err != nil || busy != tt.wantBusy {
				t.Errorf("Busy = %v, %v; want %v", busy, err, tt.wantBusy)
			}
			labels, background, err := pair.Background("g-a")
			if err != nil || background != tt.wantBackground || !reflect.DeepEqual(labels, tt.wantLabels) {
				t.Errorf("Background = %v, %v, %v; want %v, %v", labels, background, err, tt.wantLabels, tt.wantBackground)
			}
		})
	}
}

func TestRevivePending(t *testing.T) {
	t.Parallel()
	posted := creelengine.LogEntry{Event: "posted", ID: 7}
	outcome := func(kind creelengine.OutcomeKind) creelengine.LogEntry {
		return creelengine.LogEntry{Event: string(kind), ID: 7}
	}
	tests := []struct {
		name    string
		entries []creelengine.LogEntry
		want    bool
	}{
		{"None", nil, false},
		{"PostedWithNoOutcome", []creelengine.LogEntry{posted}, true},
		{"PostedThenInjected", []creelengine.LogEntry{posted, outcome(creelengine.OutcomeInjected)}, false},
		{"PostedThenUnconfirmed", []creelengine.LogEntry{posted, outcome(creelengine.OutcomeUnconfirmed)}, false},
		{"PostedThenExpired", []creelengine.LogEntry{posted, outcome(creelengine.OutcomeExpired)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := revivePending(tt.entries); got != tt.want {
				t.Errorf("revivePending(%+v) = %v; want %v", tt.entries, got, tt.want)
			}
		})
	}
}

func TestPairOwners(t *testing.T) {
	t.Parallel()
	const here = "desktop"
	noParent := func(string) (string, error) { return "", nil }
	// "here" and "mine" are members of the beam this machine owns; every other entry reads the prime's owner through root.
	beamParent := func(slug string) (string, error) {
		if slug == "here" || slug == "mine" {
			return "desk", nil
		}
		return "", nil
	}
	owners := boardengine.Owners{Prime: "laptop", Entries: map[string]boardengine.OwnerEntry{
		"desk":    {Owner: here, IsProject: true, Beam: true},
		"here":    {IsTask: true, RunHeld: true},
		"away":    {IsTask: true, RunHeld: true},
		"remote":  {IsTask: true, RunHeld: true},
		"mine":    {IsTask: true, RunHeld: true},
		"waiting": {IsTask: true},
	}}
	tests := []struct {
		name     string
		owners   boardengine.Owners
		slugs    []string
		parentOf func(string) (string, error)
		want     []tenderengine.PairOwner
		wantErr  bool
	}{
		{
			name:     "LocalPairsAndRunHeldEntriesOwnedElsewhere",
			owners:   owners,
			slugs:    []string{"here", "away"},
			parentOf: beamParent,
			want: []tenderengine.PairOwner{
				{Slug: "here", Owner: here, Local: true, OwnedHere: true},
				{Slug: "away", Owner: "laptop", Local: true},
				{Slug: "remote", Owner: "laptop"},
			},
		},
		{
			name:     "UnownedHubReadsOwnedByNoOtherMachine",
			owners:   boardengine.Owners{Entries: map[string]boardengine.OwnerEntry{"here": {IsTask: true, RunHeld: true}}},
			slugs:    []string{"here"},
			parentOf: noParent,
			want:     []tenderengine.PairOwner{{Slug: "here", Local: true, OwnedHere: true}},
		},
		{
			name:     "ParentOfErrorIsReturned",
			owners:   boardengine.Owners{Prime: "laptop", Entries: map[string]boardengine.OwnerEntry{"child": {IsTask: true}}},
			slugs:    []string{"child"},
			parentOf: func(string) (string, error) { return "", errors.New("origin record unreadable") },
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := pairOwners(tt.owners, tt.slugs, here, tt.parentOf)
			if (err != nil) != tt.wantErr {
				t.Fatalf("pairOwners() error = %v; want error %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("pairOwners() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestSyncBoardIfOwed(t *testing.T) {
	t.Parallel()
	probe := func(owed bool, err error) func() (bool, error) {
		return func() (bool, error) { return owed, err }
	}
	probeFailure := errors.New("probe failed")
	tests := []struct {
		name        string
		ahead       func() (bool, error)
		uncommitted func() (bool, error)
		wantSpawns  int
		wantErr     error
	}{
		{"Ahead_Spawns", probe(true, nil), probe(false, nil), 1, nil},
		{"LevelWithUncommittedChanges_Spawns", probe(false, nil), probe(true, nil), 1, nil},
		{"LevelAndClean_SpawnsNothing", probe(false, nil), probe(false, nil), 0, nil},
		{"AheadProbeError_ReturnedWithNothingSpawned", probe(false, probeFailure), probe(true, nil), 0, probeFailure},
		{"UncommittedProbeError_ReturnedWithNothingSpawned", probe(false, nil), probe(false, probeFailure), 0, probeFailure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spawns := 0
			err := syncBoardIfOwed(tt.ahead, tt.uncommitted, func() error {
				spawns++
				return nil
			})
			if !errors.Is(err, tt.wantErr) || spawns != tt.wantSpawns {
				t.Errorf("syncBoardIfOwed() = %v with %d spawns; want %v with %d", err, spawns, tt.wantErr, tt.wantSpawns)
			}
		})
	}
}

func TestOwnedElsewhereEnder(t *testing.T) {
	t.Parallel()
	failure := errors.New("seam failed")
	survivors := &runproc.SurvivorsError{Survivors: []runproc.Ended{{PID: 7, Role: runproc.RoleDriver}}}
	webster := shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Webster"}
	finalizeRunning := shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Finalize"}
	finalizeFailed := shedengine.Status{State: shedengine.StateFailed, CurrentProducer: "Finalize", Transient: "git-transport"}
	tests := []struct {
		name        string
		status      shedengine.Status
		noStatus    bool
		statusErr   error
		restepped   bool
		restepErr   error
		parentHolds bool
		parentErr   error
		endErr      error
		wantEnds    int
		wantErr     error
	}{
		{name: "StatusOffFinalize_Ends", status: webster, wantEnds: 1},
		{name: "FinalizeRunningWithParentHoldingTheWork_EndsNothing", status: finalizeRunning, parentHolds: true},
		{name: "FinalizeRunningWithParentLackingTheWork_Ends", status: finalizeRunning, wantEnds: 1},
		{name: "FailedTransientWithoutRestepAndHolding_EndsNothing", status: finalizeFailed, parentHolds: true},
		{name: "FailedTransientWithRestep_Ends", status: finalizeFailed, restepped: true, parentHolds: true, wantEnds: 1},
		{name: "StatusReadError_EndsNothing", statusErr: failure, wantErr: failure},
		{name: "RestepReadError_EndsNothing", status: finalizeFailed, restepErr: failure, wantErr: failure},
		{name: "ParentReadError_EndsNothing", status: finalizeRunning, parentErr: failure, wantErr: failure},
		{name: "NoStatusFileOrWorktree_Ends", noStatus: true, wantEnds: 1},
		{name: "SurvivorsError_ReturnedWithTheWayForward", status: webster, endErr: survivors, wantEnds: 1, wantErr: survivors},
		{name: "EndError_ReturnedWithTheWayForward", status: webster, endErr: failure, wantEnds: 1, wantErr: failure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ends := 0
			ender := ownedElsewhereEnder{
				slug: "task",
				readStatus: func() (shedengine.Status, bool, error) {
					return tt.status, !tt.noStatus && tt.statusErr == nil, tt.statusErr
				},
				restepped:   func(shedengine.Status) (bool, error) { return tt.restepped, tt.restepErr },
				parentHolds: func() (bool, error) { return tt.parentHolds, tt.parentErr },
				end: func() ([]runproc.Ended, error) {
					ends++
					return nil, tt.endErr
				},
			}

			_, err := ender.run()

			if ends != tt.wantEnds || !errors.Is(err, tt.wantErr) {
				t.Errorf("run() = %v after %d ends; want %v after %d", err, ends, tt.wantErr, tt.wantEnds)
			}
			var survivorsError *runproc.SurvivorsError
			if tt.endErr == survivors && !errors.As(err, &survivorsError) {
				t.Errorf("run() = %v; want it to match *runproc.SurvivorsError", err)
			}
			if tt.endErr != nil {
				for _, step := range []string{"`lym fabric remove task`", "`lym tender reap`"} {
					if !strings.Contains(err.Error(), step) {
						t.Errorf("run() = %q; want it to name %s", err, step)
					}
				}
			}
		})
	}
}

func TestReviveError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		report     tenderengine.ReviveReport
		wantErr    bool
		wantNaming string
	}{
		{name: "SessionSent", report: tenderengine.ReviveReport{Sent: []string{"driver"}, Refused: []tenderengine.SessionNote{{Strand: "other", Slug: "task", Reason: "draft in the box"}}}},
		{name: "OnlyARefusal", report: tenderengine.ReviveReport{Refused: []tenderengine.SessionNote{{Strand: "driver", Slug: "task", Reason: "draft in the box"}}}, wantErr: true, wantNaming: "draft in the box"},
		{name: "OnlyALineThatDidNotLand", report: tenderengine.ReviveReport{NotLanded: []tenderengine.SessionNote{{Strand: "driver", Slug: "task", Reason: "submission not landed"}}}, wantErr: true, wantNaming: "submission not landed"},
		{name: "EmptyReport", report: tenderengine.ReviveReport{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := reviveError(tt.report)
			if (err != nil) != tt.wantErr {
				t.Fatalf("reviveError() = %v; want error %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.wantNaming) {
				t.Errorf("reviveError() = %q; want it to name %q", err, tt.wantNaming)
			}
		})
	}
}

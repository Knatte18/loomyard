// deps_test.go pins the readings shuttlePair derives from a run's session state: running turn, and background work left by an ended turn, and the pending reading of tender's orch revive injects.
// It spawns nothing and touches neither reed nor the runner, so it stays out of the integration-tagged cli_test.go.

package tendercli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
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
				location: &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "p1"},
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
	owners := boardengine.Owners{Prime: "laptop", Entries: map[string]boardengine.OwnerEntry{
		"here":    {Owner: here, IsTask: true, RunHeld: true},
		"away":    {Owner: "laptop", IsTask: true, RunHeld: true},
		"remote":  {Owner: "laptop", IsTask: true, RunHeld: true},
		"mine":    {Owner: here, IsTask: true, RunHeld: true},
		"waiting": {Owner: "laptop", IsTask: true},
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
			parentOf: noParent,
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

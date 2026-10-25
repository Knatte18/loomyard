//go:build tier0

// continue_pass_test.go pins the continue pass dobby schedules into a run it already drives: what lets it through and skips, and the conditions that refuse it.

package battencli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestBattenPreRun_ContinuePass runs the pre-run under --continue-pass over a hand-populated receiver and a temp dobby state, with a pause covering the slug:
// a continue pass of a running Run-Shed run the state names runs no reconcile, no reopening and no pause refusal;
// a run at Land, a run blocked at Run-Shed, a run with no status, a run the state does not name, a run with a full pass owed and a run with an origin-owed marker each refuse naming `lyx batten run <slug>`, with the continue-refused exit code.
func TestBattenPreRun_ContinuePass(t *testing.T) {
	t.Parallel()

	const slug = "some-slug"
	runShed := func(state shedengine.State, producer string) *shedengine.Status {
		return &shedengine.Status{
			CurrentProducer: producer,
			State:           state,
			History:         []shedengine.HistoryEntry{{Producer: battenrecipe.NameOpenWindow, Outcome: shedengine.Done}},
		}
	}
	driven := map[string]dobbyengine.RunState{slug: {}}
	tests := []struct {
		name        string
		status      *shedengine.Status
		dobbyRuns   map[string]dobbyengine.RunState
		marker      bool
		wantRefusal string
	}{
		{name: "a running Run-Shed run the state names passes", status: runShed(shedengine.StateRunning, battenrecipe.NameRunShed), dobbyRuns: driven},
		{name: "a run at Land refuses", status: runShed(shedengine.StateRunning, battenrecipe.NameLand), dobbyRuns: driven, wantRefusal: "not running at"},
		{name: "a run blocked at Run-Shed refuses", status: runShed(shedengine.StateBlocked, battenrecipe.NameRunShed), dobbyRuns: driven, wantRefusal: "not running at"},
		{name: "a run with no status refuses", dobbyRuns: driven, wantRefusal: "no status file"},
		{name: "a run the state does not name refuses", status: runShed(shedengine.StateRunning, battenrecipe.NameRunShed), dobbyRuns: map[string]dobbyengine.RunState{"other-slug": {}}, wantRefusal: "does not name it"},
		{name: "a run with no state record refuses", status: runShed(shedengine.StateRunning, battenrecipe.NameRunShed), wantRefusal: "does not name it"},
		{name: "a run with a full pass owed refuses", status: runShed(shedengine.StateRunning, battenrecipe.NameRunShed), dobbyRuns: map[string]dobbyengine.RunState{slug: {FullPassOwed: true}}, wantRefusal: "owes it a full pass"},
		{name: "a run with an origin-owed marker refuses", status: runShed(shedengine.StateRunning, battenrecipe.NameRunShed), dobbyRuns: driven, marker: true, wantRefusal: "without origin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newFakeReceiver(t, nil)
			if err := os.MkdirAll(filepath.Join(c.location.WorktreePath(), ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			c.continuePass = true

			geometry, err := hubgeom.DobbyGeometry(c.location, c.location.WorktreeName)
			if err != nil {
				t.Fatalf("hubgeom.DobbyGeometry: %v", err)
			}
			if tt.dobbyRuns != nil {
				if err := dobbyengine.UpdateState(geometry.StatePath, func(state *dobbyengine.State) error {
					state.Runs = tt.dobbyRuns
					return nil
				}); err != nil {
					t.Fatalf("dobbyengine.UpdateState: %v", err)
				}
			}
			pauseGeometry, err := hubgeom.PauseGeometry(c.location)
			if err != nil {
				t.Fatalf("hubgeom.PauseGeometry: %v", err)
			}
			now := time.Now()
			if _, err := pauserecord.Update(pauseGeometry.RecordPath, now, func(record *pauserecord.Record) error {
				record.Live = append(record.Live, pauserecord.Entry{Scope: slug, PausedAt: now, Caller: "EXTERNAL"})
				return nil
			}); err != nil {
				t.Fatalf("pauserecord.Update: %v", err)
			}
			if tt.marker {
				marker := shedrun.OriginOwedMarker(c.location, slug)
				if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(marker, []byte("{}"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.status != nil {
				writeStatus(t, c, *tt.status)
			}

			reconciles := 0
			c.reconcile = func(string) (fabricengine.OriginReport, error) {
				reconciles++
				return fabricengine.OriginReport{}, nil
			}
			var commands []string
			c.env.Setting = func(module, key string) (any, error) { return true, nil }
			c.env.ExecutableSetting = func(module, key string) (any, error) { return "open-window {slug}", nil }
			c.env.Command = shedadapters.CommandDeps{
				Slug:    c.slug,
				Dir:     t.TempDir(),
				LogPath: func(row string) string { return row + ".log" },
				Run: func(ctx context.Context, dir, command, logPath string, timeout time.Duration) (shedadapters.CommandResult, error) {
					commands = append(commands, command)
					return shedadapters.CommandResult{}, nil
				},
				GOOS: "linux",
			}
			c.clientAttached = func() (bool, error) { return false, nil }

			err = c.battenPreRun(context.Background())

			if tt.wantRefusal == "" {
				if err != nil {
					t.Fatalf("battenPreRun() = %v; want nil", err)
				}
				if reconciles != 0 || len(commands) != 0 {
					t.Errorf("battenPreRun() ran the reconcile %d times and commands %q; want neither", reconciles, commands)
				}
				return
			}
			var refusal *continueRefusal
			if !errors.As(err, &refusal) {
				t.Fatalf("battenPreRun() = %v; want a continue refusal", err)
			}
			for _, want := range []string{tt.wantRefusal, "lyx batten run " + slug} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("battenPreRun() error = %q; want it to carry %q", err.Error(), want)
				}
			}
			if got := passExitCode(err); got != dobbyengine.PassExitContinueRefused {
				t.Errorf("passExitCode(%v) = %d; want %d", err, got, dobbyengine.PassExitContinueRefused)
			}
			if reconciles != 0 || len(commands) != 0 {
				t.Errorf("a refused continue pass ran the reconcile %d times and commands %q; want neither", reconciles, commands)
			}
		})
	}
}

// weaver_test.go covers the implementer guard both implementer rows run, and the lazy Weaver producer.

package loomshed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// plantWebsterBegun writes a Webster state file holding one begun batch under anchorPath.
func plantWebsterBegun(t *testing.T, anchorPath string) {
	t.Helper()
	st := &websterengine.State{State: runplan.State{Batches: map[int]*runplan.BatchState{1: {Slug: "first"}}}}
	if err := websterengine.SaveState(websterengine.Dir(anchorPath), websterengine.ScratchDir(anchorPath), st); err != nil {
		t.Fatal(err)
	}
}

// plantWeaverBegun writes a Weaver state file holding one begun batch under anchorPath.
func plantWeaverBegun(t *testing.T, anchorPath string) {
	t.Helper()
	st := &weaverengine.State{State: runplan.State{Batches: map[int]*runplan.BatchState{1: {Slug: "first"}}}}
	if err := weaverengine.SaveState(weaverengine.Dir(anchorPath), weaverengine.ScratchDir(anchorPath), st); err != nil {
		t.Fatal(err)
	}
}

// archiveWebsterRecord archives Webster's record in place, as `lym webster reset --to start` leaves it.
func archiveWebsterRecord(t *testing.T, anchorPath string) {
	t.Helper()
	geom := websterengine.Geometry{
		WebsterDir:   websterengine.Dir(anchorPath),
		ReportsDir:   websterengine.ReportsDir(anchorPath),
		PromptsDir:   websterengine.PromptsDir(anchorPath),
		ScratchDir:   websterengine.ScratchDir(anchorPath),
		AnchorRoot:   anchorPath,
		WorktreeRoot: anchorPath,
	}
	if _, err := websterengine.ArchiveRunAfterReset(nil, geom, nil); err != nil {
		t.Fatal(err)
	}
}

// archiveWeaverRecord moves Weaver's record out of its run directory, as `lym weaver reset` leaves it.
func archiveWeaverRecord(t *testing.T, anchorPath string) {
	t.Helper()
	geom := weaverengine.Geometry{RunDir: weaverengine.Dir(anchorPath), ScratchDir: weaverengine.ScratchDir(anchorPath)}
	if err := weaverengine.ArchiveRunRecord(geom, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

// plantCorruptState writes unparsable bytes as the state file in runDir.
func plantCorruptState(t *testing.T, runDir string) {
	t.Helper()
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "state.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImplementerGuard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		this       weaverengine.Implementer
		setting    string
		plant      func(t *testing.T, anchorPath string)
		zeroAnchor bool
		wantErr    bool
		// wantRefusal lists the fragments a refusal holds; empty means the row proceeds.
		wantRefusal []string
	}{
		{
			name: "WeaverRowRefusesOverWebstersBegunBatches", this: weaverengine.ImplementerWeaver, setting: "weaver",
			plant:       plantWebsterBegun,
			wantRefusal: []string{"`lym webster reset --to start`", "`lym config tapestry --set implementer=webster`"},
		},
		{
			name: "WeaverRowProceedsOnceWebstersRecordIsArchived", this: weaverengine.ImplementerWeaver, setting: "weaver",
			plant: func(t *testing.T, anchorPath string) {
				plantWebsterBegun(t, anchorPath)
				archiveWebsterRecord(t, anchorPath)
			},
		},
		{
			name: "WebsterRowRefusesOverWeaversBegunBatches", this: weaverengine.ImplementerWebster, setting: "webster",
			plant:       plantWeaverBegun,
			wantRefusal: []string{"`lym weaver reset`", "`lym config tapestry --set implementer=weaver`"},
		},
		{
			name: "WebsterRowProceedsOnceWeaversRecordIsArchived", this: weaverengine.ImplementerWebster, setting: "webster",
			plant: func(t *testing.T, anchorPath string) {
				plantWeaverBegun(t, anchorPath)
				archiveWeaverRecord(t, anchorPath)
			},
		},
		{
			name: "WebsterRowProceedsOverItsOwnBegunBatches", this: weaverengine.ImplementerWebster, setting: "webster",
			plant: plantWebsterBegun,
		},
		{
			name: "WeaverRowRefusesWebstersCorruptState", this: weaverengine.ImplementerWeaver, setting: "weaver",
			plant: func(t *testing.T, anchorPath string) {
				plantCorruptState(t, websterengine.Dir(anchorPath))
			},
			wantRefusal: []string{"way forward: repair", "state.json"},
		},
		{
			name: "WebsterRowRefusesWeaversCorruptState", this: weaverengine.ImplementerWebster, setting: "webster",
			plant: func(t *testing.T, anchorPath string) {
				plantCorruptState(t, weaverengine.Dir(anchorPath))
			},
			wantRefusal: []string{"`lym weaver reset`"},
		},
		{
			name: "BothImplementersBegunRefuses", this: weaverengine.ImplementerWeaver, setting: "weaver",
			plant: func(t *testing.T, anchorPath string) {
				plantWebsterBegun(t, anchorPath)
				plantWeaverBegun(t, anchorPath)
			},
			wantRefusal: []string{"`lym webster reset --to start`", "`lym weaver reset`"},
		},
		{
			name: "NeitherBegunProceeds", this: weaverengine.ImplementerWeaver, setting: "weaver",
		},
		{
			name: "ZeroAnchorIsAReturnedError", this: weaverengine.ImplementerWebster, setting: "webster",
			zeroAnchor: true, wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchorPath := t.TempDir()
			if tt.plant != nil {
				tt.plant(t, anchorPath)
			}
			active := weaverengine.ActiveDeps{
				AnchorRoot: anchorPath,
				Setting:    func() (string, error) { return tt.setting, nil },
			}
			if tt.zeroAnchor {
				active.AnchorRoot = ""
			}

			reason, refused, err := implementerGuard(tt.this, active)
			if (err != nil) != tt.wantErr {
				t.Fatalf("implementerGuard() error = %v; want error %v", err, tt.wantErr)
			}
			if refused != (len(tt.wantRefusal) > 0) {
				t.Fatalf("implementerGuard() refused = %v (%q); want %v", refused, reason, len(tt.wantRefusal) > 0)
			}
			for _, fragment := range tt.wantRefusal {
				if !strings.Contains(reason, fragment) {
					t.Errorf("implementerGuard() reason = %q; want it to contain %q", reason, fragment)
				}
			}
		})
	}
}

// fakeWeaverRun is a shedadapters.WeaverRunner fake recording the RunDeps it was called with.
type fakeWeaverRun struct {
	receivedDeps []weaverengine.RunDeps
}

func (f *fakeWeaverRun) run(deps weaverengine.RunDeps) (weaverengine.RunResult, error) {
	f.receivedDeps = append(f.receivedDeps, deps)
	return weaverengine.RunResult{Outcome: "done"}, nil
}

func TestWeaverProducer_Call(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("GuardRefusalIsStuckAndSpawnsNothing", func(t *testing.T) {
		t.Parallel()
		anchorPath := t.TempDir()
		plantWebsterBegun(t, anchorPath)

		fake := &fakeWeaverRun{}
		active := weaverengine.ActiveDeps{AnchorRoot: anchorPath}
		p := NewWeaverProducer("Weaver", configengine.Dirs{Anchor: anchorPath}, fake.run, weaverengine.RunDeps{}, active)
		pointer := shedfake.RequireOutcome(t, p, shedengine.Stuck)
		if !strings.Contains(pointer.Reason, "`lym webster reset --to start`") {
			t.Errorf("Call() Reason = %q; want it to name `lym webster reset --to start`", pointer.Reason)
		}
		if len(fake.receivedDeps) != 0 {
			t.Errorf("fake runner was called %d time(s); want 0", len(fake.receivedDeps))
		}
	})

	t.Run("ProceedingRunReachesRunnerWithTheResolvedBatcher", func(t *testing.T) {
		t.Parallel()
		anchorPath := t.TempDir()

		fake := &fakeWeaverRun{}
		active := weaverengine.ActiveDeps{AnchorRoot: anchorPath}
		p := NewWeaverProducer("Weaver", configengine.Dirs{Anchor: anchorPath}, fake.run, weaverengine.RunDeps{}, active)
		shedfake.RequireOutcome(t, p, shedengine.Done)
		if len(fake.receivedDeps) != 1 {
			t.Fatalf("fake runner was called %d time(s); want 1", len(fake.receivedDeps))
		}
		if fake.receivedDeps[0].Batcher == nil || fake.receivedDeps[0].Batcher.Name() != batcher.DefaultName {
			t.Errorf("run() received Batcher = %v; want the resolved %q batchifier", fake.receivedDeps[0].Batcher, batcher.DefaultName)
		}
	})

	t.Run("BrokenBatcherConfigIsStuckAndSpawnsNothing", func(t *testing.T) {
		t.Parallel()
		anchorPath := t.TempDir()

		fake := &fakeWeaverRun{}
		active := weaverengine.ActiveDeps{AnchorRoot: anchorPath}
		deps := weaverengine.RunDeps{Config: weaverengine.Config{BatchProfile: "no-such-batcher"}}
		p := NewWeaverProducer("Weaver", configengine.Dirs{Anchor: anchorPath}, fake.run, deps, active)
		pointer := shedfake.RequireOutcome(t, p, shedengine.Stuck)
		if !strings.HasPrefix(pointer.Reason, batchifierReasonPrefix) {
			t.Errorf("Call() Reason = %q; want the batchifier prefix", pointer.Reason)
		}
		if len(fake.receivedDeps) != 0 {
			t.Errorf("fake runner was called %d time(s); want 0", len(fake.receivedDeps))
		}
	})

	t.Run("CancelledContextReturnsErrorNotVerdict", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		anchorPath := t.TempDir()
		p := NewWeaverProducer("Weaver", configengine.Dirs{Anchor: anchorPath}, (&fakeWeaverRun{}).run, weaverengine.RunDeps{}, weaverengine.ActiveDeps{AnchorRoot: anchorPath})
		outcome, _, err := p.Call(ctx)
		if err == nil || outcome == shedengine.Done || outcome == shedengine.Stuck {
			t.Errorf("Call(cancelled) = (%q, %v); want an error and no verdict", outcome, err)
		}
	})
}

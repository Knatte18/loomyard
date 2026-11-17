// state_test.go covers Weaver's state file: its round-trip, its tolerance of unknown keys and its loud refusals.

package weaverengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

func TestState(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	newDirs := func(t *testing.T) (runDir, scratchDir string) {
		t.Helper()
		anchor := t.TempDir()
		return weaverengine.Dir(anchor), weaverengine.ScratchDir(anchor)
	}

	t.Run("a save and load round-trip keeps every field", func(t *testing.T) {
		t.Parallel()
		runDir, scratchDir := newDirs(t)
		want := &weaverengine.State{
			RunGUID:           "guid",
			PlanRecordsCommit: "records",
			State: runplan.State{
				PlanFingerprint: "fp",
				Batches:         map[int]*runplan.BatchState{1: {Slug: "one", Terminal: true, Status: "done"}},
			},
			Attempts: []*weaverengine.AttemptState{{
				Batch: 1, Number: 2, Rung: 3, ReportPath: "report", StrandGUID: "sg", StrandName: "sn",
				InFlight: true, Outcome: "lost", Cause: "cause", Uncommitted: []string{"a"},
				Delivered: []string{"## D1"}, BeganAt: "2026-01-01T00:00:00Z", Reentry: true,
			}},
			BatchEnds: map[int]string{1: "end"},
			LastEnd:   "end",
			Fixers: []*weaverengine.FixerState{{
				Number: 1, ReplyPath: "fix-1.md", Rung: 2, Record: &gatefixer.Record{StrandGUID: "fg"},
				StrandName: "fn", Outcome: "done", Findings: "findings", Delivered: []string{"## D2"}, Reentry: true,
			}},
			Stop: &weaverengine.StopRecord{Kind: "batch", Batch: 1, Rung: 2, Reentered: true},
		}
		if err := weaverengine.SaveState(runDir, scratchDir, want); err != nil {
			t.Fatalf("SaveState() error = %v", err)
		}
		got, err := weaverengine.LoadState(runDir, scratchDir)
		if err != nil {
			t.Fatalf("LoadState() error = %v", err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("LoadState() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("a missing file loads nil", func(t *testing.T) {
		t.Parallel()
		runDir, scratchDir := newDirs(t)
		if got, err := weaverengine.LoadState(runDir, scratchDir); got != nil || err != nil {
			t.Errorf("LoadState() = (%v, %v); want (nil, nil)", got, err)
		}
	})

	t.Run("a file with an unknown key loads", func(t *testing.T) {
		t.Parallel()
		runDir, scratchDir := newDirs(t)
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runDir, "state.json"), []byte(`{"runGuid":"g","fromTheFuture":1}`), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := weaverengine.LoadState(runDir, scratchDir)
		if err != nil || got == nil || got.RunGUID != "g" {
			t.Errorf("LoadState() = (%+v, %v); want the state with RunGUID g", got, err)
		}
	})

	t.Run("a malformed file errors naming the path", func(t *testing.T) {
		t.Parallel()
		runDir, scratchDir := newDirs(t)
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatal(err)
		}
		statePath := filepath.Join(runDir, "state.json")
		if err := os.WriteFile(statePath, []byte(`{not json`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := weaverengine.LoadState(runDir, scratchDir); err == nil || !strings.Contains(err.Error(), statePath) {
			t.Errorf("LoadState() error = %v; want one naming %s", err, statePath)
		}
	})

	t.Run("StateRel is the anchor-relative state file", func(t *testing.T) {
		t.Parallel()
		if got, want := weaverengine.StateRel(), "_lym/weaver/state.json"; got != want {
			t.Errorf("StateRel() = %q; want %q", got, want)
		}
	})

	t.Run("BegunBatches reads nil, empty and begun states", func(t *testing.T) {
		t.Parallel()
		begun := &weaverengine.State{State: runplan.State{Batches: map[int]*runplan.BatchState{1: {}}}}
		tests := []struct {
			name string
			st   *weaverengine.State
			want bool
		}{
			{"nil", nil, false},
			{"empty", &weaverengine.State{}, false},
			{"begun", begun, true},
		}
		for _, tt := range tests {
			if got := weaverengine.BegunBatches(tt.st); got != tt.want {
				t.Errorf("BegunBatches(%s) = %v; want %v", tt.name, got, tt.want)
			}
		}
	})
}

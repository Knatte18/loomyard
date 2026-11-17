// runrecords_test.go covers the implementer run-record wiring over temp anchors.

package loomcli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// plantRunRecordFile writes a one-line file at path, creating its directory.
func plantRunRecordFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// begunBatches is a run-plan state holding one begun batch.
func begunBatches() runplan.State {
	return runplan.State{Batches: map[int]*runplan.BatchState{1: {Slug: "first"}}}
}

func TestImplementerRunRecords(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("ArchiveMovesBothRunDirectories", func(t *testing.T) {
		t.Parallel()
		anchor := t.TempDir()
		plantRunRecordFile(t, filepath.Join(websterengine.Dir(anchor), "state.json"))
		plantRunRecordFile(t, filepath.Join(weaverengine.Dir(anchor), "state.json"))
		prompts := filepath.Join(anchor, "prompts")
		if err := os.MkdirAll(prompts, 0o755); err != nil {
			t.Fatal(err)
		}
		webster := websterengine.Geometry{WebsterDir: websterengine.Dir(anchor), ScratchDir: websterengine.ScratchDir(anchor), PromptsDir: prompts}
		weaver := weaverengine.Geometry{RunDir: weaverengine.Dir(anchor), ScratchDir: weaverengine.ScratchDir(anchor)}

		dest := filepath.Join(t.TempDir(), "prior-generation", "webster")
		if err := archiveRunRecords(webster, weaver)(dest); err != nil {
			t.Fatalf("archiveRunRecords()(dest) = %v; want nil", err)
		}

		weaverSlot := filepath.Join(filepath.Dir(dest), loomshed.ReworkPriorWeaver)
		for _, moved := range []string{filepath.Join(dest, "state.json"), filepath.Join(weaverSlot, "state.json")} {
			if _, err := os.Stat(moved); err != nil {
				t.Errorf("archived record %s: %v; want it present", moved, err)
			}
		}
		for _, left := range []string{websterengine.Dir(anchor), weaverengine.Dir(anchor)} {
			if entries, _ := os.ReadDir(left); len(entries) != 0 {
				t.Errorf("run directory %s still holds %d entries; want none", left, len(entries))
			}
		}
	})

	tests := []struct {
		name          string
		webster       bool
		weaver        bool
		wantWeaverDir bool
		wantRefused   bool
	}{
		{name: "WeaverBegun", weaver: true, wantWeaverDir: true},
		{name: "WebsterBegun", webster: true},
		{name: "NeitherBegunReadsTheSettingsDefault"},
		{name: "BothBegunReturnsTheSeamRefusal", webster: true, weaver: true, wantRefused: true},
	}
	for _, tt := range tests {
		t.Run("ActiveRunRecordPath_"+tt.name, func(t *testing.T) {
			t.Parallel()
			anchor := t.TempDir()
			if tt.webster {
				if err := websterengine.SaveState(websterengine.Dir(anchor), websterengine.ScratchDir(anchor), &websterengine.State{State: begunBatches()}); err != nil {
					t.Fatal(err)
				}
			}
			if tt.weaver {
				if err := weaverengine.SaveState(weaverengine.Dir(anchor), weaverengine.ScratchDir(anchor), &weaverengine.State{State: begunBatches()}); err != nil {
					t.Fatal(err)
				}
			}

			got, err := activeRunRecordPath(weaverengine.ActiveDeps{AnchorRoot: anchor})
			if tt.wantRefused {
				if !errors.Is(err, weaverengine.ErrActiveRefused) {
					t.Fatalf("activeRunRecordPath() = %q, %v; want an error wrapping ErrActiveRefused", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("activeRunRecordPath() error = %v; want nil", err)
			}
			want := summaryparser.Path(websterengine.Dir(anchor))
			if tt.wantWeaverDir {
				want = summaryparser.Path(weaverengine.Dir(anchor))
			}
			if got != want {
				t.Errorf("activeRunRecordPath() = %q; want %q", got, want)
			}
		})
	}
}

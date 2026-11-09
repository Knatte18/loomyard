// raddlewiring_integration_test.go drives createBeamFromParent over a hub forged by hubforge.
// The package's testmain_test.go arms the hermetic git test environment for the whole binary,
// so this file adds no TestMain.

package loomcli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestCreateBeamFromParent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, hub, "the-sketch")
	location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree("the-sketch"))
	if err != nil {
		t.Fatalf("ResolveWorktree() error = %v; want nil", err)
	}

	var gotDir string
	var gotArgs []string
	err = createBeamFromParent(location, "the-project", func(dir string, args ...string) error {
		gotDir, gotArgs = dir, args
		return nil
	})
	if err != nil {
		t.Fatalf("createBeamFromParent() error = %v; want nil", err)
	}
	if want := filepath.Clean(hub.PrimeWorktree()); filepath.Clean(gotDir) != want {
		t.Errorf("run dir = %q; want the prime worktree %q, the parent of a pair forked from it", gotDir, want)
	}
	if want := []string{"beam", "create", "the-project"}; !slices.Equal(gotArgs, want) {
		t.Errorf("run args = %v; want %v", gotArgs, want)
	}

	failure := errors.New("beam create failed")
	err = createBeamFromParent(location, "the-project", func(string, ...string) error { return failure })
	if !errors.Is(err, failure) {
		t.Errorf("createBeamFromParent() error = %v; want the run's error returned", err)
	}
}

func TestRaddleRunFiling(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	created := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	const (
		boardWithEntry = `{"version":1,"entries":[{"id":1,"slug":"plan","title":"Plan","kind":"task","created":"2026-11-01T09:00:00Z"}]}`
		boardWithout   = `{"version":1,"entries":[]}`
		boardBroken    = `{not json`
	)
	tests := []struct {
		name  string
		board string
		// storeSketch is the Sketch the store.json belongs to, empty for a store with none.
		storeSketch  string
		storeCreated time.Time
		wantFiled    []string
		wantFound    bool
		wantErr      bool
	}{
		{name: "NoStoreEntry", board: boardWithEntry},
		{name: "StoreEntryOfAnotherSketch", board: boardWithEntry, storeSketch: "other", storeCreated: created},
		{name: "StoreEntryOfAnEarlierEntryWithTheSlug", board: boardWithEntry, storeSketch: "plan", storeCreated: created.Add(-time.Hour)},
		{name: "MatchingStoreEntry", board: boardWithEntry, storeSketch: "plan", storeCreated: created, wantFiled: []string{"plan", "part-a"}, wantFound: true},
		{name: "MatchingStoreEntryWithItsBoardEntryGone", board: boardWithout, storeSketch: "plan", storeCreated: created, wantFiled: []string{"plan", "part-a"}, wantFound: true},
		{name: "BoardReadFailure", board: boardBroken, storeSketch: "plan", storeCreated: created, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := forgeRaddleRun(t, "plan", tt.board)
			if tt.storeSketch != "" {
				// Filed names the run's slug too, so a store entry of another Sketch still holds it.
				data, err := json.Marshal(boardengine.RaddleStoreEntry{Created: tt.storeCreated, Hash: "h", Filed: []string{"plan", "part-a"}})
				if err != nil {
					t.Fatalf("marshal the store record: %v", err)
				}
				dir := boardengine.RaddleStoreDir(fabricengine.BoardDir(location.HubPath), tt.storeSketch)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("create the store directory: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "store.json"), data, 0o644); err != nil {
					t.Fatalf("write store.json: %v", err)
				}
			}

			filed, found, err := raddleRunFiling(location, "plan")
			if (err != nil) != tt.wantErr {
				t.Fatalf("raddleRunFiling() error = %v; want error %v", err, tt.wantErr)
			}
			if found != tt.wantFound || !slices.Equal(filed, tt.wantFiled) {
				t.Errorf("raddleRunFiling() = %v, %v; want %v, %v", filed, found, tt.wantFiled, tt.wantFound)
			}
		})
	}
}

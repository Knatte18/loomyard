// raddlewiring_test.go pins the history rule that gives Raddle-Write a fresh start and the done marker Raddle-File reads from the raddle store.

package loomcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestFreshRaddleEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name         string
		history      []shedengine.HistoryEntry
		wantFresh    bool
		wantPosition int
	}{
		{"NoHistory", nil, false, 0},
		{"EntryFromRecordWrite", []shedengine.HistoryEntry{{Producer: loomshed.NameRecordWrite, Outcome: shedengine.Done}}, false, 0},
		{
			"ReturnFromRaddleFile",
			[]shedengine.HistoryEntry{
				{Producer: loomshed.NameRecordWrite, Outcome: shedengine.Done},
				{Producer: loomshed.NameRaddleWrite, Outcome: shedengine.Done},
				{Producer: loomshed.NameRaddleFile, Outcome: shedengine.Stuck, RoutedTo: loomshed.NameRaddleWrite},
			},
			true, 3,
		},
		{"GotoIntoTheRow", []shedengine.HistoryEntry{{Producer: loomshed.NameRaddleFile, Outcome: shedengine.OutcomeGoto, RoutedTo: loomshed.NameRaddleWrite}}, true, 1},
		{"GotoElsewhere", []shedengine.HistoryEntry{{Producer: loomshed.NameRaddleFile, Outcome: shedengine.OutcomeGoto, RoutedTo: loomshed.NameRecordWrite}}, false, 0},
		{"StuckAtTheRowItself", []shedengine.HistoryEntry{{Producer: loomshed.NameRaddleWrite, Outcome: shedengine.Stuck}}, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fresh, position := freshRaddleEntry(tt.history)
			if fresh != tt.wantFresh || position != tt.wantPosition {
				t.Errorf("freshRaddleEntry() = %v, %d; want %v, %d", fresh, position, tt.wantFresh, tt.wantPosition)
			}
		})
	}
}

func TestRaddleFileDepsStoreDone(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	created := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		// storeSketch is the Sketch the store.json under the board directory belongs to, empty for a store with none.
		storeSketch  string
		storedHash   string
		askedCreated time.Time
		askedHash    string
		want         bool
	}{
		{"MatchingCreatedStampAndHash", "plan", "hash-1", created, "hash-1", true},
		{"AnotherHash", "plan", "hash-1", created, "hash-2", false},
		{"AnotherCreatedStamp", "plan", "hash-1", created.Add(time.Hour), "hash-1", false},
		{"AnotherSketchSlug", "other", "hash-1", created, "hash-1", false},
		{"EmptyStore", "", "", created, "hash-1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
			if tt.storeSketch != "" {
				// Filed names the run's slug too, so the closure must tell a Sketch that filed that slug from the run's own Sketch.
				data, err := json.Marshal(boardengine.RaddleStoreEntry{Created: created, Hash: tt.storedHash, Filed: []string{"plan", "part-a"}})
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

			got, err := raddleFileDeps(location, "plan").StoreDone(tt.askedCreated, tt.askedHash)
			if err != nil || got != tt.want {
				t.Errorf("StoreDone(%v, %q) = %t, %v; want %t, nil", tt.askedCreated, tt.askedHash, got, err, tt.want)
			}
		})
	}
}

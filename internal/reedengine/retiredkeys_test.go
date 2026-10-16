// retiredkeys_test.go pins that a reed.json still carrying the retired display keys fixedRows, shrinkWhenWaitingOnChild and size, the retired strand key contentRows and the retired anchors below-parent and top, keeps loading with all ignored.
// It decodes exactly like one without them, and a rewrite of it writes own-window.

package reedengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const retiredKeysState = `{
  "strands": [
    {"guid": "root", "paneId": "%1", "display": {"anchor": "below-parent", "focus": false, "shrinkWhenWaitingOnChild": true, "fixedRows": 3}},
    {"guid": "leaf", "paneId": "%2", "contentRows": 4, "display": {"anchor": "top", "focus": true, "shrinkWhenWaitingOnChild": false, "fixedRows": 7, "size": {"kind": "content", "shareNum": 1, "shareDen": 3, "minRows": 2}}}
  ]
}`

const currentKeysState = `{
  "strands": [
    {"guid": "root", "paneId": "%1", "display": {"anchor": "own-window", "focus": false}},
    {"guid": "leaf", "paneId": "%2", "display": {"anchor": "own-window", "focus": true}}
  ]
}`

func loadStateFromJSON(t *testing.T, doc string) (string, *ReedState) {
	t.Helper()
	dotLyxDir := filepath.Join(t.TempDir(), ".lyx")
	if err := os.MkdirAll(dotLyxDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dotLyxDir, reedStateFileName), []byte(doc), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := LoadState(dotLyxDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got == nil {
		t.Fatal("LoadState = nil, want a state")
	}
	return dotLyxDir, got
}

//testtiming:keep pins a reed.json still carrying the retired anchors, display keys, the size object and the strand's contentRows loading as own-window with them ignored, and a rewrite writing own-window; its covering tests run this code without asserting it
func TestLoadState_RetiredDisplayKeysIgnoredAndLayoutUnchanged(t *testing.T) {
	oldDir, old := loadStateFromJSON(t, retiredKeysState)
	_, current := loadStateFromJSON(t, currentKeysState)

	if len(old.Strands) != 2 {
		t.Fatalf("strands = %d, want 2", len(old.Strands))
	}
	for i := range old.Strands {
		if old.Strands[i].Display != current.Strands[i].Display {
			t.Errorf("strand %d Display = %+v, want %+v", i, old.Strands[i].Display, current.Strands[i].Display)
		}
	}

	if err := SaveState(oldDir, old); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	rewritten, err := os.ReadFile(filepath.Join(oldDir, reedStateFileName))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var file struct {
		Strands []struct {
			Display struct {
				Anchor string `json:"anchor"`
			} `json:"display"`
		} `json:"strands"`
	}
	if err := json.Unmarshal(rewritten, &file); err != nil {
		t.Fatalf("Unmarshal rewritten state: %v", err)
	}
	if len(file.Strands) != 2 {
		t.Fatalf("rewritten strands = %d, want 2", len(file.Strands))
	}
	for i, s := range file.Strands {
		if s.Display.Anchor != "own-window" {
			t.Errorf("rewritten strand %d anchor = %q, want own-window", i, s.Display.Anchor)
		}
	}
}

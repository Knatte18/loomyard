// retiredkeys_test.go pins that a reed.json still carrying the retired display keys fixedRows, shrinkWhenWaitingOnChild and size, and the retired strand key contentRows, keeps loading with all ignored.
// It decodes exactly like one without them.

package reedengine

import (
	"os"
	"path/filepath"
	"testing"
)

const retiredKeysState = `{
  "strands": [
    {"guid": "root", "paneId": "%1", "display": {"anchor": "below-parent", "focus": false, "shrinkWhenWaitingOnChild": true, "fixedRows": 3}},
    {"guid": "leaf", "paneId": "%2", "contentRows": 4, "display": {"anchor": "below-parent", "focus": true, "shrinkWhenWaitingOnChild": false, "fixedRows": 7, "size": {"kind": "content", "shareNum": 1, "shareDen": 3, "minRows": 2}}}
  ]
}`

const currentKeysState = `{
  "strands": [
    {"guid": "root", "paneId": "%1", "display": {"anchor": "below-parent", "focus": false}},
    {"guid": "leaf", "paneId": "%2", "display": {"anchor": "below-parent", "focus": true}}
  ]
}`

func loadStateFromJSON(t *testing.T, doc string) *ReedState {
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
	return got
}

//testtiming:keep pins a reed.json still carrying the retired display keys, the size object and the strand's contentRows loading with them ignored and decoding exactly like one without; its covering tests run this code without asserting it
func TestLoadState_RetiredDisplayKeysIgnoredAndLayoutUnchanged(t *testing.T) {
	old := loadStateFromJSON(t, retiredKeysState)
	current := loadStateFromJSON(t, currentKeysState)

	if len(old.Strands) != 2 {
		t.Fatalf("strands = %d, want 2", len(old.Strands))
	}
	for i := range old.Strands {
		if old.Strands[i].Display != current.Strands[i].Display {
			t.Errorf("strand %d Display = %+v, want %+v", i, old.Strands[i].Display, current.Strands[i].Display)
		}
	}
}

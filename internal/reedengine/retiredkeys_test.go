// retiredkeys_test.go pins that a reed.json still carrying the retired display keys fixedRows and
// shrinkWhenWaitingOnChild keeps loading, with both keys ignored: it decodes and lays out exactly
// like one without them.

package reedengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

const retiredKeysState = `{
  "strands": [
    {"guid": "root", "paneId": "%1", "display": {"anchor": "below-parent", "focus": false, "shrinkWhenWaitingOnChild": true, "fixedRows": 3}},
    {"guid": "leaf", "paneId": "%2", "display": {"anchor": "below-parent", "focus": true, "shrinkWhenWaitingOnChild": false, "fixedRows": 7}}
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

//testtiming:keep pins a reed.json still carrying the retired display keys loading with them ignored and laying out exactly like one without; its covering tests run this code without asserting it
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

	live := map[string]bool{"%1": true, "%2": true}
	box := render.Box{W: 100, H: 30}
	params := render.Params{CollapsedRows: 3, MinFullRows: 3}
	wantLayout, wantFocus, err := render.Rules(toRenderStrands(current.Strands, live), box, params, nil)
	if err != nil {
		t.Fatalf("Rules(current): %v", err)
	}
	gotLayout, gotFocus, err := render.Rules(toRenderStrands(old.Strands, live), box, params, nil)
	if err != nil {
		t.Fatalf("Rules(retired keys): %v", err)
	}
	if gotLayout != wantLayout || gotFocus != wantFocus {
		t.Errorf("layout = %q focus = %q, want %q focus %q", gotLayout, gotFocus, wantLayout, wantFocus)
	}
}

// state_test.go round-trips MuxState through Save/Load and verifies
// toRenderStrands' field mapping and Live derivation.

package muxengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/muxengine/render"
)

func TestLoadState_AbsentFileReturnsNilNil(t *testing.T) {
	dotLyxDir := filepath.Join(t.TempDir(), ".lyx")

	got, err := LoadState(dotLyxDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("LoadState(absent) = %+v, want nil", got)
	}
}

func TestSaveState_ThenLoadState_RoundTrips(t *testing.T) {
	dotLyxDir := filepath.Join(t.TempDir(), ".lyx")

	want := &MuxState{
		Socket:      "lyx-loomyard-HUB-abcd1234",
		Session:     "internal-mux",
		StrippedEnv: []string{"CLAUDECODE", "CLAUDE_CODE_SESSION_ID"},
		Strands: []Strand{
			{
				GUID:      "guid-1",
				Name:      "main:1:abc12345",
				Worktree:  `C:\Code\loomyard\wts\internal-mux`,
				Cmd:       "claude --session-id abc",
				ResumeCmd: "claude --resume abc",
				SessionID: "abc",
				PaneID:    "%1",
				Display: render.Display{
					Anchor:                   render.AnchorBelowParent,
					Focus:                    true,
					ShrinkWhenWaitingOnChild: false,
				},
			},
			{
				GUID:     "guid-2",
				Name:     "review:1:def67890",
				Worktree: `C:\Code\loomyard\wts\internal-mux`,
				Parent:   "guid-1",
				Cmd:      "claude --session-id def",
				PaneID:   "%2",
				Display: render.Display{
					Anchor:                   render.AnchorTop,
					ShrinkWhenWaitingOnChild: true,
				},
			},
		},
	}

	if err := SaveState(dotLyxDir, want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	got, err := LoadState(dotLyxDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got == nil {
		t.Fatal("LoadState after SaveState = nil, want non-nil")
	}

	if got.Socket != want.Socket || got.Session != want.Session {
		t.Errorf("top-level fields = %+v, want %+v", got, want)
	}
	if len(got.StrippedEnv) != len(want.StrippedEnv) {
		t.Fatalf("StrippedEnv = %v, want %v", got.StrippedEnv, want.StrippedEnv)
	}
	if len(got.Strands) != len(want.Strands) {
		t.Fatalf("Strands = %+v, want %+v", got.Strands, want.Strands)
	}
	for i := range want.Strands {
		if got.Strands[i] != want.Strands[i] {
			t.Errorf("Strands[%d] = %+v, want %+v", i, got.Strands[i], want.Strands[i])
		}
	}
}

func TestLoadState_CorruptFileErrors(t *testing.T) {
	dotLyxDir := filepath.Join(t.TempDir(), ".lyx")
	if err := SaveState(dotLyxDir, &MuxState{Socket: "s"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// Corrupt the file directly, bypassing the lock-protected write path.
	path := filepath.Join(dotLyxDir, muxStateFileName)
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("corrupt file: %v", err)
	}

	if _, err := LoadState(dotLyxDir); err == nil {
		t.Error("LoadState(corrupt) = nil error, want error")
	}
}

func TestToRenderStrands_MapsFieldsAndSetsLiveFromPaneSet(t *testing.T) {
	strands := []Strand{
		{GUID: "g1", Parent: "", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true}},
		{GUID: "g2", Parent: "g1", PaneID: "%2", Display: render.Display{Anchor: render.AnchorTop}},
		{GUID: "g3", Parent: "g1", PaneID: "", Display: render.Display{Anchor: render.AnchorHidden}},
	}
	liveIDs := map[string]bool{"%1": true}

	got := toRenderStrands(strands, liveIDs)

	if len(got) != len(strands) {
		t.Fatalf("toRenderStrands returned %d strands, want %d (must map all, not filter)", len(got), len(strands))
	}

	want := []render.Strand{
		{GUID: "g1", Parent: "", Display: strands[0].Display, PaneID: "%1", Live: true},
		{GUID: "g2", Parent: "g1", Display: strands[1].Display, PaneID: "%2", Live: false},
		{GUID: "g3", Parent: "g1", Display: strands[2].Display, PaneID: "", Live: false},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("toRenderStrands()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

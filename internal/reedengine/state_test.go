// state_test.go round-trips ReedState through Save/Load and verifies toRenderStrands' field mapping
// and Live derivation.

package reedengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
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

	want := &ReedState{
		Socket:      "lyx-loomyard-HUB-abcd1234",
		Session:     "internal-reed",
		StrippedEnv: []string{"CLAUDECODE", "CLAUDE_CODE_SESSION_ID"},
		Strands: []Strand{
			{
				GUID:      "guid-1",
				Name:      "main:1:abc12345",
				Worktree:  `C:\Code\loomyard\wts\internal-reed`,
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
				Worktree: `C:\Code\loomyard\wts\internal-reed`,
				Parent:   "guid-1",
				Cmd:      "claude --session-id def",
				PaneID:   "%2",
				Display: render.Display{
					Anchor:                   render.AnchorBelowParent,
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
	if err := SaveState(dotLyxDir, &ReedState{Socket: "s"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// Corrupt the file directly, bypassing the lock-protected write path.
	path := filepath.Join(dotLyxDir, reedStateFileName)
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
		{GUID: "g2", Parent: "g1", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent}},
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

// TestLoadState_UnreadableFileIsActionable is the regression guard for the R5 review's R5-F1.
//
// Before the fix a corrupt reed.json made every verb that loads state refuse with
// `load state: unmarshal state: unexpected end of JSON input` — naming neither the file nor a
// remedy — while the tmux session and every strand process it describes stayed alive, and the only
// verb that still worked was the one that destroys them. The `null` row is the worse half: it
// decoded to a valid EMPTY state, so `status` answered ok:true with zero strands and the whole
// persisted table vanished silently.
func TestLoadState_UnreadableFileIsActionable(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "truncated mid-object", content: `{"socket":"lyx-x","session":"svc","stra`},
		{name: "empty file", content: ""},
		{name: "garbage bytes", content: "\x00\x00\x00\x00"},
		{name: "a bare null document", content: "null"},
		{name: "a bare null document with surrounding whitespace", content: "  null\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "reed.json")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			st, err := LoadState(dir)
			if err == nil {
				t.Fatalf("LoadState() = (%+v, nil); want an error — a state file reed cannot read must never decode to a plausible empty state", st)
			}
			for _, want := range []string{path, "lyx reed down"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("LoadState() error = %v; want it to contain %q so the operator can act on it", err, want)
				}
			}
		})
	}
}

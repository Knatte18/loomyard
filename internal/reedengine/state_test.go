// state_test.go round-trips ReedState through Save/Load and pins the unreadable-file refusals.

package reedengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSaveState_ThenLoadState_RoundTrips pins that LoadState answers (nil, nil) for an absent file.
// It also pins that LoadState returns what SaveState wrote.
//
//testtiming:keep pins LoadState answering (nil, nil) for an absent file and returning every field SaveState wrote, strand records included; its covering tests run this code without asserting it
func TestSaveState_ThenLoadState_RoundTrips(t *testing.T) {
	tiergate.Require(t, 0)
	dotLymDir := filepath.Join(t.TempDir(), ".lym")
	absent, err := LoadState(dotLymDir)
	if err != nil || absent != nil {
		t.Fatalf("LoadState(absent) = (%+v, %v), want (nil, nil)", absent, err)
	}

	want := &ReedState{
		Socket:      "lym-loomyard-LYMHUB-abcd1234",
		Session:     "internal-reed",
		StrippedEnv: []string{"CLAUDECODE", "CLAUDE_CODE_SESSION_ID"},
		Strands: []Strand{
			{
				GUID:       "guid-1",
				Name:       "main:1:abc12345",
				Worktree:   `C:\Code\loomyard\wts\internal-reed`,
				Cmd:        "claude --session-id abc",
				ResumeCmd:  "claude --resume abc",
				SessionID:  "abc",
				Provider:   "claude",
				CreelHooks: true,
				LymBin:     "/opt/lym/bin/lym",
				PaneID:     "%1",
				Display: render.Display{
					Anchor: render.AnchorOwnWindow,
					Focus:  true,
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
					Anchor: render.AnchorOwnWindow},
			},
		},
	}

	if err := SaveState(dotLymDir, want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	got, err := LoadState(dotLymDir)
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
		if !reflect.DeepEqual(got.Strands[i], want.Strands[i]) {
			t.Errorf("Strands[%d] = %+v, want %+v", i, got.Strands[i], want.Strands[i])
		}
	}
}

// TestLoadState_UnreadableFileIsActionable pins the refusal of an unreadable reed.json.
//
// A corrupt file must not make every state-loading verb fail with a bare JSON error.
// The tmux session and its strand processes stay alive, and the only verb that still works would destroy them.
// A `null` file decodes to a valid empty state: status answers ok:true with zero strands.
// The persisted table vanishes silently.
func TestLoadState_UnreadableFileIsActionable(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name    string
		content string
	}{
		{name: "truncated mid-object", content: `{"socket":"lym-x","session":"svc","stra`},
		{name: "empty file", content: ""},
		{name: "garbage bytes", content: "\x00\x00\x00\x00"},
		{name: "a bare null document", content: "null"},
		{name: "a bare null document with surrounding whitespace", content: "  null\n"},
		{name: "not json", content: "not json"},
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
			for _, want := range []string{path, "lym reed down"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("LoadState() error = %v; want it to contain %q so the operator can act on it", err, want)
				}
			}
		})
	}
}

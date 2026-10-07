package claudeengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestTurnEndActivity(t *testing.T) {
	t.Parallel()

	const (
		normalTurnEnd = `{"type":"assistant","message":{"stop_reason":"end_turn","content":[{"type":"text","text":"all done"}]}}`
		toolUse       = `{"type":"assistant","message":{"stop_reason":"tool_use","content":[{"type":"text","text":"working"}]}}`
		apiError      = `{"type":"assistant","isApiErrorMessage":true,"message":{"stop_reason":"stop_sequence","content":[{"type":"text","text":"API Error: 529 overloaded"}]}}`
		sidechainErr  = `{"type":"assistant","isSidechain":true,"isApiErrorMessage":true,"message":{"stop_reason":"stop_sequence","content":[{"type":"text","text":"API Error: 500"}]}}`
	)
	written := time.Date(2026, 10, 7, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name       string
		transcript []string
		want       shuttleengine.TurnEndActivity
	}{
		{
			name:       "newest main-chain turn end is an API error",
			transcript: []string{normalTurnEnd, apiError},
			want:       shuttleengine.TurnEndActivity{TranscriptModTime: written, APIError: true, APIErrorText: "API Error: 529 overloaded"},
		},
		{
			name:       "a tool-use entry after the error does not hide it",
			transcript: []string{apiError, toolUse},
			want:       shuttleengine.TurnEndActivity{TranscriptModTime: written, APIError: true, APIErrorText: "API Error: 529 overloaded"},
		},
		{
			name:       "an error followed by a normal turn end is not one",
			transcript: []string{apiError, normalTurnEnd},
			want:       shuttleengine.TurnEndActivity{TranscriptModTime: written},
		},
		{
			name:       "an error only on a sidechain entry is not one",
			transcript: []string{normalTurnEnd, sidechainErr},
			want:       shuttleengine.TurnEndActivity{TranscriptModTime: written},
		},
		{
			name:       "a transcript with no turn end reports its time alone",
			transcript: []string{toolUse},
			want:       shuttleengine.TurnEndActivity{TranscriptModTime: written},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "transcript.jsonl")
			if err := os.WriteFile(path, []byte(strings.Join(tt.transcript, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, written, written); err != nil {
				t.Fatal(err)
			}

			got := (&Claude{}).TurnEndActivity(stopEventFor(t, path))

			if !got.TranscriptModTime.Equal(tt.want.TranscriptModTime) || got.APIError != tt.want.APIError || got.APIErrorText != tt.want.APIErrorText {
				t.Errorf("TurnEndActivity = %+v; want %+v", got, tt.want)
			}
		})
	}

	t.Run("a missing transcript path or file reads as unreadable", func(t *testing.T) {
		t.Parallel()
		noPath, err := json.Marshal(map[string]any{"hook_event_name": "Stop"})
		if err != nil {
			t.Fatal(err)
		}
		for name, event := range map[string]shuttleengine.Event{
			"no transcript_path": {Kind: shuttleengine.EventStop, Raw: noPath},
			"missing file":       stopEventFor(t, filepath.Join(t.TempDir(), "absent.jsonl")),
		} {
			if got := (&Claude{}).TurnEndActivity(event); got != (shuttleengine.TurnEndActivity{}) {
				t.Errorf("%s: TurnEndActivity = %+v; want the zero reading", name, got)
			}
		}
	})
}

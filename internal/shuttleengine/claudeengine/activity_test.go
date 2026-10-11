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
	now := written.Add(2 * time.Hour)
	newClaude := func() *Claude { return &Claude{now: func() time.Time { return now }} }

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

			got := newClaude().TurnEndActivity(stopEventFor(t, path))

			if !got.TranscriptModTime.Equal(tt.want.TranscriptModTime) || got.APIError != tt.want.APIError || got.APIErrorText != tt.want.APIErrorText {
				t.Errorf("TurnEndActivity = %+v; want %+v", got, tt.want)
			}
		})
	}

	const (
		forkCall   = `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_gate","name":"Bash"}]}}`
		siblingRun = `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_quick","name":"Bash"}]}}`
		siblingEnd = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_quick"}]}}`
		gateEnd    = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_gate"}]}}`

		// A parallel batch is one message, msg_batch, written as one entry per call.
		batchFirst    = `{"type":"assistant","message":{"id":"msg_batch","content":[{"type":"tool_use","id":"toolu_first","name":"Bash"}]}}`
		batchLast     = `{"type":"assistant","message":{"id":"msg_batch","content":[{"type":"tool_use","id":"toolu_last","name":"Bash"}]}}`
		batchLastEnd  = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_last"}]}}`
		batchFirstEnd = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_first"}]}}`
		olderCall     = `{"type":"assistant","message":{"id":"msg_older","content":[{"type":"tool_use","id":"toolu_older","name":"Bash"}]}}`
		finalText     = `{"type":"assistant","message":{"id":"msg_final","content":[{"type":"text","text":"done"}]}}`
	)
	forkTests := []struct {
		name      string
		fork      []string
		forkWrote time.Time
		want      time.Time
	}{
		{"a fork written after the main transcript reads its write time", []string{forkCall, gateEnd}, written.Add(time.Minute), written.Add(time.Minute)},
		{"a pending call written within the ceiling reads now", []string{forkCall}, now.Add(-bashMaxTimeout / 2), now},
		{"a pending call followed by a sibling's result still reads now", []string{forkCall, siblingRun, siblingEnd}, now.Add(-time.Minute), now},
		{"a pending call written beyond the ceiling reads its write time", []string{forkCall}, now.Add(-bashMaxTimeout - time.Minute), now.Add(-bashMaxTimeout - time.Minute)},
		{"a parallel batch whose earlier call is pending and whose last call is answered reads now", []string{batchFirst, batchLast, batchLastEnd}, now.Add(-time.Minute), now},
		{"a fully answered newest message reads its write time whatever older messages hold", []string{olderCall, batchFirst, batchLast, batchLastEnd, batchFirstEnd}, now.Add(-time.Minute), now.Add(-time.Minute)},
		{"a transcript with no tool use reads its write time", []string{finalText}, now.Add(-time.Minute), now.Add(-time.Minute)},
		{"a fork whose calls all have results, older than the main transcript, leaves the main time", []string{forkCall, gateEnd}, written.Add(-time.Hour), written},
	}
	for _, tt := range forkTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "session.jsonl")
			forkPath := filepath.Join(dir, "session", "subagents", "agent-1.jsonl")
			if err := os.MkdirAll(filepath.Dir(forkPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(normalTurnEnd+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(forkPath, []byte(strings.Join(tt.fork, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			for p, at := range map[string]time.Time{path: written, forkPath: tt.forkWrote} {
				if err := os.Chtimes(p, at, at); err != nil {
					t.Fatal(err)
				}
			}

			got := newClaude().TurnEndActivity(stopEventFor(t, path))

			if !got.TranscriptModTime.Equal(tt.want) {
				t.Errorf("TranscriptModTime = %v; want %v", got.TranscriptModTime, tt.want)
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
			if got := newClaude().TurnEndActivity(event); got != (shuttleengine.TurnEndActivity{}) {
				t.Errorf("%s: TurnEndActivity = %+v; want the zero reading", name, got)
			}
		}
	})
}

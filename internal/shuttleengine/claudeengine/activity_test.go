//go:build tier0

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
		usageLimit    = `{"type":"assistant","timestamp":"2026-10-28T11:03:42.000Z","isApiErrorMessage":true,"error":"rate_limit","message":{"stop_reason":"stop_sequence","content":[{"type":"text","text":"You've hit your weekly limit · resets 2pm (Europe/Oslo)"}]}}`
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
			name:       "a usage-limit turn end reads the limit and its reset time",
			transcript: []string{normalTurnEnd, usageLimit},
			want: shuttleengine.TurnEndActivity{TranscriptModTime: written, APIError: true, APIErrorText: "You've hit your weekly limit · resets 2pm (Europe/Oslo)",
				UsageLimit: shuttleengine.UsageLimit{Limited: true, ResetsAt: time.Date(2026, 10, 28, 13, 0, 0, 0, time.UTC)}},
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

			if !got.TranscriptModTime.Equal(tt.want.TranscriptModTime) || got.APIError != tt.want.APIError || got.APIErrorText != tt.want.APIErrorText ||
				got.UsageLimit.Limited != tt.want.UsageLimit.Limited || !got.UsageLimit.ResetsAt.Equal(tt.want.UsageLimit.ResetsAt) {
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
	// batchPadding is an entry of msg_batch longer than the first backward read, so an entry written before it lies outside that read.
	batchPadding := `{"type":"assistant","message":{"id":"msg_batch","content":[{"type":"text","text":"` + strings.Repeat("x", initialReadChunk) + `"}]}}`
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
		{"a pending call in a batch entry before the first read's window reads now", []string{batchFirst, batchPadding, batchLast, batchLastEnd}, now.Add(-time.Minute), now},
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

func TestTurnEndUsageLimited(t *testing.T) {
	t.Parallel()

	const (
		earlierTurnEnd = `{"type":"assistant","timestamp":"2026-10-07T09:00:00.000Z","message":{"stop_reason":"end_turn","content":[{"type":"text","text":"first turn done"}]}}`
		agentTurnEnd   = `{"type":"assistant","timestamp":"2026-10-07T09:20:00.000Z","message":{"stop_reason":"end_turn","content":[{"type":"text","text":"stopping here"}]}}`
		limitTurnEnd   = `{"type":"assistant","timestamp":"2026-10-07T09:20:00.000Z","isApiErrorMessage":true,"error":"rate_limit","message":{"stop_reason":"stop_sequence","content":[{"type":"text","text":"You've hit your limit"}]}}`
		serverTurnEnd  = `{"type":"assistant","timestamp":"2026-10-07T09:20:00.000Z","isApiErrorMessage":true,"error":"server_error","message":{"stop_reason":"stop_sequence","content":[{"type":"text","text":"API Error: 500"}]}}`
		forkLimit      = `{"type":"assistant","isSidechain":true,"isApiErrorMessage":true,"error":"rate_limit","message":{"stop_reason":"stop_sequence","content":[{"type":"text","text":"You've hit your limit"}]}}`
		forkServer     = `{"type":"assistant","isSidechain":true,"isApiErrorMessage":true,"error":"server_error","message":{"stop_reason":"stop_sequence","content":[{"type":"text","text":"API Error: 500"}]}}`
		sidechainLimit = `{"type":"assistant","isSidechain":true,"timestamp":"2026-10-07T09:25:00.000Z","isApiErrorMessage":true,"error":"rate_limit","message":{"stop_reason":"stop_sequence","content":[{"type":"text","text":"You've hit your limit"}]}}`
		forkAnswer     = `{"type":"assistant","isSidechain":true,"message":{"stop_reason":"end_turn","content":[{"type":"text","text":"found it"}]}}`
	)
	previousTurnEnd := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		transcript []string
		// fork is the one fork transcript's lines, none when empty, and forkWrote its write time.
		fork      []string
		forkWrote time.Time
		want      bool
	}{
		{"a main-chain usage-limit turn end is limited", []string{earlierTurnEnd, limitTurnEnd}, nil, time.Time{}, true},
		{"a main-chain server-error turn end is not limited", []string{earlierTurnEnd, serverTurnEnd}, nil, time.Time{}, false},
		{"a limit an earlier turn end reported does not limit a later plain turn end", []string{earlierTurnEnd, limitTurnEnd, agentTurnEnd}, nil, time.Time{}, false},
		{"a sidechain limit entry in the main transcript is ignored", []string{earlierTurnEnd, agentTurnEnd, sidechainLimit}, nil, time.Time{}, false},
		{"a plain turn end with no fork is not limited", []string{earlierTurnEnd, agentTurnEnd}, nil, time.Time{}, false},
		{"a fork stopped on the limit during the turn makes the agent's own turn end limited", []string{earlierTurnEnd, agentTurnEnd}, []string{forkLimit}, previousTurnEnd.Add(10 * time.Minute), true},
		{"a fork stopped on the limit before the previous turn end does not count", []string{earlierTurnEnd, agentTurnEnd}, []string{forkLimit}, previousTurnEnd.Add(-time.Minute), false},
		{"a fork that answered after a limit entry does not count", []string{earlierTurnEnd, agentTurnEnd}, []string{forkLimit, forkAnswer}, previousTurnEnd.Add(10 * time.Minute), false},
		{"a fork stopped on a server error does not count", []string{earlierTurnEnd, agentTurnEnd}, []string{forkServer}, previousTurnEnd.Add(10 * time.Minute), false},
		{"a first turn counts every fork", []string{agentTurnEnd}, []string{forkLimit}, previousTurnEnd.Add(-time.Hour), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "session.jsonl")
			if err := os.WriteFile(path, []byte(strings.Join(tt.transcript, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if len(tt.fork) > 0 {
				forkPath := filepath.Join(dir, "session", "subagents", "agent-1.jsonl")
				if err := os.MkdirAll(filepath.Dir(forkPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(forkPath, []byte(strings.Join(tt.fork, "\n")+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(forkPath, tt.forkWrote, tt.forkWrote); err != nil {
					t.Fatal(err)
				}
			}

			if got := (&Claude{}).TurnEndUsageLimited(stopEventFor(t, path)); got != tt.want {
				t.Errorf("TurnEndUsageLimited() = %v; want %v", got, tt.want)
			}
		})
	}

	t.Run("a missing transcript path or file is not limited", func(t *testing.T) {
		t.Parallel()
		noPath, err := json.Marshal(map[string]any{"hook_event_name": "Stop"})
		if err != nil {
			t.Fatal(err)
		}
		for name, event := range map[string]shuttleengine.Event{
			"no transcript_path": {Kind: shuttleengine.EventStop, Raw: noPath},
			"missing file":       stopEventFor(t, filepath.Join(t.TempDir(), "absent.jsonl")),
		} {
			if (&Claude{}).TurnEndUsageLimited(event) {
				t.Errorf("%s: TurnEndUsageLimited() = true; want false", name)
			}
		}
	})
}

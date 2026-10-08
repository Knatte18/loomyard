package claudeengine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// stopEventFor builds a Stop event whose payload names the given transcript path.
func stopEventFor(t *testing.T, transcriptPath string) shuttleengine.Event {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"hook_event_name": "Stop",
		"transcript_path": transcriptPath,
	})
	if err != nil {
		t.Fatalf("marshal stop payload: %v", err)
	}
	return shuttleengine.Event{Kind: shuttleengine.EventStop, Raw: raw}
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return abs
}

func TestContextTokens_LastMainChainAssistantEntry(t *testing.T) {
	c := &Claude{}
	got := c.ContextTokens(stopEventFor(t, fixturePath(t, "usage-main-chain.jsonl")))
	if !got.Known || got.Tokens != 3+40+5000 || got.Compacted || !got.BoundaryAt.IsZero() {
		t.Fatalf("got %+v, want 5043 known, not compacted", got)
	}
}

func TestContextTokens_IgnoresSidechainAndZeroUsage(t *testing.T) {
	c := &Claude{}
	got := c.ContextTokens(stopEventFor(t, fixturePath(t, "usage-sidechain-last.jsonl")))
	if !got.Known || got.Tokens != 7+70+700 {
		t.Fatalf("got %+v, want 777 known", got)
	}
}

func TestContextTokens_FollowsTranscriptNamedByEachStop(t *testing.T) {
	c := &Claude{}
	before := c.ContextTokens(stopEventFor(t, fixturePath(t, "usage-main-chain.jsonl")))
	if !before.Known || before.Tokens != 5043 {
		t.Fatalf("before clear: got %+v, want 5043 known", before)
	}
	after := c.ContextTokens(stopEventFor(t, fixturePath(t, "usage-after-clear.jsonl")))
	if !after.Known || after.Tokens != 2+30+400 {
		t.Fatalf("after clear: got %+v, want 432 known", after)
	}
}

func TestContextTokens_BoundaryAfterLastAssistantIsTheReading(t *testing.T) {
	c := &Claude{}
	got := c.ContextTokens(stopEventFor(t, fixturePath(t, "usage-boundary-last.jsonl")))
	want := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if !got.Known || got.Tokens != 321 || !got.Compacted || !got.BoundaryAt.Equal(want) {
		t.Fatalf("got %+v, want 321 known, compacted at %v", got, want)
	}
}

func TestContextTokens_AssistantAfterBoundaryWins(t *testing.T) {
	c := &Claude{}
	got := c.ContextTokens(stopEventFor(t, fixturePath(t, "usage-assistant-after-boundary.jsonl")))
	if !got.Known || got.Tokens != 4+50+400 || got.Compacted || !got.BoundaryAt.IsZero() {
		t.Fatalf("got %+v, want 454 known, not compacted", got)
	}
}

func TestReadContext_EntrySpanningChunkEdgeIsReadWhole(t *testing.T) {
	for _, name := range []string{"usage-main-chain.jsonl", "usage-boundary-last.jsonl", "usage-assistant-after-boundary.jsonl"} {
		data, err := os.ReadFile(fixturePath(t, name))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		whole := readContext(bytes.NewReader(data), int64(len(data)), len(data)+1)
		for _, chunk := range []int{1, 7, 50, 113} {
			got := readContext(bytes.NewReader(data), int64(len(data)), chunk)
			if got != whole {
				t.Errorf("%s chunk %d: got %+v, want %+v", name, chunk, got, whole)
			}
		}
	}
}

func TestReadContext_NoQualifyingEntryIsUnknown(t *testing.T) {
	data := []byte("{\"type\":\"user\",\"message\":{\"content\":\"hi\"}}\nnot json\n{\"type\":\"system\",\"subtype\":\"compact_boundary\",\"isSidechain\":true,\"timestamp\":\"2026-03-04T05:06:07Z\",\"compactMetadata\":{\"postTokens\":5}}\n")
	for _, chunk := range []int{1, 16, 4096} {
		if got := readContext(bytes.NewReader(data), int64(len(data)), chunk); got != (shuttleengine.ContextReading{}) {
			t.Errorf("chunk %d: got %+v, want unknown", chunk, got)
		}
	}
}

func TestContextTokens_Unknown(t *testing.T) {
	c := &Claude{}
	missing := filepath.Join(t.TempDir(), "absent.jsonl")
	if _, err := os.Stat(missing); err == nil {
		t.Fatal("fixture path unexpectedly exists")
	}
	noPath, _ := json.Marshal(map[string]any{"hook_event_name": "Stop"})

	cases := map[string]shuttleengine.Event{
		"malformed":       stopEventFor(t, fixturePath(t, "usage-malformed.jsonl")),
		"missing file":    stopEventFor(t, missing),
		"no path":         {Kind: shuttleengine.EventStop, Raw: noPath},
		"undecodable raw": {Kind: shuttleengine.EventStop, Raw: []byte("not json")},
	}
	for name, ev := range cases {
		if got := c.ContextTokens(ev); got != (shuttleengine.ContextReading{}) {
			t.Errorf("%s: got %+v, want unknown", name, got)
		}
	}
}

// timedTranscript is a transcript with timestamps on every entry: a boundary at 01:00, an assistant turn at 02:00, a sidechain boundary at 03:00 and a second boundary at 04:00 followed by an assistant turn at 05:00.
const timedTranscript = `{"type":"assistant","isSidechain":false,"timestamp":"2026-03-04T00:30:00Z","message":{"usage":{"input_tokens":1}}}
{"type":"system","subtype":"compact_boundary","isSidechain":false,"timestamp":"2026-03-04T01:00:00Z","compactMetadata":{"postTokens":10}}
{"type":"assistant","isSidechain":false,"timestamp":"2026-03-04T02:00:00Z","message":{"usage":{"input_tokens":2}}}
{"type":"system","subtype":"compact_boundary","isSidechain":true,"timestamp":"2026-03-04T03:00:00Z","compactMetadata":{"postTokens":30}}
{"type":"system","subtype":"compact_boundary","isSidechain":false,"timestamp":"2026-03-04T04:00:00Z","compactMetadata":{"postTokens":40}}
{"type":"assistant","isSidechain":false,"timestamp":"2026-03-04T05:00:00Z","message":{"usage":{"input_tokens":5}}}
`

func TestCompactedSince_FindsNewestMainChainBoundaryAfterSince(t *testing.T) {
	t.Parallel()

	hour := func(h int) time.Time { return time.Date(2026, 3, 4, h, 0, 0, 0, time.UTC) }
	data := []byte(timedTranscript)
	cases := []struct {
		name      string
		since     time.Time
		want      time.Time
		wantFound bool
	}{
		{"before every boundary", hour(0), hour(4), true},
		{"between the boundaries", hour(2), hour(4), true},
		{"at the newest boundary", hour(4), time.Time{}, false},
		{"after every entry", hour(6), time.Time{}, false},
	}
	for _, tc := range cases {
		for _, chunk := range []int{1, 64, 4096} {
			got, found := compactedSince(bytes.NewReader(data), int64(len(data)), chunk, tc.since, "")
			if found != tc.wantFound || !got.At.Equal(tc.want) {
				t.Errorf("%s chunk %d: got %v, %v; want %v, %v", tc.name, chunk, got.At, found, tc.want, tc.wantFound)
			}
		}
	}
}

func TestCompactedSince_NoBoundaryInTranscript(t *testing.T) {
	c := &Claude{}
	got, found := c.CompactedSince(stopEventFor(t, fixturePath(t, "usage-main-chain.jsonl")), time.Time{})
	if found || got != (shuttleengine.CompactionBoundary{}) {
		t.Errorf("got %v, %v; want not found", got, found)
	}
}

func TestCompactedSince_ReadsTheTranscriptNamedByTheTurnEnd(t *testing.T) {
	c := &Claude{}
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(timedTranscript), 0o644); err != nil {
		t.Fatal(err)
	}
	got, found := c.CompactedSince(stopEventFor(t, path), time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC))
	if want := time.Date(2026, 3, 4, 4, 0, 0, 0, time.UTC); !found || !got.At.Equal(want) {
		t.Errorf("got %v, %v; want %v, true", got, found, want)
	}
}

func TestCompactedSince_DegradesToNotFound(t *testing.T) {
	c := &Claude{}
	missing := filepath.Join(t.TempDir(), "absent.jsonl")
	noPath, _ := json.Marshal(map[string]any{"hook_event_name": "Stop"})
	cases := map[string]shuttleengine.Event{
		"missing file":    stopEventFor(t, missing),
		"no path":         {Kind: shuttleengine.EventStop, Raw: noPath},
		"undecodable raw": {Kind: shuttleengine.EventStop, Raw: []byte("not json")},
		"malformed":       stopEventFor(t, fixturePath(t, "usage-malformed.jsonl")),
	}
	for name, ev := range cases {
		if got, found := c.CompactedSince(ev, time.Time{}); found || got != (shuttleengine.CompactionBoundary{}) {
			t.Errorf("%s: got %v, %v; want not found", name, got, found)
		}
	}
}

// TestCompactedSince_CountsTurnEndsAfterTheBoundary covers the turn-end count and the match of the read turn end against the transcript's newest one.
func TestCompactedSince_CountsTurnEndsAfterTheBoundary(t *testing.T) {
	t.Parallel()

	const boundary = `{"type":"system","subtype":"compact_boundary","isSidechain":false,"timestamp":"2026-03-04T01:00:00Z","compactMetadata":{"postTokens":10}}` + "\n"
	turnEnd := func(hour int, stopReason, text string) string {
		return fmt.Sprintf(`{"type":"assistant","isSidechain":false,"timestamp":"2026-03-04T%02d:00:00Z","message":{"stop_reason":%s,"content":[{"type":"text","text":%q}]}}`+"\n", hour, stopReason, text)
	}
	sidechainTurnEnd := `{"type":"assistant","isSidechain":true,"timestamp":"2026-03-04T05:00:00Z","message":{"stop_reason":"end_turn","content":[{"type":"text","text":"side"}]}}` + "\n"
	beforeBoundary := turnEnd(0, `"end_turn"`, "old")
	cases := []struct {
		name        string
		transcript  string
		stopMessage *string
		wantCount   int
		wantRead    bool
	}{
		{"no turn end after the boundary", beforeBoundary + boundary, ptr("done"), 0, false},
		{"one turn end matching the payload", beforeBoundary + boundary + turnEnd(2, `"end_turn"`, "done"), ptr("done"), 1, true},
		{"one turn end differing from the payload", beforeBoundary + boundary + turnEnd(2, `"end_turn"`, "done"), ptr("other"), 1, false},
		{"one turn end, payload without a message", beforeBoundary + boundary + turnEnd(2, `"end_turn"`, "done"), nil, 1, false},
		{"one turn end matching up to surrounding whitespace", beforeBoundary + boundary + turnEnd(2, `"end_turn"`, "\n done \n"), ptr("  done"), 1, true},
		{"two turn ends, the payload names the newest", boundary + turnEnd(2, `"end_turn"`, "first") + turnEnd(3, `"end_turn"`, "second"), ptr("second"), 2, true},
		{"two turn ends, the payload names the older", boundary + turnEnd(2, `"end_turn"`, "first") + turnEnd(3, `"end_turn"`, "second"), ptr("first"), 2, false},
		{"a sidechain turn end and a tool_use stop are not counted", boundary + turnEnd(2, `"tool_use"`, "calling") + sidechainTurnEnd + turnEnd(3, `null`, "streaming"), ptr("calling"), 0, false},
	}
	for _, tc := range cases {
		path := filepath.Join(t.TempDir(), "transcript.jsonl")
		if err := os.WriteFile(path, []byte(tc.transcript), 0o644); err != nil {
			t.Fatal(err)
		}
		fields := map[string]any{"hook_event_name": "Stop", "transcript_path": path}
		if tc.stopMessage != nil {
			fields["last_assistant_message"] = *tc.stopMessage
		}
		raw, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		got, found := (&Claude{}).CompactedSince(shuttleengine.Event{Kind: shuttleengine.EventStop, Raw: raw}, time.Date(2026, 3, 4, 0, 30, 0, 0, time.UTC))
		if !found || got.TurnEndsAfter != tc.wantCount || got.ReadTurnEndAfter != tc.wantRead {
			t.Errorf("%s: got %+v, %v; want count %d, read %v", tc.name, got, found, tc.wantCount, tc.wantRead)
		}
	}
}

func ptr(s string) *string { return &s }

// assistantLine builds one transcript line of an assistant message carrying the given usage.
func assistantLine(id string, sidechain bool, input, cacheCreation, cacheRead, output int) string {
	return fmt.Sprintf(`{"type":"assistant","isSidechain":%t,"message":{"id":%q,"usage":{"input_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"output_tokens":%d}}}`,
		sidechain, id, input, cacheCreation, cacheRead, output)
}

// TestSessionUsage_SumsParentAndForks covers the session-usage reading over fixture transcripts: a message repeated across lines counts once and sidechain and malformed lines are skipped, forks are summed into the totals and broken out, a session without a subagents directory has zero forks, and a missing transcript reads unknown.
func TestSessionUsage_SumsParentAndForks(t *testing.T) {
	const workdir = "/home/op/usage"
	const sessionID = "sess-usage"

	parentLines := []string{
		assistantLine("m1", false, 1, 10, 100, 5),
		assistantLine("m1", false, 1, 10, 100, 5),
		assistantLine("m2", false, 2, 20, 200, 7),
		assistantLine("side", true, 999, 999, 999, 999),
		`not json`,
	}
	forkLines := func(id string, input, cacheRead int) []string {
		return []string{
			assistantLine("m2", true, 2, 20, 200, 7),
			assistantLine(id, true, input, 0, cacheRead, 1),
		}
	}

	tests := []struct {
		name        string
		parent      []string
		forks       map[string][]string
		noTranscript bool
		want        shuttleengine.SessionUsage
	}{
		{
			name:   "repeated message lines count once",
			parent: parentLines,
			want:   shuttleengine.SessionUsage{Known: true, Fresh: 16 + 29, CacheRead: 300},
		},
		{
			name:   "two forks are summed and broken out",
			parent: parentLines,
			forks:  map[string][]string{"a.jsonl": forkLines("f1", 4, 40), "b.jsonl": forkLines("f2", 6, 60)},
			want: shuttleengine.SessionUsage{
				Known: true, Fresh: 16 + 29 + 5 + 7, CacheRead: 300 + 100,
				Forks: 2, ForkFresh: 12, ForkCacheRead: 100,
			},
		},
		{
			name:        "a missing transcript reads unknown",
			noTranscript: true,
			want:        shuttleengine.SessionUsage{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			projectDir := filepath.Join(home, ".claude", "projects", encodeForTest(workdir))
			if !tt.noTranscript {
				writeParentTranscript(t, projectDir, sessionID, tt.parent)
			}
			for name, lines := range tt.forks {
				writeForkTranscript(t, filepath.Join(projectDir, sessionID, "subagents"), name, lines)
			}

			if got := New().SessionUsage(sessionID, workdir); got != tt.want {
				t.Errorf("SessionUsage() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

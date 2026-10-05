package claudeengine

import (
	"bytes"
	"encoding/json"
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
			got, found := compactedSince(bytes.NewReader(data), int64(len(data)), chunk, tc.since)
			if found != tc.wantFound || !got.Equal(tc.want) {
				t.Errorf("%s chunk %d: got %v, %v; want %v, %v", tc.name, chunk, got, found, tc.want, tc.wantFound)
			}
		}
	}
}

func TestCompactedSince_IgnoresBoundaryAtOrBeforeSince(t *testing.T) {
	data := []byte("{\"type\":\"system\",\"subtype\":\"compact_boundary\",\"isSidechain\":false,\"timestamp\":\"2026-03-04T01:00:00Z\",\"compactMetadata\":{\"postTokens\":10}}\n")
	since := time.Date(2026, 3, 4, 1, 0, 0, 0, time.UTC)
	if got, found := compactedSince(bytes.NewReader(data), int64(len(data)), 4096, since); found {
		t.Errorf("boundary at since reported found at %v", got)
	}
}

func TestCompactedSince_NoBoundaryInTranscript(t *testing.T) {
	c := &Claude{}
	got, found := c.CompactedSince(stopEventFor(t, fixturePath(t, "usage-main-chain.jsonl")), time.Time{})
	if found || !got.IsZero() {
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
	if want := time.Date(2026, 3, 4, 4, 0, 0, 0, time.UTC); !found || !got.Equal(want) {
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
		if got, found := c.CompactedSince(ev, time.Time{}); found || !got.IsZero() {
			t.Errorf("%s: got %v, %v; want not found", name, got, found)
		}
	}
}

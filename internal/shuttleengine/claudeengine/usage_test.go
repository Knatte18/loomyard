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

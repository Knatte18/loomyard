package claudeengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
	got, ok := c.ContextTokens(stopEventFor(t, fixturePath(t, "usage-main-chain.jsonl")))
	if !ok || got != 3+40+5000 {
		t.Fatalf("got (%d, %v), want (5043, true)", got, ok)
	}
}

func TestContextTokens_IgnoresSidechainAndZeroUsage(t *testing.T) {
	c := &Claude{}
	got, ok := c.ContextTokens(stopEventFor(t, fixturePath(t, "usage-sidechain-last.jsonl")))
	if !ok || got != 7+70+700 {
		t.Fatalf("got (%d, %v), want (777, true)", got, ok)
	}
}

func TestContextTokens_FollowsTranscriptNamedByEachStop(t *testing.T) {
	c := &Claude{}
	before, ok := c.ContextTokens(stopEventFor(t, fixturePath(t, "usage-main-chain.jsonl")))
	if !ok || before != 5043 {
		t.Fatalf("before clear: got (%d, %v), want (5043, true)", before, ok)
	}
	after, ok := c.ContextTokens(stopEventFor(t, fixturePath(t, "usage-after-clear.jsonl")))
	if !ok || after != 2+30+400 {
		t.Fatalf("after clear: got (%d, %v), want (432, true)", after, ok)
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
		if got, ok := c.ContextTokens(ev); ok || got != 0 {
			t.Errorf("%s: got (%d, %v), want (0, false)", name, got, ok)
		}
	}
}

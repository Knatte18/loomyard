// prober_test.go covers Claude's SessionProber: process liveness over a temp session registry with injected liveness and start-time probes,
// and the interrupt marker over temp transcripts named by a turn start's payload.

package claudeengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestProcessLiveness(t *testing.T) {
	t.Parallel()

	entry := func(pid int, sessionID, procStart string) string {
		return fmt.Sprintf(`{"pid":%d,"sessionId":%q,"procStart":%q}`, pid, sessionID, procStart)
	}
	aliveAll := func(int) bool { return true }
	deadAll := func(int) bool { return false }
	startsAt := func(value string, ok bool) func(int) (string, bool) {
		return func(int) (string, bool) { return value, ok }
	}

	tests := []struct {
		name   string
		files  map[string]string
		noDir  bool
		alive  func(int) bool
		start  func(int) (string, bool)
		wanted shuttleengine.Liveness
	}{
		{
			name:   "live pid with a matching start time is alive",
			files:  map[string]string{"a.json": entry(4242, resumeTestID, "100")},
			alive:  aliveAll,
			start:  startsAt("100", true),
			wanted: shuttleengine.LivenessAlive,
		},
		{
			name:   "a pid that is gone is dead",
			files:  map[string]string{"a.json": entry(4242, resumeTestID, "100")},
			alive:  deadAll,
			start:  startsAt("", false),
			wanted: shuttleengine.LivenessDead,
		},
		{
			name:   "a live pid with a different start time is a reused pid and unproven",
			files:  map[string]string{"a.json": entry(4242, resumeTestID, "99")},
			alive:  aliveAll,
			start:  startsAt("100", true),
			wanted: shuttleengine.LivenessUnproven,
		},
		{
			name:   "an unreadable start time is unproven",
			files:  map[string]string{"a.json": entry(4242, resumeTestID, "100")},
			alive:  aliveAll,
			start:  startsAt("", false),
			wanted: shuttleengine.LivenessUnproven,
		},
		{
			name:   "an entry without procStart is unproven",
			files:  map[string]string{"a.json": fmt.Sprintf(`{"pid":4242,"sessionId":%q}`, resumeTestID)},
			alive:  aliveAll,
			start:  startsAt("100", true),
			wanted: shuttleengine.LivenessUnproven,
		},
		{
			name:   "no entry for the session is unproven",
			files:  map[string]string{"a.json": entry(4242, "ffffffff-ffff-4fff-8fff-ffffffffffff", "100")},
			alive:  deadAll,
			start:  startsAt("", false),
			wanted: shuttleengine.LivenessUnproven,
		},
		{
			name:   "an undecodable entry leaves a dead one unproven",
			files:  map[string]string{"a.json": entry(4242, resumeTestID, "100"), "bad.json": "{not json"},
			alive:  deadAll,
			start:  startsAt("", false),
			wanted: shuttleengine.LivenessUnproven,
		},
		{
			name:   "an unreadable registry is unproven",
			noDir:  true,
			alive:  deadAll,
			start:  startsAt("", false),
			wanted: shuttleengine.LivenessUnproven,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			registryDir := filepath.Join(t.TempDir(), "sessions")
			if !tt.noDir {
				if err := os.MkdirAll(registryDir, 0o755); err != nil {
					t.Fatal(err)
				}
				for name, content := range tt.files {
					writeRegistryEntry(t, registryDir, name, content)
				}
			}
			if got := processLiveness(resumeTestID, registryDir, tt.alive, tt.start); got != tt.wanted {
				t.Fatalf("processLiveness = %v, want %v", got, tt.wanted)
			}
		})
	}
}

func TestTurnStartInterrupt(t *testing.T) {
	t.Parallel()

	const (
		interruptAt = "2026-10-08T07:00:05.250Z"
		markerLine  = `{"type":"user","timestamp":"` + interruptAt + `","message":{"content":[{"type":"text","text":"[Request interrupted by user]"}]}}`
		promptLine  = `{"type":"user","timestamp":"2026-10-08T07:00:00Z","message":{"content":"do the thing"}}`
		laterPrompt = `{"type":"user","timestamp":"2026-10-08T07:00:09Z","message":{"content":"carry on"}}`
		laterAnswer = `{"type":"assistant","timestamp":"2026-10-08T07:00:09Z","message":{"stop_reason":"end_turn","content":[{"type":"text","text":"ok"}]}}`
		sidechain   = `{"type":"user","isSidechain":true,"timestamp":"2026-10-08T07:00:09Z","message":{"content":"[Request interrupted by user]"}}`
		bookkeeping = `{"type":"system","subtype":"turn_duration","timestamp":"2026-10-08T07:00:09Z"}`
	)
	wantAt, err := time.Parse(time.RFC3339Nano, interruptAt)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		transcript  []string
		noPath      bool
		missingFile bool
		wantAt      time.Time
		wantFound   bool
	}{
		{name: "marker as the newest entry is an interrupt with its own time", transcript: []string{promptLine, markerLine}, wantAt: wantAt, wantFound: true},
		{name: "unrelated entries after the marker do not hide it", transcript: []string{promptLine, markerLine, sidechain, bookkeeping, "{not json"}, wantAt: wantAt, wantFound: true},
		{name: "a later prompt means no interrupt", transcript: []string{promptLine, markerLine, laterPrompt}},
		{name: "a later assistant entry means no interrupt", transcript: []string{promptLine, markerLine, laterAnswer}},
		{name: "a marker on a sidechain entry is ignored", transcript: []string{promptLine, sidechain}},
		{name: "an unparsable marker timestamp is no interrupt", transcript: []string{`{"type":"user","timestamp":"soon","message":{"content":"[Request interrupted by user]"}}`}},
		{name: "a missing transcript_path is no interrupt", noPath: true},
		{name: "an unreadable transcript is no interrupt", missingFile: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			payload := map[string]any{"hook_event_name": "UserPromptSubmit"}
			if !tt.noPath {
				path := filepath.Join(t.TempDir(), "transcript.jsonl")
				if !tt.missingFile {
					if err := os.WriteFile(path, []byte(strings.Join(tt.transcript, "\n")+"\n"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				payload["transcript_path"] = path
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			at, interrupted := (&Claude{}).TurnStartInterrupt(shuttleengine.SessionSignal{Kind: shuttleengine.SessionSignalTurnStart, Raw: raw})
			if interrupted != tt.wantFound || !at.Equal(tt.wantAt) {
				t.Fatalf("TurnStartInterrupt = (%v, %v), want (%v, %v)", at, interrupted, tt.wantAt, tt.wantFound)
			}
		})
	}
}

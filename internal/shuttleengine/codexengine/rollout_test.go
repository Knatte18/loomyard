// rollout_test.go reads rollout fixtures for the interrupt reading of a turn start and the context size of a turn end.

package codexengine

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// rolloutLine returns one event_msg line of a rollout at timestamp, its payload the JSON object body payload.
func rolloutLine(timestamp, payload string) string {
	return `{"timestamp":"` + timestamp + `","type":"event_msg","payload":{` + payload + `}}`
}

// tokenCountLine returns a token_count line whose latest call totals total tokens.
func tokenCountLine(timestamp string, total int) string {
	return rolloutLine(timestamp, `"type":"token_count","info":{"total_token_usage":{"input_tokens":99999,"total_tokens":99999},"last_token_usage":{"input_tokens":13866,"cached_input_tokens":11392,"output_tokens":5,"total_tokens":`+strconv.Itoa(total)+`},"model_context_window":258400},"rate_limits":{"limit_id":"codex"}`)
}

// writeRollout writes lines as a rollout file and returns its path.
func writeRollout(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	writeFile(t, path, strings.Join(lines, "\n")+"\n")
	return path
}

// TestTurnStartInterrupt pins the interrupt reading.
// A newest turn_aborted of the turn reads interrupted at its time.
// A task_complete after it or a turn_aborted of another turn does not.
// An event older than an id-less turn start is not the turn's, and an unreadable rollout reads not interrupted.
func TestTurnStartInterrupt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	abortedAt := time.Date(2026, 10, 24, 11, 58, 2, 500_000_000, time.UTC)
	tests := []struct {
		name    string
		lines   []string
		turnID  string
		startAt time.Time
		want    bool
	}{
		{
			name:   "AbortedTurn",
			lines:  []string{rolloutLine("2026-10-24T11:58:00Z", `"type":"task_started","turn_id":"t1"`), rolloutLine("2026-10-24T11:58:02.5Z", `"type":"turn_aborted","turn_id":"t1","reason":"interrupted"`)},
			turnID: "t1",
			want:   true,
		},
		{
			name:   "CompletedAfterAbort",
			lines:  []string{rolloutLine("2026-10-24T11:58:02.5Z", `"type":"turn_aborted","turn_id":"t1"`), rolloutLine("2026-10-24T11:58:05Z", `"type":"task_complete","turn_id":"t1"`)},
			turnID: "t1",
		},
		{
			name:   "AbortOfAnotherTurn",
			lines:  []string{rolloutLine("2026-10-24T11:58:02.5Z", `"type":"turn_aborted","turn_id":"t0"`)},
			turnID: "t1",
		},
		{
			name:    "IdlessAbortBeforeTheTurnStart",
			lines:   []string{rolloutLine("2026-10-24T11:58:02.5Z", `"type":"turn_aborted"`)},
			startAt: time.Date(2026, 10, 24, 11, 59, 0, 0, time.UTC),
		},
		{
			name:    "IdlessAbortAfterTheTurnStart",
			lines:   []string{rolloutLine("2026-10-24T11:58:02.5Z", `"type":"turn_aborted"`)},
			startAt: time.Date(2026, 10, 24, 11, 58, 0, 0, time.UTC),
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeRollout(t, tt.lines...)
			raw := `{"hook_event_name":"UserPromptSubmit","transcript_path":"` + path + `","turn_id":"` + tt.turnID + `"}`
			at, interrupted := New().TurnStartInterrupt(shuttleengine.SessionSignal{Kind: shuttleengine.SessionSignalTurnStart, At: tt.startAt, Raw: []byte(raw)})
			if interrupted != tt.want {
				t.Fatalf("TurnStartInterrupt() interrupted = %v; want %v", interrupted, tt.want)
			}
			if tt.want && !at.Equal(abortedAt) {
				t.Errorf("TurnStartInterrupt() at = %v; want %v", at, abortedAt)
			}
		})
	}
	if _, interrupted := New().TurnStartInterrupt(shuttleengine.SessionSignal{Raw: []byte(`{"transcript_path":"/absent/rollout.jsonl"}`)}); interrupted {
		t.Error("TurnStartInterrupt() over an absent rollout = interrupted; want not")
	}
}

// TestContextTokens pins that the context size is the newest token_count's latest call total.
// A rollout without one, or none at all, reads unknown.
func TestContextTokens(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		lines []string
		want  shuttleengine.ContextReading
	}{
		{
			name:  "NewestTokenCount",
			lines: []string{tokenCountLine("2026-10-24T11:55:31Z", 13871), rolloutLine("2026-10-24T11:55:32Z", `"type":"token_count","info":null`), tokenCountLine("2026-10-24T11:56:00Z", 20000), rolloutLine("2026-10-24T11:56:01Z", `"type":"task_complete"`)},
			want:  shuttleengine.ContextReading{Tokens: 20000, Known: true},
		},
		{
			name:  "NoTokenCount",
			lines: []string{rolloutLine("2026-10-24T11:56:01Z", `"type":"task_complete"`)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeRollout(t, tt.lines...)
			got := New().ContextTokens(shuttleengine.Event{Raw: []byte(`{"hook_event_name":"Stop","transcript_path":"` + path + `"}`)})
			if got != tt.want {
				t.Errorf("ContextTokens() = %+v; want %+v", got, tt.want)
			}
		})
	}
	if got := New().ContextTokens(shuttleengine.Event{Raw: []byte(`{"transcript_path":"/absent/rollout.jsonl"}`)}); got.Known {
		t.Errorf("ContextTokens() over an absent rollout = %+v; want unknown", got)
	}
}

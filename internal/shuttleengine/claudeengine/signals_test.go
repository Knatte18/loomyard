package claudeengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// transcriptPlaceholder in a chunk is replaced by the JSON-encoded path of the row's transcript file.
const transcriptPlaceholder = `"TRANSCRIPT"`

// signalStamp builds one stamp line for hook with the given lyx_at text.
func signalStamp(hook, at string) string {
	return fmt.Sprintf(`{"lyx_stamp":%q,"lyx_at":%q}`+"\n", hook, at)
}

// signalPayload builds one newline-terminated payload line from fields plus the hook name and a session id.
func signalPayload(t *testing.T, hook string, fields map[string]any) string {
	t.Helper()
	payload := map[string]any{"hook_event_name": hook, "session_id": "sess-1"}
	for key, value := range fields {
		payload[key] = value
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(b) + "\n"
}

// TestParseSessionSignals covers how hook lines become session signals.
// Its rows cover each hook's kind and fields, the stamp pairing and its zero-time cases, the held trailing stamps across incremental reads, and the background tasks a turn end reads from the transcript at parse time.
// Every row feeds its chunks one at a time, each call resuming at the previous call's consumed, and the signals of all calls must equal one whole-data call's.
func TestParseSessionSignals(t *testing.T) {
	t.Parallel()

	const stampTime = "2026-10-08T09:00:00Z"
	const otherStampTime = "2026-10-08T09:00:05Z"
	at := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	otherAt := time.Date(2026, 10, 8, 9, 0, 5, 0, time.UTC)

	stop := signalPayload(t, "Stop", map[string]any{"last_assistant_message": "done"})
	userPrompt := signalPayload(t, "UserPromptSubmit", nil)
	runningShell := []any{map[string]any{"id": "bsh1", "type": "shell", "status": "running", "command": "sleep 600"}}
	askTool := signalPayload(t, "PreToolUse", map[string]any{"tool_name": "AskUserQuestion", "tool_input": map[string]any{"questions": []any{map[string]any{"question": "which?"}}}})
	transcriptStop := signalPayload(t, "Stop", map[string]any{"transcript_path": "TRANSCRIPT"})

	tests := []struct {
		name string
		// chunks are appended to the data one at a time.
		chunks []string
		want   []shuttleengine.SessionSignal
		// wantUnconsumed is the data left after the last call's consumed.
		wantUnconsumed string
		// transcript, when non-empty, is written as the transcript file the chunks' TRANSCRIPT placeholder names;
		// appendToTranscript is then appended and the whole data parsed again, which must read wantAfterAppend.
		transcript         string
		appendToTranscript string
		wantAfterAppend    []shuttleengine.SessionSignal
	}{
		{
			name:   "stop_with_stamp_is_a_turn_end_with_its_hook_time",
			chunks: []string{signalStamp("Stop", stampTime) + stop},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalTurnEnd, At: at, SessionID: "sess-1", Event: true}},
		},
		{
			name:   "stop_carries_payload_reported_background_tasks",
			chunks: []string{signalStamp("Stop", stampTime) + signalPayload(t, "Stop", map[string]any{"background_tasks": runningShell})},
			want: []shuttleengine.SessionSignal{{
				Kind: shuttleengine.SessionSignalTurnEnd, At: at, SessionID: "sess-1", Event: true,
				Outstanding: []shuttleengine.BackgroundTask{{Kind: shuttleengine.BackgroundShell, ID: "bsh1", Label: "sleep 600", Signal: shuttleengine.SignalPayload}},
			}},
		},
		{
			name:   "stop_failure_text_from_error",
			chunks: []string{signalStamp("StopFailure", stampTime) + signalPayload(t, "StopFailure", map[string]any{"error": "rate_limit", "error_details": "d", "last_assistant_message": "m"})},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalAPIErrorTurnEnd, At: at, SessionID: "sess-1", ErrorText: "rate_limit"}},
		},
		{
			name:   "stop_failure_text_falls_back_to_error_details",
			chunks: []string{signalStamp("StopFailure", stampTime) + signalPayload(t, "StopFailure", map[string]any{"error_details": "details", "last_assistant_message": "m"})},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalAPIErrorTurnEnd, At: at, SessionID: "sess-1", ErrorText: "details"}},
		},
		{
			name:   "stop_failure_text_falls_back_to_last_assistant_message",
			chunks: []string{signalStamp("StopFailure", stampTime) + signalPayload(t, "StopFailure", map[string]any{"last_assistant_message": "API Error: 529"})},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalAPIErrorTurnEnd, At: at, SessionID: "sess-1", ErrorText: "API Error: 529"}},
		},
		{
			name:   "user_prompt_submit_is_a_turn_start",
			chunks: []string{signalStamp("UserPromptSubmit", stampTime) + userPrompt},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalTurnStart, At: at, SessionID: "sess-1"}},
		},
		{
			name:   "ask_user_question_pre_tool_use_is_an_ask_with_an_event",
			chunks: []string{signalStamp("PreToolUse", stampTime) + askTool},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalAsk, At: at, SessionID: "sess-1", Event: true}},
		},
		{
			name:   "other_tool_pre_tool_use_yields_nothing",
			chunks: []string{signalStamp("PreToolUse", stampTime) + signalPayload(t, "PreToolUse", map[string]any{"tool_name": "Bash"})},
		},
		{
			name:   "permission_prompt_notification_is_an_ask_without_an_event",
			chunks: []string{signalStamp("Notification", stampTime) + signalPayload(t, "Notification", map[string]any{"notification_type": "permission_prompt"})},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalAsk, At: at, SessionID: "sess-1"}},
		},
		{
			name:   "elicitation_dialog_notification_is_an_ask",
			chunks: []string{signalStamp("Notification", stampTime) + signalPayload(t, "Notification", map[string]any{"notification_type": "elicitation_dialog"})},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalAsk, At: at, SessionID: "sess-1"}},
		},
		{
			name:   "idle_prompt_notification_is_an_idle_notice",
			chunks: []string{signalStamp("Notification", stampTime) + signalPayload(t, "Notification", map[string]any{"notification_type": "idle_prompt"})},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalIdleNotice, At: at, SessionID: "sess-1"}},
		},
		{
			name:   "other_notification_type_yields_nothing",
			chunks: []string{signalStamp("Notification", stampTime) + signalPayload(t, "Notification", map[string]any{"notification_type": "auth_success"})},
		},
		{
			name: "session_end_reasons_decide_whether_the_process_ends",
			chunks: []string{
				signalPayload(t, "SessionEnd", map[string]any{"reason": "logout"}),
				signalPayload(t, "SessionEnd", map[string]any{"reason": "prompt_input_exit"}),
				signalPayload(t, "SessionEnd", map[string]any{"reason": "other"}),
				signalPayload(t, "SessionEnd", map[string]any{"reason": "clear"}),
				signalPayload(t, "SessionEnd", map[string]any{"reason": "resume"}),
				signalPayload(t, "SessionEnd", map[string]any{"reason": "something_new"}),
			},
			want: []shuttleengine.SessionSignal{
				{Kind: shuttleengine.SessionSignalSessionEnd, SessionID: "sess-1", Reason: "logout", EndsProcess: true},
				{Kind: shuttleengine.SessionSignalSessionEnd, SessionID: "sess-1", Reason: "prompt_input_exit", EndsProcess: true},
				{Kind: shuttleengine.SessionSignalSessionEnd, SessionID: "sess-1", Reason: "other", EndsProcess: true},
				{Kind: shuttleengine.SessionSignalSessionEnd, SessionID: "sess-1", Reason: "clear"},
				{Kind: shuttleengine.SessionSignalSessionEnd, SessionID: "sess-1", Reason: "resume"},
				{Kind: shuttleengine.SessionSignalSessionEnd, SessionID: "sess-1", Reason: "something_new"},
			},
		},
		{
			name:   "session_start_is_a_signal_carrying_its_source",
			chunks: []string{signalStamp("SessionStart", stampTime) + signalPayload(t, "SessionStart", map[string]any{"source": "compact"})},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalSessionStart, At: at, SessionID: "sess-1", Source: "compact"}},
		},
		{
			name: "two_stamps_pair_each_payload_with_its_own_hook",
			chunks: []string{
				signalStamp("UserPromptSubmit", stampTime) + signalStamp("Stop", otherStampTime) + stop + userPrompt,
			},
			want: []shuttleengine.SessionSignal{
				{Kind: shuttleengine.SessionSignalTurnEnd, At: otherAt, SessionID: "sess-1", Event: true},
				{Kind: shuttleengine.SessionSignalTurnStart, At: at, SessionID: "sess-1"},
			},
		},
		{
			name:   "payload_without_a_stamp_has_a_zero_time",
			chunks: []string{stop},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalTurnEnd, SessionID: "sess-1", Event: true}},
		},
		{
			name:   "empty_stamp_time_reads_a_zero_time",
			chunks: []string{signalStamp("Stop", "") + stop},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalTurnEnd, SessionID: "sess-1", Event: true}},
		},
		{
			name:   "malformed_stamp_time_reads_a_zero_time",
			chunks: []string{signalStamp("Stop", "yesterday-ish") + stop},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalTurnEnd, SessionID: "sess-1", Event: true}},
		},
		{
			name:   "stamp_split_from_its_payload_across_two_calls_still_pairs",
			chunks: []string{signalStamp("Stop", stampTime), stop},
			want:   []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalTurnEnd, At: at, SessionID: "sess-1", Event: true}},
		},
		{
			name:           "trailing_stamp_is_left_unconsumed",
			chunks:         []string{signalStamp("Stop", stampTime)},
			wantUnconsumed: signalStamp("Stop", stampTime),
		},
		{
			name:           "two_trailing_stamps_are_both_left_unconsumed",
			chunks:         []string{signalStamp("UserPromptSubmit", stampTime) + signalStamp("Stop", otherStampTime)},
			wantUnconsumed: signalStamp("UserPromptSubmit", stampTime) + signalStamp("Stop", otherStampTime),
		},
		{
			name: "two_trailing_stamps_pair_when_both_payloads_arrive_later",
			chunks: []string{
				signalStamp("UserPromptSubmit", stampTime) + signalStamp("Stop", otherStampTime),
				userPrompt + stop,
			},
			want: []shuttleengine.SessionSignal{
				{Kind: shuttleengine.SessionSignalTurnStart, At: at, SessionID: "sess-1"},
				{Kind: shuttleengine.SessionSignalTurnEnd, At: otherAt, SessionID: "sess-1", Event: true},
			},
		},
		{
			name:   "stamp_lines_and_unknown_hooks_yield_nothing",
			chunks: []string{signalStamp("PostToolUse", stampTime) + signalPayload(t, "PostToolUse", nil) + "not json\n"},
		},
		{
			name:           "unterminated_last_line_is_left_unconsumed",
			chunks:         []string{strings.TrimSuffix(stop, "\n")},
			wantUnconsumed: strings.TrimSuffix(stop, "\n"),
		},
		{
			name:       "transcript_fallback_tasks_are_read_when_parsed",
			chunks:     []string{signalStamp("Stop", stampTime) + transcriptStop},
			transcript: monitorLaunchTranscript,
			want: []shuttleengine.SessionSignal{{
				Kind: shuttleengine.SessionSignalTurnEnd, At: at, SessionID: "sess-1", Event: true,
				Outstanding: []shuttleengine.BackgroundTask{{Kind: shuttleengine.BackgroundShell, ID: "mon12345", Label: "tail -f x", Signal: shuttleengine.SignalTranscript}},
			}},
			appendToTranscript: taskNotification,
			wantAfterAppend:    []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalTurnEnd, At: at, SessionID: "sess-1", Event: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			substitute := func(chunk string) string { return chunk }
			transcriptPath := ""
			if tt.transcript != "" {
				transcriptPath = filepath.Join(t.TempDir(), "transcript.jsonl")
				if err := os.WriteFile(transcriptPath, []byte(tt.transcript), 0o644); err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(transcriptPath)
				if err != nil {
					t.Fatal(err)
				}
				substitute = func(chunk string) string { return strings.ReplaceAll(chunk, transcriptPlaceholder, string(encoded)) }
			}

			claude := &Claude{}
			var data []byte
			var got []shuttleengine.SessionSignal
			consumed := 0
			for _, chunk := range tt.chunks {
				data = append(data, substitute(chunk)...)
				signals, n := claude.ParseSessionSignals(data[consumed:])
				got = append(got, signals...)
				consumed += n
			}
			assertSignals(t, "incremental calls", got, tt.want)
			if unconsumed := string(data[consumed:]); unconsumed != substitute(tt.wantUnconsumed) {
				t.Errorf("unconsumed data = %q; want %q", unconsumed, substitute(tt.wantUnconsumed))
			}

			whole, _ := claude.ParseSessionSignals(data)
			assertSignals(t, "one whole-data call", whole, tt.want)

			if tt.appendToTranscript != "" {
				file, err := os.OpenFile(transcriptPath, os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := file.WriteString(tt.appendToTranscript); err != nil {
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
				again, _ := claude.ParseSessionSignals(data)
				assertSignals(t, "whole-data call after the transcript grew", again, tt.wantAfterAppend)
			}
		})
	}
}

// assertSignals fails when got differs from want in every field but Raw, and when a signal's Raw is not the JSON line it was read from.
func assertSignals(t *testing.T, label string, got, want []shuttleengine.SessionSignal) {
	t.Helper()
	stripped := make([]shuttleengine.SessionSignal, len(got))
	for i, signal := range got {
		if !json.Valid(signal.Raw) || strings.Contains(string(signal.Raw), "\n") {
			t.Errorf("%s: signal %d Raw = %q; want one JSON line", label, i, signal.Raw)
		}
		signal.Raw = nil
		stripped[i] = signal
	}
	if len(stripped) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(stripped, want) {
		t.Errorf("%s: signals = %+v; want %+v", label, stripped, want)
	}
}

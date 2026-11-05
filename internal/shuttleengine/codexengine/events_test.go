// events_test.go parses events files built from the spike's recorded hook payloads into events and session signals.

package codexengine

import (
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// Hook payloads as the spike recorded them (docs/research/codex/codex-spike-2026-10-24/report.md, Hook payloads), with the transcript path made absolute.
const (
	sessionStartPayload      = `{"session_id":"01a16d56","transcript_path":"/s/rollout.jsonl","cwd":"/w","hook_event_name":"SessionStart","model":"gpt-6-luna","permission_mode":"bypassPermissions","source":"startup"}`
	userPromptSubmitPayload  = `{"session_id":"01a16d56","turn_id":"t1","transcript_path":"/s/rollout.jsonl","cwd":"/w","hook_event_name":"UserPromptSubmit","model":"gpt-6-luna","permission_mode":"bypassPermissions","prompt":"Reply with the single word ok."}`
	stopPayload              = `{"session_id":"01a16d56","turn_id":"t1","transcript_path":"/s/rollout.jsonl","cwd":"/w","hook_event_name":"Stop","model":"gpt-6-luna","permission_mode":"bypassPermissions","stop_hook_active":false,"last_assistant_message":"ok"}`
	permissionRequestPayload = `{"session_id":"01a16d5d","turn_id":"t2","transcript_path":"/s/rollout.jsonl","cwd":"/w","hook_event_name":"PermissionRequest","model":"gpt-6-luna","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"touch /x"}}`
	preToolUsePayload        = `{"session_id":"01a16d5e","turn_id":"t3","transcript_path":"/s/rollout.jsonl","cwd":"/w","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"echo hi"}}`
)

// stampLine returns the stamp line a recording hook writes before the payload of hook at at.
func stampLine(hook, at string) string {
	return `{"` + stampHookKey + `":"` + hook + `","` + stampAtKey + `":"` + at + `"}`
}

// TestParseEvents pins that each Stop payload becomes a turn end carrying its message and that stamp lines, other hooks and malformed lines yield nothing.
func TestParseEvents(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	data := stampLine(hookEventStop, "2026-10-24T11:55:31Z") + "\n" + sessionStartPayload + "\n" + "not json\n" + preToolUsePayload + "\n" + stopPayload + "\n"
	got, err := New().ParseEvents([]byte(data))
	if err != nil {
		t.Fatalf("ParseEvents() error: %v", err)
	}
	want := []shuttleengine.Event{{Kind: shuttleengine.EventStop, Message: "ok", Raw: []byte(stopPayload)}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseEvents() = %+v; want %+v", got, want)
	}
}

// TestParseSessionSignals pins the four hooks' signals with each payload's stamp time and session id, and the exit record read as a process-ending session end with no time.
// It also pins a slash command read as no instruction and a fork's source read as none.
// It pins consumed stopping before a trailing stamp with no payload yet.
func TestParseSessionSignals(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	at := func(text string) time.Time {
		parsed, err := time.Parse(time.RFC3339, text)
		if err != nil {
			t.Fatalf("parse %s: %v", text, err)
		}
		return parsed
	}
	slashPrompt := `{"session_id":"s","hook_event_name":"UserPromptSubmit","prompt":"/status"}`
	forkStart := `{"session_id":"s","hook_event_name":"SessionStart","source":"fork"}`
	lines := []string{
		stampLine(hookEventSessionStart, "2026-10-24T11:55:30Z"), sessionStartPayload,
		stampLine(hookEventUserPromptSubmit, "2026-10-24T11:55:30Z"), userPromptSubmitPayload,
		stampLine(hookEventStop, "2026-10-24T11:55:31Z"), stopPayload,
		stampLine(hookEventPermissionRequest, ""), permissionRequestPayload,
		preToolUsePayload, slashPrompt, forkStart, exitRecordLine,
	}
	complete := ""
	for _, line := range lines {
		complete += line + "\n"
	}
	trailing := stampLine(hookEventStop, "2026-10-24T11:56:00Z") + "\n"

	got, consumed := New().ParseSessionSignals([]byte(complete + trailing))
	if consumed != len(complete) {
		t.Errorf("consumed = %d; want %d, before the trailing stamp", consumed, len(complete))
	}
	want := []shuttleengine.SessionSignal{
		{Kind: shuttleengine.SessionSignalSessionStart, Source: shuttleengine.SessionStartStartup, At: at("2026-10-24T11:55:30Z"), SessionID: "01a16d56", Raw: []byte(sessionStartPayload)},
		{Kind: shuttleengine.SessionSignalTurnStart, Instruction: "Reply with the single word ok.", At: at("2026-10-24T11:55:30Z"), SessionID: "01a16d56", Raw: []byte(userPromptSubmitPayload)},
		{Kind: shuttleengine.SessionSignalTurnEnd, Event: true, At: at("2026-10-24T11:55:31Z"), SessionID: "01a16d56", Raw: []byte(stopPayload)},
		{Kind: shuttleengine.SessionSignalAsk, SessionID: "01a16d5d", Raw: []byte(permissionRequestPayload)},
		{Kind: shuttleengine.SessionSignalTurnStart, SessionID: "s", Raw: []byte(slashPrompt)},
		{Kind: shuttleengine.SessionSignalSessionStart, SessionID: "s", Raw: []byte(forkStart)},
		{Kind: shuttleengine.SessionSignalSessionEnd, Reason: exitRecordReason, EndsProcess: true, Raw: []byte(exitRecordLine)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseSessionSignals() =\n%+v\nwant\n%+v", got, want)
	}
}

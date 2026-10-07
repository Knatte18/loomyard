package claudeengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const monitorLaunchTranscript = `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Monitor","input":{"command":"tail -f x"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"Monitor started (task mon12345)"}]},"toolUseResult":{"taskId":"mon12345"}}
`

const taskNotification = `{"type":"user","message":{"content":"<task-notification><task-id>mon12345</task-id><status>completed</status></task-notification>"}}
`

// stopLine builds one events.jsonl Stop line.
func stopLine(t *testing.T, transcript string, tasks any) []byte {
	t.Helper()
	fields := map[string]any{
		"hook_event_name":        "Stop",
		"last_assistant_message": "waiting on it",
	}
	if transcript != "" {
		fields["transcript_path"] = transcript
	}
	if tasks != nil {
		fields["background_tasks"] = tasks
	}
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeBgTranscript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func parseOneKind(t *testing.T, line []byte) shuttleengine.Event {
	t.Helper()
	events, err := (&Claude{}).ParseEvents(line)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	return events[0]
}

func TestParseEvents_StopWithRunningBackgroundTaskIsWaiting(t *testing.T) {
	tasks := []any{map[string]any{"id": "a1", "type": "subagent", "status": "running"}}
	ev := parseOneKind(t, stopLine(t, "", tasks))
	if ev.Kind != shuttleengine.EventWaiting {
		t.Fatalf("kind = %v, want EventWaiting", ev.Kind)
	}
	if ev.Message != "waiting on it" {
		t.Errorf("Message = %q", ev.Message)
	}
}

func TestParseEvents_StopWithCompletedTasksAndCleanTranscriptIsStop(t *testing.T) {
	tasks := []any{map[string]any{"id": "a1", "type": "subagent", "status": "completed"}}
	path := writeBgTranscript(t, monitorLaunchTranscript+taskNotification)
	ev := parseOneKind(t, stopLine(t, path, tasks))
	if ev.Kind != shuttleengine.EventStop {
		t.Fatalf("kind = %v, want EventStop", ev.Kind)
	}
}

// queueOperationNotification and queuedCommandNotification are the trimmed shapes Claude Code
// writes when a task notification is absorbed mid-turn instead of arriving as a user message.
const queueOperationNotification = `{"type":"queue-operation","operation":"enqueue","content":"<task-notification>\n<task-id>mon12345</task-id>\n<status>completed</status>\n</task-notification>"}
`

const queuedCommandNotification = `{"attachment":{"type":"queued_command","prompt":"<task-notification>\n<task-id>mon12345</task-id>\n<status>completed</status>\n</task-notification>"}}
`

func TestParseEvents_BackgroundTasksKeyIsAuthoritativeOverTranscript(t *testing.T) {
	unmatched := writeBgTranscript(t, monitorLaunchTranscript)
	absorbedByQueueOperation := writeBgTranscript(t, monitorLaunchTranscript+queueOperationNotification)
	absorbedByAttachment := writeBgTranscript(t, monitorLaunchTranscript+queuedCommandNotification)
	notificationBeforeResult := writeBgTranscript(t, queueOperationNotification+monitorLaunchTranscript)
	running := []any{map[string]any{"id": "mon12345", "type": "monitor", "status": "running", "command": "tail -f x"}}
	completed := []any{map[string]any{"id": "mon12345", "type": "monitor", "status": "completed"}}

	tests := []struct {
		name       string
		transcript string
		tasks      any
		want       shuttleengine.EventKind
	}{
		{"absent key reads an unmatched launch", unmatched, nil, shuttleengine.EventWaiting},
		{"non-list value reads an unmatched launch", unmatched, "none", shuttleengine.EventWaiting},
		{"queue-operation absorbs the launch", absorbedByQueueOperation, nil, shuttleengine.EventStop},
		{"queued_command attachment absorbs the launch", absorbedByAttachment, nil, shuttleengine.EventStop},
		{"notification before the tool result absorbs nothing", notificationBeforeResult, nil, shuttleengine.EventWaiting},
		{"non-list value falls back to the transcript", absorbedByQueueOperation, "none", shuttleengine.EventStop},
		{"empty list wins over an unmatched launch", unmatched, []any{}, shuttleengine.EventStop},
		{"empty list over an absorbed launch", absorbedByQueueOperation, []any{}, shuttleengine.EventStop},
		{"non-running entry wins over an unmatched launch", unmatched, completed, shuttleengine.EventStop},
		{"running entry waits over an absorbed launch", absorbedByQueueOperation, running, shuttleengine.EventWaiting},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev := parseOneKind(t, stopLine(t, tc.transcript, tc.tasks))
			if ev.Kind != tc.want {
				t.Fatalf("kind = %v, want %v", ev.Kind, tc.want)
			}
		})
	}
}

func TestParseEvents_MatchingTaskNotificationIsStop(t *testing.T) {
	path := writeBgTranscript(t, monitorLaunchTranscript+taskNotification)
	ev := parseOneKind(t, stopLine(t, path, nil))
	if ev.Kind != shuttleengine.EventStop {
		t.Fatalf("kind = %v, want EventStop", ev.Kind)
	}
}

func TestParseEvents_MissingTranscriptIsStop(t *testing.T) {
	ev := parseOneKind(t, stopLine(t, filepath.Join(t.TempDir(), "absent.jsonl"), nil))
	if ev.Kind != shuttleengine.EventStop {
		t.Fatalf("kind = %v, want EventStop", ev.Kind)
	}
}

func TestTranscriptHasUnmatchedLaunch_BashAndAgentLaunches(t *testing.T) {
	bash := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_b","name":"Bash","input":{"command":"sleep 99","run_in_background":true}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_b","content":"Command running in background with ID: bqx98765"}]}}
`
	got := transcriptHasUnmatchedLaunch([]byte(bash))
	wantBash := shuttleengine.BackgroundTask{Kind: shuttleengine.BackgroundShell, ID: "bqx98765", Label: "sleep 99", Signal: shuttleengine.SignalTranscript}
	if len(got) != 1 || got[0] != wantBash {
		t.Errorf("backgrounded Bash with no notification = %+v, want [%+v]", got, wantBash)
	}
	note := `{"type":"user","message":{"content":"<task-notification><task-id>bqx98765</task-id></task-notification>"}}` + "\n"
	if got := transcriptHasUnmatchedLaunch([]byte(bash + note)); len(got) != 0 {
		t.Errorf("notification naming the id should match the Bash launch, got %+v", got)
	}

	foreground := strings.Replace(bash, `,"run_in_background":true`, "", 1)
	if got := transcriptHasUnmatchedLaunch([]byte(foreground)); len(got) != 0 {
		t.Errorf("foreground Bash is not a background launch, got %+v", got)
	}

	agent := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_a","name":"Agent","input":{"run_in_background":true}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_a","content":"async launched"}]},"toolUseResult":{"agentId":"agent7654321"}}
`
	got = transcriptHasUnmatchedLaunch([]byte(agent))
	wantAgent := shuttleengine.BackgroundTask{Kind: shuttleengine.BackgroundFork, ID: "agent7654321", Signal: shuttleengine.SignalTranscript}
	if len(got) != 1 || got[0] != wantAgent {
		t.Errorf("async Agent launch with no notification = %+v, want [%+v]", got, wantAgent)
	}
}

func TestParseEvents_RunningShellEntryYieldsShellTask(t *testing.T) {
	tasks := []any{map[string]any{"id": "bsh12345", "type": "shell", "status": "running", "command": "sleep 600"}}
	ev := parseOneKind(t, stopLine(t, "", tasks))
	want := []shuttleengine.BackgroundTask{{Kind: shuttleengine.BackgroundShell, ID: "bsh12345", Label: "sleep 600", Signal: shuttleengine.SignalPayload}}
	if ev.Kind != shuttleengine.EventWaiting || !reflect.DeepEqual(ev.Outstanding, want) {
		t.Errorf("kind = %v, Outstanding = %+v, want EventWaiting with %+v", ev.Kind, ev.Outstanding, want)
	}
}

func TestParseEvents_RunningForkEntryCarriesDescriptionAsLabel(t *testing.T) {
	tasks := []any{
		map[string]any{"id": "a1", "type": "subagent", "status": "running", "description": "review the diff"},
		map[string]any{"id": "a2", "type": "subagent", "status": "running"},
	}
	ev := parseOneKind(t, stopLine(t, "", tasks))
	want := []shuttleengine.BackgroundTask{
		{Kind: shuttleengine.BackgroundFork, ID: "a1", Label: "review the diff", Signal: shuttleengine.SignalPayload},
		{Kind: shuttleengine.BackgroundFork, ID: "a2", Signal: shuttleengine.SignalPayload},
	}
	if ev.Kind != shuttleengine.EventWaiting || !reflect.DeepEqual(ev.Outstanding, want) {
		t.Errorf("kind = %v, Outstanding = %+v, want EventWaiting with %+v", ev.Kind, ev.Outstanding, want)
	}
}

func TestParseEvents_ShellEntryLabelFallsBackToDescriptionThenID(t *testing.T) {
	tasks := []any{
		map[string]any{"id": "m1", "type": "monitor", "status": "running", "description": "watch logs"},
		map[string]any{"id": "m2", "type": "monitor", "status": "running"},
	}
	ev := parseOneKind(t, stopLine(t, "", tasks))
	if len(ev.Outstanding) != 2 || ev.Outstanding[0].Label != "watch logs" || ev.Outstanding[1].Label != "m2" {
		t.Errorf("Outstanding = %+v", ev.Outstanding)
	}
}

func TestParseEvents_TranscriptOnlyBashYieldsShellTask(t *testing.T) {
	bash := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_b","name":"Bash","input":{"command":"sleep 99","run_in_background":true}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_b","content":"Command running in background with ID: bqx98765"}]}}
`
	ev := parseOneKind(t, stopLine(t, writeBgTranscript(t, bash), nil))
	want := []shuttleengine.BackgroundTask{{Kind: shuttleengine.BackgroundShell, ID: "bqx98765", Label: "sleep 99", Signal: shuttleengine.SignalTranscript}}
	if ev.Kind != shuttleengine.EventWaiting || !reflect.DeepEqual(ev.Outstanding, want) {
		t.Errorf("kind = %v, Outstanding = %+v, want %+v", ev.Kind, ev.Outstanding, want)
	}
}

func TestParseEvents_PayloadListIsTheWholeOutstandingListAndTranscriptIsUnread(t *testing.T) {
	agent := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_a","name":"Agent","input":{"run_in_background":true}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_a","content":"async launched"}]},"toolUseResult":{"agentId":"agent7654321"}}
`
	tasks := []any{
		map[string]any{"id": "bsh12345", "type": "shell", "status": "running", "command": "sleep 600"},
		map[string]any{"id": "bsh12345", "type": "shell", "status": "running", "command": "sleep 600"},
	}
	ev := parseOneKind(t, stopLine(t, writeBgTranscript(t, agent), tasks))
	want := []shuttleengine.BackgroundTask{{Kind: shuttleengine.BackgroundShell, ID: "bsh12345", Label: "sleep 600", Signal: shuttleengine.SignalPayload}}
	if !reflect.DeepEqual(ev.Outstanding, want) {
		t.Errorf("Outstanding = %+v, want %+v", ev.Outstanding, want)
	}
}

func TestParseEvents_StopWithNothingOutstandingCarriesNoList(t *testing.T) {
	ev := parseOneKind(t, stopLine(t, "", nil))
	if ev.Kind != shuttleengine.EventStop || ev.Outstanding != nil {
		t.Errorf("kind = %v, Outstanding = %+v", ev.Kind, ev.Outstanding)
	}
}

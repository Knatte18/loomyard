package claudeengine

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestParseEvents_UnmatchedMonitorLaunchIsWaiting(t *testing.T) {
	path := writeBgTranscript(t, monitorLaunchTranscript)
	for name, tasks := range map[string]any{"absent": nil, "empty": []any{}} {
		t.Run(name, func(t *testing.T) {
			ev := parseOneKind(t, stopLine(t, path, tasks))
			if ev.Kind != shuttleengine.EventWaiting {
				t.Fatalf("kind = %v, want EventWaiting", ev.Kind)
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
	if !transcriptHasUnmatchedLaunch([]byte(bash)) {
		t.Error("backgrounded Bash with no notification should be outstanding")
	}
	note := `{"type":"user","message":{"content":"<task-notification><task-id>bqx98765</task-id></task-notification>"}}` + "\n"
	if transcriptHasUnmatchedLaunch([]byte(bash + note)) {
		t.Error("notification naming the id should match the Bash launch")
	}

	foreground := strings.Replace(bash, `,"run_in_background":true`, "", 1)
	if transcriptHasUnmatchedLaunch([]byte(foreground)) {
		t.Error("foreground Bash is not a background launch")
	}

	agent := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_a","name":"Agent","input":{"run_in_background":true}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_a","content":"async launched"}]},"toolUseResult":{"agentId":"agent7654321"}}
`
	if !transcriptHasUnmatchedLaunch([]byte(agent)) {
		t.Error("async Agent launch with no notification should be outstanding")
	}
}

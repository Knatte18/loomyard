// hookrun_test.go runs the hook commands the launch line carries under sh, as Codex runs them, and reads what they write back through the parsers.

package codexengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// runHook runs command under sh with payload on standard input and returns what it prints.
func runHook(t *testing.T, command, payload string) string {
	t.Helper()
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = strings.NewReader(payload)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run hook %q: %v", command, err)
	}
	return string(out)
}

// TestHookCommands_RecordAndStamp pins that each recording hook appends a stamped payload the parsers read back with its time and prints the empty decision,
// and that the activity hook touches the stamp beside the events file.
func TestHookCommands_RecordAndStamp(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	events := filepath.Join(t.TempDir(), "run dir's", "events.jsonl")
	mkdirAll(t, filepath.Dir(events))
	for _, step := range []struct{ hook, payload string }{
		{hookEventSessionStart, sessionStartPayload},
		{hookEventUserPromptSubmit, userPromptSubmitPayload},
		{hookEventStop, stopPayload},
	} {
		if out := runHook(t, recordingCommand(step.hook, events), step.payload); out != "{}\n" {
			t.Errorf("%s hook printed %q; want the empty decision", step.hook, out)
		}
	}

	data, err := os.ReadFile(events)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	signals, consumed := New().ParseSessionSignals(data)
	if consumed != len(data) {
		t.Errorf("consumed = %d; want the whole file, %d", consumed, len(data))
	}
	wantKinds := []shuttleengine.SessionSignalKind{shuttleengine.SessionSignalSessionStart, shuttleengine.SessionSignalTurnStart, shuttleengine.SessionSignalTurnEnd}
	if len(signals) != len(wantKinds) {
		t.Fatalf("signals = %+v; want %d", signals, len(wantKinds))
	}
	for i, want := range wantKinds {
		if signals[i].Kind != want || signals[i].At.IsZero() {
			t.Errorf("signal %d = %+v; want kind %s with its stamp time", i, signals[i], want)
		}
	}
	if parsed, _ := New().ParseEvents(data); len(parsed) != 1 || parsed[0].Message != "ok" {
		t.Errorf("ParseEvents() = %+v; want the one Stop with message ok", parsed)
	}

	if out := runHook(t, activityCommand(events), "{}"); out != "{}\n" {
		t.Errorf("activity hook printed %q; want the empty decision", out)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(events), shuttleengine.ActivityStampFileName)); err != nil {
		t.Errorf("activity stamp: %v; want it touched", err)
	}
}

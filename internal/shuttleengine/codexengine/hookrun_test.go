// hookrun_test.go runs the hook commands the launch line carries under sh, as Codex runs them, and reads what they write back through the parsers.
// It also runs the idle wake against a stand-in codex executable.

package codexengine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
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

// writeExecutable writes a shell script with content to path and makes it executable.
func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestCreelHookCommands pins the creel hooks under sh, with a stand-in lym on PATH that records its arguments and input and answers as told.
// The mail hook prints the empty decision and runs nothing with no flag, and passes hook-read's output through when the flag stands.
// The merged Stop hook records the turn end with no flag or when hook-read hands out nothing, and records nothing when hook-read hands out mail, whose block is the hook's output.
func TestCreelHookCommands(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "lym"), "#!/bin/sh\nprintf '%s|' \"$*\" >> \"$FAKE_CALLS\"; cat >> \"$FAKE_CALLS\"; printf '%s' \"$FAKE_OUT\"; exit \"$FAKE_CODE\"\n")
	block := `{"decision":"block","reason":"[creel] #1"}`
	toolPayload := `{"hook_event_name":"PostToolUse"}`
	tests := []struct {
		name string
		// stop selects the merged Stop hook over the mail hook.
		stop     bool
		flag     bool
		fakeOut  string
		fakeCode int
		wantOut  string
		// wantCall is what the stand-in lym recorded; empty means it never ran.
		wantCall  string
		wantStops int
	}{
		{name: "MailHookNoFlag", wantOut: "{}\n"},
		{name: "MailHookFlagStands", flag: true, fakeOut: block, wantOut: block, wantCall: "creel hook-read codex|" + toolPayload},
		{name: "StopNoFlag", stop: true, wantOut: "{}\n", wantStops: 1},
		{name: "StopHandsOutMail", stop: true, flag: true, fakeOut: block, wantOut: block, wantCall: "creel hook-read codex|" + stopPayload},
		{name: "StopHandsOutNothing", stop: true, flag: true, fakeCode: 1, wantCall: "creel hook-read codex|" + stopPayload, wantStops: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			flagDir, calls, events := filepath.Join(dir, "flags"), filepath.Join(dir, "calls"), filepath.Join(dir, "events.jsonl")
			mkdirAll(t, flagDir)
			if tt.flag {
				writeFile(t, filepath.Join(flagDir, "ly.alpha"), "")
			}
			command, payload := creelMailHookCommand(flagDir), toolPayload
			if tt.stop {
				command, payload = creelStopHookCommand(flagDir, events), stopPayload
			}
			env := fmt.Sprintf("LYM_STRAND_NAME=ly:alpha; PATH=%s:$PATH; FAKE_CALLS=%s; FAKE_OUT=%s; FAKE_CODE=%d; export FAKE_CALLS FAKE_OUT FAKE_CODE; ",
				shell.Posix().Quote(bin), shell.Posix().Quote(calls), shell.Posix().Quote(tt.fakeOut), tt.fakeCode)

			if out := runHook(t, env+command, payload); out != tt.wantOut {
				t.Errorf("hook printed %q; want %q", out, tt.wantOut)
			}
			if got, _ := os.ReadFile(calls); string(got) != tt.wantCall {
				t.Errorf("stand-in lym recorded %q; want %q", got, tt.wantCall)
			}
			data, _ := os.ReadFile(events)
			if parsed, _ := New().ParseEvents(data); len(parsed) != tt.wantStops {
				t.Errorf("ParseEvents() = %+v; want %d turn ends", parsed, tt.wantStops)
			}
		})
	}
}

// TestWakeIdle pins the idle wake.
// A stand-in codex is run as `queue --thread <session_id> --message <text>` under the CODEX_HOME of the environment.
// A payload naming no session id starts nothing.
// A queue that fails returns an error carrying its output.
// It sets LYM_CODEX_BINARY and CODEX_HOME, so it does not run in parallel.
func TestWakeIdle(t *testing.T) {
	tiergate.Require(t, 1)

	dir := t.TempDir()
	queued := filepath.Join(dir, "queued")
	codex := filepath.Join(dir, "codex")
	writeExecutable(t, codex, "#!/bin/sh\nprintf '%s\\n' \"$CODEX_HOME\" \"$@\" > "+shell.Posix().Quote(queued)+"\n")
	t.Setenv(codexBinaryEnvKey, codex)
	t.Setenv(codexHomeEnvKey, "/lym-home")
	text := "[creel] #1 from \"ly:orch\": go\n[creel] #2 from \"ly:orch\": stop"

	if err := New().WakeIdle([]byte(`{"hook_event_name":"Stop","session_id":"thread-1"}`), text); err != nil {
		t.Fatalf("WakeIdle() error: %v", err)
	}
	if got, want := readText(t, queued), "/lym-home\nqueue\n--thread\nthread-1\n--message\n"+text+"\n"; got != want {
		t.Errorf("stand-in codex ran with %q; want %q", got, want)
	}

	if err := os.Remove(queued); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := New().WakeIdle([]byte(`{"hook_event_name":"Stop"}`), text); err == nil || !strings.Contains(err.Error(), "session_id") {
		t.Errorf("WakeIdle() with no session id error = %v; want a refusal naming session_id", err)
	}
	if _, err := os.Stat(queued); !os.IsNotExist(err) {
		t.Errorf("stand-in codex ran for a payload with no session id: stat error %v", err)
	}

	failing := filepath.Join(dir, "failing-codex")
	writeExecutable(t, failing, "#!/bin/sh\necho no such thread >&2\nexit 3\n")
	t.Setenv(codexBinaryEnvKey, failing)
	if err := New().WakeIdle([]byte(`{"session_id":"thread-1"}`), text); err == nil || !strings.Contains(err.Error(), "no such thread") {
		t.Errorf("WakeIdle() with a failing queue error = %v; want one carrying its output", err)
	}
}

// readText returns the content of the file at path.
func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

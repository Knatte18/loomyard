//go:build integration

// hookcmd_integration_test.go runs the hook commands settings.go builds under `sh`:
// the python deny's against PreToolUse payloads, pinning which Bash commands it denies and which it lets through,
// and the recording hooks' against payloads on standard input, pinning what they append to the events file.

package claudeengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestPythonDenyHook_DeniesCommandPositionPythonOnly feeds the python row's hook command a PreToolUse payload per Bash command and asserts the deny JSON, or empty output and exit 0 for a command it lets through.
func TestPythonDenyHook_DeniesCommandPositionPythonOnly(t *testing.T) {
	t.Parallel()

	var hookCommand string
	for _, deny := range standingDenies {
		if deny.steer == steerPythonDeny {
			hookCommand = deny.command
		}
	}
	if hookCommand == "" {
		t.Fatal("standingDenies has no python row")
	}

	tests := []struct {
		name    string
		command string
		denied  bool
	}{
		{"python3_with_flag", "python3 -c 'print(1)'", true},
		{"after_and_separator", "cd x && python script.py", true},
		{"second_line_starts_python3", "echo hi\npython3 x.py", true},
		{"path_prefixed_with_heredoc", "/usr/bin/python3 - <<EOF\nprint(1)\nEOF", true},
		{"after_xargs", "ls | xargs python3", true},
		{"python_as_argument", "grep python file", false},
		{"python_in_directory_name", "ls python-tools/", false},
		{"behind_bash_dash_c", "bash -c 'python3 x'", false},
		{"through_launcher", "uv run x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var payload bytes.Buffer
			encoder := json.NewEncoder(&payload)
			encoder.SetEscapeHTML(false)
			if err := encoder.Encode(map[string]any{
				"hook_event_name": "PreToolUse",
				"tool_name":       "Bash",
				"tool_input":      map[string]any{"command": tt.command},
			}); err != nil {
				t.Fatalf("encode payload: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "sh", "-c", hookCommand)
			cmd.Stdin = &payload
			var out bytes.Buffer
			cmd.Stdout = &out
			if err := cmd.Run(); err != nil {
				t.Fatalf("hook exited with %v (ctx: %v); stdout=%q", err, ctx.Err(), out.String())
			}

			want := ""
			if tt.denied {
				want = denyJSON(steerPythonDeny) + "\n"
			}
			if got := out.String(); got != want {
				t.Errorf("hook output for %q = %q; want %q", tt.command, got, want)
			}
		})
	}
}

// recordingHookCommands returns the command of each recording hook buildSettings installs for an events file at eventsPath, keyed by hook event name;
// the interactive AskUserQuestion record is keyed "AskUserQuestion".
func recordingHookCommands(t *testing.T, eventsPath string) map[string]string {
	t.Helper()
	commands := map[string]string{}
	for _, interactive := range []bool{false, true} {
		data, err := buildSettings(eventsPath, interactive, shuttleengine.Config{}, false, false, "")
		if err != nil {
			t.Fatalf("buildSettings() error: %v", err)
		}
		var doc struct {
			Hooks map[string][]struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("unmarshal settings: %v", err)
		}
		for event, entries := range doc.Hooks {
			for _, entry := range entries {
				switch {
				case event == "PreToolUse" && entry.Matcher == "AskUserQuestion":
					commands["AskUserQuestion"] = entry.Hooks[0].Command
				case event != "PreToolUse":
					commands[event] = entry.Hooks[0].Command
				}
			}
		}
	}
	return commands
}

// runHookCommand runs command under sh with stdin as its standard input and env as its environment, returning its standard output and exit code.
func runHookCommand(t *testing.T, command, stdin string, env []string) (stdout string, exitCode int) {
	t.Helper()
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Fatalf("sh not found: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shPath, "-c", command)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return out.String(), 0
	case errors.As(err, &exitErr):
		return out.String(), exitErr.ExitCode()
	}
	t.Fatalf("run hook: %v (ctx: %v)", err, ctx.Err())
	return "", -1
}

// paddedPayload marshals fields plus a padding field sized so the line is exactly size bytes.
func paddedPayload(t *testing.T, fields map[string]any, size int) string {
	t.Helper()
	fields["padding"] = ""
	base, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	fields["padding"] = strings.Repeat("x", size-len(base))
	line, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if len(line) != size {
		t.Fatalf("payload is %d bytes; want %d", len(line), size)
	}
	return string(line)
}

// TestRecordingHooks_AppendStampThenPayload runs the recording hooks' commands under sh with a payload on standard input.
// The Stop and ask record commands append a stamp line naming their hook and then the payload byte for byte, so ParseEvents reads the file as it reads the payload alone and ParseSessionSignals pairs the payload with the stamp's time;
// a failing `date` still appends the payload, with no time;
// the four newer hooks exit 0 and Stop and the ask record exit 1 when the events file cannot be written;
// and no recording hook writes to standard output.
func TestRecordingHooks_AppendStampThenPayload(t *testing.T) {
	t.Parallel()

	const payloadSize = 20000
	realEnv := os.Environ()

	roundTrips := []struct {
		hook    string
		stamped string
		fields  map[string]any
	}{
		{"Stop", "Stop", map[string]any{"hook_event_name": "Stop", "session_id": "s1", "last_assistant_message": "done"}},
		{"AskUserQuestion", "PreToolUse", map[string]any{
			"hook_event_name": "PreToolUse", "session_id": "s1", "tool_name": "AskUserQuestion",
			"tool_input": map[string]any{"questions": []any{map[string]any{"question": "which?"}}},
		}},
	}
	for _, tt := range roundTrips {
		t.Run(tt.hook+"_round_trip", func(t *testing.T) {
			t.Parallel()

			eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
			command := recordingHookCommands(t, eventsPath)[tt.hook]
			payload := paddedPayload(t, tt.fields, payloadSize)

			before := time.Now().UTC().Truncate(time.Second)
			stdout, exitCode := runHookCommand(t, command, payload, realEnv)
			after := time.Now().UTC()
			if stdout != "" || exitCode != 0 {
				t.Fatalf("hook stdout = %q, exit = %d; want no output and exit 0", stdout, exitCode)
			}

			file, err := os.ReadFile(eventsPath)
			if err != nil {
				t.Fatal(err)
			}
			stampLine, rest, found := strings.Cut(string(file), "\n")
			if !found || rest != payload+"\n" {
				t.Fatalf("events file = %d bytes; want a stamp line then the %d-byte payload and a newline", len(file), payloadSize)
			}
			var stamp map[string]string
			if err := json.Unmarshal([]byte(stampLine), &stamp); err != nil || stamp["lyx_stamp"] != tt.stamped {
				t.Fatalf("stamp line = %q (err %v); want it to name %q", stampLine, err, tt.stamped)
			}

			claude := &Claude{}
			fromFile, err := claude.ParseEvents(file)
			if err != nil {
				t.Fatal(err)
			}
			fromPayload, err := claude.ParseEvents([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			if len(fromPayload) != 1 || !reflect.DeepEqual(fromFile, fromPayload) {
				t.Errorf("ParseEvents over the file = %+v; want the payload's %+v", fromFile, fromPayload)
			}

			signals, _ := claude.ParseSessionSignals(file)
			if len(signals) != 1 || signals[0].At.Before(before) || signals[0].At.After(after) {
				t.Errorf("signals = %+v; want one signal timed between %v and %v", signals, before, after)
			}
		})
	}

	t.Run("failing_date_still_appends_the_payload_with_no_time", func(t *testing.T) {
		t.Parallel()

		binDir := t.TempDir()
		catPath, err := exec.LookPath("cat")
		if err != nil {
			t.Fatalf("cat not found: %v", err)
		}
		if err := os.Symlink(catPath, filepath.Join(binDir, "cat")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "date"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}

		eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
		payload := paddedPayload(t, map[string]any{"hook_event_name": "Stop", "session_id": "s1"}, 200)
		stdout, exitCode := runHookCommand(t, recordingHookCommands(t, eventsPath)["Stop"], payload, []string{"PATH=" + binDir})
		if stdout != "" || exitCode != 0 {
			t.Fatalf("hook stdout = %q, exit = %d; want no output and exit 0", stdout, exitCode)
		}
		file, err := os.ReadFile(eventsPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(string(file), "\n"+payload+"\n") {
			t.Fatalf("events file = %q; want it to end with the payload", file)
		}
		signals, _ := (&Claude{}).ParseSessionSignals(file)
		if len(signals) != 1 || !signals[0].At.IsZero() {
			t.Errorf("signals = %+v; want one signal with a zero time", signals)
		}
	})

	t.Run("unwritable_events_file", func(t *testing.T) {
		t.Parallel()

		eventsPath := filepath.Join(t.TempDir(), "missing-dir", "events.jsonl")
		commands := recordingHookCommands(t, eventsPath)
		// The failing exit status is the shell's own redirection failure: 1 under bash, 2 under dash.
		wantFailure := map[string]bool{"Stop": true, "AskUserQuestion": true, "UserPromptSubmit": false, "StopFailure": false, "Notification": false, "SessionEnd": false}
		for hook, wantFails := range wantFailure {
			stdout, exitCode := runHookCommand(t, commands[hook], `{"hook_event_name":"x"}`, realEnv)
			if stdout != "" || (exitCode != 0) != wantFails {
				t.Errorf("%s hook stdout = %q, exit = %d; want no output and failure = %v", hook, stdout, exitCode, wantFails)
			}
		}
	})
}

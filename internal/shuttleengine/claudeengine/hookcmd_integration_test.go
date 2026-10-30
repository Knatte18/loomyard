//go:build tier1

// hookcmd_integration_test.go runs the recording hook commands settings.go builds under `sh` against payloads on standard input, pinning what they append to the events file.
// It runs rendered shell guard commands the same way, pinning that the rendering greps each pattern as written.
// The deny pattern tables live with the shell guard rows in shellguard.

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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// recordingHookCommands returns the command of each recording hook buildSettings installs for an events file at eventsPath, keyed by hook event name;
// the interactive AskUserQuestion record is keyed "AskUserQuestion".
func recordingHookCommands(t *testing.T, eventsPath string) map[string]string {
	t.Helper()
	commands := map[string]string{}
	for _, interactive := range []bool{false, true} {
		data, err := buildSettings(eventsPath, interactive, shuttleengine.Config{}, Config{}, false, false, false, "", "")
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
			if err := json.Unmarshal([]byte(stampLine), &stamp); err != nil || stamp["lym_stamp"] != tt.stamped {
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

// TestShellGuardDenies_RenderedCommandsGrepThePayload runs the rendered shell guard commands under sh against a Bash PreToolUse payload.
// A row whose pattern holds a single quote and a row of several patterns each echo their deny JSON on a matching command and print nothing otherwise, and every command exits 0.
func TestShellGuardDenies_RenderedCommandsGrepThePayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		row     string
		command string
		denied  bool
	}{
		{"machine-sleep", "systemctl hibernate", true},
		{"machine-sleep", "systemctl status", false},
		{"machine-sleep", "grep 'x; systemctl hibernate' f", false},
		{"computed-rm", "rm $DIR/f", true},
		{"computed-rm", "rm .scratch/a", false},
	}
	for _, tt := range tests {
		t.Run(tt.row+"_"+tt.command, func(t *testing.T) {
			t.Parallel()

			var payload bytes.Buffer
			encoder := json.NewEncoder(&payload)
			encoder.SetEscapeHTML(false)
			if err := encoder.Encode(map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": tt.command}}); err != nil {
				t.Fatal(err)
			}
			row := shellRow(t, tt.row)
			stdout, exitCode := runHookCommand(t, renderShellGuard(row).command, payload.String(), os.Environ())
			want := ""
			if tt.denied {
				want = denyJSON(row.Steer) + "\n"
			}
			if stdout != want || exitCode != 0 {
				t.Errorf("%q hook on %q: stdout = %q, exit = %d; want %q and exit 0", tt.row, tt.command, stdout, exitCode, want)
			}
		})
	}
}

// TestCreelMailHook_RunsLymOnlyWhenTheStrandsFlagStands runs the creel mail hook and the idle waiter hook under sh with a stub `lym` on PATH that prints its directory and arguments.
// The mail hook forms the flag's file name as agentname.FileName does, runs `lym creel hook-read claude` from the session's project directory only while that flag stands, and otherwise exits 0 having printed nothing;
// the waiter hook runs `lym creel hook-wait claude` from the project directory whether or not a flag stands.
func TestCreelMailHook_RunsLymOnlyWhenTheStrandsFlagStands(t *testing.T) {
	t.Parallel()

	const name = "ly:creel-hook:driver-2"
	root := t.TempDir()
	flagDir, binDir, projectDir := filepath.Join(root, "flags"), filepath.Join(root, "bin"), filepath.Join(root, "project")
	for _, dir := range []string{flagDir, binDir, projectDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(binDir, "lym"), []byte("#!/bin/sh\necho \"$(pwd -P) $*\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flagDir, agentname.FileName(name)), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	resolvedProjectDir, err := filepath.EvalSymlinks(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	command := creelMailHookCommand(shQuote(flagDir))
	baseEnv := []string{"PATH=" + binDir + ":/usr/bin:/bin", "CLAUDE_PROJECT_DIR=" + projectDir}

	tests := []struct {
		name string
		env  []string
		want string
	}{
		{"FlagStands", []string{agentname.StrandNameEnv + "=" + name}, resolvedProjectDir + " creel hook-read claude\n"},
		{"AnotherStrandsFlag", []string{agentname.StrandNameEnv + "=ly:creel-hook:driver"}, ""},
		{"EmptyName", []string{agentname.StrandNameEnv + "="}, ""},
		{"UnsetName", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stdout, exitCode := runHookCommand(t, command, `{"hook_event_name":"PostToolUse"}`, append(slices.Clone(baseEnv), tt.env...))
			if stdout != tt.want || exitCode != 0 {
				t.Errorf("hook stdout = %q, exit = %d; want %q and exit 0", stdout, exitCode, tt.want)
			}
		})
	}
	// The idle waiter runs from the project directory whether or not a flag stands.
	waitCommand := creelWaitHookCommand()
	waiters := []struct {
		name string
		env  []string
	}{
		{"WaiterWithTheFlagStanding", []string{agentname.StrandNameEnv + "=" + name}},
		{"WaiterWithNoFlag", nil},
	}
	for _, tt := range waiters {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stdout, exitCode := runHookCommand(t, waitCommand, `{"hook_event_name":"Stop"}`, append(slices.Clone(baseEnv), tt.env...))
			if want := resolvedProjectDir + " creel hook-wait claude\n"; stdout != want || exitCode != 0 {
				t.Errorf("hook stdout = %q, exit = %d; want %q and exit 0", stdout, exitCode, want)
			}
		})
	}
}

// TestCreelStopHook_RecordsATurnEndOnlyWithoutABlock runs the merged Stop command under sh with a stub `lym` on PATH that records the standard input it got, prints a fixed output and exits as the row says.
// Mail written (the stub exits 0) leaves the events file empty and the output on standard output; otherwise, with the flag absent as well, the file holds the Stop stamp and then the payload byte for byte, which ParseEvents reads as the payload alone.
// An exit 1 that printed an echo alone records the turn end and leaves the printed output as the hook's.
// An unwritable events file fails the command as the plain recording command does.
func TestCreelStopHook_RecordsATurnEndOnlyWithoutABlock(t *testing.T) {
	t.Parallel()

	const (
		name = "ly:creel-stop:driver"
		mail = `{"hookSpecificOutput":{"hookEventName":"Stop","additionalContext":"mail"}}`
		echo = `{"systemMessage":"echo"}`
	)
	payload := paddedPayload(t, map[string]any{"hook_event_name": "Stop", "session_id": "s1", "last_assistant_message": "done"}, 20000)
	tests := []struct {
		name        string
		flagStands  bool
		stubExit    string
		stubOut     string
		eventsDir   string
		wantStdout  string
		wantRecord  bool
		wantStubRun bool
		wantFailure bool
	}{
		{"FlagStandsAndMailIsWritten", true, "0", mail, "", mail + "\n", false, true, false},
		{"FlagStandsAndAnEchoAloneIsWritten", true, "1", echo, "", echo + "\n", true, true, false},
		{"FlagStandsAndNothingIsClaimed", true, "1", "", "", "", true, true, false},
		{"FlagAbsent", false, "0", mail, "", "", true, false, false},
		{"UnwritableEventsFile", true, "1", "", "missing-dir", "", false, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			flagDir, binDir, projectDir := filepath.Join(root, "flags"), filepath.Join(root, "bin"), filepath.Join(root, "project")
			for _, dir := range []string{flagDir, binDir, projectDir} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			stdinPath := filepath.Join(root, "stub-stdin")
			stub := "#!/bin/sh\ncat > \"$STUB_STDIN\"\n[ -n \"$STUB_OUT\" ] && printf '%s\\n' \"$STUB_OUT\"\nexit \"$STUB_EXIT\"\n"
			if err := os.WriteFile(filepath.Join(binDir, "lym"), []byte(stub), 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.flagStands {
				if err := os.WriteFile(filepath.Join(flagDir, agentname.FileName(name)), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			eventsPath := filepath.Join(root, tt.eventsDir, "events.jsonl")
			command := creelStopHookCommand(shQuote(flagDir), shQuote(eventsPath))
			env := []string{
				"PATH=" + binDir + ":/usr/bin:/bin", "CLAUDE_PROJECT_DIR=" + projectDir, agentname.StrandNameEnv + "=" + name,
				"STUB_STDIN=" + stdinPath, "STUB_OUT=" + tt.stubOut, "STUB_EXIT=" + tt.stubExit,
			}

			stdout, exitCode := runHookCommand(t, command, payload, env)
			if stdout != tt.wantStdout || (exitCode != 0) != tt.wantFailure {
				t.Fatalf("hook stdout = %q, exit = %d; want %q and failure = %v", stdout, exitCode, tt.wantStdout, tt.wantFailure)
			}
			got, err := os.ReadFile(stdinPath)
			switch {
			case tt.wantStubRun && (err != nil || string(got) != payload):
				t.Errorf("the stub's standard input = %d bytes (err %v); want the %d-byte payload", len(got), err, len(payload))
			case !tt.wantStubRun && err == nil:
				t.Errorf("the stub ran with the flag absent")
			}

			file, err := os.ReadFile(eventsPath)
			if !tt.wantRecord {
				if len(file) != 0 {
					t.Errorf("events file = %q; want it empty", file)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			stampLine, rest, found := strings.Cut(string(file), "\n")
			var stamp map[string]string
			if !found || rest != payload+"\n" || json.Unmarshal([]byte(stampLine), &stamp) != nil || stamp["lym_stamp"] != "Stop" {
				t.Fatalf("events file = %d bytes; want the Stop stamp line, then the payload byte for byte and a newline", len(file))
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
		})
	}
}

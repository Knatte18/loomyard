// hookcmd_integration_test.go runs the recording hook commands settings.go builds under `sh`.
// It pins what each appends to the events file, and that rendered shell guard commands grep each pattern as written.
// The deny pattern tables live with the shell guard rows in `internal/shellguard`.

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
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recordingHookCommands returns the command of each recording hook in the generated settings, keyed by hook event name.
// The settings are generated for the events file at eventsPath.
// The interactive AskUserQuestion record is keyed "AskUserQuestion".
func recordingHookCommands(t *testing.T, eventsPath string) map[string]string {
	t.Helper()
	commands := map[string]string{}
	for _, interactive := range []bool{false, true} {
		data, err := buildSettings(eventsPath, interactive, shuttleengine.Config{}, Config{}, false, false, "", "", "", sessionScope{})
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

// runHookCommand runs command under sh with stdin as its standard input and env as its environment.
// It returns the command's standard output and exit code.
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

// TestRecordingHooks_AppendStampThenPayload runs the recording hooks' commands under sh with a payload on stdin.
// The Stop and ask record commands append a stamp line naming their hook, then the payload byte for byte.
// ParseEvents reads the file as it reads the payload alone.
// ParseSessionSignals pairs the payload with the stamp's time.
// A failing `date` still appends the payload, with no time.
// The other hooks exit 0, and the Stop and ask record commands exit 1, when the events file cannot be written.
// No recording hook writes to standard output.
func TestRecordingHooks_AppendStampThenPayload(t *testing.T) {
	tiergate.Require(t, 1)
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

// turnHookFixture is the directory tree and stub scripts the turn hook tests run the composed commands against.
// The stub turn hook records its working directory and arguments, and its standard input, then prints and fails as told.
// The stub `lym` stands in for creel's mail command.
type turnHookFixture struct {
	binDir, flagDir, projectDir, eventsPath string
	hookArgvPath, hookStdinPath             string
	turnHookCommand                         string
}

const turnHookStrandName = "ly:turn-hook:driver"

// newTurnHookFixture lays out the fixture under a temp dir; the events file lives in eventsDir below it.
func newTurnHookFixture(t *testing.T, eventsDir string) turnHookFixture {
	t.Helper()
	root := t.TempDir()
	fx := turnHookFixture{
		binDir: filepath.Join(root, "bin"), flagDir: filepath.Join(root, "flags"), projectDir: filepath.Join(root, "project"),
		eventsPath:   filepath.Join(root, eventsDir, "events.jsonl"),
		hookArgvPath: filepath.Join(root, "hook-argv"), hookStdinPath: filepath.Join(root, "hook-stdin"),
	}
	for _, dir := range []string{fx.binDir, fx.flagDir, fx.projectDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	resolvedProjectDir, err := filepath.EvalSymlinks(fx.projectDir)
	if err != nil {
		t.Fatal(err)
	}
	fx.projectDir = resolvedProjectDir
	hookPath := filepath.Join(fx.binDir, "turnhook")
	hookScript := "#!/bin/sh\nprintf '%s %s\\n' \"$(pwd -P)\" \"$*\" > \"$HOOK_ARGV\"\ncat > \"$HOOK_STDIN\"\necho hook-output\necho hook-error >&2\nexit \"$HOOK_EXIT\"\n"
	if err := os.WriteFile(hookPath, []byte(hookScript), 0o755); err != nil {
		t.Fatal(err)
	}
	lymScript := "#!/bin/sh\ncat > /dev/null\n[ -n \"$STUB_OUT\" ] && printf '%s\\n' \"$STUB_OUT\"\nexit \"$STUB_EXIT\"\n"
	if err := os.WriteFile(filepath.Join(fx.binDir, "lym"), []byte(lymScript), 0o755); err != nil {
		t.Fatal(err)
	}
	fx.turnHookCommand = shQuote(hookPath)
	return fx
}

// standFlag raises the strand's mail flag in the fixture's flag directory.
func (fx turnHookFixture) standFlag(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(fx.flagDir, agentname.FileName(turnHookStrandName)), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// env returns the environment the commands run in.
// The stub turn hook exits with hookExit, and the stub `lym` exits with lymExit after printing lymOut.
func (fx turnHookFixture) env(hookExit, lymExit, lymOut string) []string {
	return []string{
		"PATH=" + fx.binDir + ":/usr/bin:/bin", "CLAUDE_PROJECT_DIR=" + fx.projectDir, agentname.StrandNameEnv + "=" + turnHookStrandName,
		"HOOK_ARGV=" + fx.hookArgvPath, "HOOK_STDIN=" + fx.hookStdinPath, "HOOK_EXIT=" + hookExit,
		"STUB_EXIT=" + lymExit, "STUB_OUT=" + lymOut,
	}
}

// commands returns the UserPromptSubmit and Stop commands buildSettings composes for the fixture.
// An empty turnHook composes the commands of a run with none; inHub gives the Stop command its creel merge.
func (fx turnHookFixture) commands(t *testing.T, turnHook string, inHub bool) (prompt, stop string) {
	t.Helper()
	flagDir := ""
	if inHub {
		flagDir = fx.flagDir
	}
	data, err := buildSettings(fx.eventsPath, false, shuttleengine.Config{}, Config{}, false, false, "", turnHook, flagDir, sessionScope{})
	if err != nil {
		t.Fatalf("buildSettings() error: %v", err)
	}
	doc := parseSettings(t, data)
	return firstHookCommand(t, doc, "UserPromptSubmit"), firstHookCommand(t, doc, "Stop")
}

// TestTurnHook_GetsThePayloadAndTheEngineNameWithoutPrinting runs the prompt and Stop commands under sh.
// They are composed with a turn hook.
// The recording is appended as it is without a turn hook.
// The hook runs from the project directory.
// Its only argument is the engine name `claude`, and the payload is on standard input.
// What it prints reaches neither the command's standard output nor its exit status.
func TestTurnHook_GetsThePayloadAndTheEngineNameWithoutPrinting(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const payload = `{"hook_event_name":"Stop","session_id":"s1"}`
	tests := []struct {
		name  string
		event string
		inHub bool
	}{
		{"UserPromptSubmit", "UserPromptSubmit", false},
		{"Stop", "Stop", false},
		{"StopInAHubWithNoMail", "Stop", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fx := newTurnHookFixture(t, "")
			prompt, stop := fx.commands(t, fx.turnHookCommand, tt.inHub)
			command := stop
			if tt.event == "UserPromptSubmit" {
				command = prompt
			}
			stdout, exitCode := runHookCommand(t, command, payload, fx.env("0", "1", ""))
			if stdout != "" || exitCode != 0 {
				t.Fatalf("hook stdout = %q, exit = %d; want no output and exit 0", stdout, exitCode)
			}
			argv, err := os.ReadFile(fx.hookArgvPath)
			if want := fx.projectDir + " claude\n"; err != nil || string(argv) != want {
				t.Errorf("turn hook working directory and arguments = %q (err %v); want %q", argv, err, want)
			}
			stdin, err := os.ReadFile(fx.hookStdinPath)
			if err != nil || string(stdin) != payload {
				t.Errorf("turn hook standard input = %q (err %v); want the payload %q", stdin, err, payload)
			}
			file, err := os.ReadFile(fx.eventsPath)
			if err != nil {
				t.Fatal(err)
			}
			stampLine, rest, found := strings.Cut(string(file), "\n")
			var stamp map[string]string
			if !found || rest != payload+"\n" || json.Unmarshal([]byte(stampLine), &stamp) != nil || stamp["lym_stamp"] != tt.event {
				t.Errorf("events file = %q; want the %s stamp line, then the payload and a newline", file, tt.event)
			}
		})
	}
}

// TestTurnHook_LeavesTheCommandsExitStatusToTheRecording runs the composed commands under sh with a failing turn hook.
// The prompt command exits 0 whatever the hook does.
// The Stop command, plain or merged with creel's delivery, exits as it does without a turn hook:
// 0 when the events file is written, and the shell's redirection failure when it is not.
func TestTurnHook_LeavesTheCommandsExitStatusToTheRecording(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name        string
		prompt      bool
		inHub       bool
		eventsDir   string
		wantFailure bool
	}{
		{"PromptWithAFailingHook", true, false, "", false},
		{"PromptWithAFailingHookAndAnUnwritableEventsFile", true, false, "missing-dir", false},
		{"StopWithAFailingHook", false, false, "", false},
		{"StopWithAFailingHookAndAnUnwritableEventsFile", false, false, "missing-dir", true},
		{"StopInAHubWithAFailingHook", false, true, "", false},
		{"StopInAHubWithAFailingHookAndAnUnwritableEventsFile", false, true, "missing-dir", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fx := newTurnHookFixture(t, tt.eventsDir)
			env := fx.env("3", "1", "")
			commandOf := func(turnHook string) string {
				prompt, stop := fx.commands(t, turnHook, tt.inHub)
				if tt.prompt {
					return prompt
				}
				return stop
			}
			_, baselineExit := runHookCommand(t, commandOf(""), `{"hook_event_name":"Stop"}`, env)
			stdout, exitCode := runHookCommand(t, commandOf(fx.turnHookCommand), `{"hook_event_name":"Stop"}`, env)
			if stdout != "" || exitCode != baselineExit || (exitCode != 0) != tt.wantFailure {
				t.Errorf("hook stdout = %q, exit = %d; want no output and the exit %d of the same command without a turn hook (failure = %v)", stdout, exitCode, baselineExit, tt.wantFailure)
			}
			if _, err := os.Stat(fx.hookArgvPath); err != nil {
				t.Errorf("the turn hook did not run: %v", err)
			}
		})
	}
}

// TestTurnHook_MergedCreelStopRunsItOnEveryTurnEnd runs the merged Stop command.
// The strand's flag is standing.
// Mail handed out (the stub `lym` exits 0) continues the turn.
// So the command records nothing, yet the turn hook runs.
// Otherwise the turn end is recorded and the turn hook runs too, and the stub `lym` output stays the command's output.
func TestTurnHook_MergedCreelStopRunsItOnEveryTurnEnd(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const (
		mail    = `{"hookSpecificOutput":{"hookEventName":"Stop","additionalContext":"mail"}}`
		echo    = `{"systemMessage":"echo"}`
		payload = `{"hook_event_name":"Stop","session_id":"s1"}`
	)
	tests := []struct {
		name       string
		lymExit    string
		lymOut     string
		wantStdout string
		wantRecord bool
	}{
		{"MailHandedOut", "0", mail, mail + "\n", false},
		{"EchoAloneHandedOut", "1", echo, echo + "\n", true},
		{"NothingClaimed", "1", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fx := newTurnHookFixture(t, "")
			fx.standFlag(t)
			_, stop := fx.commands(t, fx.turnHookCommand, true)
			stdout, exitCode := runHookCommand(t, stop, payload, fx.env("0", tt.lymExit, tt.lymOut))
			if stdout != tt.wantStdout || exitCode != 0 {
				t.Errorf("hook stdout = %q, exit = %d; want %q and exit 0", stdout, exitCode, tt.wantStdout)
			}
			_, hookErr := os.Stat(fx.hookArgvPath)
			if hookErr != nil {
				t.Errorf("the turn hook did not run: %v", hookErr)
			}
			file, _ := os.ReadFile(fx.eventsPath)
			if recorded := len(file) != 0; recorded != tt.wantRecord {
				t.Errorf("events file = %q; want recorded = %v", file, tt.wantRecord)
			}
		})
	}
}

// TestShellGuardDenies_RenderedCommandsGrepThePayload runs the rendered shell guard commands under sh against a Bash PreToolUse payload.
// A row whose pattern holds a single quote echoes its deny JSON on a matching command.
// So does a row of several patterns.
// Otherwise the rows print nothing, and every command exits 0.
func TestShellGuardDenies_RenderedCommandsGrepThePayload(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		row     string
		command string
		denied  bool
	}{
		{"machine-sleep", "systemctl hibernate", true},
		{"machine-sleep", "systemctl status", false},
		{"machine-sleep", "grep 'x; systemctl hibernate' f", false},
		{"shell-edit", "sed -i s/a/b/ f", true},
		{"shell-edit", "sed -n 1p f", false},
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

// TestCreelMailHook_RunsLymOnlyWhenTheStrandsFlagStands runs the creel mail hook and the idle waiter hook under sh.
// A stub `lym` on PATH prints its directory and arguments.
// The mail hook forms the flag's file name as `agentname.FileName` does.
// It runs `lym creel hook-read claude` from the session's project directory only while that flag stands.
// Otherwise it exits 0 having printed nothing.
// The waiter hook runs `lym creel hook-wait claude` from the project directory whether or not a flag stands.
func TestCreelMailHook_RunsLymOnlyWhenTheStrandsFlagStands(t *testing.T) {
	tiergate.Require(t, 1)
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

// TestCreelStopHook_RecordsATurnEndOnlyWithoutABlock runs the merged Stop command under sh.
// A stub `lym` on PATH records the standard input each of its verbs got and prints a fixed output.
// It exits as each row says.
// Mail written (the stub exits 0) leaves the events file empty and the output on standard output.
// A turn hook gets the payload whether or not mail is written, and its output never reaches the hook's.
// Otherwise, with the flag absent as well, the file holds the Stop stamp and then the payload byte for byte.
// ParseEvents reads that as the payload alone.
// An exit 1 that printed an echo alone records the turn end and leaves the printed output as the hook's.
// An unwritable events file fails the command as the plain recording command does.
func TestCreelStopHook_RecordsATurnEndOnlyWithoutABlock(t *testing.T) {
	tiergate.Require(t, 1)
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
		// open runs `creelOpenStopHookCommand` in place of the merged command, which records nothing and always exits 0.
		open bool
		// turnHook gives the merged command the turn hook `lym slack hook`.
		turnHook bool
	}{
		{"FlagStandsAndMailIsWritten", true, "0", mail, "", mail + "\n", false, true, false, false, false},
		{"FlagStandsAndAnEchoAloneIsWritten", true, "1", echo, "", echo + "\n", true, true, false, false, false},
		{"FlagStandsAndNothingIsClaimed", true, "1", "", "", "", true, true, false, false, false},
		{"FlagAbsent", false, "0", mail, "", "", true, false, false, false, false},
		{"UnwritableEventsFile", true, "1", "", "missing-dir", "", false, true, true, false, false},
		{"OpenFlagStandsAndMailIsWritten", true, "0", mail, "", mail + "\n", false, true, false, true, false},
		{"OpenFlagStandsAndAnEchoReachesTheOperator", true, "1", echo, "", echo + "\n", false, true, false, true, false},
		{"OpenFlagAbsent", false, "0", mail, "", "", false, false, false, true, false},
		{"TurnHookGetsTheInputWhenMailIsWritten", true, "0", mail, "", mail + "\n", false, true, false, false, true},
		{"TurnHookGetsTheInputWhenTheTurnEnds", false, "0", mail, "", "", true, false, false, false, true},
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
			stdinPrefix := filepath.Join(root, "stub-stdin")
			stdinPath, turnHookStdinPath := stdinPrefix+".creel", stdinPrefix+".slack"
			stub := "#!/bin/sh\ncat > \"$STUB_STDIN.$1\"\n[ -n \"$STUB_OUT\" ] && printf '%s\\n' \"$STUB_OUT\"\nexit \"$STUB_EXIT\"\n"
			if err := os.WriteFile(filepath.Join(binDir, "lym"), []byte(stub), 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.flagStands {
				if err := os.WriteFile(filepath.Join(flagDir, agentname.FileName(name)), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			eventsPath := filepath.Join(root, tt.eventsDir, "events.jsonl")
			turnHookCommand := ""
			if tt.turnHook {
				turnHookCommand = "lym slack hook"
			}
			command := creelStopHookCommand(shQuote(flagDir), shQuote(eventsPath), turnHookCommand)
			if tt.open {
				command = creelOpenStopHookCommand(shQuote(flagDir))
			}
			env := []string{
				"PATH=" + binDir + ":/usr/bin:/bin", "CLAUDE_PROJECT_DIR=" + projectDir, agentname.StrandNameEnv + "=" + name,
				"STUB_STDIN=" + stdinPrefix, "STUB_OUT=" + tt.stubOut, "STUB_EXIT=" + tt.stubExit,
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
			turnHookInput, err := os.ReadFile(turnHookStdinPath)
			switch {
			case tt.turnHook && (err != nil || string(turnHookInput) != payload):
				t.Errorf("the turn hook's standard input = %d bytes (err %v); want the %d-byte payload", len(turnHookInput), err, len(payload))
			case !tt.turnHook && err == nil:
				t.Errorf("a turn hook ran with none given")
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

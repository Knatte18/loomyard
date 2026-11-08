// settings_test.go tests the settings document buildSettings builds for a run, across the deny toggles and run modes.
// It also runs Prepare end to end against a temp directory.

package claudeengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// parseSettings unmarshals the JSON from buildSettings into a generic map.
// Tests assert on that map, not on the typed settings struct's (de)serialization.
func parseSettings(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal settings: %v; data: %s", err, data)
	}
	return doc
}

// hooksFor returns doc["hooks"][event] as a slice, or nil if absent.
func hooksFor(doc map[string]any, event string) []any {
	hooks, _ := doc["hooks"].(map[string]any)
	entries, _ := hooks[event].([]any)
	return entries
}

// TestBuildSettings_LeavesWorktreeSettingsOut pins that a session's document holds none of the worktree-wide keys.
// Those are the keys the worktree's rendered .claude/settings.local.json sets.
// It holds in every run mode, in a hub and out of one.
//
//testtiming:keep pins the absence of the worktree-wide keys in every run mode, which its covering tests do not assert
func TestBuildSettings_LeavesWorktreeSettingsOut(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	worktreeKeys := []string{
		"promptSuggestionEnabled", "attribution", "disableClaudeAiConnectors",
		"disableBundledSkills", "disableWorkflows", "permissions",
	}
	tests := []struct {
		name        string
		interactive bool
		fork        bool
		flagDir     string
	}{
		{"interactive", true, false, ""},
		{"autonomous", false, false, ""},
		{"fork", false, true, ""},
		{"interactive_in_a_hub", true, false, "/c/hub/flags"},
		{"autonomous_in_a_hub", false, false, "/c/hub/flags"},
		{"fork_in_a_hub", false, true, "/c/hub/flags"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data, err := buildSettings("/c/run/events.jsonl", tt.interactive, shuttleengine.Config{}, Config{}, tt.fork, false, "", "", tt.flagDir, sessionScope{})
			if err != nil {
				t.Fatalf("buildSettings() error: %v", err)
			}
			doc := parseSettings(t, data)
			for _, key := range worktreeKeys {
				if _, ok := doc[key]; ok {
					t.Errorf("document holds %q; want it left to the worktree settings (data: %s)", key, data)
				}
			}
		})
	}
}

// stampPrintf writes out by hand the stamp line a recording hook for hookEventName runs before its payload.
// Tests pin the line shape against this copy.
func stampPrintf(hookEventName, quotedEventsPath string) string {
	return `printf '{"lym_stamp":"` + hookEventName + `","lym_at":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> ` + quotedEventsPath
}

// TestBuildSettings_RecordingHooks pins the recording hooks every document carries.
// Each event gets one command entry with no tool matcher.
// The entry appends a stamp line and then the payload to the events path in its POSIX form.
// Stop keeps the payload append's exit status.
// The other hooks end in `; true`, so they exit 0 whatever the append does.
// Outside a hub, PostToolUse, PreCompact and PostCompact each carry one command.
// It touches the activity stamp beside the events file and exits 0.
// A run directory path with a literal apostrophe must not break out of the hooks' single-quoted shell argument.
// The embedded quote is escaped with the standard sh idiom.
//
//testtiming:keep pins the recording hooks' exact commands and their single-quote escaping, which its covering test does not assert
func TestBuildSettings_RecordingHooks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		eventsPath string
		quoted     string
		// quotedStamp is the quoted activity stamp path beside the events file.
		quotedStamp string
	}{
		{"plain_path", "/c/run/events.jsonl", `'/c/run/events.jsonl'`, `'/c/run/activity-stamp'`},
		{"embedded_single_quote_escaped", `/c/run's dir/events.jsonl`, `'/c/run'\''s dir/events.jsonl'`, `'/c/run'\''s dir/activity-stamp'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := buildSettings(tt.eventsPath, false, shuttleengine.Config{}, Config{}, false, false, "", "", "", sessionScope{})
			if err != nil {
				t.Fatalf("buildSettings() error: %v", err)
			}
			doc := parseSettings(t, data)
			payload := `cat >> ` + tt.quoted + ` && printf '\n' >> ` + tt.quoted
			for _, hook := range []struct {
				event   string
				command string
			}{
				{"Stop", stampPrintf("Stop", tt.quoted) + "; " + payload},
				{"UserPromptSubmit", stampPrintf("UserPromptSubmit", tt.quoted) + "; " + payload + "; true"},
				{"StopFailure", stampPrintf("StopFailure", tt.quoted) + "; " + payload + "; true"},
				{"Notification", stampPrintf("Notification", tt.quoted) + "; " + payload + "; true"},
				{"SessionEnd", stampPrintf("SessionEnd", tt.quoted) + "; " + payload + "; true"},
				{"SessionStart", stampPrintf("SessionStart", tt.quoted) + "; " + payload + "; true"},
				{"PermissionRequest", stampPrintf("PermissionRequest", tt.quoted) + "; " + payload + "; true"},
				{"PostToolUse", "touch " + tt.quotedStamp + "; true"},
				{"PreCompact", "touch " + tt.quotedStamp + "; true"},
				{"PostCompact", "touch " + tt.quotedStamp + "; true"},
			} {
				entries := hooksFor(doc, hook.event)
				if len(entries) != 1 {
					t.Fatalf("%s hooks = %v; want exactly one entry", hook.event, entries)
				}
				entry, _ := entries[0].(map[string]any)
				if _, hasMatcher := entry["matcher"]; hasMatcher {
					t.Errorf("%s entry has a matcher field; want none: %v", hook.event, entry)
				}
				innerHooks, _ := entry["hooks"].([]any)
				if len(innerHooks) != 1 {
					t.Fatalf("%s hooks list = %v; want exactly one command", hook.event, innerHooks)
				}
				cmd, _ := innerHooks[0].(map[string]any)
				if cmd["type"] != "command" {
					t.Errorf("%s hook type = %v; want %q", hook.event, cmd["type"], "command")
				}
				if command, _ := cmd["command"].(string); command != hook.command {
					t.Errorf("%s hook command = %q; want %q", hook.event, command, hook.command)
				}
			}
		})
	}
}

// TestBuildSettings_SessionStartHook pins the SessionStart entries: every source is recorded exactly once.
// An empty command gives one recording entry with no matcher.
// Otherwise the sources other than compaction record in one entry.
// The compaction entry runs the recording command, which ends in `; true`, and then the given command verbatim.
func TestBuildSettings_SessionStartHook(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const quoted = `'/c/run/events.jsonl'`
	const command = "lym orch resume-context"
	recording := stampPrintf("SessionStart", quoted) + "; cat >> " + quoted + " && printf '\\n' >> " + quoted + "; true"
	tests := []struct {
		name         string
		command      string
		wantMatchers []string
		wantCommands [][]string
	}{
		{"empty_command_records_every_source_in_one_entry", "", []string{""}, [][]string{{recording}}},
		{
			"set_command_splits_compaction_from_the_other_sources",
			command,
			[]string{"startup|resume|clear", "compact"},
			[][]string{{recording}, {recording, command}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data, err := buildSettings("/c/run/events.jsonl", false, shuttleengine.Config{}, Config{}, false, false, tt.command, "", "", sessionScope{})
			if err != nil {
				t.Fatalf("buildSettings() error: %v", err)
			}
			entries := hooksFor(parseSettings(t, data), "SessionStart")
			if len(entries) != len(tt.wantMatchers) {
				t.Fatalf("SessionStart hooks = %v; want %d entries", entries, len(tt.wantMatchers))
			}
			for i, raw := range entries {
				entry, _ := raw.(map[string]any)
				matcher, _ := entry["matcher"].(string)
				if matcher != tt.wantMatchers[i] {
					t.Errorf("entry %d matcher = %q; want %q", i, matcher, tt.wantMatchers[i])
				}
				inner, _ := entry["hooks"].([]any)
				if len(inner) != len(tt.wantCommands[i]) {
					t.Fatalf("entry %d commands = %v; want %d", i, inner, len(tt.wantCommands[i]))
				}
				for j, want := range tt.wantCommands[i] {
					cmd, _ := inner[j].(map[string]any)
					if got, _ := cmd["command"].(string); got != want {
						t.Errorf("entry %d command %d = %q; want %q", i, j, got, want)
					}
				}
			}
		})
	}
}

// TestHookContext pins the exact JSON the session-start hook and creel's hook output print, with the text escaped:
// The mail is the additional context, and the operator's system message is the echo.
// The mail after a tool call comes with no echo, and no output carries a decision or a reason.
func TestHookContext(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const mail = `read "role.md"`
	const echo = "[creel] Delivered while idle:\nread \"role.md\""
	hookOutput := func(event shuttleengine.HookEvent, mail, echo string) func() ([]byte, error) {
		return func() ([]byte, error) { return New().HookOutput(event, mail, echo) }
	}
	sessionStart := func() ([]byte, error) { return SessionStartContext(mail) }
	tests := []struct {
		name   string
		render func() ([]byte, error)
		want   string
	}{
		{"SessionStart", sessionStart, `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"read \"role.md\""}}`},
		{"ToolBoundaryWithMail", hookOutput(shuttleengine.HookEventToolBoundary, mail, ""), `{"systemMessage":"read \"role.md\"","hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"read \"role.md\""}}`},
		{"ToolBoundaryWithMailAndEcho", hookOutput(shuttleengine.HookEventToolBoundary, mail, echo), `{"systemMessage":"[creel] Delivered while idle:\nread \"role.md\"\nread \"role.md\"","hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"read \"role.md\""}}`},
		{"ToolBoundaryWithEchoAlone", hookOutput(shuttleengine.HookEventToolBoundary, "", echo), `{"systemMessage":"[creel] Delivered while idle:\nread \"role.md\""}`},
		{"StopWithMail", hookOutput(shuttleengine.HookEventStop, mail, ""), `{"hookSpecificOutput":{"hookEventName":"Stop","additionalContext":"read \"role.md\""}}`},
		{"StopWithMailAndEcho", hookOutput(shuttleengine.HookEventStop, mail, echo), `{"systemMessage":"[creel] Delivered while idle:\nread \"role.md\"","hookSpecificOutput":{"hookEventName":"Stop","additionalContext":"read \"role.md\""}}`},
		{"StopWithEchoAlone", hookOutput(shuttleengine.HookEventStop, "", echo), `{"systemMessage":"[creel] Delivered while idle:\nread \"role.md\""}`},
	}
	for _, tt := range tests {
		got, err := tt.render()
		if err != nil {
			t.Fatalf("%s context error: %v", tt.name, err)
		}
		if string(got) != tt.want {
			t.Errorf("%s context = %s; want %s", tt.name, got, tt.want)
		}
	}
}

// TestBuildSettings_CreelMailHook pins the creel hooks in a hub and out of one:
// PostToolUse carries the mail hook, an entry with no matcher after the activity stamp.
// The hook checks the flag directory it is given.
// Stop carries two entries: the merged turn-end command, and the idle waiter.
// The idle waiter sets asyncRewake and an 86400-second timeout.
// Told no flag directory, PostToolUse carries the activity stamp alone and Stop the one plain recording.
func TestBuildSettings_CreelMailHook(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const eventsPath = "/c/run/events.jsonl"
	quotedEventsPath := shQuote(eventsPath)
	quotedFlagDir := `'/c/hub'\''s/` + fabricengine.BoltDirName + `/.lym/creel/flags'`
	type stopHook struct {
		command       string
		asyncRewake   bool
		timeout       float64
		rewakeMessage string
		rewakeSummary string
	}
	tests := []struct {
		name     string
		flagDir  string
		wantPost []string
		wantStop []stopHook
	}{
		{
			"InAHub", `/c/hub's/` + fabricengine.BoltDirName + `/.lym/creel/flags`,
			[]string{"touch '/c/run/activity-stamp'; true", creelMailHookCommand(quotedFlagDir)},
			[]stopHook{
				{command: creelStopHookCommand(quotedFlagDir, quotedEventsPath, "")},
				{command: creelWaitHookCommand(), asyncRewake: true, timeout: 86400, rewakeMessage: creelRewakeMessage, rewakeSummary: creelRewakeSummary},
			},
		},
		{"Standalone", "", []string{"touch '/c/run/activity-stamp'; true"}, []stopHook{{command: recordingCommand(hookEventStop, quotedEventsPath)}}},
	}
	for _, tt := range tests {
		data, err := buildSettings(eventsPath, false, shuttleengine.Config{}, Config{}, false, false, "", "", tt.flagDir, sessionScope{})
		if err != nil {
			t.Fatalf("buildSettings(%s) error: %v", tt.name, err)
		}
		var gotPost []string
		for _, raw := range hooksFor(parseSettings(t, data), "PostToolUse") {
			entry, _ := raw.(map[string]any)
			if _, hasMatcher := entry["matcher"]; hasMatcher {
				t.Errorf("%s: PostToolUse entry carries a matcher: %v", tt.name, entry)
			}
			hooks, _ := entry["hooks"].([]any)
			for _, hook := range hooks {
				command, _ := hook.(map[string]any)["command"].(string)
				gotPost = append(gotPost, command)
			}
		}
		if !reflect.DeepEqual(gotPost, tt.wantPost) {
			t.Errorf("%s: PostToolUse commands = %q; want %q", tt.name, gotPost, tt.wantPost)
		}
		var gotStop []stopHook
		for _, raw := range hooksFor(parseSettings(t, data), "Stop") {
			entry, _ := raw.(map[string]any)
			if _, hasMatcher := entry["matcher"]; hasMatcher {
				t.Errorf("%s: Stop entry carries a matcher: %v", tt.name, entry)
			}
			hooks, _ := entry["hooks"].([]any)
			for _, raw := range hooks {
				hook, _ := raw.(map[string]any)
				command, _ := hook["command"].(string)
				asyncRewake, _ := hook["asyncRewake"].(bool)
				timeout, _ := hook["timeout"].(float64)
				rewakeMessage, _ := hook["rewakeMessage"].(string)
				rewakeSummary, _ := hook["rewakeSummary"].(string)
				gotStop = append(gotStop, stopHook{command, asyncRewake, timeout, rewakeMessage, rewakeSummary})
			}
		}
		if !reflect.DeepEqual(gotStop, tt.wantStop) {
			t.Errorf("%s: Stop hooks = %+v; want %+v", tt.name, gotStop, tt.wantStop)
		}
	}
}

// firstHookCommand returns the command of the first hook of the first entry of the settings document's event.
func firstHookCommand(t *testing.T, doc map[string]any, event string) string {
	t.Helper()
	entries := hooksFor(doc, event)
	if len(entries) == 0 {
		t.Fatalf("%s hooks are absent", event)
	}
	entry, _ := entries[0].(map[string]any)
	hooks, _ := entry["hooks"].([]any)
	if len(hooks) == 0 {
		t.Fatalf("%s entry holds no hook", event)
	}
	command, _ := hooks[0].(map[string]any)["command"].(string)
	return command
}

// TestBuildSettings_TurnHookCommand pins the UserPromptSubmit and Stop commands a turn hook command adds.
// Both read the input once and pipe it to the recording, then to the hook step run from the project directory.
// The prompt command ends in `; true`; the Stop command exits with the recording's status.
// In a hub the merged creel Stop command runs the step first, before creel's delivery and the recording.
// The commands without a turn hook are pinned by the recording and creel tests.
func TestBuildSettings_TurnHookCommand(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		eventsPath = "/c/run/events.jsonl"
		quoted     = `'/c/run/events.jsonl'`
		flagDir    = "/c/hub/flags"
	)
	step := `printf '%s' "$in" | (cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null; lym slack hook claude) >/dev/null 2>&1`
	record := func(event string) string {
		return `printf '%s' "$in" | { ` + stampPrintf(event, quoted) + "; cat >> " + quoted + " && printf '\\n' >> " + quoted + `; }`
	}
	wantPrompt := `in=$(cat); ` + record("UserPromptSubmit") + `; ` + step + `; true`
	wantStop := `in=$(cat); ` + record("Stop") + `; s=$?; ` + step + `; exit $s`
	wantMergedStop := `in=$(cat); ` + step + `; ` + creelStopDelivery(shQuote(flagDir)) + `; ` + record("Stop")

	tests := []struct {
		name     string
		flagDir  string
		wantStop string
	}{
		{"OutsideAHub", "", wantStop},
		{"InAHub", flagDir, wantMergedStop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data, err := buildSettings(eventsPath, false, shuttleengine.Config{}, Config{}, false, false, "", "lym slack hook", tt.flagDir, sessionScope{})
			if err != nil {
				t.Fatalf("buildSettings() error: %v", err)
			}
			doc := parseSettings(t, data)
			if got := firstHookCommand(t, doc, "UserPromptSubmit"); got != wantPrompt {
				t.Errorf("UserPromptSubmit command = %q; want %q", got, wantPrompt)
			}
			if got := firstHookCommand(t, doc, "Stop"); got != tt.wantStop {
				t.Errorf("Stop command = %q; want %q", got, tt.wantStop)
			}
		})
	}
}

func TestBuildSettings_DenyToggleMatrix(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name             string
		askUserDeny      bool
		interactive      bool
		wantAskUserEntry bool
		pythonDeny       bool
		fork             bool
		denyPublish      bool
	}{
		{"all_off_autonomous", false, false, false, false, false, false},
		{"askuser_on_autonomous", true, false, true, false, false, false},
		// Interactive runs always carry the non-denying AskUserQuestion marker entry, regardless of DenyAskUserQuestion.
		// The deny is autonomous-only, and the marker and the deny are mutually exclusive.
		{"askuser_on_interactive_marker_not_deny", true, true, true, false, false, false},
		{"askuser_off_interactive_marker_still_present", false, true, true, false, false, false},
		{"askuser_on_fork", true, false, true, false, true, false},
		// The python deny is independent of the others and installed in every run mode.
		{"python_only_autonomous", false, false, false, true, false, false},
		{"python_only_interactive", false, true, true, true, false, false},
		{"python_only_fork", false, false, false, true, true, false},
		// The publish deny is installed only on the launch-spec switch, in every run mode.
		{"publish_on", false, false, false, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := shuttleengine.Config{DenyPython: tt.pythonDeny}
			claude := Config{DenyAskUserQuestion: tt.askUserDeny}
			data, err := buildSettings("/c/run/events.jsonl", tt.interactive, cfg, claude, tt.fork, tt.denyPublish, "", "", "", sessionScope{})
			if err != nil {
				t.Fatalf("buildSettings() error: %v", err)
			}
			doc := parseSettings(t, data)
			preToolUse := hooksFor(doc, "PreToolUse")

			askUserCommand := func() (string, bool) {
				for _, e := range preToolUse {
					entry, _ := e.(map[string]any)
					if entry["matcher"] != "AskUserQuestion" {
						continue
					}
					hooks, _ := entry["hooks"].([]any)
					if len(hooks) == 0 {
						return "", true
					}
					cmd, _ := hooks[0].(map[string]any)
					command, _ := cmd["command"].(string)
					return command, true
				}
				return "", false
			}
			command, present := askUserCommand()
			if present != tt.wantAskUserEntry {
				t.Errorf("AskUserQuestion PreToolUse entry present = %v; want %v (preToolUse: %v)", present, tt.wantAskUserEntry, preToolUse)
			}
			if present && tt.interactive {
				// The interactive marker must be non-denying, with no deny JSON.
				// It must equal the Stop hook's command, with its stamp naming PreToolUse.
				if strings.Contains(command, "permissionDecision") {
					t.Errorf("interactive AskUserQuestion command = %q; want no deny JSON", command)
				}
				stop := hooksFor(doc, "Stop")
				stopEntry, _ := stop[0].(map[string]any)
				stopHooks, _ := stopEntry["hooks"].([]any)
				stopCmd, _ := stopHooks[0].(map[string]any)
				stopCommand, _ := stopCmd["command"].(string)
				wantCommand := strings.Replace(stopCommand, `"lym_stamp":"Stop"`, `"lym_stamp":"PreToolUse"`, 1)
				if command != wantCommand {
					t.Errorf("interactive AskUserQuestion command = %q; want it to equal the Stop hook command with a PreToolUse stamp %q", command, wantCommand)
				}
			}
			if present && !tt.interactive {
				// The autonomous deny must carry the deny JSON payload.
				if !strings.Contains(command, "permissionDecision") {
					t.Errorf("autonomous AskUserQuestion command = %q; want the deny JSON payload", command)
				}
			}
			pythonInstalled, rawGoInstalled, shellEditInstalled, computedRmInstalled, sleepInstalled, headlessClaudeInstalled, publishInstalled := false, false, false, false, false, false, false
			for _, command := range matcherCommands(doc, "Bash") {
				pythonInstalled = pythonInstalled || strings.Contains(command, shellRow(t, "python").Steer)
				rawGoInstalled = rawGoInstalled || strings.Contains(command, shellRow(t, "raw-go").Steer)
				shellEditInstalled = shellEditInstalled || strings.Contains(command, shellRow(t, "shell-edit").Steer)
				computedRmInstalled = computedRmInstalled || strings.Contains(command, shellRow(t, "computed-rm").Steer)
				sleepInstalled = sleepInstalled || strings.Contains(command, shellRow(t, "machine-sleep").Steer)
				headlessClaudeInstalled = headlessClaudeInstalled || strings.Contains(command, shellRow(t, "headless-claude").Steer)
				publishInstalled = publishInstalled || strings.Contains(command, shellRow(t, "publish").Steer)
			}
			if pythonInstalled != tt.pythonDeny {
				t.Errorf("python Bash PreToolUse entry present = %v; want %v (preToolUse: %v)", pythonInstalled, tt.pythonDeny, preToolUse)
			}
			// The raw go deny has no toggle: every run mode installs it.
			if !rawGoInstalled {
				t.Errorf("raw go Bash PreToolUse entry present = false; want it installed under every input (preToolUse: %v)", preToolUse)
			}
			// The shell-edit, computed-rm, sleep and headless claude denies have no toggle either.
			if !headlessClaudeInstalled {
				t.Errorf("headless claude Bash PreToolUse entry present = false; want it installed under every input (preToolUse: %v)", preToolUse)
			}
			if !shellEditInstalled {
				t.Errorf("shell-edit Bash PreToolUse entry present = false; want it installed under every input (preToolUse: %v)", preToolUse)
			}
			if !computedRmInstalled {
				t.Errorf("computed-rm Bash PreToolUse entry present = false; want it installed under every input (preToolUse: %v)", preToolUse)
			}
			if !sleepInstalled {
				t.Errorf("sleep Bash PreToolUse entry present = false; want it installed under every input (preToolUse: %v)", preToolUse)
			}
			if publishInstalled != tt.denyPublish {
				t.Errorf("publish Bash PreToolUse entry present = %v; want %v (preToolUse: %v)", publishInstalled, tt.denyPublish, preToolUse)
			}
			if !tt.wantAskUserEntry && !tt.pythonDeny && !tt.denyPublish && !tt.fork && len(preToolUse) != 5 {
				t.Errorf("PreToolUse = %v with no denies/marker configured; want only the raw go, shell-edit, computed-rm, sleep and headless claude denies", preToolUse)
			}
		})
	}
}

// TestStandingDenies_RenderEveryShellGuardRow pins that every shell guard row has exactly one standingDeniesFor entry.
// Each entry has matcher Bash and carries its row's steer and notice and the command shape of its pattern count.
// The refuse-raw row of a non-empty list is included, in table order after the Claude rows.
func TestStandingDenies_RenderEveryShellGuardRow(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	rows := shellguard.Rows(bashForegroundPhrase, sampleRefuseRaw)
	denies := standingDeniesFor(sampleRefuseRaw)
	claudeCount := len(claudeDenies())
	if len(denies) != claudeCount+len(rows) {
		t.Fatalf("standingDeniesFor has %d entries; want %d Claude entries plus %d shell guard rows", len(denies), claudeCount, len(rows))
	}
	for i, row := range rows {
		deny := denies[claudeCount+i]
		if deny.matcher != "Bash" || deny.steer != row.Steer || deny.notice != row.Notice {
			t.Errorf("standingDeniesFor[%d] = {%q, steer %q, notice %q}; want the %q row's Bash entry with its steer and notice", claudeCount+i, deny.matcher, deny.steer, deny.notice, row.Name)
		}
		multiPattern := strings.HasPrefix(deny.command, "in=$(cat); ")
		if multiPattern != (len(row.Patterns) > 1) {
			t.Errorf("%q command = %q; want the several-pattern shape exactly when the row has several patterns", row.Name, deny.command)
		}
		if got := strings.Count(deny.command, "grep -Eq '"); got != len(row.Patterns) {
			t.Errorf("%q command greps %d times; want one grep per pattern (%d)", row.Name, got, len(row.Patterns))
		}
		if !strings.HasSuffix(deny.command, "echo '"+denyJSON(row.Steer)+"'; true") {
			t.Errorf("%q command = %q; want it to end with its deny JSON and `; true`", row.Name, deny.command)
		}
	}
}

// TestBuildSettings_NoForbiddenCharsInSteerText pins that no steer or notice text carries a forbidden character.
//
//testtiming:keep a guard that fires when a steer constant gains a forbidden character, which no covering test asserts
func TestBuildSettings_NoForbiddenCharsInSteerText(t *testing.T) {
	tiergate.Require(t, 0)
	// Each steer constant rides inside a JSON string literal, so a literal `"` or `\` would corrupt the payload.
	// The JSON sits inside a single-quoted echo argument under git-bash, so a literal `'` would corrupt the hook command.
	// All three characters must stay absent.
	for _, deny := range standingDeniesFor(sampleRefuseRaw) {
		if strings.ContainsAny(deny.steer, steerTextForbiddenChars) {
			t.Errorf("steer text contains a forbidden character (one of %q): %q", steerTextForbiddenChars, deny.steer)
		}
		if strings.ContainsAny(deny.notice, noticeTextForbiddenChars) {
			t.Errorf("notice text contains a forbidden character (one of %q): %q", noticeTextForbiddenChars, deny.notice)
		}
	}
}

// matcherCommands returns the first command of every PreToolUse entry with the given matcher.
// An entry with no hooks contributes an empty command.
func matcherCommands(doc map[string]any, matcher string) []string {
	var commands []string
	for _, e := range hooksFor(doc, "PreToolUse") {
		entry, _ := e.(map[string]any)
		if entry["matcher"] != matcher {
			continue
		}
		hooks, _ := entry["hooks"].([]any)
		if len(hooks) == 0 {
			commands = append(commands, "")
			continue
		}
		cmd, _ := hooks[0].(map[string]any)
		command, _ := cmd["command"].(string)
		commands = append(commands, command)
	}
	return commands
}

// TestBuildSettings_NoAgentHookAndForkGuard covers the PreToolUse entries that forkSubagents and AskUserQuestion decide.
// It also checks that no input installs an Agent entry.
// The raw go, shell-edit, computed-rm, sleep and headless claude denies are the Bash entries every document carries.
// The fork-loop-deadlock guard is the only other Bash entry, so no run mode installs a Bash stdin rewrite.
// A fork-mode run emits a PreToolUse(Bash) hook that denies a call with steerWebsterForkDeny.
// It matches only a fork-context agent_id together with a `lym webster` command.
// The hook always exits 0 through the trailing `; true`.
// The guard is absent entirely when fork mode is off (no Master, so no fork could reach the loop).
//
//testtiming:keep pins the Agent and Bash PreToolUse entries for every fork and AskUserQuestion combination, which its covering test does not assert
func TestBuildSettings_NoAgentHookAndForkGuard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	askDeny := Config{DenyAskUserQuestion: true}
	tests := []struct {
		name          string
		claude        Config
		interactive   bool
		fork          bool
		wantBashGuard bool
		wantAskDeny   bool
	}{
		{name: "fork_on_installs_the_bash_guard", claude: Config{}, fork: true, wantBashGuard: true},
		{name: "fork_off_installs_no_bash_guard", claude: Config{}},
		{name: "autonomous", claude: askDeny, wantAskDeny: true},
		{name: "interactive", claude: askDeny, interactive: true},
		{name: "fork_with_ask_deny", claude: askDeny, fork: true, wantBashGuard: true, wantAskDeny: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := buildSettings("/c/run/events.jsonl", tt.interactive, shuttleengine.Config{}, tt.claude, tt.fork, false, "", "", "", sessionScope{})
			if err != nil {
				t.Fatalf("buildSettings() error: %v", err)
			}
			doc := parseSettings(t, data)

			if agentCommands := matcherCommands(doc, "Agent"); len(agentCommands) != 0 {
				t.Errorf("Agent PreToolUse entries = %q; want none (data: %s)", agentCommands, data)
			}

			// The raw go, shell-edit, computed-rm, sleep and headless claude denies are installed under every input;
			// the rest of the Bash entries are the fork guard.
			var bashCommands, rawGoCommands, shellEditCommands, computedRmCommands, sleepCommands, headlessClaudeCommands []string
			for _, command := range matcherCommands(doc, "Bash") {
				switch {
				case strings.Contains(command, shellRow(t, "raw-go").Steer):
					rawGoCommands = append(rawGoCommands, command)
				case strings.Contains(command, shellRow(t, "shell-edit").Steer):
					shellEditCommands = append(shellEditCommands, command)
				case strings.Contains(command, shellRow(t, "computed-rm").Steer):
					computedRmCommands = append(computedRmCommands, command)
				case strings.Contains(command, shellRow(t, "machine-sleep").Steer):
					sleepCommands = append(sleepCommands, command)
				case strings.Contains(command, shellRow(t, "headless-claude").Steer):
					headlessClaudeCommands = append(headlessClaudeCommands, command)
				default:
					bashCommands = append(bashCommands, command)
				}
			}
			if len(headlessClaudeCommands) != 1 {
				t.Errorf("headless claude Bash PreToolUse entries = %q; want exactly one (data: %s)", headlessClaudeCommands, data)
			}
			if len(rawGoCommands) != 1 {
				t.Errorf("raw go Bash PreToolUse entries = %q; want exactly one (data: %s)", rawGoCommands, data)
			}
			if len(shellEditCommands) != 1 {
				t.Errorf("shell-edit Bash PreToolUse entries = %q; want exactly one (data: %s)", shellEditCommands, data)
			}
			if len(computedRmCommands) != 1 {
				t.Errorf("computed-rm Bash PreToolUse entries = %q; want exactly one (data: %s)", computedRmCommands, data)
			}
			if len(sleepCommands) != 1 {
				t.Errorf("sleep Bash PreToolUse entries = %q; want exactly one (data: %s)", sleepCommands, data)
			}
			if !tt.wantBashGuard {
				if len(bashCommands) != 0 {
					t.Errorf("Bash PreToolUse entries = %q; want none (data: %s)", bashCommands, data)
				}
			} else {
				if len(bashCommands) != 1 {
					t.Fatalf("Bash PreToolUse entries = %q; want exactly the webster fork guard (data: %s)", bashCommands, data)
				}
				command := bashCommands[0]
				// The two AND-ed detection predicates: fork context (agent_id) and a lym webster command.
				for _, want := range []string{`"agent_id"`, `lym[[:space:]]+webster`, steerWebsterForkDeny} {
					if !strings.Contains(command, want) {
						t.Errorf("Bash guard command = %q; want it to contain %q", command, want)
					}
				}
				// A non-matching grep exits non-zero, and the trailing `; true` keeps the hook's exit code 0.
				// A non-fork or non-webster call is therefore allowed rather than reported as a hook error.
				if !strings.HasSuffix(command, "; true") {
					t.Errorf("Bash guard command = %q; want it to end with `; true` so a non-deny path exits 0", command)
				}
			}

			askCommands := matcherCommands(doc, "AskUserQuestion")
			notice := buildDenyNotice(tt.interactive, shuttleengine.Config{}, tt.claude, tt.fork, false)
			if tt.wantAskDeny {
				if len(askCommands) != 1 || !strings.Contains(askCommands[0], steerAskUserQuestionDeny) {
					t.Errorf("AskUserQuestion commands = %q; want the deny kept", askCommands)
				}
				if !strings.Contains(notice, noticeAskUserQuestionDeny) {
					t.Errorf("notice = %q; want the AskUserQuestion sentence kept", notice)
				}
				for _, text := range append(askCommands, notice) {
					if !strings.Contains(text, "waits for the answer, which arrives as your next turn") {
						t.Errorf("AskUserQuestion text = %q; want it to say the run waits for the answer as the next turn", text)
					}
				}
			}
		})
	}
}

// TestPrepare_PromptLaunchLimit pins that the launch line carries a pointer to prompt.md, never the prompt text,
// so a prompt too large for the launch line still launches, and prompt.md holds all of it.
// Only a pointer over maxLaunchPromptBytes is rejected, which a pathological run-directory path can cause.
// The rejection happens before any run artifact is written.
func TestPrepare_PromptLaunchLimit(t *testing.T) {
	tiergate.Require(t, 0)
	cfg := shuttleengine.Config{}
	c := engineWith(t, templateConfig(t))

	t.Run("LargePrompt_LaunchesWithPointer", func(t *testing.T) {
		runDir := t.TempDir()
		prompt := strings.Repeat("p", maxLaunchPromptBytes+1)
		launch, err := c.Prepare(runDir, shuttleengine.Spec{Prompt: prompt}, cfg)
		if err != nil {
			t.Fatalf("Prepare() with an over-30000-byte prompt error: %v; want nil", err)
		}
		promptPath, err := filepath.Abs(filepath.Join(runDir, "prompt.md"))
		if err != nil {
			t.Fatalf("Abs: %v", err)
		}
		if !strings.Contains(launch.Cmd, launchPointer(promptPath)) {
			t.Errorf("launch cmd = %q; want it to carry the pointer to %s", launch.Cmd, promptPath)
		}
		if strings.Contains(launch.Cmd, prompt) {
			t.Error("launch cmd carries the prompt text; want only the pointer")
		}
		got, err := os.ReadFile(promptPath)
		if err != nil {
			t.Fatalf("read prompt.md: %v", err)
		}
		if string(got) != prompt {
			t.Errorf("prompt.md holds %d bytes; want the full %d-byte prompt", len(got), len(prompt))
		}
	})

	t.Run("PathologicalRunDir_RejectedBeforeArtifacts", func(t *testing.T) {
		base := t.TempDir()
		runDir := filepath.Join(base, strings.Repeat("d", maxLaunchPromptBytes))
		_, err := c.Prepare(runDir, shuttleengine.Spec{Prompt: "p"}, cfg)
		if err == nil {
			t.Fatal("Prepare() with an over-limit pointer = nil error; want the launch-limit rejection")
		}
		if !strings.Contains(err.Error(), "launch limit") {
			t.Errorf("Prepare() error = %q; want it to name the launch limit", err)
		}
		// The run directory itself is too long to stat, so assert nothing was created beside it.
		if entries, readErr := os.ReadDir(base); readErr != nil || len(entries) != 0 {
			t.Errorf("ReadDir(%s) = %v entries, err=%v; want no artifacts written after a rejected Prepare", base, len(entries), readErr)
		}
	})
}

// TestPrepare_CreelMailHookFollowsTheBoardDirectory pins the creel flag directory Prepare derives from the board.
// It marks the launch CreelHooks.
// An engine in no hub writes the activity stamp alone on PostToolUse and marks nothing.
func TestPrepare_CreelMailHookFollowsTheBoardDirectory(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	boardDir := t.TempDir()
	flagDir := shuttleengine.CreelFlagDir(boardDir)
	if runtime.GOOS == "windows" {
		posixFlagDir, err := shuttleengine.PosixPath(flagDir)
		if err != nil {
			t.Fatalf("PosixPath(%q) error: %v", flagDir, err)
		}
		flagDir = posixFlagDir
	}
	tests := []struct {
		name  string
		board string
		// mail is the creel mail command expected after the activity stamp; empty expects the stamp alone.
		mail string
	}{
		{"InAHub", boardDir, creelMailHookCommand(shQuote(flagDir))},
		{"Standalone", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			engine := engineWith(t, templateConfig(t))
			engine.dirs.Board = tt.board
			runDir := t.TempDir()
			launch, err := engine.Prepare(runDir, shuttleengine.Spec{Prompt: "do the thing"}, shuttleengine.Config{})
			if err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}
			if launch.CreelHooks != (tt.board != "") {
				t.Errorf("Launch.CreelHooks = %v; want %v", launch.CreelHooks, tt.board != "")
			}
			settingsBytes, err := os.ReadFile(filepath.Join(runDir, "settings.json"))
			if err != nil {
				t.Fatalf("read settings.json: %v", err)
			}
			var got []string
			for _, raw := range hooksFor(parseSettings(t, settingsBytes), "PostToolUse") {
				entry, _ := raw.(map[string]any)
				if _, hasMatcher := entry["matcher"]; hasMatcher {
					t.Errorf("PostToolUse entry carries a matcher: %v", entry)
				}
				hooks, _ := entry["hooks"].([]any)
				for _, hook := range hooks {
					command, _ := hook.(map[string]any)["command"].(string)
					got = append(got, command)
				}
			}
			eventsPathForHook := filepath.Join(runDir, "events.jsonl")
			if runtime.GOOS == "windows" {
				if eventsPathForHook, err = shuttleengine.PosixPath(eventsPathForHook); err != nil {
					t.Fatal(err)
				}
			}
			want := []string{"touch " + shQuote(path.Join(path.Dir(eventsPathForHook), shuttleengine.ActivityStampFileName)) + "; true"}
			if tt.mail != "" {
				want = append(want, tt.mail)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("PostToolUse commands = %q; want %q", got, want)
			}
		})
	}
}

// TestPrepare_WritesArtifactsAndReturnsConsistentLaunch pins the artifacts Prepare leaves on disk.
// It also pins that the launch and resume lines agree with them.
//
//testtiming:keep pins the prompt.md, settings.json and bash-env.sh contents and the env-file lead on both lines, which its covering test does not assert
func TestPrepare_WritesArtifactsAndReturnsConsistentLaunch(t *testing.T) {
	tiergate.Require(t, 0)
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing", Interactive: false}
	claude := templateConfig(t)
	claude.DenyAskUserQuestion = true

	c := engineWith(t, claude)
	launch, err := c.Prepare(runDir, spec, shuttleengine.Config{})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}

	promptPath := filepath.Join(runDir, "prompt.md")
	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read prompt.md: %v", err)
	}
	if string(promptBytes) != spec.Prompt {
		t.Errorf("prompt.md = %q; want %q", promptBytes, spec.Prompt)
	}

	settingsPath := filepath.Join(runDir, "settings.json")
	settingsBytes, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	doc := parseSettings(t, settingsBytes)
	if len(hooksFor(doc, "Stop")) != 1 {
		t.Errorf("settings.json missing its Stop hook entry: %s", settingsBytes)
	}

	appendPromptBytes, err := os.ReadFile(filepath.Join(runDir, "append-system-prompt.txt"))
	if err != nil {
		t.Fatalf("read append-system-prompt.txt: %v", err)
	}
	if want := buildDenyNotice(false, shuttleengine.Config{}, claude, false, false) + " " + shuttleengine.CreelTrustSentence; string(appendPromptBytes) != want {
		t.Errorf("append-system-prompt.txt = %q; want the deny notice and the trust sentence %q", appendPromptBytes, want)
	}

	if launch.SessionID == "" {
		t.Error("Launch.SessionID is empty")
	}
	if !launch.RecordsSessionStarts {
		t.Error("Launch.RecordsSessionStarts = false; want true, since the settings record every session start")
	}
	if !strings.Contains(launch.Cmd, launch.SessionID) {
		t.Errorf("Launch.Cmd = %q; want it to embed SessionID %q", launch.Cmd, launch.SessionID)
	}
	if !strings.Contains(launch.Cmd, "--dangerously-skip-permissions") {
		t.Errorf("Launch.Cmd = %q; want --dangerously-skip-permissions for an autonomous spec", launch.Cmd)
	}
	if !strings.Contains(launch.ResumeCmd, launch.SessionID) {
		t.Errorf("Launch.ResumeCmd = %q; want it to embed SessionID %q", launch.ResumeCmd, launch.SessionID)
	}
	if !strings.Contains(launch.ResumeCmd, "--resume") {
		t.Errorf("Launch.ResumeCmd = %q; want --resume, never --continue", launch.ResumeCmd)
	}
	if strings.Contains(launch.ResumeCmd, "--continue") {
		t.Errorf("Launch.ResumeCmd = %q; must never use --continue (ambiguous under concurrent runs)", launch.ResumeCmd)
	}

	envFilePath, err := filepath.Abs(filepath.Join(runDir, "bash-env.sh"))
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	envBytes, err := os.ReadFile(envFilePath)
	if err != nil {
		t.Fatalf("read bash-env.sh: %v", err)
	}
	if want := envFileContentFor(runtime.GOOS); string(envBytes) != want {
		t.Errorf("bash-env.sh = %q; want %q", envBytes, want)
	}
	wantLead := shell.ForGOOS().WithEnv(envFileKey, envFilePath, "")
	if !strings.HasPrefix(launch.Cmd, wantLead) {
		t.Errorf("Launch.Cmd = %q; want it to lead with %q", launch.Cmd, wantLead)
	}
	if !strings.HasPrefix(launch.ResumeCmd, wantLead) {
		t.Errorf("Launch.ResumeCmd = %q; want it to lead with %q", launch.ResumeCmd, wantLead)
	}

	// The refuse list comes from the gate config under the engine's dirs:
	// a seeded list reaches settings.json and the notice, an absent file adds no row,
	// and an unparsable file refuses the spawn before any artifact.
	var refuseRow shellguard.Row
	for _, row := range shellguard.Rows(bashForegroundPhrase, []string{"go test"}) {
		if row.Name == "refuse-raw" {
			refuseRow = row
		}
	}
	gateTests := []struct {
		name        string
		gate        string
		wantBash    int
		wantRefused bool
		wantErr     bool
	}{
		{"SeededRefuseRawReachesSettingsAndNotice", "slots: 2\nrefuse_raw: [\"go test\"]\n", 6, true, false},
		{"AbsentGateConfigRendersNoRefuseRawRow", "", 5, false, false},
		{"UnparsableGateConfigRefusesBeforeArtifacts", "slots: [\n", 0, false, true},
	}
	for _, tt := range gateTests {
		t.Run(tt.name, func(t *testing.T) {
			engine, anchor := engineWithAnchor(t)
			if tt.gate != "" {
				writeClaudeLayer(t, configengine.ConfigFile(anchor, "gate"), tt.gate)
			}
			gateRunDir := t.TempDir()
			gateLaunch, err := engine.Prepare(gateRunDir, spec, shuttleengine.Config{})
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "gate") {
					t.Errorf("Prepare() error = %v; want a refusal naming the gate config", err)
				}
				if entries, readErr := os.ReadDir(gateRunDir); readErr != nil || len(entries) != 0 {
					t.Errorf("ReadDir(%s) = %d entries, err=%v; want no artifacts after a refused Prepare", gateRunDir, len(entries), readErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}
			gateSettings, err := os.ReadFile(filepath.Join(gateRunDir, "settings.json"))
			if err != nil {
				t.Fatalf("read settings.json: %v", err)
			}
			commands := matcherCommands(parseSettings(t, gateSettings), "Bash")
			if len(commands) != tt.wantBash {
				t.Errorf("settings.json carries %d Bash denies; want %d", len(commands), tt.wantBash)
			}
			gotRefuse := false
			for _, command := range commands {
				if strings.Contains(command, refuseRow.Steer) {
					gotRefuse = true
				}
			}
			if gotRefuse != tt.wantRefused {
				t.Errorf("settings.json carries the refuse-raw deny = %v; want %v", gotRefuse, tt.wantRefused)
			}
			gatePrompt, err := os.ReadFile(filepath.Join(gateRunDir, "append-system-prompt.txt"))
			if err != nil {
				t.Fatalf("read append-system-prompt.txt: %v", err)
			}
			if got := strings.Contains(string(gatePrompt), refuseRow.Notice); got != tt.wantRefused {
				t.Errorf("append-system-prompt.txt names the refuse-raw notice = %v; want %v", got, tt.wantRefused)
			}
			if !strings.Contains(gateLaunch.Cmd, "--append-system-prompt-file") {
				t.Errorf("Launch.Cmd = %q; want it to name the append-system-prompt file", gateLaunch.Cmd)
			}
		})
	}
}

// TestBuildDenyNotice_MatchesInstalledDenies pins that the notice names exactly the denies buildSettings installs.
// It checks every deny combination.
//
//testtiming:keep pins the notice sentences against the installed hooks over every combination, which its covering test checks only for the append-system-prompt.txt file
func TestBuildDenyNotice_MatchesInstalledDenies(t *testing.T) {
	tiergate.Require(t, 0)
	for _, askUserDeny := range []bool{false, true} {
		for _, pythonDeny := range []bool{false, true} {
			for _, interactive := range []bool{false, true} {
				for _, fork := range []bool{false, true} {
					cfg := shuttleengine.Config{DenyPython: pythonDeny, RefuseRaw: sampleRefuseRaw}
					claude := Config{DenyAskUserQuestion: askUserDeny}
					data, err := buildSettings("/c/run/events.jsonl", interactive, cfg, claude, fork, false, "", "", "", sessionScope{})
					if err != nil {
						t.Fatalf("buildSettings() error: %v", err)
					}
					var wantSentences []string
					for _, e := range hooksFor(parseSettings(t, data), "PreToolUse") {
						entry, _ := e.(map[string]any)
						hooks, _ := entry["hooks"].([]any)
						cmd, _ := hooks[0].(map[string]any)
						command, _ := cmd["command"].(string)
						for _, deny := range standingDeniesFor(cfg.RefuseRaw) {
							if deny.command == command && deny.notice != "" {
								wantSentences = append(wantSentences, deny.notice)
							}
						}
					}
					want := strings.Join(wantSentences, " ")
					notice := buildDenyNotice(interactive, cfg, claude, fork, false)
					if notice != want {
						t.Errorf("ask=%v python=%v interactive=%v fork=%v: notice = %q; want the sentences of the installed denies %q", askUserDeny, pythonDeny, interactive, fork, notice, want)
					}
					if strings.ContainsAny(notice, noticeTextForbiddenChars) {
						t.Errorf("notice contains a forbidden character: %q", notice)
					}
					if strings.Contains(notice, "lym"+" webster") {
						t.Errorf("notice mentions the webster verbs: %q", notice)
					}
				}
			}
		}
	}
}

// TestBuildDenyNotice_SentenceContent pins the notice wording of the autonomous, interactive and fork run modes.
//
//testtiming:keep pins the computed-rm, sleep, headless claude, gate rule and final-message wording of the notice, which its covering test does not assert
func TestBuildDenyNotice_SentenceContent(t *testing.T) {
	tiergate.Require(t, 0)
	cfg := shuttleengine.Config{}
	claude := Config{DenyAskUserQuestion: true}
	nonFork := buildDenyNotice(false, cfg, claude, false, false)
	for name, notice := range map[string]string{"autonomous": nonFork, "interactive": buildDenyNotice(true, cfg, claude, false, false), "fork": buildDenyNotice(false, cfg, claude, true, false)} {
		for _, row := range []string{"computed-rm", "machine-sleep", "headless-claude"} {
			if !strings.Contains(notice, shellRow(t, row).Notice) {
				t.Errorf("%s notice = %q; want it to contain the %s sentence", name, notice, row)
			}
		}
	}
	for _, want := range []string{
		"Gate rule: run lym gate test in the foreground with the Bash tool timeout set to 3600000, never in the background",
		"never cd into the directory you already work in",
	} {
		if !strings.Contains(nonFork, want) {
			t.Errorf("non-fork notice = %q; want it to contain %q", nonFork, want)
		}
	}
	if strings.Contains(nonFork, "background Bash call") {
		t.Errorf("non-fork notice = %q; want no background Bash call route", nonFork)
	}
	if !strings.Contains(nonFork, "final message") {
		t.Errorf("notice = %q; want the AskUserQuestion final-message channel", nonFork)
	}
}

// goldenLabel names one combination of the deny inputs as the settings golden file keys it.
func goldenLabel(interactive, fork, denyAsk, denyPython bool) string {
	return fmt.Sprintf("interactive=%t fork=%t denyAsk=%t denyPython=%t", interactive, fork, denyAsk, denyPython)
}

// TestBuildSettings_MatchesGolden pins the settings document and deny notice byte for byte.
// It covers every combination of the four deny inputs.
func TestBuildSettings_MatchesGolden(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	type golden struct {
		Settings string `json:"settings"`
		Notice   string `json:"notice"`
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "settings-golden.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var want map[string]golden
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}

	rendered := map[string]bool{}
	for mask := 0; mask < 16; mask++ {
		bit := func(position int) bool { return mask&(1<<position) != 0 }
		interactive, fork := bit(0), bit(1)
		cfg := shuttleengine.Config{DenyPython: bit(3)}
		claude := Config{DenyAskUserQuestion: bit(2)}
		label := goldenLabel(interactive, fork, claude.DenyAskUserQuestion, cfg.DenyPython)
		rendered[label] = true

		entry, ok := want[label]
		if !ok {
			t.Errorf("golden lacks %q", label)
			continue
		}
		settings, err := buildSettings("/c/run/events.jsonl", interactive, cfg, claude, fork, false, "", "", "", sessionScope{})
		if err != nil {
			t.Fatalf("buildSettings(%s): %v", label, err)
		}
		if string(settings) != entry.Settings {
			t.Errorf("settings differ for %q:\ngot  %s\nwant %s", label, settings, entry.Settings)
		}
		if notice := buildDenyNotice(interactive, cfg, claude, fork, false); notice != entry.Notice {
			t.Errorf("notice differs for %q:\ngot  %q\nwant %q", label, notice, entry.Notice)
		}
	}
	for label := range want {
		if !rendered[label] {
			t.Errorf("golden holds %q, which the test does not render", label)
		}
	}
}

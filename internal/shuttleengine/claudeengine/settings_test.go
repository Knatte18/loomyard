//go:build tier0

// settings_test.go covers buildSettings' JSON composition across the agent-deny/askuser-deny toggle matrix and the interactive/autonomous split, the fork-mode conditional Agent hook and Bash guard, asserts the events path is embedded in its POSIX form, checks the no-single-quote steer invariant, and exercises Prepare end to end against a real temp directory.

package claudeengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// parseSettings unmarshals data into the generic shape buildSettings
// produces so tests can assert on it without depending on the unexported
// settingsDoc type's own (de)serialization.
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

// TestBuildSettings_PromptSuggestionOff pins promptSuggestionEnabled as false in every run mode.
//
//testtiming:keep pins promptSuggestionEnabled=false in the interactive, autonomous and fork documents, which its covering tests do not assert
func TestBuildSettings_PromptSuggestionOff(t *testing.T) {
	cases := []struct {
		name        string
		interactive bool
		fork        bool
	}{
		{"interactive", true, false},
		{"autonomous", false, false},
		{"fork", false, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			data, err := buildSettings("/c/run/events.jsonl", tt.interactive, shuttleengine.Config{}, Config{}, tt.fork, false, "", "")
			if err != nil {
				t.Fatalf("buildSettings() error: %v", err)
			}
			doc := parseSettings(t, data)
			v, ok := doc["promptSuggestionEnabled"]
			if !ok {
				t.Fatalf("promptSuggestionEnabled missing; data: %s", data)
			}
			if b, isBool := v.(bool); !isBool || b {
				t.Errorf("promptSuggestionEnabled = %v, want false", v)
			}
		})
	}
}

// stampPrintf is the stamp line command a recording hook runs for hookEventName before its payload, written out by hand so the test pins the line shape.
func stampPrintf(hookEventName, quotedEventsPath string) string {
	return `printf '{"lym_stamp":"` + hookEventName + `","lym_at":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> ` + quotedEventsPath
}

// TestBuildSettings_RecordingHooks pins the recording hooks every document carries: one command entry per event with no tool matcher, each appending a stamp line and then the payload to the events path in its POSIX form.
// Stop keeps the payload append's exit status, and the other hooks end in `; true` so they exit 0 whatever the append does.
// A run directory path containing a literal apostrophe (an unusual but legal Windows path character, e.g. a worktree named "operator's-box") must not break out of the hooks' single-quoted shell argument: the embedded quote is escaped via the standard sh idiom rather than passed through raw.
//
//testtiming:keep pins the recording hooks' exact commands and their single-quote escaping, which its covering test does not assert
func TestBuildSettings_RecordingHooks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		eventsPath string
		quoted     string
	}{
		{"plain_path", "/c/run/events.jsonl", `'/c/run/events.jsonl'`},
		{"embedded_single_quote_escaped", `/c/run's dir/events.jsonl`, `'/c/run'\''s dir/events.jsonl'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := buildSettings(tt.eventsPath, false, shuttleengine.Config{}, Config{}, false, false, "", "")
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
// An empty command gives one recording entry with no matcher; otherwise the sources but compaction record in one entry, and the compaction entry runs the recording command ending in `; true` and then the given command verbatim.
func TestBuildSettings_SessionStartHook(t *testing.T) {
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
			data, err := buildSettings("/c/run/events.jsonl", false, shuttleengine.Config{}, Config{}, false, false, tt.command, "")
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

// TestHookContext pins the exact JSON the session-start hook and creel's boundary block print, with the text escaped.
func TestHookContext(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		render func(string) ([]byte, error)
		want   string
	}{
		{"SessionStart", SessionStartContext, `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"read \"role.md\""}}`},
		{"BoundaryBlock", New().BoundaryBlock, `{"decision":"block","reason":"read \"role.md\"","systemMessage":"read \"role.md\""}`},
	}
	for _, tt := range tests {
		got, err := tt.render(`read "role.md"`)
		if err != nil {
			t.Fatalf("%s context error: %v", tt.name, err)
		}
		if string(got) != tt.want {
			t.Errorf("%s context = %s; want %s", tt.name, got, tt.want)
		}
	}
}

// TestBuildSettings_CreelMailHook pins the creel hooks in a hub and out of one:
// the PostToolUse mail hook as one entry with no matcher testing the flag directory it is told, and the two Stop entries, the merged turn-end command and the idle waiter with asyncRewake and its 86400-second timeout.
// Told no flag directory, the document carries no PostToolUse key and the one plain Stop recording.
func TestBuildSettings_CreelMailHook(t *testing.T) {
	t.Parallel()
	const eventsPath = "/c/run/events.jsonl"
	quotedEventsPath := shQuote(eventsPath)
	type stopHook struct {
		command     string
		asyncRewake bool
		timeout     float64
	}
	tests := []struct {
		name     string
		flagDir  string
		wantPost []string
		wantStop []stopHook
	}{
		{
			"InAHub", `/c/hub's/_board/.lym/creel/flags`,
			[]string{creelMailHookCommand(`'/c/hub'\''s/_board/.lym/creel/flags'`)},
			[]stopHook{
				{command: creelStopHookCommand(`'/c/hub'\''s/_board/.lym/creel/flags'`, quotedEventsPath)},
				{command: creelWaitHookCommand(), asyncRewake: true, timeout: 86400},
			},
		},
		{"Standalone", "", nil, []stopHook{{command: recordingCommand(hookEventStop, quotedEventsPath)}}},
	}
	for _, tt := range tests {
		data, err := buildSettings(eventsPath, false, shuttleengine.Config{}, Config{}, false, false, "", tt.flagDir)
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
				gotStop = append(gotStop, stopHook{command, asyncRewake, timeout})
			}
		}
		if !reflect.DeepEqual(gotStop, tt.wantStop) {
			t.Errorf("%s: Stop hooks = %+v; want %+v", tt.name, gotStop, tt.wantStop)
		}
	}
}

func TestBuildSettings_DenyToggleMatrix(t *testing.T) {
	tests := []struct {
		name             string
		agentDeny        bool
		askUserDeny      bool
		interactive      bool
		wantAgentEntry   bool
		wantAskUserEntry bool
		pythonDeny       bool
		fork             bool
	}{
		{"both_off_autonomous", false, false, false, false, false, false, false},
		{"agent_only_autonomous", true, false, false, true, false, false, false},
		{"askuser_only_autonomous", false, true, false, false, true, false, false},
		{"both_on_autonomous", true, true, false, true, true, false, false},
		// Interactive runs always carry the non-denying AskUserQuestion
		// marker entry, regardless of DenyAskUserQuestion — the deny
		// is autonomous-only and the two are mutually exclusive.
		{"both_on_interactive_marker_not_deny", true, true, true, true, true, false, false},
		{"askuser_only_interactive_marker_not_deny", false, true, true, false, true, false, false},
		{"both_off_interactive_marker_still_present", false, false, true, false, true, false, false},
		// The python deny is independent of the other two and installed in every run mode.
		{"python_only_autonomous", false, false, false, false, false, true, false},
		{"python_only_interactive", false, false, true, false, true, true, false},
		{"python_only_fork", false, false, false, false, false, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := shuttleengine.Config{DenyPython: tt.pythonDeny}
			claude := Config{DenyAgentTool: tt.agentDeny, DenyAskUserQuestion: tt.askUserDeny}
			data, err := buildSettings("/c/run/events.jsonl", tt.interactive, cfg, claude, tt.fork, false, "", "")
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
			hasMatcher := func(matcher string) bool {
				for _, e := range preToolUse {
					entry, _ := e.(map[string]any)
					if entry["matcher"] == matcher {
						return true
					}
				}
				return false
			}

			if got := hasMatcher("Agent"); got != tt.wantAgentEntry {
				t.Errorf("Agent PreToolUse entry present = %v; want %v (preToolUse: %v)", got, tt.wantAgentEntry, preToolUse)
			}
			command, present := askUserCommand()
			if present != tt.wantAskUserEntry {
				t.Errorf("AskUserQuestion PreToolUse entry present = %v; want %v (preToolUse: %v)", present, tt.wantAskUserEntry, preToolUse)
			}
			if present && tt.interactive {
				// The interactive marker must be non-denying (no deny JSON)
				// and must be the Stop hook's command with its stamp naming PreToolUse.
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
			pythonInstalled, rawGoInstalled, computedRmInstalled, sleepInstalled := false, false, false, false
			for _, command := range matcherCommands(doc, "Bash") {
				pythonInstalled = pythonInstalled || strings.Contains(command, shellRow(t, "python").Steer)
				rawGoInstalled = rawGoInstalled || strings.Contains(command, shellRow(t, "raw-go").Steer)
				computedRmInstalled = computedRmInstalled || strings.Contains(command, shellRow(t, "computed-rm").Steer)
				sleepInstalled = sleepInstalled || strings.Contains(command, shellRow(t, "machine-sleep").Steer)
			}
			if pythonInstalled != tt.pythonDeny {
				t.Errorf("python Bash PreToolUse entry present = %v; want %v (preToolUse: %v)", pythonInstalled, tt.pythonDeny, preToolUse)
			}
			// The raw go deny has no toggle: every run mode installs it.
			if !rawGoInstalled {
				t.Errorf("raw go Bash PreToolUse entry present = false; want it installed under every input (preToolUse: %v)", preToolUse)
			}
			// The computed-rm and sleep denies have no toggle either.
			if !computedRmInstalled {
				t.Errorf("computed-rm Bash PreToolUse entry present = false; want it installed under every input (preToolUse: %v)", preToolUse)
			}
			if !sleepInstalled {
				t.Errorf("sleep Bash PreToolUse entry present = false; want it installed under every input (preToolUse: %v)", preToolUse)
			}
			if !tt.wantAgentEntry && !tt.wantAskUserEntry && !tt.pythonDeny && len(preToolUse) != 3 {
				t.Errorf("PreToolUse = %v with no denies/marker configured; want only the raw go, computed-rm and sleep denies", preToolUse)
			}
		})
	}
}

// TestStandingDenies_RenderEveryShellGuardRow pins that every shell guard row, the refuse-raw row of a non-empty list included, has exactly one standingDeniesFor entry with matcher Bash, its steer and notice and the command shape of its pattern count, in table order after the Claude rows.
func TestStandingDenies_RenderEveryShellGuardRow(t *testing.T) {
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

// TestBuildSettings_NoForbiddenCharsInSteerText pins that no steer constant carries a character that would corrupt the hook command it rides in.
//
//testtiming:keep a guard that fires when a steer constant gains a forbidden character, which no covering test asserts
func TestBuildSettings_NoForbiddenCharsInSteerText(t *testing.T) {
	// Each steer constant rides inside a JSON string literal (so a literal
	// `"` or `\` would corrupt the payload) nested inside a single-quoted
	// echo argument under git-bash (so a literal `'` would corrupt the
	// hook command) — all three characters must stay absent.
	for _, deny := range standingDeniesFor(sampleRefuseRaw) {
		if strings.ContainsAny(deny.steer, steerTextForbiddenChars) {
			t.Errorf("steer text contains a forbidden character (one of %q): %q", steerTextForbiddenChars, deny.steer)
		}
		if strings.ContainsAny(deny.notice, noticeTextForbiddenChars) {
			t.Errorf("notice text contains a forbidden character (one of %q): %q", noticeTextForbiddenChars, deny.notice)
		}
	}
}

// matcherCommands returns the first command of every PreToolUse entry with the given matcher; an entry with no hooks contributes an empty command.
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

// agentHook names which Agent PreToolUse hook buildSettings installs.
type agentHook int

const (
	agentHookNone agentHook = iota
	agentHookBlanketDeny
	agentHookForkConditional
)

// TestBuildSettings_AgentAndBashHooks covers how claude.DenyAgentTool, forkSubagents and the per-run Agent allowance decide the Agent and Bash PreToolUse entries.
// Fork mode on with the deny configured replaces the blanket Agent deny with the conditional grep hook;
// fork mode off leaves the blanket deny;
// the deny configured off, or the per-run allowance, emits no Agent entry and drops its notice sentence while the AskUserQuestion deny and its sentence stay.
// Beside the always-installed raw go, computed-rm and sleep denies, the fork-loop-deadlock guard is the only Bash entry any document carries, so no run mode installs a Bash stdin rewrite:
// a fork-mode run emits a PreToolUse(Bash) hook that greps the payload for a fork-context agent_id AND a `lym webster` command before denying (with steerWebsterForkDeny), and always exits 0 via the trailing `; true`.
// The guard is independent of DenyAgentTool and of the allowance, and absent entirely when fork mode is off (no Master, so no fork could reach the loop).
//
//testtiming:keep pins the Agent and Bash PreToolUse entries for every deny, fork and allowance combination, which its covering test does not assert
func TestBuildSettings_AgentAndBashHooks(t *testing.T) {
	t.Parallel()

	denyBoth := Config{DenyAgentTool: true, DenyAskUserQuestion: true}
	tests := []struct {
		name          string
		claude        Config
		interactive   bool
		fork          bool
		allowAgent    bool
		wantAgent     agentHook
		wantBashGuard bool
		wantAskDeny   bool
	}{
		{name: "fork_on_replaces_blanket_deny", claude: Config{DenyAgentTool: true}, fork: true, wantAgent: agentHookForkConditional, wantBashGuard: true},
		{name: "fork_off_keeps_blanket_deny", claude: Config{DenyAgentTool: true}, wantAgent: agentHookBlanketDeny},
		{name: "deny_off_and_fork_on_emits_no_agent_entry_but_keeps_bash_guard", claude: Config{}, fork: true, wantAgent: agentHookNone, wantBashGuard: true},
		{name: "autonomous", claude: denyBoth, wantAgent: agentHookBlanketDeny, wantAskDeny: true},
		{name: "interactive", claude: denyBoth, interactive: true, wantAgent: agentHookBlanketDeny},
		{name: "allow_agent_without_fork", claude: denyBoth, allowAgent: true, wantAgent: agentHookNone, wantAskDeny: true},
		{name: "allow_agent_with_fork_keeps_bash_guard", claude: denyBoth, fork: true, allowAgent: true, wantAgent: agentHookNone, wantBashGuard: true, wantAskDeny: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := buildSettings("/c/run/events.jsonl", tt.interactive, shuttleengine.Config{}, tt.claude, tt.fork, tt.allowAgent, "", "")
			if err != nil {
				t.Fatalf("buildSettings() error: %v", err)
			}
			doc := parseSettings(t, data)

			agentCommands := matcherCommands(doc, "Agent")
			switch tt.wantAgent {
			case agentHookNone:
				if len(agentCommands) != 0 {
					t.Errorf("Agent PreToolUse entries = %q; want none (data: %s)", agentCommands, data)
				}
			case agentHookBlanketDeny:
				if len(agentCommands) != 1 || !strings.Contains(agentCommands[0], steerAgentDeny) || strings.Contains(agentCommands[0], `"subagent_type":"fork"`) {
					t.Errorf("Agent PreToolUse entries = %q; want exactly the unchanged blanket steerAgentDeny with no conditional fork-allow grep", agentCommands)
				}
			case agentHookForkConditional:
				if len(agentCommands) != 1 || !strings.Contains(agentCommands[0], `"subagent_type":"fork"`) ||
					!strings.Contains(agentCommands[0], steerAgentNonForkDeny) || strings.Contains(agentCommands[0], steerAgentDeny) {
					t.Errorf("Agent PreToolUse entries = %q; want exactly the conditional hook with the subagent_type fork grep and steerAgentNonForkDeny, not the blanket steerAgentDeny", agentCommands)
				}
			}

			// The raw go, computed-rm and sleep denies are installed under every input; the rest of the Bash entries are the fork guard.
			var bashCommands, rawGoCommands, computedRmCommands, sleepCommands []string
			for _, command := range matcherCommands(doc, "Bash") {
				switch {
				case strings.Contains(command, shellRow(t, "raw-go").Steer):
					rawGoCommands = append(rawGoCommands, command)
				case strings.Contains(command, shellRow(t, "computed-rm").Steer):
					computedRmCommands = append(computedRmCommands, command)
				case strings.Contains(command, shellRow(t, "machine-sleep").Steer):
					sleepCommands = append(sleepCommands, command)
				default:
					bashCommands = append(bashCommands, command)
				}
			}
			if len(rawGoCommands) != 1 {
				t.Errorf("raw go Bash PreToolUse entries = %q; want exactly one (data: %s)", rawGoCommands, data)
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
				// A non-matching grep exits non-zero; the trailing `; true` keeps the hook's own exit code 0 so a non-fork/non-webster call is allowed, never a spurious hook error.
				if !strings.HasSuffix(command, "; true") {
					t.Errorf("Bash guard command = %q; want it to end with `; true` so a non-deny path exits 0", command)
				}
			}

			askCommands := matcherCommands(doc, "AskUserQuestion")
			notice := buildDenyNotice(tt.interactive, shuttleengine.Config{}, tt.claude, tt.fork, tt.allowAgent)
			if tt.allowAgent {
				if strings.Contains(notice, noticeAgentDeny) || strings.Contains(notice, noticeAgentForkDeny) {
					t.Errorf("notice = %q; want no Agent sentence under AllowAgentTool", notice)
				}
			}
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
// so a prompt over the old 30000-byte bound launches and prompt.md holds all of it.
// Only a pointer over maxLaunchPromptBytes, reached by a pathological run-directory path, is rejected, before any run artifact is written.
func TestPrepare_PromptLaunchLimit(t *testing.T) {
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

// TestPrepare_CreelMailHookFollowsTheBoardDirectory pins that Prepare derives the creel flag directory from the hub's board directory and marks the launch CreelHooks, and that an engine in no hub writes no PostToolUse hook and marks nothing.
func TestPrepare_CreelMailHookFollowsTheBoardDirectory(t *testing.T) {
	t.Parallel()
	boardDir := t.TempDir()
	flagDir := CreelFlagDir(boardDir)
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
		want  []string
	}{
		{"InAHub", boardDir, []string{creelMailHookCommand(shQuote(flagDir))}},
		{"Standalone", "", nil},
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PostToolUse commands = %q; want %q", got, tt.want)
			}
		})
	}
}

// TestPrepare_WritesArtifactsAndReturnsConsistentLaunch pins the artifacts Prepare leaves on disk and that the launch and resume lines agree with them.
//
//testtiming:keep pins the prompt.md, settings.json and bash-env.sh contents and the env-file lead on both lines, which its covering test does not assert
func TestPrepare_WritesArtifactsAndReturnsConsistentLaunch(t *testing.T) {
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing", Interactive: false}
	claude := templateConfig(t)
	claude.DenyAgentTool, claude.DenyAskUserQuestion = true, true

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
	if want := buildDenyNotice(false, shuttleengine.Config{}, claude, false, false) + " " + creelTrustSentence; string(appendPromptBytes) != want {
		t.Errorf("append-system-prompt.txt = %q; want the deny notice, a space and the trust sentence %q", appendPromptBytes, want)
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

	// The refuse list comes from the gate config under the engine's dirs: a seeded list reaches settings.json and the notice, an absent file adds no row, and an unparsable file refuses the spawn before any artifact.
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
		{"SeededRefuseRawReachesSettingsAndNotice", "slots: 2\nrefuse_raw: [\"go test\"]\n", 4, true, false},
		{"AbsentGateConfigRendersNoRefuseRawRow", "", 3, false, false},
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

// TestBuildDenyNotice_MatchesInstalledDenies pins that the notice names exactly the denies buildSettings installs, over every deny combination.
//
//testtiming:keep pins the notice sentences against the installed hooks over every combination, which its covering test checks only for the append-system-prompt.txt file
func TestBuildDenyNotice_MatchesInstalledDenies(t *testing.T) {
	for _, agentDeny := range []bool{false, true} {
		for _, askUserDeny := range []bool{false, true} {
			for _, pythonDeny := range []bool{false, true} {
				for _, interactive := range []bool{false, true} {
					for _, fork := range []bool{false, true} {
						cfg := shuttleengine.Config{DenyPython: pythonDeny, RefuseRaw: sampleRefuseRaw}
						claude := Config{DenyAgentTool: agentDeny, DenyAskUserQuestion: askUserDeny}
						data, err := buildSettings("/c/run/events.jsonl", interactive, cfg, claude, fork, false, "", "")
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
							t.Errorf("agent=%v ask=%v python=%v interactive=%v fork=%v: notice = %q; want the sentences of the installed denies %q", agentDeny, askUserDeny, pythonDeny, interactive, fork, notice, want)
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
}

// TestBuildDenyNotice_SentenceContent pins the wording the fork and non-fork notices carry.
//
//testtiming:keep pins the fork-subagent and Agent-unavailable wording of the notice, which its covering test does not assert
func TestBuildDenyNotice_SentenceContent(t *testing.T) {
	cfg := shuttleengine.Config{}
	claude := Config{DenyAgentTool: true, DenyAskUserQuestion: true}
	forkNotice := buildDenyNotice(false, cfg, claude, true, false)
	if !strings.Contains(forkNotice, "fork subagents") || !strings.Contains(forkNotice, "never spawns further subagents") {
		t.Errorf("fork notice = %q; want it to name fork subagents as permitted and forbid a fork spawning more", forkNotice)
	}
	nonFork := buildDenyNotice(false, cfg, claude, false, false)
	for name, notice := range map[string]string{"autonomous": nonFork, "interactive": buildDenyNotice(true, cfg, claude, false, false), "fork": forkNotice} {
		if !strings.Contains(notice, shellRow(t, "computed-rm").Notice) {
			t.Errorf("%s notice = %q; want it to contain the computed-rm sentence", name, notice)
		}
		if !strings.Contains(notice, shellRow(t, "machine-sleep").Notice) {
			t.Errorf("%s notice = %q; want it to contain the sleep sentence", name, notice)
		}
	}
	if !strings.Contains(nonFork, "The Agent tool is unavailable") {
		t.Errorf("non-fork notice = %q; want the Agent tool named unavailable", nonFork)
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

// TestBuildSettings_MatchesGolden pins the settings document and deny notice byte for byte across every combination of the six deny inputs.
func TestBuildSettings_MatchesGolden(t *testing.T) {
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
	for mask := 0; mask < 64; mask++ {
		bit := func(position int) bool { return mask&(1<<position) != 0 }
		interactive, fork, allowAgent := bit(0), bit(1), bit(2)
		cfg := shuttleengine.Config{DenyPython: bit(5)}
		claude := Config{DenyAgentTool: bit(3), DenyAskUserQuestion: bit(4)}
		label := fmt.Sprintf("interactive=%t fork=%t allowAgent=%t denyAgent=%t denyAsk=%t denyPython=%t",
			interactive, fork, allowAgent, claude.DenyAgentTool, claude.DenyAskUserQuestion, cfg.DenyPython)
		rendered[label] = true

		entry, ok := want[label]
		if !ok {
			t.Errorf("golden lacks %q", label)
			continue
		}
		settings, err := buildSettings("/c/run/events.jsonl", interactive, cfg, claude, fork, allowAgent, "", "")
		if err != nil {
			t.Fatalf("buildSettings(%s): %v", label, err)
		}
		if string(settings) != entry.Settings {
			t.Errorf("settings differ for %q:\ngot  %s\nwant %s", label, settings, entry.Settings)
		}
		if notice := buildDenyNotice(interactive, cfg, claude, fork, allowAgent); notice != entry.Notice {
			t.Errorf("notice differs for %q:\ngot  %q\nwant %q", label, notice, entry.Notice)
		}
	}
	for label := range want {
		if !rendered[label] {
			t.Errorf("golden holds %q, which the test does not render", label)
		}
	}
}

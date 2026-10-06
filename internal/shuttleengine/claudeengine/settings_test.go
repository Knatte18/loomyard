// settings_test.go covers buildSettings' JSON composition across the agent-deny/askuser-deny toggle matrix and the interactive/autonomous split, the fork-mode conditional Agent hook and Bash guard, asserts the events path is embedded in its POSIX form, checks the no-single-quote steer invariant, and exercises Prepare end to end against a real temp directory.

package claudeengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
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
			data, err := buildSettings("/c/run/events.jsonl", tt.interactive, shuttleengine.Config{}, tt.fork, false)
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

// TestBuildSettings_StopHook pins the one Stop hook every document carries: a single command entry with no tool matcher that appends the payload to the events path in its POSIX form, followed by a newline guarantee.
// A run directory path containing a literal apostrophe (an unusual but legal Windows path character, e.g. a worktree named "operator's-box") must not break out of the hook's single-quoted shell argument: the embedded quote is escaped via the standard sh idiom rather than passed through raw.
//
//testtiming:keep pins the Stop hook's exact command and its single-quote escaping, which its covering test does not assert
func TestBuildSettings_StopHook(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		eventsPath  string
		wantCommand string
	}{
		{"plain_path", "/c/run/events.jsonl", `cat >> '/c/run/events.jsonl' && printf '\n' >> '/c/run/events.jsonl'`},
		{"embedded_single_quote_escaped", `/c/run's dir/events.jsonl`, `cat >> '/c/run'\''s dir/events.jsonl' && printf '\n' >> '/c/run'\''s dir/events.jsonl'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := buildSettings(tt.eventsPath, false, shuttleengine.Config{}, false, false)
			if err != nil {
				t.Fatalf("buildSettings() error: %v", err)
			}
			stop := hooksFor(parseSettings(t, data), "Stop")
			if len(stop) != 1 {
				t.Fatalf("Stop hooks = %v; want exactly one entry", stop)
			}
			entry, _ := stop[0].(map[string]any)
			if _, hasMatcher := entry["matcher"]; hasMatcher {
				t.Errorf("Stop entry has a matcher field; want none (Stop carries no tool matcher): %v", entry)
			}
			innerHooks, _ := entry["hooks"].([]any)
			if len(innerHooks) != 1 {
				t.Fatalf("Stop hooks list = %v; want exactly one command", innerHooks)
			}
			cmd, _ := innerHooks[0].(map[string]any)
			if cmd["type"] != "command" {
				t.Errorf("Stop hook type = %v; want %q", cmd["type"], "command")
			}
			if command, _ := cmd["command"].(string); command != tt.wantCommand {
				t.Errorf("Stop hook command = %q; want %q", command, tt.wantCommand)
			}
		})
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
	}{
		{"both_off_autonomous", false, false, false, false, false},
		{"agent_only_autonomous", true, false, false, true, false},
		{"askuser_only_autonomous", false, true, false, false, true},
		{"both_on_autonomous", true, true, false, true, true},
		// Interactive runs always carry the non-denying AskUserQuestion
		// marker entry, regardless of ClaudeDenyAskUserQuestion — the deny
		// is autonomous-only and the two are mutually exclusive.
		{"both_on_interactive_marker_not_deny", true, true, true, true, true},
		{"askuser_only_interactive_marker_not_deny", false, true, true, false, true},
		{"both_off_interactive_marker_still_present", false, false, true, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := shuttleengine.Config{ClaudeDenyAgentTool: tt.agentDeny, ClaudeDenyAskUserQuestion: tt.askUserDeny}
			data, err := buildSettings("/c/run/events.jsonl", tt.interactive, cfg, false, false)
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
				// and must reuse the Stop hook's exact append command.
				if strings.Contains(command, "permissionDecision") {
					t.Errorf("interactive AskUserQuestion command = %q; want no deny JSON", command)
				}
				stop := hooksFor(doc, "Stop")
				stopEntry, _ := stop[0].(map[string]any)
				stopHooks, _ := stopEntry["hooks"].([]any)
				stopCmd, _ := stopHooks[0].(map[string]any)
				wantCommand, _ := stopCmd["command"].(string)
				if command != wantCommand {
					t.Errorf("interactive AskUserQuestion command = %q; want it to equal the Stop hook command %q", command, wantCommand)
				}
			}
			if present && !tt.interactive {
				// The autonomous deny must carry the deny JSON payload.
				if !strings.Contains(command, "permissionDecision") {
					t.Errorf("autonomous AskUserQuestion command = %q; want the deny JSON payload", command)
				}
			}
			if !tt.wantAgentEntry && !tt.wantAskUserEntry && len(preToolUse) != 0 {
				t.Errorf("PreToolUse = %v with no denies/marker configured; want none", preToolUse)
			}
		})
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
	for _, steer := range []string{steerAgentDeny, steerAskUserQuestionDeny, steerAgentNonForkDeny, steerWebsterForkDeny} {
		if strings.ContainsAny(steer, steerTextForbiddenChars) {
			t.Errorf("steer text contains a forbidden character (one of %q): %q", steerTextForbiddenChars, steer)
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

// TestBuildSettings_AgentAndBashHooks covers how cfg.ClaudeDenyAgentTool, forkSubagents and the per-run Agent allowance decide the Agent and Bash PreToolUse entries.
// Fork mode on with the deny configured replaces the blanket Agent deny with the conditional grep hook;
// fork mode off leaves the blanket deny;
// the deny configured off, or the per-run allowance, emits no Agent entry and drops its notice sentence while the AskUserQuestion deny and its sentence stay.
// The fork-loop-deadlock guard is the only Bash entry any document carries, so no run mode installs a Bash stdin rewrite:
// a fork-mode run emits a PreToolUse(Bash) hook that greps the payload for a fork-context agent_id AND a `lyx webster` command before denying (with steerWebsterForkDeny), and always exits 0 via the trailing `; true`.
// The guard is independent of ClaudeDenyAgentTool and of the allowance, and absent entirely when fork mode is off (no Master, so no fork could reach the loop).
//
//testtiming:keep pins the Agent and Bash PreToolUse entries for every deny, fork and allowance combination, which its covering test does not assert
func TestBuildSettings_AgentAndBashHooks(t *testing.T) {
	t.Parallel()

	denyBoth := shuttleengine.Config{ClaudeDenyAgentTool: true, ClaudeDenyAskUserQuestion: true}
	tests := []struct {
		name          string
		cfg           shuttleengine.Config
		interactive   bool
		fork          bool
		allowAgent    bool
		wantAgent     agentHook
		wantBashGuard bool
		wantAskDeny   bool
	}{
		{name: "fork_on_replaces_blanket_deny", cfg: shuttleengine.Config{ClaudeDenyAgentTool: true}, fork: true, wantAgent: agentHookForkConditional, wantBashGuard: true},
		{name: "fork_off_keeps_blanket_deny", cfg: shuttleengine.Config{ClaudeDenyAgentTool: true}, wantAgent: agentHookBlanketDeny},
		{name: "deny_off_and_fork_on_emits_no_agent_entry_but_keeps_bash_guard", cfg: shuttleengine.Config{}, fork: true, wantAgent: agentHookNone, wantBashGuard: true},
		{name: "autonomous", cfg: denyBoth, wantAgent: agentHookBlanketDeny, wantAskDeny: true},
		{name: "interactive", cfg: denyBoth, interactive: true, wantAgent: agentHookBlanketDeny},
		{name: "allow_agent_without_fork", cfg: denyBoth, allowAgent: true, wantAgent: agentHookNone, wantAskDeny: true},
		{name: "allow_agent_with_fork_keeps_bash_guard", cfg: denyBoth, fork: true, allowAgent: true, wantAgent: agentHookNone, wantBashGuard: true, wantAskDeny: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := buildSettings("/c/run/events.jsonl", tt.interactive, tt.cfg, tt.fork, tt.allowAgent)
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

			bashCommands := matcherCommands(doc, "Bash")
			if !tt.wantBashGuard {
				if len(bashCommands) != 0 {
					t.Errorf("Bash PreToolUse entries = %q; want none (data: %s)", bashCommands, data)
				}
			} else {
				if len(bashCommands) != 1 {
					t.Fatalf("Bash PreToolUse entries = %q; want exactly the webster fork guard (data: %s)", bashCommands, data)
				}
				command := bashCommands[0]
				// The two AND-ed detection predicates: fork context (agent_id) and a lyx webster command.
				for _, want := range []string{`"agent_id"`, `lyx[[:space:]]+webster`, steerWebsterForkDeny} {
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
			notice := buildDenyNotice(tt.interactive, tt.cfg, tt.fork, tt.allowAgent)
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
			}
		})
	}
}

// TestPrepare_PromptLaunchLimit pins that the launch line carries a pointer to prompt.md, never the prompt text,
// so a prompt over the old 30000-byte bound launches and prompt.md holds all of it.
// Only a pointer over maxLaunchPromptBytes, reached by a pathological run-directory path, is rejected, before any run artifact is written.
func TestPrepare_PromptLaunchLimit(t *testing.T) {
	cfg := shuttleengine.Config{}
	c := New()

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

// TestPrepare_WritesArtifactsAndReturnsConsistentLaunch pins the artifacts Prepare leaves on disk and that the launch and resume lines agree with them.
//
//testtiming:keep pins the prompt.md, settings.json and bash-env.sh contents and the env-file lead on both lines, which its covering test does not assert
func TestPrepare_WritesArtifactsAndReturnsConsistentLaunch(t *testing.T) {
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing", Interactive: false}
	cfg := shuttleengine.Config{ClaudeDenyAgentTool: true, ClaudeDenyAskUserQuestion: true}

	c := New()
	launch, err := c.Prepare(runDir, spec, cfg)
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

	if launch.SessionID == "" {
		t.Error("Launch.SessionID is empty")
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
	if string(envBytes) != envFileContent {
		t.Errorf("bash-env.sh = %q; want %q", envBytes, envFileContent)
	}
	wantLead := shell.ForGOOS().WithEnv(envFileKey, envFilePath, "")
	if !strings.HasPrefix(launch.Cmd, wantLead) {
		t.Errorf("Launch.Cmd = %q; want it to lead with %q", launch.Cmd, wantLead)
	}
	if !strings.HasPrefix(launch.ResumeCmd, wantLead) {
		t.Errorf("Launch.ResumeCmd = %q; want it to lead with %q", launch.ResumeCmd, wantLead)
	}
}

// TestBuildDenyNotice_MatchesInstalledDenies pins that the notice names exactly the denies buildSettings installs, over every deny combination.
//
//testtiming:keep pins the notice sentences against the installed hooks over every combination, which its covering test checks only for the --append-system-prompt flag
func TestBuildDenyNotice_MatchesInstalledDenies(t *testing.T) {
	for _, agentDeny := range []bool{false, true} {
		for _, askUserDeny := range []bool{false, true} {
			for _, interactive := range []bool{false, true} {
				for _, fork := range []bool{false, true} {
					cfg := shuttleengine.Config{ClaudeDenyAgentTool: agentDeny, ClaudeDenyAskUserQuestion: askUserDeny}
					data, err := buildSettings("/c/run/events.jsonl", interactive, cfg, fork, false)
					if err != nil {
						t.Fatalf("buildSettings() error: %v", err)
					}
					wantAgent, wantAsk := false, false
					for _, e := range hooksFor(parseSettings(t, data), "PreToolUse") {
						entry, _ := e.(map[string]any)
						switch entry["matcher"] {
						case "Agent":
							wantAgent = true
						case "AskUserQuestion":
							hooks, _ := entry["hooks"].([]any)
							cmd, _ := hooks[0].(map[string]any)
							command, _ := cmd["command"].(string)
							wantAsk = strings.Contains(command, "permissionDecision")
						}
					}
					notice := buildDenyNotice(interactive, cfg, fork, false)
					hasAgent := strings.Contains(notice, noticeAgentDeny) || strings.Contains(notice, noticeAgentForkDeny)
					if hasAgent != wantAgent {
						t.Errorf("agent=%v ask=%v interactive=%v fork=%v: notice has Agent sentence = %v; want %v", agentDeny, askUserDeny, interactive, fork, hasAgent, wantAgent)
					}
					if got := strings.Contains(notice, noticeAskUserQuestionDeny); got != wantAsk {
						t.Errorf("agent=%v ask=%v interactive=%v fork=%v: notice has AskUserQuestion sentence = %v; want %v", agentDeny, askUserDeny, interactive, fork, got, wantAsk)
					}
					if (notice == "") != (!wantAgent && !wantAsk) {
						t.Errorf("agent=%v ask=%v interactive=%v fork=%v: notice = %q; want empty exactly when no deny installed", agentDeny, askUserDeny, interactive, fork, notice)
					}
					if strings.ContainsAny(notice, noticeTextForbiddenChars) {
						t.Errorf("notice contains a forbidden character: %q", notice)
					}
					if strings.Contains(notice, "lyx"+" webster") {
						t.Errorf("notice mentions the webster verbs: %q", notice)
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
	cfg := shuttleengine.Config{ClaudeDenyAgentTool: true, ClaudeDenyAskUserQuestion: true}
	forkNotice := buildDenyNotice(false, cfg, true, false)
	if !strings.Contains(forkNotice, "fork subagents") || !strings.Contains(forkNotice, "never spawns further subagents") {
		t.Errorf("fork notice = %q; want it to name fork subagents as permitted and forbid a fork spawning more", forkNotice)
	}
	nonFork := buildDenyNotice(false, cfg, false, false)
	if !strings.Contains(nonFork, "The Agent tool is unavailable") {
		t.Errorf("non-fork notice = %q; want the Agent tool named unavailable", nonFork)
	}
	if !strings.Contains(nonFork, "final message") {
		t.Errorf("notice = %q; want the AskUserQuestion final-message channel", nonFork)
	}
}

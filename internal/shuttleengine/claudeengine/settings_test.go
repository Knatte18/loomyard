// settings_test.go covers buildSettings' JSON composition across the
// agent-deny/askuser-deny toggle matrix and the interactive/autonomous
// split, the fork-mode conditional Agent hook, asserts the events path is
// embedded in its POSIX form, checks the no-single-quote steer invariant,
// and exercises Prepare end to end against a real temp directory.

package claudeengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestBuildSettings_StopHookAlwaysPresent(t *testing.T) {
	data, err := buildSettings("/c/run/events.jsonl", false, shuttleengine.Config{}, false)
	if err != nil {
		t.Fatalf("buildSettings() error: %v", err)
	}
	doc := parseSettings(t, data)

	stop := hooksFor(doc, "Stop")
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
	command, _ := cmd["command"].(string)
	if !strings.Contains(command, "/c/run/events.jsonl") {
		t.Errorf("Stop hook command = %q; want it to embed the POSIX events path", command)
	}
	if !strings.HasPrefix(command, "cat >> '/c/run/events.jsonl'") {
		t.Errorf("Stop hook command = %q; want it to start with the cat-append", command)
	}
	if !strings.Contains(command, "printf '\\n' >> '/c/run/events.jsonl'") {
		t.Errorf("Stop hook command = %q; want the trailing printf newline guarantee", command)
	}
}

func TestBuildSettings_EventsPathSingleQuoteEscaped(t *testing.T) {
	// A run directory path containing a literal apostrophe (an unusual but
	// legal Windows path character, e.g. a worktree named "operator's-box")
	// must not be able to break out of the Stop hook's single-quoted shell
	// argument: the embedded quote is escaped via the standard sh idiom
	// rather than passed through raw.
	data, err := buildSettings(`/c/run's dir/events.jsonl`, false, shuttleengine.Config{}, false)
	if err != nil {
		t.Fatalf("buildSettings() error: %v", err)
	}
	doc := parseSettings(t, data)
	stop := hooksFor(doc, "Stop")
	entry, _ := stop[0].(map[string]any)
	innerHooks, _ := entry["hooks"].([]any)
	cmd, _ := innerHooks[0].(map[string]any)
	command, _ := cmd["command"].(string)

	want := `cat >> '/c/run'\''s dir/events.jsonl' && printf '\n' >> '/c/run'\''s dir/events.jsonl'`
	if command != want {
		t.Errorf("Stop hook command = %q; want %q (embedded single quote must be sh-escaped, not passed through raw)", command, want)
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
			data, err := buildSettings("/c/run/events.jsonl", tt.interactive, cfg, false)
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
			if !tt.wantAgentEntry && !tt.wantAskUserEntry {
				hooks, _ := doc["hooks"].(map[string]any)
				if _, present := hooks["PreToolUse"]; present {
					t.Errorf("PreToolUse key present with no denies/marker configured; want the key omitted entirely: %v", hooks)
				}
			}
		})
	}
}

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

// bashCommand returns the Bash PreToolUse entry's command string and whether a
// Bash entry is present at all — the fork-context webster-verb guard's hook.
func bashCommand(t *testing.T, doc map[string]any) (string, bool) {
	t.Helper()
	preToolUse := hooksFor(doc, "PreToolUse")
	for _, e := range preToolUse {
		entry, _ := e.(map[string]any)
		if entry["matcher"] != "Bash" {
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

// TestBuildSettings_ForkContextWebsterGuard pins the fork-loop-deadlock guard:
// a fork-mode run emits a PreToolUse(Bash) hook that greps the payload for a
// fork-context agent_id AND a `lyx webster` command before denying (with
// steerWebsterForkDeny), and always exits 0 via the trailing `; true`. The
// guard is independent of ClaudeDenyAgentTool and absent entirely when fork
// mode is off (no Master, so no fork could reach the loop).
func TestBuildSettings_ForkContextWebsterGuard(t *testing.T) {
	t.Run("fork_mode_emits_bash_guard_independent_of_agent_deny", func(t *testing.T) {
		for _, agentDeny := range []bool{true, false} {
			cfg := shuttleengine.Config{ClaudeDenyAgentTool: agentDeny}
			data, err := buildSettings("/c/run/events.jsonl", false, cfg, true)
			if err != nil {
				t.Fatalf("buildSettings() error: %v", err)
			}
			doc := parseSettings(t, data)
			command, present := bashCommand(t, doc)
			if !present {
				t.Fatalf("Bash PreToolUse guard absent with ClaudeDenyAgentTool=%v; want it present in fork mode regardless", agentDeny)
			}
			// The two AND-ed detection predicates: fork context (agent_id) and
			// a lyx webster command.
			if !strings.Contains(command, `"agent_id"`) {
				t.Errorf("Bash guard command = %q; want the fork-context agent_id grep", command)
			}
			if !strings.Contains(command, `lyx[[:space:]]+webster`) {
				t.Errorf("Bash guard command = %q; want the lyx-webster command grep", command)
			}
			if !strings.Contains(command, steerWebsterForkDeny) {
				t.Errorf("Bash guard command = %q; want it to carry steerWebsterForkDeny", command)
			}
			// A non-matching grep exits non-zero; the trailing `; true` keeps
			// the hook's own exit code 0 so a non-fork/non-webster call is
			// allowed, never a spurious hook error.
			if !strings.HasSuffix(command, "; true") {
				t.Errorf("Bash guard command = %q; want it to end with `; true` so a non-deny path exits 0", command)
			}
		}
	})

	t.Run("non_fork_mode_emits_no_bash_guard", func(t *testing.T) {
		cfg := shuttleengine.Config{ClaudeDenyAgentTool: true}
		data, err := buildSettings("/c/run/events.jsonl", false, cfg, false)
		if err != nil {
			t.Fatalf("buildSettings() error: %v", err)
		}
		doc := parseSettings(t, data)
		if _, present := bashCommand(t, doc); present {
			t.Error("Bash PreToolUse guard present with forkSubagents=false; want none (no fork can reach the loop)")
		}
	})
}

// agentCommand returns the Agent PreToolUse entry's command string and
// whether an Agent entry is present at all.
func agentCommand(t *testing.T, doc map[string]any) (string, bool) {
	t.Helper()
	preToolUse := hooksFor(doc, "PreToolUse")
	for _, e := range preToolUse {
		entry, _ := e.(map[string]any)
		if entry["matcher"] != "Agent" {
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

// TestBuildSettings_ForkMode covers the three-way interaction between
// cfg.ClaudeDenyAgentTool and forkSubagents: fork mode on with the deny
// configured replaces the blanket deny with the conditional grep hook; fork
// mode off leaves today's blanket deny unchanged; and the deny configured
// off entirely emits no Agent entry regardless of fork mode.
func TestBuildSettings_ForkMode(t *testing.T) {
	t.Run("fork_mode_on_replaces_blanket_deny", func(t *testing.T) {
		cfg := shuttleengine.Config{ClaudeDenyAgentTool: true}
		data, err := buildSettings("/c/run/events.jsonl", false, cfg, true)
		if err != nil {
			t.Fatalf("buildSettings() error: %v", err)
		}
		doc := parseSettings(t, data)
		command, present := agentCommand(t, doc)
		if !present {
			t.Fatal("Agent PreToolUse entry absent; want the conditional fork-allow hook")
		}
		if !strings.Contains(command, `"subagent_type":"fork"`) {
			t.Errorf("Agent command = %q; want it to contain the subagent_type fork grep pattern", command)
		}
		if !strings.Contains(command, steerAgentNonForkDeny) {
			t.Errorf("Agent command = %q; want it to contain steerAgentNonForkDeny %q", command, steerAgentNonForkDeny)
		}
		if strings.Contains(command, steerAgentDeny) {
			t.Errorf("Agent command = %q; want it to NOT contain the blanket steerAgentDeny text", command)
		}
	})

	t.Run("fork_mode_off_keeps_blanket_deny", func(t *testing.T) {
		cfg := shuttleengine.Config{ClaudeDenyAgentTool: true}
		data, err := buildSettings("/c/run/events.jsonl", false, cfg, false)
		if err != nil {
			t.Fatalf("buildSettings() error: %v", err)
		}
		doc := parseSettings(t, data)
		command, present := agentCommand(t, doc)
		if !present {
			t.Fatal("Agent PreToolUse entry absent; want today's blanket deny")
		}
		if !strings.Contains(command, steerAgentDeny) {
			t.Errorf("Agent command = %q; want the unchanged blanket steerAgentDeny text", command)
		}
		if strings.Contains(command, `"subagent_type":"fork"`) {
			t.Errorf("Agent command = %q; want no conditional fork-allow grep when fork mode is off", command)
		}
	})

	t.Run("deny_off_and_fork_mode_on_emits_no_agent_entry", func(t *testing.T) {
		cfg := shuttleengine.Config{ClaudeDenyAgentTool: false}
		data, err := buildSettings("/c/run/events.jsonl", false, cfg, true)
		if err != nil {
			t.Fatalf("buildSettings() error: %v", err)
		}
		doc := parseSettings(t, data)
		if _, present := agentCommand(t, doc); present {
			t.Error("Agent PreToolUse entry present; want none when ClaudeDenyAgentTool is false, regardless of fork mode")
		}
	})
}

// TestPrepare_PromptLaunchLimit pins the maxLaunchPromptBytes guard: a
// prompt over the limit is rejected up front with a self-describing error
// (before any run artifact is written), because past the Windows
// command-line ceiling the pane launch is guaranteed to fail and would
// otherwise surface only as an opaque `died` a full startup window later. A
// prompt exactly at the limit still prepares normally.
func TestPrepare_PromptLaunchLimit(t *testing.T) {
	cfg := shuttleengine.Config{}
	c := New()

	t.Run("OverLimit_RejectedBeforeArtifacts", func(t *testing.T) {
		runDir := t.TempDir()
		spec := shuttleengine.Spec{Prompt: strings.Repeat("p", maxLaunchPromptBytes+1)}
		_, err := c.Prepare(runDir, spec, cfg)
		if err == nil {
			t.Fatal("Prepare() with an over-limit prompt = nil error; want the launch-limit rejection")
		}
		if !strings.Contains(err.Error(), "launch limit") {
			t.Errorf("Prepare() error = %q; want it to name the launch limit", err)
		}
		// The rejection must precede artifact writes — a half-prepared run
		// dir would look resumable to a later diagnosis pass.
		if _, statErr := os.Stat(filepath.Join(runDir, "prompt.md")); !os.IsNotExist(statErr) {
			t.Errorf("prompt.md exists after a rejected Prepare (stat err=%v); want no artifacts written", statErr)
		}
	})

	t.Run("AtLimit_Accepted", func(t *testing.T) {
		runDir := t.TempDir()
		spec := shuttleengine.Spec{Prompt: strings.Repeat("p", maxLaunchPromptBytes)}
		if _, err := c.Prepare(runDir, spec, cfg); err != nil {
			t.Fatalf("Prepare() with an at-limit prompt error: %v; want nil", err)
		}
	})
}

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
}

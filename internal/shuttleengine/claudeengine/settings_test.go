// settings_test.go covers buildSettings' JSON composition across the
// agent-deny/askuser-deny toggle matrix and the interactive/autonomous
// split, asserts the events path is embedded in its POSIX form, checks the
// no-single-quote steer invariant, and exercises Prepare end to end against
// a real temp directory.

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
	data, err := buildSettings("/c/run/events.jsonl", false, shuttleengine.Config{})
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
	data, err := buildSettings(`/c/run's dir/events.jsonl`, false, shuttleengine.Config{})
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
		{"both_off", false, false, false, false, false},
		{"agent_only_autonomous", true, false, false, true, false},
		{"askuser_only_autonomous", false, true, false, false, true},
		{"both_on_autonomous", true, true, false, true, true},
		{"both_on_interactive_suppresses_askuser", true, true, true, true, false},
		{"askuser_only_interactive_suppressed", false, true, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := shuttleengine.Config{ClaudeDenyAgentTool: tt.agentDeny, ClaudeDenyAskUserQuestion: tt.askUserDeny}
			data, err := buildSettings("/c/run/events.jsonl", tt.interactive, cfg)
			if err != nil {
				t.Fatalf("buildSettings() error: %v", err)
			}
			doc := parseSettings(t, data)
			preToolUse := hooksFor(doc, "PreToolUse")

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
			if got := hasMatcher("AskUserQuestion"); got != tt.wantAskUserEntry {
				t.Errorf("AskUserQuestion PreToolUse entry present = %v; want %v (preToolUse: %v)", got, tt.wantAskUserEntry, preToolUse)
			}
			if !tt.wantAgentEntry && !tt.wantAskUserEntry {
				hooks, _ := doc["hooks"].(map[string]any)
				if _, present := hooks["PreToolUse"]; present {
					t.Errorf("PreToolUse key present with no denies configured; want the key omitted entirely: %v", hooks)
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
	for _, steer := range []string{steerAgentDeny, steerAskUserQuestionDeny} {
		if strings.ContainsAny(steer, steerTextForbiddenChars) {
			t.Errorf("steer text contains a forbidden character (one of %q): %q", steerTextForbiddenChars, steer)
		}
	}
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

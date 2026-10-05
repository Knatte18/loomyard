//go:build integration

// bashstdin_integration_test.go runs the composed Bash stdin rewrite hook under `sh` with real PreToolUse payloads,
// and runs the rewritten commands to pin that the prefix closes the default stdin without touching an attached heredoc.

package claudeengine

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// bashPayload builds a PreToolUse(Bash) payload the way Claude Code encodes it:
// JSON without HTML escaping, so the hook sees `<` and `>` unescaped.
func bashPayload(t *testing.T, command string) string {
	t.Helper()
	payload := map[string]any{
		"session_id":      "s",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": command},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return buf.String()
}

// runRewriteHook runs the hook command under sh with payload on stdin and returns its stdout.
func runRewriteHook(t *testing.T, payload string) string {
	t.Helper()
	cmd := exec.Command("sh", "-c", bashStdinHookCommand())
	cmd.Stdin = strings.NewReader(payload)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("hook exited non-zero (want 0 always): %v; stdout=%q", err, out)
	}
	return string(out)
}

// rewrittenCommand decodes the hook's answer and returns the command in its updatedInput,
// failing if the answer decides permission.
func rewrittenCommand(t *testing.T, out string) string {
	t.Helper()
	var answer struct {
		HookSpecificOutput map[string]any `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &answer); err != nil {
		t.Fatalf("hook output is not JSON: %v; output: %q", err, out)
	}
	if _, has := answer.HookSpecificOutput["permissionDecision"]; has {
		t.Errorf("hook output carries a permissionDecision; want none: %q", out)
	}
	if answer.HookSpecificOutput["hookEventName"] != "PreToolUse" {
		t.Errorf("hookEventName = %v; want PreToolUse", answer.HookSpecificOutput["hookEventName"])
	}
	updated, _ := answer.HookSpecificOutput["updatedInput"].(map[string]any)
	command, ok := updated["command"].(string)
	if !ok {
		t.Fatalf("hook output has no updatedInput.command: %q", out)
	}
	return command
}

func TestBashStdinHook_PlainCommandGainsPrefix(t *testing.T) {
	got := rewrittenCommand(t, runRewriteHook(t, bashPayload(t, "git status")))
	if want := bashStdinPrefix + "git status"; got != want {
		t.Errorf("rewritten command = %q; want %q", got, want)
	}
}

func TestBashStdinHook_AwkwardCommandRoundTripsByteExact(t *testing.T) {
	command := "printf '%s\\n' \"a \\\"quoted\\\" b\"\ncat <<'EOF'\n\tback\\slash and 'single' \"double\"\nEOF\n"
	got := rewrittenCommand(t, runRewriteHook(t, bashPayload(t, command)))
	if want := bashStdinPrefix + command; got != want {
		t.Errorf("rewritten command = %q; want %q", got, want)
	}
}

func TestBashStdinHook_PrefixedCommandIsLeftAlone(t *testing.T) {
	first := rewrittenCommand(t, runRewriteHook(t, bashPayload(t, "echo hi")))
	if out := runRewriteHook(t, bashPayload(t, first)); out != "" {
		t.Errorf("hook output for an already prefixed command = %q; want none", out)
	}
}

func TestBashStdinHook_PayloadWithoutCommandProducesNothing(t *testing.T) {
	if out := runRewriteHook(t, `{"tool_name":"Bash","tool_input":{}}`); out != "" {
		t.Errorf("hook output for a payload with no command = %q; want none", out)
	}
}

// TestBashStdinHook_RewrittenCatEndsAtOnce holds the shell's stdin open and expects the prefixed `cat -` to end with empty output.
func TestBashStdinHook_RewrittenCatEndsAtOnce(t *testing.T) {
	command := rewrittenCommand(t, runRewriteHook(t, bashPayload(t, "cat -")))

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = r
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("rewritten cat - exited with %v; want 0", err)
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatal("rewritten cat - still running with the shell's stdin held open; want it to end at once")
	}
	if out.Len() != 0 {
		t.Errorf("rewritten cat - printed %q; want nothing", out.String())
	}
}

// TestBashStdinHook_RewrittenHeredocStillFeedsStdin pins that an attached heredoc still supplies the command's stdin.
func TestBashStdinHook_RewrittenHeredocStillFeedsStdin(t *testing.T) {
	command := rewrittenCommand(t, runRewriteHook(t, bashPayload(t, "cat - <<'EOF'\nheredoc body\nEOF")))
	out, err := exec.Command("sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("run rewritten heredoc command: %v", err)
	}
	if got := string(out); got != "heredoc body\n" {
		t.Errorf("output = %q; want the heredoc body", got)
	}
}

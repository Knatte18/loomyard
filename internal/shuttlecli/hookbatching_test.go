// hookbatching_test.go pins the hidden hook-batching verb.
// It pins the one line of hook output it prints for a breach and the silence of every other call;
// it also pins the exit code 0 it keeps on every failure.

package shuttlecli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeHookTranscript writes a transcript into dir and returns its path.
// The transcript holds one main-chain assistant message per tool name in calls;
// the i-th message carries a tool use with the id call-i.
func writeHookTranscript(t *testing.T, dir string, calls []string) string {
	t.Helper()
	var lines []string
	for i, name := range calls {
		assistant := map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"id":      fmt.Sprintf("msg-%d", i),
				"content": []map[string]any{{"type": "tool_use", "id": fmt.Sprintf("call-%d", i), "name": name, "input": map[string]any{}}},
				"usage":   map[string]any{"input_tokens": 1000},
			},
		}
		data, err := json.Marshal(assistant)
		if err != nil {
			t.Fatalf("marshal transcript line: %v", err)
		}
		lines = append(lines, string(data))
	}
	transcript := filepath.Join(dir, "transcript.jsonl")
	if err := os.WriteFile(transcript, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return transcript
}

// TestHookBatchingVerb_RunsOutsideAHub pins that the verb skips hub resolution outside a git repository.
// Run through the shuttle command tree, the verb exits 0 and prints its own report.
// It does not print the error envelope that hub resolution would give.
func TestHookBatchingVerb_RunsOutsideAHub(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	var out bytes.Buffer
	code := RunCLIIn(t.TempDir(), &out, []string{"hook-batching", "no-such-engine", "unused-state-file"})
	if code != 0 || !strings.HasPrefix(out.String(), "shuttle: the engine") {
		t.Errorf("RunCLIIn(hook-batching) = exit %d, output %q; want exit 0 and the verb's own report", code, out.String())
	}
}

// TestHookBatchingVerb pins the verb's output and exit code.
// A nudge prints the engine's hook output and a newline; any other outcome prints nothing.
// An unknown engine or a malformed input is reported on standard error, and the verb still returns no error.
func TestHookBatchingVerb(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name   string
		engine string
		calls  []string
		// input is the hook input on standard input; empty builds a PostToolUse input for the last call of calls.
		input string
		// state is the state file's content; empty seeds a valid one.
		state string
		// wantContext is the start of the nudge line the output carries; empty wants no output.
		wantContext string
		// wantStderr is a text the standard error names; empty wants it empty.
		wantStderr string
	}{
		{name: "BreachPrintsTheHookOutputAndANewline", engine: "claude", calls: []string{"Grep", "Glob", "Grep"}, wantContext: "[lym batching] Your last 3 messages"},
		{name: "NoBreachPrintsNothing", engine: "claude", calls: []string{"Bash", "Grep"}},
		{name: "UnknownEngineIsReportedAndExitsZero", engine: "no-such-engine", calls: []string{"Grep"}, input: "{}", wantStderr: `the engine "no-such-engine" has no batching nudge`},
		{name: "MalformedInputIsReportedAndExitsZero", engine: "claude", calls: []string{"Grep"}, input: "{not json", wantStderr: "parse the batching hook input"},
		{name: "CorruptStateIsReportedAndExitsZero", engine: "claude", calls: []string{"Grep"}, state: "{not json", wantStderr: "update the batching nudge state"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			transcript := writeHookTranscript(t, dir, tt.calls)
			statePath := filepath.Join(dir, "batching-nudge.json")
			seeded := `{"settings":{"enabled":true,"lookup_chain":true,"lookup_run":3,"edit_run":2,"slice_lines":20,"max_nudges":5},"nudges":0,"nudged_message":""}`
			if tt.state != "" {
				seeded = tt.state
			}
			if err := os.WriteFile(statePath, []byte(seeded), 0o644); err != nil {
				t.Fatalf("seed state: %v", err)
			}
			input := tt.input
			if input == "" {
				last := len(tt.calls) - 1
				data, err := json.Marshal(map[string]any{"transcript_path": transcript, "tool_name": tt.calls[last], "tool_use_id": fmt.Sprintf("call-%d", last), "tool_input": map[string]any{}})
				if err != nil {
					t.Fatalf("marshal hook input: %v", err)
				}
				input = string(data)
			}

			cmd := hookBatchingCmd()
			var stdout, stderr bytes.Buffer
			cmd.SetIn(strings.NewReader(input))
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{tt.engine, statePath})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error: %v; want nil on every path", err)
			}

			if tt.wantStderr == "" && stderr.Len() != 0 {
				t.Errorf("standard error = %q; want empty", stderr.String())
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("standard error = %q; want it to name %q", stderr.String(), tt.wantStderr)
			}
			if tt.wantContext == "" {
				if stdout.Len() != 0 {
					t.Errorf("standard output = %q; want empty", stdout.String())
				}
				return
			}
			line, ok := strings.CutSuffix(stdout.String(), "\n")
			if !ok || strings.Contains(line, "\n") {
				t.Fatalf("standard output = %q; want one line ending in a newline", stdout.String())
			}
			var output struct {
				HookSpecificOutput struct {
					HookEventName     string `json:"hookEventName"`
					AdditionalContext string `json:"additionalContext"`
				} `json:"hookSpecificOutput"`
			}
			if err := json.Unmarshal([]byte(line), &output); err != nil {
				t.Fatalf("standard output %q is not hook output: %v", line, err)
			}
			if output.HookSpecificOutput.HookEventName != "PostToolUse" || !strings.HasPrefix(output.HookSpecificOutput.AdditionalContext, tt.wantContext) {
				t.Errorf("hook output = %+v; want a PostToolUse context starting %q", output.HookSpecificOutput, tt.wantContext)
			}
		})
	}
}

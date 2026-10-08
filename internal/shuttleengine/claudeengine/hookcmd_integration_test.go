//go:build integration

// hookcmd_integration_test.go runs the python deny's hook command under `sh` against PreToolUse payloads,
// and pins which Bash commands it denies and which it lets through.

package claudeengine

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"
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

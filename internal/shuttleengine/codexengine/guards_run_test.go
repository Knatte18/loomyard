// guards_run_test.go pins the behavior of the hook commands a Codex launch renders.
// It runs them under sh against Codex PreToolUse payloads.

package codexengine

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestGuardCommand_DeniesAMatchingCommand runs rendered guard commands under sh against Codex PreToolUse payloads.
// A matching command prints the row's Codex deny, and any other prints the empty hook output.
// A row of several patterns denies only when all match, and a pattern holding a single quote greps as written.
func TestGuardCommand_DeniesAMatchingCommand(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	rows := map[string]shellguard.Row{}
	for _, row := range installedGuards(sampleRefuseRaw, guardInputs{denyPython: true, denyPublish: true}) {
		rows[row.Name] = row
	}
	tests := []struct {
		row     string
		command string
		denied  bool
	}{
		{"machine-sleep", "systemctl hibernate", true},
		{"machine-sleep", "systemctl status", false},
		{"machine-sleep", "grep 'x; systemctl hibernate' f", false},
		{"shell-edit", "sed -i s/a/b/ f.go", true},
		{"shell-edit", "sed -n 1p f.go", false},
		{"computed-rm", "rm $DIR/f", true},
		{"computed-rm", "rm .scratch/a", false},
		{"refuse-raw", "go test ./internal/x", true},
		{"publish", "git push origin main", true},
	}
	for _, tt := range tests {
		t.Run(tt.row+"_"+tt.command, func(t *testing.T) {
			t.Parallel()
			var payload bytes.Buffer
			encoder := json.NewEncoder(&payload)
			encoder.SetEscapeHTML(false)
			if err := encoder.Encode(map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]any{"command": tt.command}}); err != nil {
				t.Fatal(err)
			}
			row := rows[tt.row]
			want := "{}\n"
			if tt.denied {
				want = denyDecision(row.SteerFor(engineName)) + "\n"
			}
			if got := runHook(t, guardCommand(row), payload.String()); got != want {
				t.Errorf("%s hook on %q printed %q; want %q", tt.row, tt.command, got, want)
			}
		})
	}
}

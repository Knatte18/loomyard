// guards_run_test.go pins the behavior of the hook commands a Codex launch renders.
// It runs them under sh against Codex PreToolUse payloads.

package codexengine

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestQuestionDenyCommand_DeniesAQuestionTool runs the question-tool deny under sh against Codex PreToolUse payloads.
// A tool name ending in request_user_input or request_user_input_async prints the deny, a namespace prefix included;
// any other tool, and a shell command that only mentions the name, print the empty hook output.
//
//testtiming:keep runs the deny's grep pattern under sh, including the shell-command false-positive row, which no test of the rendered settings can do
func TestQuestionDenyCommand_DeniesAQuestionTool(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name     string
		toolName string
		command  string
		denied   bool
	}{
		{"QuestionTool", "request_user_input", "", true},
		{"AsyncQuestionToolWithANamespacePrefix", "functionsrequest_user_input_async", "", true},
		{"SpawnAgent", "collaborationspawn_agent", "", false},
		{"ShellCommandMentioningTheTool", "Bash", `echo "tool_name":"request_user_input"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var payload bytes.Buffer
			encoder := json.NewEncoder(&payload)
			encoder.SetEscapeHTML(false)
			if err := encoder.Encode(map[string]any{"hook_event_name": "PreToolUse", "tool_name": tt.toolName, "tool_input": map[string]any{"command": tt.command}}); err != nil {
				t.Fatal(err)
			}
			want := "{}\n"
			if tt.denied {
				want = denyDecision(steerQuestionDeny) + "\n"
			}
			if got := runHook(t, questionDenyCommand(), payload.String()); got != want {
				t.Errorf("question deny hook on tool %q printed %q; want %q", tt.toolName, got, want)
			}
		})
	}
}

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
			if got := runHook(t, guardCommand(row, testLymPath), payload.String()); got != want {
				t.Errorf("%s hook on %q printed %q; want %q", tt.row, tt.command, got, want)
			}
		})
	}
}

// TestGuardCommand_PythonRowAsksTheHubOnAMatch runs the python row's rendered hook under sh.
// Its input is a Codex PreToolUse payload.
// A stub `lym` at the path the row is rendered over logs its directory and arguments and exits as each row says.
// On a python command the hook runs that stub as `shuttle hook-python-allowed` from its own working directory.
// It prints the deny unless the stub exits 0, and it prints the deny when no lym stands at that path.
// A command that runs no python never runs lym and prints the empty hook output.
func TestGuardCommand_PythonRowAsksTheHubOnAMatch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name    string
		command string
		// lymExit is the stub's exit code; empty leaves no lym at the path the row is rendered over.
		lymExit string
		denied  bool
		// wantAsked reports whether the hook ran the stub.
		wantAsked bool
	}{
		{name: "AllowedByTheHub", command: "python3 -c 1", lymExit: "0", wantAsked: true},
		{name: "DeniedByTheHub", command: "python3 -c 1", lymExit: "1", denied: true, wantAsked: true},
		{name: "NoLymAtThePath", command: "python3 -c 1", denied: true},
		{name: "NoPythonCommand", command: "ls -la", lymExit: "1"},
	}
	var row shellguard.Row
	for _, installed := range installedGuards(nil, guardInputs{}) {
		if installed.Name == "python" {
			row = installed
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			binDir, workDir, logPath := filepath.Join(root, "bin"), filepath.Join(root, "work"), filepath.Join(root, "lym.log")
			mkdirAll(t, binDir)
			mkdirAll(t, workDir)
			lymPath := filepath.Join(binDir, "lym")
			if tt.lymExit != "" {
				writeFile(t, lymPath, "#!/bin/sh\necho \"$(pwd -P) $*\" >> "+shell.Posix().Quote(logPath)+"\nexit "+tt.lymExit+"\n")
				if err := os.Chmod(lymPath, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			var payload bytes.Buffer
			encoder := json.NewEncoder(&payload)
			encoder.SetEscapeHTML(false)
			if err := encoder.Encode(map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]any{"command": tt.command}}); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command("sh", "-c", guardCommand(row, lymPath))
			cmd.Dir = workDir
			cmd.Env = []string{"PATH=/usr/bin:/bin"}
			cmd.Stdin = strings.NewReader(payload.String())
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("run hook: %v", err)
			}
			want := "{}\n"
			if tt.denied {
				want = denyDecision(row.SteerFor(engineName)) + "\n"
			}
			if string(out) != want {
				t.Errorf("python hook on %q printed %q; want %q", tt.command, out, want)
			}
			resolvedWorkDir, err := filepath.EvalSymlinks(workDir)
			if err != nil {
				t.Fatal(err)
			}
			wantLog := ""
			if tt.wantAsked {
				wantLog = resolvedWorkDir + " shuttle " + shuttleengine.PythonAllowedHookVerb + "\n"
			}
			if logged, _ := os.ReadFile(logPath); string(logged) != wantLog {
				t.Errorf("lym runs = %q; want %q", logged, wantLog)
			}
		})
	}
}

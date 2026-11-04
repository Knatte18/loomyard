// command_test.go pins the launch and resume lines, the `-c` hook settings and the TOML quoting they carry.

package codexengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestBuildLaunchCmd pins the POSIX launch line: CODEX_HOME leading, every flag of the spike's line with the writable root, model and effort when set, each hook setting, the bypass flag, the first turn, and the shell exit.
// It also pins an opened session's command: no hook bypass, no first turn and no exit when the line has no hooks and no first turn.
func TestBuildLaunchCmd(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		line launchLine
		want string
	}{
		{
			name: "EveryInput",
			line: launchLine{bin: "codex", home: "/h", writableRoots: []string{"/links"}, model: "gpt-6-luna", effort: "low", hooks: []string{`hooks.Stop=[{hooks=[]}]`}, firstTurn: "Read it"},
			want: `CODEX_HOME='/h' 'codex' --no-alt-screen -a never -s workspace-write -c 'sandbox_workspace_write.writable_roots=["/links"]' -m 'gpt-6-luna' -c 'model_reasoning_effort="low"' -c 'project_doc_fallback_filenames=["CLAUDE.md"]' -c 'hooks.Stop=[{hooks=[]}]' --dangerously-bypass-hook-trust 'Read it'; exit`,
		},
		{
			name: "NoWritableRootModelOrEffort",
			line: launchLine{bin: "codex", home: "/h", hooks: []string{`hooks.Stop=[{hooks=[]}]`}, firstTurn: "it's"},
			want: `CODEX_HOME='/h' 'codex' --no-alt-screen -a never -s workspace-write -c 'project_doc_fallback_filenames=["CLAUDE.md"]' -c 'hooks.Stop=[{hooks=[]}]' --dangerously-bypass-hook-trust 'it'\''s'; exit`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := buildLaunchCmd(shell.Posix(), tt.line); got != tt.want {
				t.Errorf("buildLaunchCmd() = %s; want %s", got, tt.want)
			}
		})
	}
	if got := buildResumeCmd(shell.Posix()); got != "exit" {
		t.Errorf("buildResumeCmd() = %q; want exit", got)
	}
	opened := launchLine{bin: "codex", home: "/h", model: "gpt-6-luna"}
	if got, want := codexCommand(shell.Posix(), opened), `CODEX_HOME='/h' 'codex' --no-alt-screen -a never -s workspace-write -m 'gpt-6-luna' -c 'project_doc_fallback_filenames=["CLAUDE.md"]'`; got != want {
		t.Errorf("codexCommand(%+v) = %s; want %s", opened, got, want)
	}
}

// TestHookSettings pins one setting per recorded hook in order, then one PreToolUse setting holding every guard entry when guards are given, then the activity hook on every tool, each running the command for the run's events file.
func TestHookSettings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	events := "/run/events.jsonl"
	recorded := []string{
		hookSetting(hookEventSessionStart, hookEntry("", recordingCommand(hookEventSessionStart, events))),
		hookSetting(hookEventUserPromptSubmit, hookEntry("", recordingCommand(hookEventUserPromptSubmit, events))),
		hookSetting(hookEventStop, hookEntry("", recordingCommand(hookEventStop, events))),
		hookSetting(hookEventPermissionRequest, hookEntry("", recordingCommand(hookEventPermissionRequest, events))),
	}
	activity := hookSetting(hookEventPostToolUse, hookEntry("*", activityCommand(events)))
	guards := installedGuards(nil, guardInputs{})
	tests := []struct {
		name   string
		guards []shellguard.Row
		want   []string
	}{
		{"NoGuards", nil, append(append([]string{}, recorded...), activity)},
		{"Guards", guards, append(append([]string{}, recorded...), hookSetting(hookEventPreToolUse, guardEntries(guards)...), activity)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hookSettings(events, tt.guards)
			if len(got) != len(tt.want) {
				t.Fatalf("hookSettings() = %d settings; want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("hookSettings()[%d] = %s; want %s", i, got[i], tt.want[i])
				}
			}
		})
	}
	if got, want := hookSetting("PostToolUse", hookEntry("*", `touch '/a'; printf '{}\n'`)), `hooks.PostToolUse=[{matcher="*",hooks=[{type="command",command="touch '/a'; printf '{}\\n'"}]}]`; got != want {
		t.Errorf("hookSetting() = %s; want %s", got, want)
	}
	if got, want := hookSetting("PreToolUse", hookEntry("Bash", "a"), hookEntry("Bash", "b")), `hooks.PreToolUse=[{matcher="Bash",hooks=[{type="command",command="a"}]},{matcher="Bash",hooks=[{type="command",command="b"}]}]`; got != want {
		t.Errorf("hookSetting() with two entries = %s; want %s", got, want)
	}
	if got, want := activityCommand(events), `touch '/run/activity-stamp'; printf '{}\n'`; got != want {
		t.Errorf("activityCommand() = %s; want %s", got, want)
	}
}

// TestTOMLString pins the basic-string escapes: quote, backslash, newline, tab and other control characters.
func TestTOMLString(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{`plain`, `"plain"`},
		{`a "b" c\d`, `"a \"b\" c\\d"`},
		{"line\nnext\ttab\x01", `"line\nnext\ttab\u0001"`},
	}
	for _, tt := range tests {
		if got := tomlString(tt.in); got != tt.want {
			t.Errorf("tomlString(%q) = %s; want %s", tt.in, got, tt.want)
		}
	}
	if got, want := tomlArray([]string{"/a", "/b"}), `["/a","/b"]`; got != want {
		t.Errorf("tomlArray() = %s; want %s", got, want)
	}
}

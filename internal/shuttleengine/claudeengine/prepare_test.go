// prepare_test.go covers Prepare's spec handling: an unrealizable effort, permission mode, resume
// session id or model-plus-version combination is rejected before any artifact is written (mirroring
// TestPrepare_PromptLaunchLimit's before-artifacts guarantee), the flags a valid spec threads into
// Launch.Cmd and Launch.ResumeCmd, the --append-system-prompt notice, ResumeSessionID adoption, and
// the skills-deferred prompt pointer.

package claudeengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestPrepare_RejectedBeforeArtifacts proves every unrealizable spec value fails Prepare before
// prompt.md/settings.json are written — the same before-artifacts guarantee
// TestPrepare_PromptLaunchLimit pins for the prompt-size guard, since a half-prepared run dir would
// look resumable to a later diagnosis pass.
// A dashed model with a Version is a contradiction: the id already pins its own version.
func TestPrepare_RejectedBeforeArtifacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		spec            shuttleengine.Spec
		wantErrContains string
	}{
		{"bad_effort", shuttleengine.Spec{Effort: "bogus"}, "bogus"},
		{"dashed_model_with_version", shuttleengine.Spec{Model: "claude-sonnet-4-5", Version: "4.5"}, ""},
		{"permission_prompt_on_autonomous", shuttleengine.Spec{PermissionMode: "prompt"}, ""},
		{"permission_unknown_value", shuttleengine.Spec{PermissionMode: "yolo", Interactive: true}, ""},
		{"session_id_too_short", shuttleengine.Spec{ResumeSessionID: "0a1b2c3d-4e5f-4a6b-8c7d"}, ""},
		{"session_id_uppercase", shuttleengine.Spec{ResumeSessionID: "0A1B2C3D-4E5F-4A6B-8C7D-9E0F1A2B3C4D"}, ""},
		{"session_id_quote", shuttleengine.Spec{ResumeSessionID: "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4'"}, ""},
		{"session_id_no_hyphens", shuttleengine.Spec{ResumeSessionID: "0a1b2c3d4e5f4a6b8c7d9e0f1a2b3c4d"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runDir := t.TempDir()
			tt.spec.Prompt = "do the thing"
			_, err := New().Prepare(runDir, tt.spec, shuttleengine.Config{})
			if err == nil {
				t.Fatalf("Prepare(%+v) = nil error; want a validation rejection", tt.spec)
			}
			if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("Prepare() error = %q; want it to contain %q", err, tt.wantErrContains)
			}
			for _, name := range []string{"prompt.md", "settings.json"} {
				if _, statErr := os.Stat(filepath.Join(runDir, name)); !os.IsNotExist(statErr) {
					t.Errorf("%s exists after a rejected Prepare (stat err=%v); want no artifacts written", name, statErr)
				}
			}
		})
	}
}

// flagExpectation names the substrings a command line must and must not contain.
type flagExpectation struct {
	present []string
	absent  []string
}

// TestPrepare_ThreadsSpecIntoLaunchCmds proves Prepare threads the spec into the commands it returns.
// A valid effort is rendered as buildLaunchCmd would; an empty effort emits no --effort flag at all,
// deferring entirely to claude's own default;
// a bare-word model plus a dotted version composes the pinned id ("sonnet" + "4.5" ->
// "claude-sonnet-4-5") rather than the bare-word model;
// ForkSubagents wraps the line in the CLAUDE_CODE_FORK_SUBAGENT env prefix, and AllowAgentTool does not remove it;
// an empty ResumeSessionID mints a session as before;
// and the resolved permission mode decides --dangerously-skip-permissions on the launch line and the
// resume line alike.
func TestPrepare_ThreadsSpecIntoLaunchCmds(t *testing.T) {
	t.Parallel()

	const skipPermissions = "--dangerously-skip-permissions"
	tests := []struct {
		name        string
		spec        shuttleengine.Spec
		cfg         shuttleengine.Config
		onLaunch    flagExpectation
		onBothLines flagExpectation
	}{
		{name: "valid_effort", spec: shuttleengine.Spec{Effort: "high"}, onLaunch: flagExpectation{present: []string{"--effort 'high'"}}},
		{name: "empty_effort", spec: shuttleengine.Spec{}, onLaunch: flagExpectation{absent: []string{"--effort"}}},
		{name: "model_and_version", spec: shuttleengine.Spec{Model: "sonnet", Version: "4.5"}, onLaunch: flagExpectation{present: []string{"--model 'claude-sonnet-4-5'"}}},
		{name: "fork_subagents_on", spec: shuttleengine.Spec{ForkSubagents: true}, onLaunch: flagExpectation{present: []string{"CLAUDE_CODE_FORK_SUBAGENT"}}},
		{name: "fork_subagents_off", spec: shuttleengine.Spec{ForkSubagents: false}, onLaunch: flagExpectation{absent: []string{"CLAUDE_CODE_FORK_SUBAGENT"}}},
		{
			name:     "fork_env_wrapping_stays_under_allow_agent_tool",
			spec:     shuttleengine.Spec{ForkSubagents: true, AllowAgentTool: true},
			cfg:      shuttleengine.Config{ClaudeDenyAgentTool: true, ClaudeDenyAskUserQuestion: true},
			onLaunch: flagExpectation{present: []string{"CLAUDE_CODE_FORK_SUBAGENT"}},
		},
		{name: "empty_resume_session_id_mints_session", spec: shuttleengine.Spec{}, onLaunch: flagExpectation{present: []string{"--session-id"}, absent: []string{"--resume"}}},
		{name: "permission_interactive_bypass", spec: shuttleengine.Spec{PermissionMode: "bypass", Interactive: true}, onBothLines: flagExpectation{present: []string{skipPermissions}}},
		{name: "permission_interactive_empty", spec: shuttleengine.Spec{PermissionMode: "", Interactive: true}, onBothLines: flagExpectation{absent: []string{skipPermissions}}},
		{name: "permission_autonomous_empty", spec: shuttleengine.Spec{PermissionMode: "", Interactive: false}, onBothLines: flagExpectation{present: []string{skipPermissions}}},
		{name: "permission_autonomous_bypass", spec: shuttleengine.Spec{PermissionMode: "bypass", Interactive: false}, onBothLines: flagExpectation{present: []string{skipPermissions}}},
		{name: "permission_interactive_prompt", spec: shuttleengine.Spec{PermissionMode: "prompt", Interactive: true}, onBothLines: flagExpectation{absent: []string{skipPermissions}}},
	}
	check := func(t *testing.T, label, line string, want flagExpectation) {
		t.Helper()
		for _, s := range want.present {
			if !strings.Contains(line, s) {
				t.Errorf("%s = %q; want it to contain %q", label, line, s)
			}
		}
		for _, s := range want.absent {
			if strings.Contains(line, s) {
				t.Errorf("%s = %q; want it NOT to contain %q", label, line, s)
			}
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.spec.Prompt = "do the thing"
			launch, err := New().Prepare(t.TempDir(), tt.spec, tt.cfg)
			if err != nil {
				t.Fatalf("Prepare() error: %v; want nil", err)
			}
			check(t, "Launch.Cmd", launch.Cmd, tt.onLaunch)
			check(t, "Launch.Cmd", launch.Cmd, tt.onBothLines)
			check(t, "Launch.ResumeCmd", launch.ResumeCmd, tt.onBothLines)
		})
	}
}

// TestPrepare_AppendSystemPromptMatchesDenyNotice proves that for every combination of the deny inputs, Launch.Cmd and Launch.ResumeCmd both carry --append-system-prompt if and only if buildDenyNotice is non-empty, and carry its sentences.
//
//testtiming:keep pins --append-system-prompt equal to the notice on both lines over every deny combination, which its covering tests do not assert
func TestPrepare_AppendSystemPromptMatchesDenyNotice(t *testing.T) {
	for _, denyAgent := range []bool{false, true} {
		for _, denyAsk := range []bool{false, true} {
			for _, interactive := range []bool{false, true} {
				for _, fork := range []bool{false, true} {
					cfg := shuttleengine.Config{ClaudeDenyAgentTool: denyAgent, ClaudeDenyAskUserQuestion: denyAsk}
					spec := shuttleengine.Spec{Prompt: "do the thing", Interactive: interactive, ForkSubagents: fork}
					notice := buildDenyNotice(interactive, cfg, fork, false)

					launch, err := New().Prepare(t.TempDir(), spec, cfg)
					if err != nil {
						t.Fatalf("Prepare(agent=%v ask=%v interactive=%v fork=%v) error: %v", denyAgent, denyAsk, interactive, fork, err)
					}
					for name, cmd := range map[string]string{"Cmd": launch.Cmd, "ResumeCmd": launch.ResumeCmd} {
						has := strings.Contains(cmd, "--append-system-prompt")
						if has != (notice != "") {
							t.Errorf("agent=%v ask=%v interactive=%v fork=%v: %s = %q; --append-system-prompt present=%v, notice non-empty=%v", denyAgent, denyAsk, interactive, fork, name, cmd, has, notice != "")
						}
						if notice != "" && !strings.Contains(cmd, notice) {
							t.Errorf("agent=%v ask=%v interactive=%v fork=%v: %s = %q; want it to carry the notice %q", denyAgent, denyAsk, interactive, fork, name, cmd, notice)
						}
					}
				}
			}
		}
	}
}

// TestPrepare_ResumeSessionID proves a spec carrying ResumeSessionID launches that session with --resume
// and names the same id on the resume line and in Launch.SessionID.
//
//testtiming:keep pins that a ResumeSessionID launches with --resume, no --session-id, and the same id on the resume line and in Launch.SessionID, which its covering tests do not assert
func TestPrepare_ResumeSessionID(t *testing.T) {
	const id = "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"
	spec := shuttleengine.Spec{
		Prompt:          "adopt",
		ResumeSessionID: id,
		PermissionMode:  "bypass",
		Interactive:     true,
		Model:           "opus",
		Effort:          "high",
	}
	runDir := t.TempDir()
	launch, err := New().Prepare(runDir, spec, shuttleengine.Config{})
	if err != nil {
		t.Fatalf("Prepare() error: %v; want nil", err)
	}
	if launch.SessionID != id {
		t.Errorf("Launch.SessionID = %q; want %q", launch.SessionID, id)
	}
	if strings.Contains(launch.Cmd, "--session-id") {
		t.Errorf("Launch.Cmd = %q; want no --session-id", launch.Cmd)
	}
	for _, want := range []string{"--resume", id, "prompt.md", filepath.Join(runDir, "settings.json"), "opus", "--effort", "--dangerously-skip-permissions"} {
		if !strings.Contains(launch.Cmd, want) {
			t.Errorf("Launch.Cmd = %q; want it to contain %q", launch.Cmd, want)
		}
	}
	if !strings.Contains(launch.ResumeCmd, "--resume") || !strings.Contains(launch.ResumeCmd, id) {
		t.Errorf("Launch.ResumeCmd = %q; want --resume naming %q", launch.ResumeCmd, id)
	}
}

// TestPrepare_SkillsDeferPromptPointer proves a spec naming skills leaves the prompt pointer off the launch line and returns it as PromptLine,
// and a spec without skills keeps the pointer on the line with an empty PromptLine, for a fresh and a resumed launch.
func TestPrepare_SkillsDeferPromptPointer(t *testing.T) {
	const id = "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"
	for _, resume := range []bool{false, true} {
		for _, skills := range [][]string{nil, {"scribe:prose"}} {
			spec := shuttleengine.Spec{Prompt: "do the thing", Skills: skills}
			if resume {
				spec.ResumeSessionID = id
			}
			runDir := t.TempDir()
			launch, err := New().Prepare(runDir, spec, shuttleengine.Config{})
			if err != nil {
				t.Fatalf("Prepare(resume=%v, skills=%v) error: %v", resume, skills, err)
			}
			pointer := launchPointer(filepath.Join(runDir, "prompt.md"))
			if len(skills) > 0 {
				if strings.Contains(launch.Cmd, "Read ") {
					t.Errorf("resume=%v: Launch.Cmd = %q; want no prompt pointer", resume, launch.Cmd)
				}
				if launch.PromptLine != pointer {
					t.Errorf("resume=%v: PromptLine = %q; want %q", resume, launch.PromptLine, pointer)
				}
			} else {
				if !strings.Contains(launch.Cmd, pointer) {
					t.Errorf("resume=%v: Launch.Cmd = %q; want the prompt pointer", resume, launch.Cmd)
				}
				if launch.PromptLine != "" {
					t.Errorf("resume=%v: PromptLine = %q; want empty", resume, launch.PromptLine)
				}
			}
			if _, err := os.Stat(filepath.Join(runDir, "prompt.md")); err != nil {
				t.Errorf("resume=%v, skills=%v: prompt.md not written: %v", resume, skills, err)
			}
		}
	}
}

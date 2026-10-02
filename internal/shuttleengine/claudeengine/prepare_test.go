// prepare_test.go covers Prepare's effort and model/version handling: an unrealizable effort is
// rejected before any artifact is written (mirroring TestPrepare_PromptLaunchLimit's
// before-artifacts guarantee), a valid effort ends up in the returned Launch.Cmd, an empty effort
// emits no --effort flag at all, a bare-word model plus version composes into the pinned model id
// in Launch.Cmd, a dashed model plus version is rejected before any artifact is written, and
// Spec.ForkSubagents threads through to Launch.Cmd's CLAUDE_CODE_FORK_SUBAGENT env prefix.

package claudeengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestPrepare_BadEffortRejectedBeforeArtifacts proves an unrealizable effort value fails Prepare
// before prompt.md/settings.json are written — the same before-artifacts guarantee
// TestPrepare_PromptLaunchLimit pins for the prompt-size guard, since a half-prepared run dir would
// look resumable to a later diagnosis pass.
func TestPrepare_BadEffortRejectedBeforeArtifacts(t *testing.T) {
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing", Effort: "bogus"}
	cfg := shuttleengine.Config{}

	c := New()
	_, err := c.Prepare(runDir, spec, cfg)
	if err == nil {
		t.Fatal("Prepare() with an unrealizable effort = nil error; want the validateEffort rejection")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("Prepare() error = %q; want it to name the invalid effort value", err)
	}

	if _, statErr := os.Stat(filepath.Join(runDir, "prompt.md")); !os.IsNotExist(statErr) {
		t.Errorf("prompt.md exists after a rejected Prepare (stat err=%v); want no artifacts written", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, "settings.json")); !os.IsNotExist(statErr) {
		t.Errorf("settings.json exists after a rejected Prepare (stat err=%v); want no artifacts written", statErr)
	}
}

// TestPrepare_ValidEffortLandsInLaunchCmd proves a valid effort survives Prepare's validation and
// is threaded into buildLaunchCmd, appearing in the returned Launch.Cmd exactly as buildLaunchCmd
// would render it.
func TestPrepare_ValidEffortLandsInLaunchCmd(t *testing.T) {
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing", Effort: "high"}
	cfg := shuttleengine.Config{}

	c := New()
	launch, err := c.Prepare(runDir, spec, cfg)
	if err != nil {
		t.Fatalf("Prepare() with a valid effort error: %v; want nil", err)
	}
	if !strings.Contains(launch.Cmd, "--effort 'high'") {
		t.Errorf("Launch.Cmd = %q; want it to contain --effort 'high'", launch.Cmd)
	}
}

// TestPrepare_EmptyEffortEmitsNoFlag proves the zero-value Effort (the common case — no operator
// override) succeeds and emits no --effort flag at all, deferring entirely to claude's own default.
func TestPrepare_EmptyEffortEmitsNoFlag(t *testing.T) {
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing"}
	cfg := shuttleengine.Config{}

	c := New()
	launch, err := c.Prepare(runDir, spec, cfg)
	if err != nil {
		t.Fatalf("Prepare() with an empty effort error: %v; want nil", err)
	}
	if strings.Contains(launch.Cmd, "--effort") {
		t.Errorf("Launch.Cmd = %q; want no --effort flag for an empty Spec.Effort", launch.Cmd)
	}
}

// TestPrepare_ModelAndVersionComposePinnedID proves Prepare threads spec.Model and spec.Version
// through resolveModelID, so a Spec naming a bare-word model plus a dotted version produces a
// launch Cmd containing the pinned model id ("sonnet" + "4.5" -> "claude-sonnet-4-5"), not the
// bare-word model.
func TestPrepare_ModelAndVersionComposePinnedID(t *testing.T) {
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing", Model: "sonnet", Version: "4.5"}
	cfg := shuttleengine.Config{}

	c := New()
	launch, err := c.Prepare(runDir, spec, cfg)
	if err != nil {
		t.Fatalf("Prepare() with model+version error: %v; want nil", err)
	}
	if !strings.Contains(launch.Cmd, "--model 'claude-sonnet-4-5'") {
		t.Errorf("Launch.Cmd = %q; want it to contain --model 'claude-sonnet-4-5'", launch.Cmd)
	}
}

// TestPrepare_DashedModelWithVersionRejectedBeforeArtifacts proves a full model id (already
// containing a dash) combined with a non-empty Version fails Prepare — the id already pins its own
// version, so a second pin is a contradiction — and that the rejection happens before any run
// artifact is written, mirroring TestPrepare_BadEffortRejectedBeforeArtifacts's before-artifacts
// guarantee.
func TestPrepare_DashedModelWithVersionRejectedBeforeArtifacts(t *testing.T) {
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "do the thing", Model: "claude-sonnet-4-5", Version: "4.5"}
	cfg := shuttleengine.Config{}

	c := New()
	_, err := c.Prepare(runDir, spec, cfg)
	if err == nil {
		t.Fatal("Prepare() with a dashed model + version = nil error; want the resolveModelID rejection")
	}

	if _, statErr := os.Stat(filepath.Join(runDir, "prompt.md")); !os.IsNotExist(statErr) {
		t.Errorf("prompt.md exists after a rejected Prepare (stat err=%v); want no artifacts written", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, "settings.json")); !os.IsNotExist(statErr) {
		t.Errorf("settings.json exists after a rejected Prepare (stat err=%v); want no artifacts written", statErr)
	}
}

// TestPrepare_ForkSubagentsThreadsIntoLaunchCmd proves Prepare threads spec.ForkSubagents through
// to buildLaunchCmd: a true value produces a Launch.Cmd containing the CLAUDE_CODE_FORK_SUBAGENT
// env prefix,
// and a false value (the zero value) produces a Launch.Cmd with no such prefix.
func TestPrepare_ForkSubagentsThreadsIntoLaunchCmd(t *testing.T) {
	tests := []struct {
		name          string
		forkSubagents bool
		wantContains  bool
	}{
		{"fork_mode_on", true, true},
		{"fork_mode_off", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			spec := shuttleengine.Spec{Prompt: "do the thing", ForkSubagents: tt.forkSubagents}
			cfg := shuttleengine.Config{}

			c := New()
			launch, err := c.Prepare(runDir, spec, cfg)
			if err != nil {
				t.Fatalf("Prepare() error: %v; want nil", err)
			}
			gotContains := strings.Contains(launch.Cmd, "CLAUDE_CODE_FORK_SUBAGENT")
			if gotContains != tt.wantContains {
				t.Errorf("Launch.Cmd = %q; contains CLAUDE_CODE_FORK_SUBAGENT = %v, want %v", launch.Cmd, gotContains, tt.wantContains)
			}
		})
	}
}

// TestPrepare_AppendSystemPromptMatchesDenyNotice proves that for every combination of the deny inputs, Launch.Cmd and Launch.ResumeCmd both carry --append-system-prompt if and only if buildDenyNotice is non-empty, and carry its sentences.
func TestPrepare_AppendSystemPromptMatchesDenyNotice(t *testing.T) {
	for _, denyAgent := range []bool{false, true} {
		for _, denyAsk := range []bool{false, true} {
			for _, interactive := range []bool{false, true} {
				for _, fork := range []bool{false, true} {
					cfg := shuttleengine.Config{ClaudeDenyAgentTool: denyAgent, ClaudeDenyAskUserQuestion: denyAsk}
					spec := shuttleengine.Spec{Prompt: "do the thing", Interactive: interactive, ForkSubagents: fork}
					notice := buildDenyNotice(interactive, cfg, fork)

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

// TestPrepare_PermissionModeThreadsIntoBothLines proves the resolved permission mode decides --dangerously-skip-permissions on the launch line and the resume line alike.
func TestPrepare_PermissionModeThreadsIntoBothLines(t *testing.T) {
	const flag = "--dangerously-skip-permissions"
	tests := []struct {
		name        string
		mode        string
		interactive bool
		wantFlag    bool
	}{
		{"interactive_bypass", "bypass", true, true},
		{"interactive_empty", "", true, false},
		{"autonomous_empty", "", false, true},
		{"autonomous_bypass", "bypass", false, true},
		{"interactive_prompt", "prompt", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := shuttleengine.Spec{Prompt: "do the thing", PermissionMode: tt.mode, Interactive: tt.interactive}
			launch, err := New().Prepare(t.TempDir(), spec, shuttleengine.Config{})
			if err != nil {
				t.Fatalf("Prepare() error: %v; want nil", err)
			}
			if got := strings.Contains(launch.Cmd, flag); got != tt.wantFlag {
				t.Errorf("Launch.Cmd = %q; contains %s = %v, want %v", launch.Cmd, flag, got, tt.wantFlag)
			}
			if got := strings.Contains(launch.ResumeCmd, flag); got != tt.wantFlag {
				t.Errorf("Launch.ResumeCmd = %q; contains %s = %v, want %v", launch.ResumeCmd, flag, got, tt.wantFlag)
			}
		})
	}
}

// TestPrepare_BadPermissionModeRejectedBeforeArtifacts proves an unrealizable permission mode fails Prepare before prompt.md/settings.json are written.
func TestPrepare_BadPermissionModeRejectedBeforeArtifacts(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		interactive bool
	}{
		{"prompt_on_autonomous", "prompt", false},
		{"unknown_value", "yolo", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			spec := shuttleengine.Spec{Prompt: "do the thing", PermissionMode: tt.mode, Interactive: tt.interactive}
			if _, err := New().Prepare(runDir, spec, shuttleengine.Config{}); err == nil {
				t.Fatal("Prepare() = nil error; want the validatePermissionMode rejection")
			}
			for _, name := range []string{"prompt.md", "settings.json"} {
				if _, statErr := os.Stat(filepath.Join(runDir, name)); !os.IsNotExist(statErr) {
					t.Errorf("%s exists after a rejected Prepare (stat err=%v); want no artifacts written", name, statErr)
				}
			}
		})
	}
}

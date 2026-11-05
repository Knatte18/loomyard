// prepare_test.go covers Prepare's spec handling: an unrealizable effort, permission mode, resume session id or model-plus-version combination is rejected before any artifact is written (mirroring TestPrepare_PromptLaunchLimit's before-artifacts guarantee), the flags a valid spec threads into Launch.Cmd and Launch.ResumeCmd, the append-system-prompt file carrying the deny notice and the creel trust sentence, ResumeSessionID adoption, and the skills-deferred prompt pointer.

package claudeengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// ttlConfig returns a Config whose prompt-cache TTL and keep-warm cap fields come from the shipped claude.yaml template, so fixtures follow the template rather than restating it.
// A non-empty defaultTTL or a non-nil roles replaces the template's value for that TTL field.
func ttlConfig(t testing.TB, defaultTTL string, roles map[string]string) Config {
	t.Helper()
	loaded, err := LoadConfig(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("load the shipped claude template: %v", err)
	}
	claude := Config{
		PromptCacheTTL:      loaded.PromptCacheTTL,
		PromptCacheTTLRoles: loaded.PromptCacheTTLRoles,
		KeepWarmCap:         loaded.KeepWarmCap,
		KeepWarmCapRoles:    loaded.KeepWarmCapRoles,
	}
	if defaultTTL != "" {
		claude.PromptCacheTTL = defaultTTL
	}
	if roles != nil {
		claude.PromptCacheTTLRoles = roles
	}
	return claude
}

// keepWarmConfig returns the template's Config with the default keep-warm cap replaced by defaultCap and, when non-nil, the cap roles by capRoles.
func keepWarmConfig(t testing.TB, defaultCap string, capRoles map[string]string) Config {
	t.Helper()
	claude := templateConfig(t)
	claude.KeepWarmCap = defaultCap
	if capRoles != nil {
		claude.KeepWarmCapRoles = capRoles
	}
	return claude
}

// templateConfig is the template's TTL and keep-warm cap fields alone.
func templateConfig(t testing.TB) Config {
	t.Helper()
	return ttlConfig(t, "", nil)
}

// engineWithAnchor returns an engine that loads its claude.yaml from the temp anchor it returns alongside.
func engineWithAnchor(t testing.TB) (*Claude, string) {
	t.Helper()
	shuttleCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: t.TempDir()}, "shuttle")
	if err != nil {
		t.Fatalf("load the shipped shuttle template: %v", err)
	}
	anchor := t.TempDir()
	return NewFromConfig(shuttleCfg, configengine.Dirs{Anchor: anchor}), anchor
}

// engineWith returns an engine whose claude.yaml is claude, written as the worktree layer of a temp anchor.
func engineWith(t testing.TB, claude Config) *Claude {
	t.Helper()
	engine, anchor := engineWithAnchor(t)
	document, err := yaml.Marshal(claude)
	if err != nil {
		t.Fatalf("marshal claude config: %v", err)
	}
	writeClaudeLayer(t, configengine.ConfigFile(anchor, "claude"), string(document))
	return engine
}

// engineWithInvalidConfig returns an engine over a temp anchor whose claude.local.yaml sets binary, a key the local layer refuses.
func engineWithInvalidConfig(t testing.TB) *Claude {
	t.Helper()
	engine, anchor := engineWithAnchor(t)
	writeClaudeLayer(t, configengine.LocalFile(anchor, "claude"), "binary: /x\n")
	return engine
}

// invalidConfigWant names what a refusal over engineWithInvalidConfig holds: the file, the key and the way forward.
var invalidConfigWant = []string{"claude.local.yaml", "binary", "lym config claude", "--layer"}

// promptCacheTTLAssignment returns the substring the pane shell's dialect renders for the TTL assignment of value.
func promptCacheTTLAssignment(value string) string {
	const marker = "|"
	return strings.TrimSuffix(shell.ForGOOS().WithEnv(promptCacheTTLEnvKey, value, marker), marker)
}

// TestPrepare_RejectedBeforeArtifacts proves every unrealizable spec value fails Prepare before prompt.md/settings.json are written — the same before-artifacts guarantee TestPrepare_PromptLaunchLimit pins for the prompt-size guard, since a half-prepared run dir would look resumable to a later diagnosis pass.
// A dashed model with a Version is a contradiction: the id already pins its own version.
func TestPrepare_RejectedBeforeArtifacts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		spec shuttleengine.Spec
		// claude is the claude.yaml the row's engine loads; invalidConfig replaces it with a file the loader refuses.
		claude          Config
		invalidConfig   bool
		wantErrContains string
		// wantTTLAdvice asserts the error names the accepted TTL values and the file to fix.
		wantTTLAdvice bool
	}{
		{name: "bad_effort", spec: shuttleengine.Spec{ModelChoice: shuttleengine.ModelChoice{Effort: "bogus"}}, claude: templateConfig(t), wantErrContains: "bogus"},
		{name: "dashed_model_with_version", spec: shuttleengine.Spec{ModelChoice: shuttleengine.ModelChoice{Model: "claude-sonnet-4-5", Version: "4.5"}}, claude: templateConfig(t), wantErrContains: ""},
		{name: "permission_prompt_on_autonomous", spec: shuttleengine.Spec{PermissionMode: "prompt"}, claude: templateConfig(t), wantErrContains: ""},
		{name: "permission_unknown_value", spec: shuttleengine.Spec{PermissionMode: "yolo", Interactive: true}, claude: templateConfig(t), wantErrContains: ""},
		{name: "session_id_too_short", spec: shuttleengine.Spec{ResumeSessionID: "0a1b2c3d-4e5f-4a6b-8c7d"}, claude: templateConfig(t), wantErrContains: ""},
		{name: "session_id_uppercase", spec: shuttleengine.Spec{ResumeSessionID: "0A1B2C3D-4E5F-4A6B-8C7D-9E0F1A2B3C4D"}, claude: templateConfig(t), wantErrContains: ""},
		{name: "session_id_quote", spec: shuttleengine.Spec{ResumeSessionID: "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4'"}, claude: templateConfig(t), wantErrContains: ""},
		{name: "session_id_no_hyphens", spec: shuttleengine.Spec{ResumeSessionID: "0a1b2c3d4e5f4a6b8c7d9e0f1a2b3c4d"}, claude: templateConfig(t), wantErrContains: ""},
		{name: "ttl_default_empty", claude: Config{}, wantErrContains: "prompt_cache_ttl", wantTTLAdvice: true},
		{name: "ttl_default_wrong_case", claude: ttlConfig(t, "1H", nil), wantErrContains: `prompt_cache_ttl "1H"`, wantTTLAdvice: true},
		{name: "ttl_default_bare_number", claude: ttlConfig(t, "5", nil), wantErrContains: `prompt_cache_ttl "5"`, wantTTLAdvice: true},
		{name: "ttl_map_value_null", claude: ttlConfig(t, "", map[string]string{"orch": ""}), wantErrContains: "prompt_cache_ttl_roles[orch]", wantTTLAdvice: true},
		{name: "ttl_map_entry_of_another_role", spec: shuttleengine.Spec{Role: "driver"}, claude: ttlConfig(t, "", map[string]string{"driver": "1h", "orch": "2h"}), wantErrContains: "prompt_cache_ttl_roles[orch]", wantTTLAdvice: true},
		{name: "keep_warm_cap_below_floor", claude: keepWarmConfig(t, "30s", nil), wantErrContains: "keep_warm_cap"},
		{name: "invalid_claude_config", invalidConfig: true, wantErrContains: "claude.local.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runDir := t.TempDir()
			tt.spec.Prompt = "do the thing"
			var engine *Claude
			if tt.invalidConfig {
				engine = engineWithInvalidConfig(t)
			} else {
				engine = engineWith(t, tt.claude)
			}
			_, err := engine.Prepare(runDir, tt.spec, shuttleengine.Config{})
			if err == nil {
				t.Fatalf("Prepare(%+v) = nil error; want a validation rejection", tt.spec)
			}
			if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("Prepare() error = %q; want it to contain %q", err, tt.wantErrContains)
			}
			if tt.invalidConfig {
				for _, want := range invalidConfigWant {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Prepare() error = %q; want it to name %q", err, want)
					}
				}
			}
			if tt.wantTTLAdvice {
				for _, want := range []string{"5m", "1h", "claude.yaml", "lym config claude"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Prepare() error = %q; want it to name %q", err, want)
					}
				}
			}
			for _, name := range []string{"prompt.md", "settings.json"} {
				if _, statErr := os.Stat(filepath.Join(runDir, name)); !os.IsNotExist(statErr) {
					t.Errorf("%s exists after a rejected Prepare (stat err=%v); want no artifacts written", name, statErr)
				}
			}
		})
	}
}

// TestPromptCacheLifetime proves PromptCacheLifetime parses the role's resolved TTL to a duration and passes a resolution error through with its claude.yaml advice.
func TestPromptCacheLifetime(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		spec shuttleengine.Spec
		// claude is the claude.yaml the row's engine loads; invalidConfig replaces it with a file the loader refuses.
		claude          Config
		invalidConfig   bool
		want            time.Duration
		wantErrContains []string
	}{
		{name: "default_is_five_minutes", claude: templateConfig(t), want: 5 * time.Minute},
		{name: "role_mapped_to_one_hour", spec: shuttleengine.Spec{Role: "orch"}, claude: ttlConfig(t, "", map[string]string{"orch": "1h"}), want: time.Hour},
		{name: "invalid_default_is_an_error", claude: ttlConfig(t, "1H", nil), wantErrContains: []string{"prompt_cache_ttl", "claude.yaml"}},
		{name: "invalid_claude_config", invalidConfig: true, wantErrContains: invalidConfigWant},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var engine *Claude
			if tt.invalidConfig {
				engine = engineWithInvalidConfig(t)
			} else {
				engine = engineWith(t, tt.claude)
			}
			got, err := engine.PromptCacheLifetime(tt.spec, shuttleengine.Config{})
			if tt.wantErrContains != nil {
				if err == nil {
					t.Fatalf("PromptCacheLifetime() error = nil; want one containing %q", tt.wantErrContains)
				}
				for _, want := range tt.wantErrContains {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("PromptCacheLifetime() error = %q; want it to contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("PromptCacheLifetime() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("PromptCacheLifetime() = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestKeepWarmCaps proves KeepWarmCaps answers a role's cap with the long cap, and refuses an unusable cap or config naming the key and the file to fix.
func TestKeepWarmCaps(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		role string
		// claude is the claude.yaml the row's engine loads; invalidConfig replaces it with a file the loader refuses.
		claude          Config
		invalidConfig   bool
		wantCap         time.Duration
		wantErrContains []string
	}{
		{name: "template_driver", role: "driver", claude: templateConfig(t), wantCap: 250 * time.Second},
		{name: "template_webster", role: "webster", claude: templateConfig(t), wantCap: 250 * time.Second},
		{name: "template_orch", role: "orch", claude: templateConfig(t), wantCap: 250 * time.Second},
		{name: "template_unmapped_role", role: "implementer", claude: templateConfig(t), wantCap: 250 * time.Second},
		{name: "role_entry_replaces_default", role: "webster", claude: keepWarmConfig(t, "250s", map[string]string{"webster": "20m"}), wantCap: 20 * time.Minute},
		{name: "role_entry_leaves_other_roles", role: "driver", claude: keepWarmConfig(t, "250s", map[string]string{"webster": "20m"}), wantCap: 250 * time.Second},
		{name: "default_below_floor", role: "driver", claude: keepWarmConfig(t, "59s", nil), wantErrContains: []string{"keep_warm_cap", "59s", "lym config claude"}},
		{name: "default_at_default_lifetime", role: "driver", claude: keepWarmConfig(t, "5m", nil), wantErrContains: []string{"keep_warm_cap", "5m", "lym config claude"}},
		{name: "role_entry_above_long_cap", role: "webster", claude: keepWarmConfig(t, "250s", map[string]string{"webster": "56m"}), wantErrContains: []string{"keep_warm_cap_roles[webster]", "56m", "lym config claude"}},
		{name: "role_entry_at_role_lifetime", role: "driver", claude: keepWarmConfig(t, "250s", map[string]string{"driver": "6m"}), wantErrContains: []string{"keep_warm_cap_roles[driver]", "6m", "lym config claude"}},
		{name: "default_not_a_duration", role: "driver", claude: keepWarmConfig(t, "abc", nil), wantErrContains: []string{"keep_warm_cap", "abc", "lym config claude"}},
		{name: "invalid_claude_config", role: "driver", invalidConfig: true, wantErrContains: invalidConfigWant},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var engine *Claude
			if tt.invalidConfig {
				engine = engineWithInvalidConfig(t)
			} else {
				engine = engineWith(t, tt.claude)
			}
			gotCap, gotLong, err := engine.KeepWarmCaps(tt.role)
			if tt.wantErrContains != nil {
				if err == nil {
					t.Fatalf("KeepWarmCaps(%q) error = nil; want one containing %q", tt.role, tt.wantErrContains)
				}
				for _, want := range tt.wantErrContains {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("KeepWarmCaps(%q) error = %q; want it to contain %q", tt.role, err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("KeepWarmCaps(%q) error = %v; want nil", tt.role, err)
			}
			if gotCap != tt.wantCap || gotLong != 55*time.Minute {
				t.Errorf("KeepWarmCaps(%q) = %v, %v; want %v, 55m", tt.role, gotCap, gotLong, tt.wantCap)
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
// A valid effort is rendered as buildLaunchCmd would; an empty effort emits no --effort flag at all, deferring entirely to claude's own default;
// a bare-word model plus a dotted version composes the pinned id ("sonnet" + "4.5" -> "claude-sonnet-4-5") rather than the bare-word model;
// ForkSubagents wraps the line in the CLAUDE_CODE_FORK_SUBAGENT env prefix;
// an empty ResumeSessionID mints a session as before;
// and the resolved permission mode decides --dangerously-skip-permissions on the launch line and the resume line alike.
func TestPrepare_ThreadsSpecIntoLaunchCmds(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const skipPermissions = "--dangerously-skip-permissions"
	tests := []struct {
		name        string
		spec        shuttleengine.Spec
		claude      Config
		onLaunch    flagExpectation
		onBothLines flagExpectation
	}{
		{name: "valid_effort", spec: shuttleengine.Spec{ModelChoice: shuttleengine.ModelChoice{Effort: "high"}}, claude: templateConfig(t), onLaunch: flagExpectation{present: []string{"--effort 'high'"}}},
		{name: "empty_effort", spec: shuttleengine.Spec{}, claude: templateConfig(t), onLaunch: flagExpectation{absent: []string{"--effort"}}},
		{name: "model_and_version", spec: shuttleengine.Spec{ModelChoice: shuttleengine.ModelChoice{Model: "sonnet", Version: "4.5"}}, claude: templateConfig(t), onLaunch: flagExpectation{present: []string{"--model 'claude-sonnet-4-5'"}}},
		{name: "fork_subagents_on", spec: shuttleengine.Spec{ForkSubagents: true}, claude: templateConfig(t), onLaunch: flagExpectation{present: []string{"CLAUDE_CODE_FORK_SUBAGENT"}}},
		{name: "fork_subagents_off", spec: shuttleengine.Spec{ForkSubagents: false}, claude: templateConfig(t), onLaunch: flagExpectation{absent: []string{"CLAUDE_CODE_FORK_SUBAGENT"}}},
		{name: "empty_resume_session_id_mints_session", spec: shuttleengine.Spec{}, claude: templateConfig(t), onLaunch: flagExpectation{present: []string{"--session-id"}, absent: []string{"--resume"}}},
		{name: "permission_interactive_bypass", spec: shuttleengine.Spec{PermissionMode: "bypass", Interactive: true}, claude: templateConfig(t), onBothLines: flagExpectation{present: []string{skipPermissions}}},
		{name: "permission_interactive_empty", spec: shuttleengine.Spec{PermissionMode: "", Interactive: true}, claude: templateConfig(t), onBothLines: flagExpectation{present: []string{skipPermissions}}},
		{name: "permission_autonomous_empty", spec: shuttleengine.Spec{PermissionMode: "", Interactive: false}, claude: templateConfig(t), onBothLines: flagExpectation{present: []string{skipPermissions}}},
		{name: "permission_autonomous_bypass", spec: shuttleengine.Spec{PermissionMode: "bypass", Interactive: false}, claude: templateConfig(t), onBothLines: flagExpectation{present: []string{skipPermissions}}},
		{name: "permission_interactive_prompt", spec: shuttleengine.Spec{PermissionMode: "prompt", Interactive: true}, claude: templateConfig(t), onBothLines: flagExpectation{absent: []string{skipPermissions}}},
		{name: "ttl_driver_gets_five_minutes", spec: shuttleengine.Spec{Role: "driver"}, claude: templateConfig(t), onBothLines: flagExpectation{present: []string{promptCacheTTLAssignment("5m")}}},
		{name: "ttl_orch_gets_one_hour", spec: shuttleengine.Spec{Role: "orch"}, claude: templateConfig(t), onBothLines: flagExpectation{present: []string{promptCacheTTLAssignment("1h")}}},
		{name: "ttl_webster_gets_one_hour", spec: shuttleengine.Spec{Role: "webster"}, claude: templateConfig(t), onBothLines: flagExpectation{present: []string{promptCacheTTLAssignment("1h")}}},
		{name: "ttl_unmapped_role_gets_default", spec: shuttleengine.Spec{Role: "implementer"}, claude: templateConfig(t), onBothLines: flagExpectation{present: []string{promptCacheTTLAssignment("5m")}}},
		{name: "ttl_empty_role_gets_default", spec: shuttleengine.Spec{}, claude: templateConfig(t), onBothLines: flagExpectation{present: []string{promptCacheTTLAssignment("5m")}}},
		{name: "ttl_empty_role_ignores_empty_map_key", spec: shuttleengine.Spec{}, claude: ttlConfig(t, "", map[string]string{"": "1h"}), onBothLines: flagExpectation{present: []string{promptCacheTTLAssignment("5m")}}},
		{name: "ttl_map_entry_overrides_default", spec: shuttleengine.Spec{Role: "orch"}, claude: ttlConfig(t, "", map[string]string{"orch": "1h"}), onBothLines: flagExpectation{present: []string{promptCacheTTLAssignment("1h")}}},
		{name: "ttl_role_missing_from_map_gets_default", spec: shuttleengine.Spec{Role: "driver"}, claude: ttlConfig(t, "", map[string]string{"webster": "1h"}), onBothLines: flagExpectation{present: []string{promptCacheTTLAssignment("5m")}}},
		{name: "ttl_default_one_hour_reaches_unmapped_role", spec: shuttleengine.Spec{Role: "orch"}, claude: ttlConfig(t, "1h", nil), onBothLines: flagExpectation{present: []string{promptCacheTTLAssignment("1h")}}},
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
			launch, err := engineWith(t, tt.claude).Prepare(t.TempDir(), tt.spec, shuttleengine.Config{})
			if err != nil {
				t.Fatalf("Prepare() error: %v; want nil", err)
			}
			check(t, "Launch.Cmd", launch.Cmd, tt.onLaunch)
			check(t, "Launch.Cmd", launch.Cmd, tt.onBothLines)
			check(t, "Launch.ResumeCmd", launch.ResumeCmd, tt.onBothLines)
		})
	}
}

// TestPrepare_AppendSystemPromptMatchesDenyNotice proves that for every combination of the deny inputs, Launch.Cmd and Launch.ResumeCmd both carry --append-system-prompt-file naming the run directory's append-system-prompt.txt and no --append-system-prompt argument,
// and that the file holds the deny notice and the creel trust sentence, space-separated, with the notice left out when empty.
//
//testtiming:keep pins append-system-prompt.txt equal to the notice and the trust sentence, named on both lines over every deny combination, which its covering tests do not assert
func TestPrepare_AppendSystemPromptMatchesDenyNotice(t *testing.T) {
	tiergate.Require(t, 0)
	for _, denyAsk := range []bool{false, true} {
		for _, denyPython := range []bool{false, true} {
			for _, interactive := range []bool{false, true} {
				for _, fork := range []bool{false, true} {
					claude := templateConfig(t)
					claude.DenyAskUserQuestion = denyAsk
					cfg := shuttleengine.Config{DenyPython: denyPython}
					spec := shuttleengine.Spec{Prompt: "do the thing", Interactive: interactive, ForkSubagents: fork}
					notice := buildDenyNotice(interactive, cfg, claude, fork, false)

					runDir := t.TempDir()
					launch, err := engineWith(t, claude).Prepare(runDir, spec, cfg)
					if err != nil {
						t.Fatalf("Prepare(ask=%v python=%v interactive=%v fork=%v) error: %v", denyAsk, denyPython, interactive, fork, err)
					}
					promptFile, err := filepath.Abs(filepath.Join(runDir, "append-system-prompt.txt"))
					if err != nil {
						t.Fatalf("Abs: %v", err)
					}
					for name, cmd := range map[string]string{"Cmd": launch.Cmd, "ResumeCmd": launch.ResumeCmd} {
						if !strings.Contains(cmd, " --append-system-prompt-file "+shell.ForGOOS().Quote(promptFile)) {
							t.Errorf("ask=%v python=%v interactive=%v fork=%v: %s = %q; want --append-system-prompt-file naming %q", denyAsk, denyPython, interactive, fork, name, cmd, promptFile)
						}
						if strings.Contains(cmd, "--append-system-prompt ") {
							t.Errorf("ask=%v python=%v interactive=%v fork=%v: %s = %q; want no --append-system-prompt argument", denyAsk, denyPython, interactive, fork, name, cmd)
						}
					}
					want := shuttleengine.CreelTrustSentence
					if notice != "" {
						want = notice + " " + want
					}
					got, err := os.ReadFile(promptFile)
					if err != nil || string(got) != want {
						t.Errorf("ask=%v python=%v interactive=%v fork=%v: append-system-prompt.txt = %q, %v; want %q", denyAsk, denyPython, interactive, fork, got, err, want)
					}
				}
			}
		}
	}
}

// TestPrepare_ResumeSessionID proves a spec carrying ResumeSessionID launches that session with --resume and names the same id on the resume line and in Launch.SessionID.
//
//testtiming:keep pins that a ResumeSessionID launches with --resume, no --session-id, and the same id on the resume line and in Launch.SessionID, which its covering tests do not assert
func TestPrepare_ResumeSessionID(t *testing.T) {
	tiergate.Require(t, 0)
	const id = "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"
	spec := shuttleengine.Spec{
		Prompt:          "adopt",
		ResumeSessionID: id,
		PermissionMode:  "bypass",
		Interactive:     true,
		ModelChoice:     shuttleengine.ModelChoice{Model: "opus", Effort: "high"},
	}
	runDir := t.TempDir()
	launch, err := engineWith(t, templateConfig(t)).Prepare(runDir, spec, shuttleengine.Config{})
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

// TestPrepare_SkillsDeferPromptPointer proves, for a fresh and a resumed launch, that a spec naming skills carries the skill-load message on the launch line and returns the prompt pointer as PromptLine.
// A spec without skills keeps the pointer on the line with an empty PromptLine.
// A skill-load message over the launch limit fails before any prompt file is written.
func TestPrepare_SkillsDeferPromptPointer(t *testing.T) {
	tiergate.Require(t, 0)
	const id = "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"
	for _, resume := range []bool{false, true} {
		for _, skills := range [][]string{nil, {"scribe:prose"}} {
			spec := shuttleengine.Spec{Prompt: "do the thing", Skills: skills}
			if resume {
				spec.ResumeSessionID = id
			}
			runDir := t.TempDir()
			launch, err := engineWith(t, templateConfig(t)).Prepare(runDir, spec, shuttleengine.Config{})
			if err != nil {
				t.Fatalf("Prepare(resume=%v, skills=%v) error: %v", resume, skills, err)
			}
			pointer := launchPointer(filepath.Join(runDir, "prompt.md"))
			if len(skills) > 0 {
				if strings.Contains(launch.Cmd, "Read ") {
					t.Errorf("resume=%v: Launch.Cmd = %q; want no prompt pointer", resume, launch.Cmd)
				}
				if !strings.Contains(launch.Cmd, New().SkillLoadMessage(skills)) {
					t.Errorf("resume=%v: Launch.Cmd = %q; want the skill-load message", resume, launch.Cmd)
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

	runDir := t.TempDir()
	longSkill := strings.Repeat("s", maxLaunchPromptBytes)
	_, err := engineWith(t, templateConfig(t)).Prepare(runDir, shuttleengine.Spec{Prompt: "do the thing", Skills: []string{longSkill}}, shuttleengine.Config{})
	if err == nil || !strings.Contains(err.Error(), "skill-load message is") || !strings.Contains(err.Error(), "launch limit") {
		t.Errorf("Prepare(over-limit skill-load message) error = %v; want one naming the message length and the launch limit", err)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, "prompt.md")); statErr == nil {
		t.Error("prompt.md written for an over-limit skill-load message; want no artifact")
	}
}

// TestPrepare_LogsResolvedPromptCacheTTL proves Prepare logs the role and the TTL it resolved for it.
// It does not run in parallel: the logger it captures is process-global.
func TestPrepare_LogsResolvedPromptCacheTTL(t *testing.T) {
	tiergate.Require(t, 0)
	logs := logcapture.CaptureVerbose(t)

	if _, err := engineWith(t, templateConfig(t)).Prepare(t.TempDir(), shuttleengine.Spec{Prompt: "p", Role: "driver"}, shuttleengine.Config{}); err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}

	got := logs.String()
	for _, want := range []string{"role=driver", "ttl=5m"} {
		if !strings.Contains(got, want) {
			t.Errorf("captured log = %q; want it to contain %q", got, want)
		}
	}
}

// TestPrepare_AppendsStandingPassages proves the append-system-prompt file carries the standing passages the reader finds for the claude engine and the spec's role after lym's own sentences, each after a blank line and in order.
// With no reader installed, or outside a hub, the file holds lym's sentences only and the reader is never called;
// a reader error leaves the launch succeeded with lym's sentences only and logs the way to inspect the store.
// It installs the process-wide passage reader and captures the logger, so it does not run in parallel.
func TestPrepare_AppendsStandingPassages(t *testing.T) {
	tiergate.Require(t, 0)
	sentences := shuttleengine.CreelTrustSentence
	tests := []struct {
		name string
		// board is the told Bolt checkout; empty means a session outside a hub.
		board string
		// role is the spec's strand role.
		role string
		// reader is nil when no reader is installed.
		reader      shuttleengine.PassageReader
		want        string
		wantLogged  string
		wantNoCalls bool
	}{
		{
			name:  "two_passages",
			board: "bolt",
			reader: func(string, string, string) ([]string, error) {
				return []string{"first passage", "second\nspans lines"}, nil
			},
			want: sentences + "\n\nfirst passage\n\nsecond\nspans lines",
		},
		{
			name:  "role_reaches_reader",
			board: "bolt",
			role:  "orch",
			reader: func(_, engine, role string) ([]string, error) {
				return []string{engine + " " + role}, nil
			},
			want: sentences + "\n\nclaude orch",
		},
		{name: "no_reader", board: "bolt", want: sentences},
		{
			name:  "outside_hub",
			board: "",
			reader: func(string, string, string) ([]string, error) {
				return []string{"never read"}, nil
			},
			want:        sentences,
			wantNoCalls: true,
		},
		{
			name:  "reader_error",
			board: "bolt",
			reader: func(string, string, string) ([]string, error) {
				return nil, errors.New("heddle.json does not decode")
			},
			want:       sentences,
			wantLogged: "lym heddle list",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.Capture(t)
			calls := 0
			if tt.reader != nil {
				reader := tt.reader
				t.Cleanup(shuttleengine.InstallPassageReader(func(boltDir, engine, role string) ([]string, error) {
					calls++
					return reader(boltDir, engine, role)
				}))
			}
			engine := engineWith(t, templateConfig(t))
			if tt.board != "" {
				engine.dirs.Board = t.TempDir()
			}
			runDir := t.TempDir()
			claude := templateConfig(t)
			spec := shuttleengine.Spec{Prompt: "do the thing", Role: tt.role}
			notice := buildDenyNotice(spec.Interactive, shuttleengine.Config{}, claude, false, false)
			if _, err := engine.Prepare(runDir, spec, shuttleengine.Config{}); err != nil {
				t.Fatalf("Prepare() error: %v; want nil", err)
			}
			want := tt.want
			if notice != "" {
				want = notice + " " + want
			}
			got, err := os.ReadFile(filepath.Join(runDir, "append-system-prompt.txt"))
			if err != nil || string(got) != want {
				t.Errorf("append-system-prompt.txt = %q, %v; want %q", got, err, want)
			}
			if tt.wantNoCalls && calls != 0 {
				t.Errorf("reader called %d times outside a hub; want none", calls)
			}
			if !strings.Contains(logs.String(), tt.wantLogged) {
				t.Errorf("log = %q; want it to name %q", logs.String(), tt.wantLogged)
			}
		})
	}
}

// config_test.go verifies shuttle.yaml's template parses, defaults resolve through LoadConfig, and
// environment overrides + the template-fallback path behave the way reedengine's config tests
// establish the pattern.

package shuttleengine_test

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// seedLyxConfig creates <tmpDir>/_lyx/config/<module>.yaml with content, the
// minimal on-disk shape LoadConfig needs (no git repository required).
func seedLyxConfig(t *testing.T, tmpDir, module, content string) {
	t.Helper()
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx/config: %v", err)
	}
	configFile := configengine.ConfigFile(tmpDir, module)
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

// TestLoadConfig_TemplateDefaultsResolve also pins the precondition the gate loop's per-attempt
// done-signal narrowing rests on: the per-attempt done-signal is the next turn boundary and nothing
// more, and that holds only because the in-process Agent tool is denied at every gated site.
// The ClaudeDenyAgentTool failure message names what the value protects.
//
//testtiming:keep pins every shipped template default, which the BackgroundShellWaitMin table does not assert
func TestLoadConfig_TemplateDefaultsResolve(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	// Seed the config file with the template itself: this is exactly the
	// file "lyx config reconcile" would produce, so LoadConfig must accept
	// it verbatim and every default must resolve.
	seedLyxConfig(t, tmpDir, "shuttle", shuttleengine.ConfigTemplate())

	cfg, err := shuttleengine.LoadConfig(tmpDir, "shuttle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.RunDir != "" {
		t.Errorf("RunDir = %q, want empty default", cfg.RunDir)
	}
	if cfg.PollIntervalMS != 1000 {
		t.Errorf("PollIntervalMS = %d, want 1000", cfg.PollIntervalMS)
	}
	if cfg.LivenessEveryNPolls != 10 {
		t.Errorf("LivenessEveryNPolls = %d, want 10", cfg.LivenessEveryNPolls)
	}
	if cfg.RunTimeoutMin != 30 {
		t.Errorf("RunTimeoutMin = %d, want 30", cfg.RunTimeoutMin)
	}
	if cfg.StartupTimeoutS != 90 {
		t.Errorf("StartupTimeoutS = %d, want 90", cfg.StartupTimeoutS)
	}
	if cfg.BackgroundShellWaitMin != 10 {
		t.Errorf("BackgroundShellWaitMin = %d, want 10", cfg.BackgroundShellWaitMin)
	}
	if cfg.SubmitRedrawSettleMS != 300 {
		t.Errorf("SubmitRedrawSettleMS = %d, want 300", cfg.SubmitRedrawSettleMS)
	}
	if cfg.SubmitSettleMS != 300 {
		t.Errorf("SubmitSettleMS = %d, want 300", cfg.SubmitSettleMS)
	}
	if cfg.SendReadyTimeoutS != 60 {
		t.Errorf("SendReadyTimeoutS = %d, want 60", cfg.SendReadyTimeoutS)
	}
	if cfg.SubmitConfirmTimeoutS != 30 {
		t.Errorf("SubmitConfirmTimeoutS = %d, want 30", cfg.SubmitConfirmTimeoutS)
	}
	if cfg.Claude != "" {
		t.Errorf("Claude = %q, want empty default", cfg.Claude)
	}
	if !cfg.ClaudeDenyAgentTool {
		t.Error("Config.ClaudeDenyAgentTool = false against the shipped template: flipping this default " +
			"re-opens the compound-quiescence question this task deliberately declined — a gate could now " +
			"fire while an async in-process subagent, spawned through the Agent tool, is still working, " +
			"undermining the narrowing that the per-attempt done-signal is the next turn boundary and " +
			"nothing more. The narrowing decision must be re-opened deliberately, not " +
			"this test updated.")
	}
	if !cfg.ClaudeDenyAskUserQuestion {
		t.Error("ClaudeDenyAskUserQuestion = false, want true")
	}
	if !cfg.ClaudeDenyPython {
		t.Error("ClaudeDenyPython = false, want true")
	}
	if cfg.ClaudePromptCacheTTL != "5m" {
		t.Errorf("ClaudePromptCacheTTL = %q, want 5m", cfg.ClaudePromptCacheTTL)
	}
	if want := map[string]string{"driver": "1h", "webster": "1h"}; !maps.Equal(cfg.ClaudePromptCacheTTLRoles, want) {
		t.Errorf("ClaudePromptCacheTTLRoles = %v, want %v", cfg.ClaudePromptCacheTTLRoles, want)
	}
}

func TestLoadConfig_PromptCacheTTLKeys(t *testing.T) {
	t.Parallel()

	// The TTL keys are the template's last lines; each row replaces them with its own and keeps every other template key.
	templateWithoutTTLKeys := shuttleengine.ConfigTemplate()
	templateWithoutTTLKeys = templateWithoutTTLKeys[:strings.Index(templateWithoutTTLKeys, "claude_prompt_cache_ttl:")]

	tests := []struct {
		name      string
		ttlKeys   string
		wantTTL   string
		wantRoles map[string]string
	}{
		{
			name:      "file lacking both keys resolves the template default and map",
			ttlKeys:   "",
			wantTTL:   "5m",
			wantRoles: map[string]string{"driver": "1h", "webster": "1h"},
		},
		{
			name:      "present map replaces the template map whole",
			ttlKeys:   "claude_prompt_cache_ttl_roles:\n  orch: 1h\n",
			wantTTL:   "5m",
			wantRoles: map[string]string{"orch": "1h"},
		},
		{
			name:      "default without a map keeps the template map",
			ttlKeys:   "claude_prompt_cache_ttl: 1h\n",
			wantTTL:   "1h",
			wantRoles: map[string]string{"driver": "1h", "webster": "1h"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			seedLyxConfig(t, tmpDir, "shuttle", templateWithoutTTLKeys+tt.ttlKeys)

			cfg, err := shuttleengine.LoadConfig(tmpDir, "shuttle")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.ClaudePromptCacheTTL != tt.wantTTL {
				t.Errorf("ClaudePromptCacheTTL = %q, want %q", cfg.ClaudePromptCacheTTL, tt.wantTTL)
			}
			if !maps.Equal(cfg.ClaudePromptCacheTTLRoles, tt.wantRoles) {
				t.Errorf("ClaudePromptCacheTTLRoles = %v, want %v", cfg.ClaudePromptCacheTTLRoles, tt.wantRoles)
			}
		})
	}
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("LYX_SHUTTLE_CLAUDE", `D:\tools\claude.exe`)
	seedLyxConfig(t, tmpDir, "shuttle", shuttleengine.ConfigTemplate())

	cfg, err := shuttleengine.LoadConfig(tmpDir, "shuttle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude != `D:\tools\claude.exe` {
		t.Errorf("Claude = %q, want env override", cfg.Claude)
	}
}

func TestLoadConfig_BackgroundShellWaitMin(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    int
		wantErr bool
	}{
		{name: "positive override loads", value: "25", want: 25},
		{name: "zero refused", value: "0", wantErr: true},
		{name: "negative refused", value: "-5", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			seeded := strings.Replace(shuttleengine.ConfigTemplate(), "background_shell_wait_min: 10", "background_shell_wait_min: "+tt.value, 1)
			seedLyxConfig(t, tmpDir, "shuttle", seeded)

			cfg, err := shuttleengine.LoadConfig(tmpDir, "shuttle")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadConfig accepted background_shell_wait_min: %s", tt.value)
				}
				if !strings.Contains(err.Error(), "background_shell_wait_min") || !strings.Contains(err.Error(), tt.value) {
					t.Errorf("error %q does not name the key and value %s", err, tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.BackgroundShellWaitMin != tt.want {
				t.Errorf("BackgroundShellWaitMin = %d, want %d", cfg.BackgroundShellWaitMin, tt.want)
			}
		})
	}
}

func TestLoadConfig_SubmitSettleKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		def     string
		value   string
		want    int
		wantErr bool
		got     func(shuttleengine.Config) int
	}{
		{name: "submit settle positive override loads", key: "submit_settle_ms", value: "150", want: 150, got: func(c shuttleengine.Config) int { return c.SubmitSettleMS }},
		{name: "submit settle zero loads", key: "submit_settle_ms", value: "0", want: 0, got: func(c shuttleengine.Config) int { return c.SubmitSettleMS }},
		{name: "submit settle negative refused", key: "submit_settle_ms", value: "-1", wantErr: true},
		{name: "redraw settle positive override loads", key: "submit_redraw_settle_ms", value: "150", want: 150, got: func(c shuttleengine.Config) int { return c.SubmitRedrawSettleMS }},
		{name: "redraw settle zero loads", key: "submit_redraw_settle_ms", value: "0", want: 0, got: func(c shuttleengine.Config) int { return c.SubmitRedrawSettleMS }},
		{name: "redraw settle negative refused", key: "submit_redraw_settle_ms", value: "-1", wantErr: true},
		{name: "send ready timeout positive override loads", key: "send_ready_timeout_s", def: "60", value: "5", want: 5, got: func(c shuttleengine.Config) int { return c.SendReadyTimeoutS }},
		{name: "send ready timeout zero refused", key: "send_ready_timeout_s", def: "60", value: "0", wantErr: true},
		{name: "send ready timeout negative refused", key: "send_ready_timeout_s", def: "60", value: "-1", wantErr: true},
		{name: "submit confirm timeout positive override loads", key: "submit_confirm_timeout_s", def: "30", value: "5", want: 5, got: func(c shuttleengine.Config) int { return c.SubmitConfirmTimeoutS }},
		{name: "submit confirm timeout zero refused", key: "submit_confirm_timeout_s", def: "30", value: "0", wantErr: true},
		{name: "submit confirm timeout negative refused", key: "submit_confirm_timeout_s", def: "30", value: "-1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			def := tt.def
			if def == "" {
				def = "300"
			}
			tmpDir := t.TempDir()
			seeded := strings.Replace(shuttleengine.ConfigTemplate(), tt.key+": "+def, tt.key+": "+tt.value, 1)
			seedLyxConfig(t, tmpDir, "shuttle", seeded)

			cfg, err := shuttleengine.LoadConfig(tmpDir, "shuttle")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadConfig accepted %s: %s", tt.key, tt.value)
				}
				if !strings.Contains(err.Error(), tt.key) || !strings.Contains(err.Error(), tt.value) {
					t.Errorf("error %q does not name the key and value %s", err, tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := tt.got(cfg); got != tt.want {
				t.Errorf("%s = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

func TestLoadConfig_ModuleArgIsThreadedThrough(t *testing.T) {
	tmpDir := t.TempDir()
	// Seed under a non-"shuttle" module name with a config whose
	// poll_interval_ms differs from the template default, so a hardcoded
	// module name would be caught either way: this module reads back its
	// seeded value, and the never-seeded "shuttle" module reads back the
	// template default instead.
	seeded := strings.Replace(shuttleengine.ConfigTemplate(), "poll_interval_ms: 1000", "poll_interval_ms: 1500", 1)
	seedLyxConfig(t, tmpDir, "othershuttle", seeded)

	logs := logcapture.Capture(t)
	cfg, err := shuttleengine.LoadConfig(tmpDir, "othershuttle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PollIntervalMS != 1500 {
		t.Errorf("PollIntervalMS = %d, want 1500 (seeded value)", cfg.PollIntervalMS)
	}
	if strings.Contains(logs.String(), "poll_interval_ms") {
		t.Errorf("a poll interval at or above the floor warned: %q", logs.String())
	}

	// A module seeded below the floor loads, logging the one warning that names the key and the floor.
	below := strings.Replace(shuttleengine.ConfigTemplate(), "poll_interval_ms: 1000", "poll_interval_ms: 250", 1)
	if err := os.WriteFile(configengine.ConfigFile(tmpDir, "belowshuttle"), []byte(below), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	logs.Reset()
	if _, err := shuttleengine.LoadConfig(tmpDir, "belowshuttle"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Count(logs.String(), "level=WARN"); got != 1 || !strings.Contains(logs.String(), "key=poll_interval_ms") || !strings.Contains(logs.String(), "floor_ms=1000") {
		t.Errorf("below-floor load logged %q, want one warning naming poll_interval_ms and the floor", logs.String())
	}

	// The never-seeded "shuttle" module must fall back to the template
	// default, not the "othershuttle" module's seeded value.
	defaultCfg, err := shuttleengine.LoadConfig(tmpDir, "shuttle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultCfg.PollIntervalMS != 1000 {
		t.Errorf("PollIntervalMS = %d, want 1000 (template default)", defaultCfg.PollIntervalMS)
	}
}

func TestLoadConfig_UninitializedFallsBackToTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/ -- LoadConfig must degrade to the embedded template.

	cfg, err := shuttleengine.LoadConfig(tmpDir, "shuttle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.PollIntervalMS != 1000 {
		t.Errorf("PollIntervalMS = %d, want 1000", cfg.PollIntervalMS)
	}
	if cfg.LivenessEveryNPolls != 10 {
		t.Errorf("LivenessEveryNPolls = %d, want 10", cfg.LivenessEveryNPolls)
	}
	if cfg.RunTimeoutMin != 30 {
		t.Errorf("RunTimeoutMin = %d, want 30", cfg.RunTimeoutMin)
	}
	if cfg.StartupTimeoutS != 90 {
		t.Errorf("StartupTimeoutS = %d, want 90", cfg.StartupTimeoutS)
	}
	if !cfg.ClaudeDenyAgentTool {
		t.Error("ClaudeDenyAgentTool = false, want true")
	}
}

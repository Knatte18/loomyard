// config_test.go verifies shuttle.yaml's template parses, defaults resolve through LoadConfig, and
// environment overrides + the template-fallback path behave the way reedengine's config tests
// establish the pattern.

package shuttleengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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
	if cfg.PollIntervalMS != 500 {
		t.Errorf("PollIntervalMS = %d, want 500", cfg.PollIntervalMS)
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
	if cfg.SubmitSettleMS != 300 {
		t.Errorf("SubmitSettleMS = %d, want 300", cfg.SubmitSettleMS)
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

func TestLoadConfig_SubmitSettleMS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    int
		wantErr bool
	}{
		{name: "positive override loads", value: "150", want: 150},
		{name: "zero loads", value: "0", want: 0},
		{name: "negative refused", value: "-1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			seeded := strings.Replace(shuttleengine.ConfigTemplate(), "submit_settle_ms: 300", "submit_settle_ms: "+tt.value, 1)
			seedLyxConfig(t, tmpDir, "shuttle", seeded)

			cfg, err := shuttleengine.LoadConfig(tmpDir, "shuttle")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadConfig accepted submit_settle_ms: %s", tt.value)
				}
				if !strings.Contains(err.Error(), "submit_settle_ms") || !strings.Contains(err.Error(), tt.value) {
					t.Errorf("error %q does not name the key and value %s", err, tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.SubmitSettleMS != tt.want {
				t.Errorf("SubmitSettleMS = %d, want %d", cfg.SubmitSettleMS, tt.want)
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
	seeded := strings.Replace(shuttleengine.ConfigTemplate(), "poll_interval_ms: 500", "poll_interval_ms: 750", 1)
	seedLyxConfig(t, tmpDir, "othershuttle", seeded)

	cfg, err := shuttleengine.LoadConfig(tmpDir, "othershuttle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PollIntervalMS != 750 {
		t.Errorf("PollIntervalMS = %d, want 750 (seeded value)", cfg.PollIntervalMS)
	}

	// The never-seeded "shuttle" module must fall back to the template
	// default, not the "othershuttle" module's seeded value.
	defaultCfg, err := shuttleengine.LoadConfig(tmpDir, "shuttle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultCfg.PollIntervalMS != 500 {
		t.Errorf("PollIntervalMS = %d, want 500 (template default)", defaultCfg.PollIntervalMS)
	}
}

func TestLoadConfig_UninitializedFallsBackToTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/ -- LoadConfig must degrade to the embedded template.

	cfg, err := shuttleengine.LoadConfig(tmpDir, "shuttle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.PollIntervalMS != 500 {
		t.Errorf("PollIntervalMS = %d, want 500", cfg.PollIntervalMS)
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

// config_test.go verifies reed.yaml's template parses, defaults resolve through LoadConfig, and
// environment overrides + the template-fallback path behave the way shuttleengine's config tests
// establish the pattern.

package reedengine_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// seedLyxConfig creates the minimal on-disk config structure for LoadConfig.
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

//testtiming:keep pins every shipped template default through a seeded reed.yaml: tmux and shell per GOOS, width, height, collapsed and minimum full rows, debug_log off and mouse on; its covering tests run this code without asserting it
func TestLoadConfig_TemplateDefaultsResolve(t *testing.T) {
	tmpDir := t.TempDir()
	seedLyxConfig(t, tmpDir, "reed", reedengine.ConfigTemplate())

	cfg, err := reedengine.LoadConfig(tmpDir, "reed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantTmux, wantShell := "tmux", "pwsh"
	if runtime.GOOS != "windows" {
		wantTmux, wantShell = "tmux", "bash"
	}
	if cfg.Tmux != wantTmux {
		t.Errorf("Tmux = %q, want %q", cfg.Tmux, wantTmux)
	}
	if cfg.Shell != wantShell {
		t.Errorf("Shell = %q, want %q", cfg.Shell, wantShell)
	}
	if cfg.Width != 220 {
		t.Errorf("Width = %d, want 220", cfg.Width)
	}
	if cfg.Height != 50 {
		t.Errorf("Height = %d, want 50", cfg.Height)
	}
	if cfg.CollapsedRows != 3 {
		t.Errorf("CollapsedRows = %d, want 3", cfg.CollapsedRows)
	}
	if cfg.MinFullRows != 3 {
		t.Errorf("MinFullRows = %d, want 3", cfg.MinFullRows)
	}
	if cfg.DebugLog != "0" {
		t.Errorf("DebugLog = %q, want %q", cfg.DebugLog, "0")
	}
	// Mouse defaults ON: with it off tmux never claims the wheel, so the terminal's own
	// alternate-screen translation delivers arrow keys into the live agent's input instead of
	// scrolling anything -- which for an interactive producer means an operator's scroll gesture
	// types into the interview.
	if cfg.Mouse != "on" {
		t.Errorf("Mouse = %q, want %q", cfg.Mouse, "on")
	}
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("LYX_REED_TMUX", `D:\tools\tmux.exe`)
	seedLyxConfig(t, tmpDir, "reed", reedengine.ConfigTemplate())

	cfg, err := reedengine.LoadConfig(tmpDir, "reed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Tmux != `D:\tools\tmux.exe` {
		t.Errorf("Tmux = %q, want env override", cfg.Tmux)
	}
}

func TestLoadConfig_ModuleArgIsThreadedThrough(t *testing.T) {
	tmpDir := t.TempDir()
	// Seed under a non-"reed" module name with a config whose width differs
	// from the template default, so a hardcoded module name would be caught
	// either way: this module reads back its seeded value, and the
	// never-seeded "reed" module reads back the template default instead.
	seeded := strings.Replace(reedengine.ConfigTemplate(), "width: 220", "width: 300", 1)
	seedLyxConfig(t, tmpDir, "otherreed", seeded)

	cfg, err := reedengine.LoadConfig(tmpDir, "otherreed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Width != 300 {
		t.Errorf("Width = %d, want 300 (seeded value)", cfg.Width)
	}

	// The never-seeded "reed" module must fall back to the template default,
	// not the "otherreed" module's seeded value.
	defaultCfg, err := reedengine.LoadConfig(tmpDir, "reed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultCfg.Width != 220 {
		t.Errorf("Width = %d, want 220 (template default)", defaultCfg.Width)
	}
}

func TestLoadConfig_UninitializedFallsBackToTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/ -- LoadConfig must degrade to the embedded
	// template. Only assert GOOS-invariant keys: Tmux/Shell differ between
	// template_posix.yaml and template_windows.yaml.

	cfg, err := reedengine.LoadConfig(tmpDir, "reed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Width != 220 {
		t.Errorf("Width = %d, want 220", cfg.Width)
	}
	if cfg.CollapsedRows != 3 {
		t.Errorf("CollapsedRows = %d, want 3", cfg.CollapsedRows)
	}
	if cfg.Selvage.HeightRows != 1 {
		t.Errorf("Selvage.HeightRows = %d, want 1", cfg.Selvage.HeightRows)
	}
}

// TestLoadConfig_UnknownKeysFromAnOlderConfigAreIgnored pins the no-removal-logic decision: an un-reconciled
// reed.yaml that still carries a stale header: block alongside the new status_line: and selvage: blocks,
// or the retired strand_name key, must unmarshal cleanly, with the old keys simply ignored
// because nothing unmarshals them into Config any more.
//
//testtiming:keep pins an older reed.yaml with a stale header block or the retired strand_name key loading cleanly with the rest of the config undisturbed; its covering tests run this code without asserting it
func TestLoadConfig_UnknownKeysFromAnOlderConfigAreIgnored(t *testing.T) {
	tests := []struct {
		name  string
		extra string
	}{
		{"StaleHeaderBlock", "\nheader:\n  template: \"stale\"\n  height_rows: 5\n"},
		{"RetiredStrandNameKey", "\nstrand_name: '<ROLE>:<ROUND>:<SHORT_GUID>'\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			seedLyxConfig(t, tmpDir, "reed", reedengine.ConfigTemplate()+tt.extra)

			cfg, err := reedengine.LoadConfig(tmpDir, "reed")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Selvage.HeightRows != 1 {
				t.Errorf("Selvage.HeightRows = %d, want 1 (an old key must not override it)", cfg.Selvage.HeightRows)
			}
			if cfg.Width != 220 {
				t.Errorf("Width = %d, want 220 (an old key must not disturb the rest)", cfg.Width)
			}
		})
	}
}

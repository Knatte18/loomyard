// config_test.go verifies shuttle.yaml's template parses, defaults resolve
// through LoadConfig, and environment overrides + the not-initialized error
// path behave the way muxengine's config tests establish the pattern.

package shuttleengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// seedLyxConfig creates <tmpDir>/_lyx/config/<module>.yaml with content, the
// minimal on-disk shape LoadConfig needs (no git repository required).
func seedLyxConfig(t *testing.T, tmpDir, module, content string) {
	t.Helper()
	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
	if err := os.Mkdir(lyxDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx: %v", err)
	}
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx/config: %v", err)
	}
	configFile := hubgeometry.ConfigFile(tmpDir, module)
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

func TestLoadConfig_TemplateDefaultsResolve(t *testing.T) {
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
	if cfg.Claude != "" {
		t.Errorf("Claude = %q, want empty default", cfg.Claude)
	}
	if !cfg.ClaudeDenyAgentTool {
		t.Error("ClaudeDenyAgentTool = false, want true")
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

func TestLoadConfig_ModuleArgIsThreadedThrough(t *testing.T) {
	tmpDir := t.TempDir()
	// Seed under a non-"shuttle" module name; LoadConfig must resolve the
	// file at that module's path, not a hardcoded "shuttle.yaml".
	seedLyxConfig(t, tmpDir, "othershuttle", shuttleengine.ConfigTemplate())

	cfg, err := shuttleengine.LoadConfig(tmpDir, "othershuttle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PollIntervalMS != 500 {
		t.Errorf("PollIntervalMS = %d, want 500", cfg.PollIntervalMS)
	}

	// Loading under the default "shuttle" module name (never seeded) must
	// fail.
	if _, err := shuttleengine.LoadConfig(tmpDir, "shuttle"); err == nil {
		t.Error("LoadConfig(tmpDir, \"shuttle\") = nil error, want error (module name must not be hardcoded)")
	}
}

func TestLoadConfig_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	cfg, err := shuttleengine.LoadConfig(tmpDir, "shuttle")
	if err == nil {
		t.Fatalf("expected error for not initialized, got nil; config: %+v", cfg)
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "not initialized") {
		t.Errorf("expected error containing 'not initialized', got: %v", err)
	}
	if !strings.Contains(errMsg, "lyx init") {
		t.Errorf("expected error containing 'lyx init', got: %v", err)
	}
}

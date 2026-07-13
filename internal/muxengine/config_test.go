// config_test.go verifies mux.yaml's template parses, defaults resolve
// through LoadConfig, and environment overrides + the not-initialized error
// path behave the way warpengine's config tests establish the pattern.

package muxengine_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/muxengine"
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
	seedLyxConfig(t, tmpDir, "mux", muxengine.ConfigTemplate())

	cfg, err := muxengine.LoadConfig(tmpDir, "mux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ConfigTemplate() is OS-split (template_windows.go / template_posix.go):
	// the Windows template pins psmux.exe/pwsh.exe install paths, the POSIX
	// template defers to the PATH names tmux/bash. Assert the default that
	// matches the host so the test tracks whichever template was embedded.
	wantPsmux, wantPwsh := `C:\Code\tools\bin\psmux.exe`, `C:\Code\tools\powershell7\pwsh.exe`
	if runtime.GOOS != "windows" {
		wantPsmux, wantPwsh = "tmux", "bash"
	}
	if cfg.Psmux != wantPsmux {
		t.Errorf("Psmux = %q, want %q", cfg.Psmux, wantPsmux)
	}
	if cfg.Pwsh != wantPwsh {
		t.Errorf("Pwsh = %q, want %q", cfg.Pwsh, wantPwsh)
	}
	if cfg.Width != 220 {
		t.Errorf("Width = %d, want 220", cfg.Width)
	}
	if cfg.Height != 50 {
		t.Errorf("Height = %d, want 50", cfg.Height)
	}
	if cfg.CollapsedStripRows != 3 {
		t.Errorf("CollapsedStripRows = %d, want 3", cfg.CollapsedStripRows)
	}
	if cfg.TopBandRows != 1 {
		t.Errorf("TopBandRows = %d, want 1", cfg.TopBandRows)
	}
	if cfg.MinFullRows != 3 {
		t.Errorf("MinFullRows = %d, want 3", cfg.MinFullRows)
	}
	if cfg.StrandName != "<ROLE>:<ROUND>:<SHORT_GUID>" {
		t.Errorf("StrandName = %q, want %q", cfg.StrandName, "<ROLE>:<ROUND>:<SHORT_GUID>")
	}
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("LYX_MUX_PSMUX", `D:\tools\psmux.exe`)
	seedLyxConfig(t, tmpDir, "mux", muxengine.ConfigTemplate())

	cfg, err := muxengine.LoadConfig(tmpDir, "mux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Psmux != `D:\tools\psmux.exe` {
		t.Errorf("Psmux = %q, want env override", cfg.Psmux)
	}
}

func TestLoadConfig_ModuleArgIsThreadedThrough(t *testing.T) {
	tmpDir := t.TempDir()
	// Seed under a non-"mux" module name; LoadConfig must resolve the file
	// at that module's path, not a hardcoded "mux.yaml".
	seedLyxConfig(t, tmpDir, "othermux", muxengine.ConfigTemplate())

	cfg, err := muxengine.LoadConfig(tmpDir, "othermux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Width != 220 {
		t.Errorf("Width = %d, want 220", cfg.Width)
	}

	// Loading under the default "mux" module name (never seeded) must fail.
	if _, err := muxengine.LoadConfig(tmpDir, "mux"); err == nil {
		t.Error("LoadConfig(tmpDir, \"mux\") = nil error, want error (module name must not be hardcoded)")
	}
}

func TestLoadConfig_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	cfg, err := muxengine.LoadConfig(tmpDir, "mux")
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

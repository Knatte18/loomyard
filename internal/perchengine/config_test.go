// config_test.go verifies perch.yaml's template parses, defaults resolve
// through LoadConfig, and the not-initialized error path behaves the way
// muxengine's and shuttleengine's config tests establish the pattern.

package perchengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/perchengine"
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
	seedLyxConfig(t, tmpDir, "perch", perchengine.ConfigTemplate())

	cfg, err := perchengine.LoadConfig(tmpDir, "perch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.JudgeModel != "haiku" {
		t.Errorf("JudgeModel = %q, want %q", cfg.JudgeModel, "haiku")
	}
	if cfg.JudgeEffort != "" {
		t.Errorf("JudgeEffort = %q, want empty (provider default)", cfg.JudgeEffort)
	}
	wantCaps := []int{5, 8, 10}
	if len(cfg.RoundCaps) != len(wantCaps) {
		t.Fatalf("RoundCaps = %v, want %v", cfg.RoundCaps, wantCaps)
	}
	for i, want := range wantCaps {
		if cfg.RoundCaps[i] != want {
			t.Errorf("RoundCaps[%d] = %d, want %d", i, cfg.RoundCaps[i], want)
		}
	}
}

func TestLoadConfig_ModuleArgIsThreadedThrough(t *testing.T) {
	tmpDir := t.TempDir()
	// Seed under a non-"perch" module name; LoadConfig must resolve the file
	// at that module's path, not a hardcoded "perch.yaml".
	seedLyxConfig(t, tmpDir, "otherperch", perchengine.ConfigTemplate())

	cfg, err := perchengine.LoadConfig(tmpDir, "otherperch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JudgeModel != "haiku" {
		t.Errorf("JudgeModel = %q, want %q", cfg.JudgeModel, "haiku")
	}

	// Loading under the default "perch" module name (never seeded) must fail.
	if _, err := perchengine.LoadConfig(tmpDir, "perch"); err == nil {
		t.Error("LoadConfig(tmpDir, \"perch\") = nil error, want error (module name must not be hardcoded)")
	}
}

func TestLoadConfig_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	cfg, err := perchengine.LoadConfig(tmpDir, "perch")
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

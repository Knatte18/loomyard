// config_test.go verifies orch.yaml's template resolves through LoadConfig, a present file overrides it, an invalid file errors, and each accessor floors a non-positive value.

package orchengine_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

// seedLyxConfig creates <tmpDir>/_lyx/config/<module>.yaml with content.
func seedLyxConfig(t *testing.T, tmpDir, module, content string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(tmpDir, lyxdirs.LyxDirName), 0o755); err != nil {
		t.Fatalf("mkdir _lyx: %v", err)
	}
	if err := os.Mkdir(configengine.ConfigDir(tmpDir), 0o755); err != nil {
		t.Fatalf("mkdir _lyx/config: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(tmpDir, module), []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

func TestLoadConfig_TemplateResolvesWithNoFile(t *testing.T) {
	cfg, err := orchengine.LoadConfig(t.TempDir(), "orch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "" || cfg.Effort != "" {
		t.Errorf("Model/Effort = %q/%q, want empty", cfg.Model, cfg.Effort)
	}
	if got := cfg.Threshold(); got != 150000 {
		t.Errorf("Threshold() = %d, want 150000", got)
	}
	if got := cfg.IdleGrace(); got != 30*time.Second {
		t.Errorf("IdleGrace() = %v, want 30s", got)
	}
	if got := cfg.HandoffTimeout(); got != 600*time.Second {
		t.Errorf("HandoffTimeout() = %v, want 600s", got)
	}
	if got := cfg.PollInterval(); got != 2000*time.Millisecond {
		t.Errorf("PollInterval() = %v, want 2s", got)
	}
}

func TestLoadConfig_PresentFileOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	seedLyxConfig(t, tmpDir, "orch", "model: opus\neffort: high\nthreshold_tokens: 90000\nidle_grace_s: 5\nhandoff_timeout_s: 60\npoll_interval_ms: 250\n")

	cfg, err := orchengine.LoadConfig(tmpDir, "orch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "opus" || cfg.Effort != "high" {
		t.Errorf("Model/Effort = %q/%q, want opus/high", cfg.Model, cfg.Effort)
	}
	if got := cfg.Threshold(); got != 90000 {
		t.Errorf("Threshold() = %d, want 90000", got)
	}
	if got := cfg.IdleGrace(); got != 5*time.Second {
		t.Errorf("IdleGrace() = %v, want 5s", got)
	}
	if got := cfg.HandoffTimeout(); got != 60*time.Second {
		t.Errorf("HandoffTimeout() = %v, want 60s", got)
	}
	if got := cfg.PollInterval(); got != 250*time.Millisecond {
		t.Errorf("PollInterval() = %v, want 250ms", got)
	}
}

func TestLoadConfig_InvalidFileErrors(t *testing.T) {
	tmpDir := t.TempDir()
	seedLyxConfig(t, tmpDir, "orch", "model: [unterminated\n")

	if _, err := orchengine.LoadConfig(tmpDir, "orch"); err == nil {
		t.Fatal("LoadConfig on an invalid file = nil error, want error")
	}
}

func TestConfig_AccessorsFloorNonPositive(t *testing.T) {
	for _, v := range []int{0, -1} {
		cfg := orchengine.Config{ThresholdTokens: v, IdleGraceS: v, HandoffTimeoutS: v, PollIntervalMS: v}
		if got := cfg.Threshold(); got != 150000 {
			t.Errorf("Threshold() with %d = %d, want 150000", v, got)
		}
		if got := cfg.IdleGrace(); got != 30*time.Second {
			t.Errorf("IdleGrace() with %d = %v, want 30s", v, got)
		}
		if got := cfg.HandoffTimeout(); got != 600*time.Second {
			t.Errorf("HandoffTimeout() with %d = %v, want 600s", v, got)
		}
		if got := cfg.PollInterval(); got != 2000*time.Millisecond {
			t.Errorf("PollInterval() with %d = %v, want 2s", v, got)
		}
	}
}

// config_test.go verifies orch.yaml's template resolves through LoadConfig, a present file overrides it, an invalid file errors, and each accessor floors a non-positive value.

package orchengine_test

import (
	"os"
	"path/filepath"
	"strings"
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
	if cfg.PermissionMode != "bypass" {
		t.Errorf("PermissionMode = %q, want bypass", cfg.PermissionMode)
	}
	if cfg.Mode() != orchengine.CycleCompact {
		t.Errorf("Mode() = %q, want compact", cfg.Mode())
	}
	if got := cfg.Threshold(); got != 400000 {
		t.Errorf("Threshold() = %d, want 400000", got)
	}
	if got := cfg.SoftThreshold(); got != 300000 {
		t.Errorf("SoftThreshold() = %d, want 300000", got)
	}
	if got := cfg.SoftIdle(); got != 300*time.Second {
		t.Errorf("SoftIdle() = %v, want 300s", got)
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
	seedLyxConfig(t, tmpDir, "orch", "model: opus\neffort: high\npermission_mode: bypass\ncycle_mode: clear\nsoft_threshold_tokens: 70000\nsoft_idle_s: 7\nthreshold_tokens: 90000\nidle_grace_s: 5\nhandoff_timeout_s: 60\npoll_interval_ms: 250\n")

	cfg, err := orchengine.LoadConfig(tmpDir, "orch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "opus" || cfg.Effort != "high" {
		t.Errorf("Model/Effort = %q/%q, want opus/high", cfg.Model, cfg.Effort)
	}
	if cfg.PermissionMode != "bypass" {
		t.Errorf("PermissionMode = %q, want bypass", cfg.PermissionMode)
	}
	if cfg.Mode() != orchengine.CycleClear {
		t.Errorf("Mode() = %q, want clear", cfg.Mode())
	}
	if got := cfg.Threshold(); got != 90000 {
		t.Errorf("Threshold() = %d, want 90000", got)
	}
	if got := cfg.SoftThreshold(); got != 70000 {
		t.Errorf("SoftThreshold() = %d, want 70000", got)
	}
	if got := cfg.SoftIdle(); got != 7*time.Second {
		t.Errorf("SoftIdle() = %v, want 7s", got)
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

func TestLoadConfig_CycleMode(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"absent key is compact", "model: opus\n", orchengine.CycleCompact},
		{"empty is compact", "cycle_mode: \"\"\n", orchengine.CycleCompact},
		{"clear loads", "cycle_mode: clear\n", orchengine.CycleClear},
		{"compact loads", "cycle_mode: compact\n", orchengine.CycleCompact},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			seedLyxConfig(t, tmpDir, "orch", tc.content)
			cfg, err := orchengine.LoadConfig(tmpDir, "orch")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := cfg.Mode(); got != tc.want {
				t.Errorf("Mode() = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("unknown value errors naming both", func(t *testing.T) {
		tmpDir := t.TempDir()
		seedLyxConfig(t, tmpDir, "orch", "cycle_mode: restart\n")
		_, err := orchengine.LoadConfig(tmpDir, "orch")
		if err == nil {
			t.Fatal("LoadConfig with cycle_mode: restart = nil error, want error")
		}
		if !strings.Contains(err.Error(), "clear") || !strings.Contains(err.Error(), "compact") {
			t.Errorf("error = %q, want both accepted values named", err)
		}
	})
}

func TestLoadConfig_PermissionMode(t *testing.T) {
	for _, content := range []string{"model: opus\n", "permission_mode: \"\"\n", "permission_mode: bypass\n"} {
		tmpDir := t.TempDir()
		seedLyxConfig(t, tmpDir, "orch", content)
		cfg, err := orchengine.LoadConfig(tmpDir, "orch")
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", content, err)
		}
		if cfg.PermissionMode != "bypass" {
			t.Errorf("%q: PermissionMode = %q, want bypass", content, cfg.PermissionMode)
		}
	}

	for _, mode := range []string{"prompt", "plan"} {
		tmpDir := t.TempDir()
		seedLyxConfig(t, tmpDir, "orch", "permission_mode: "+mode+"\n")
		_, err := orchengine.LoadConfig(tmpDir, "orch")
		if err == nil {
			t.Fatalf("permission_mode: %s = nil error, want error", mode)
		}
		if !strings.Contains(err.Error(), "set permission_mode: bypass or remove the key") {
			t.Errorf("permission_mode: %s error = %q, want the fix named", mode, err)
		}
	}
}

func TestConfig_AccessorsFloorNonPositive(t *testing.T) {
	for _, v := range []int{0, -1} {
		cfg := orchengine.Config{ThresholdTokens: v, SoftThresholdTokens: v, SoftIdleS: v, IdleGraceS: v, HandoffTimeoutS: v, PollIntervalMS: v}
		if got := cfg.Threshold(); got != 400000 {
			t.Errorf("Threshold() with %d = %d, want 400000", v, got)
		}
		if got := cfg.SoftThreshold(); got != 300000 {
			t.Errorf("SoftThreshold() with %d = %d, want 300000", v, got)
		}
		if got := cfg.SoftIdle(); got != 300*time.Second {
			t.Errorf("SoftIdle() with %d = %v, want 300s", v, got)
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

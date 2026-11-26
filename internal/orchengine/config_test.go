// config_test.go verifies orch.yaml's template resolves through LoadConfig, a present file overrides it, an invalid file errors, and each accessor floors a non-positive value.

package orchengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seedLymConfig creates <tmpDir>/_lym/config/<module>.yaml with content.
func seedLymConfig(t *testing.T, tmpDir, module, content string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(tmpDir, lymdirs.LymDirName), 0o755); err != nil {
		t.Fatalf("mkdir _lym: %v", err)
	}
	if err := os.Mkdir(configengine.ConfigDir(tmpDir), 0o755); err != nil {
		t.Fatalf("mkdir _lym/config: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(tmpDir, module), []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

func TestLoadConfig_TemplateResolvesWithNoFile(t *testing.T) {
	tiergate.Require(t, 0)
	cfg, err := orchengine.LoadConfig(configengine.Dirs{Anchor: t.TempDir()}, "orch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "opus" || cfg.Effort != "medium" {
		t.Errorf("Model/Effort = %q/%q, want opus/medium", cfg.Model, cfg.Effort)
	}
	if cfg.PermissionMode != "bypass" {
		t.Errorf("PermissionMode = %q, want bypass", cfg.PermissionMode)
	}
	if cfg.Mode() != orchengine.CycleCompact {
		t.Errorf("Mode() = %q, want compact", cfg.Mode())
	}
	if got := cfg.Threshold(); got != 200000 {
		t.Errorf("Threshold() = %d, want 200000", got)
	}
	if got := cfg.SoftThreshold(); got != 150000 {
		t.Errorf("SoftThreshold() = %d, want 150000", got)
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
	tiergate.Require(t, 0)
	tmpDir := t.TempDir()
	seedLymConfig(t, tmpDir, "orch", "model: opus\neffort: high\npermission_mode: bypass\ncycle_mode: clear\nsoft_threshold_tokens: 70000\nsoft_idle_s: 7\nthreshold_tokens: 90000\nidle_grace_s: 5\nhandoff_timeout_s: 60\npoll_interval_ms: 1500\n")

	logs := logcapture.Capture(t)
	cfg, err := orchengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "orch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(logs.String(), "poll_interval_ms") {
		t.Errorf("a poll interval at or above the floor warned: %q", logs.String())
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
	if got := cfg.PollInterval(); got != 1500*time.Millisecond {
		t.Errorf("PollInterval() = %v, want 1.5s", got)
	}

	belowDir := t.TempDir()
	seedLymConfig(t, belowDir, "orch", "poll_interval_ms: 250\n")
	logs.Reset()
	below, err := orchengine.LoadConfig(configengine.Dirs{Anchor: belowDir}, "orch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := below.PollInterval(); got != time.Second {
		t.Errorf("PollInterval() below the floor = %v, want 1s", got)
	}
	if got := strings.Count(logs.String(), "level=WARN"); got != 1 || !strings.Contains(logs.String(), "key=poll_interval_ms") || !strings.Contains(logs.String(), "floor_ms=1000") {
		t.Errorf("below-floor load logged %q, want one warning naming poll_interval_ms and the floor", logs.String())
	}

	// A key set only in the repo default resolves beneath a worktree file lacking it.
	board, anchor := t.TempDir(), t.TempDir()
	seedLymConfig(t, board, "orch", "model: opus\neffort: high\n")
	seedLymConfig(t, anchor, "orch", "effort: low\n")
	stacked, err := orchengine.LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "orch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stacked.Model != "opus" || stacked.Effort != "low" {
		t.Errorf("Model/Effort = %q/%q, want opus (repo default) / low (worktree file)", stacked.Model, stacked.Effort)
	}
}

func TestLoadConfig_InvalidFileErrors(t *testing.T) {
	tiergate.Require(t, 0)
	tmpDir := t.TempDir()
	seedLymConfig(t, tmpDir, "orch", "model: [unterminated\n")

	if _, err := orchengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "orch"); err == nil {
		t.Fatal("LoadConfig on an invalid file = nil error, want error")
	}
}

func TestLoadConfig_CycleMode(t *testing.T) {
	tiergate.Require(t, 0)
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
			seedLymConfig(t, tmpDir, "orch", tc.content)
			cfg, err := orchengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "orch")
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
		seedLymConfig(t, tmpDir, "orch", "cycle_mode: restart\n")
		_, err := orchengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "orch")
		if err == nil {
			t.Fatal("LoadConfig with cycle_mode: restart = nil error, want error")
		}
		if !strings.Contains(err.Error(), "clear") || !strings.Contains(err.Error(), "compact") {
			t.Errorf("error = %q, want both accepted values named", err)
		}
	})
}

func TestLoadConfig_PermissionMode(t *testing.T) {
	tiergate.Require(t, 0)
	for _, content := range []string{"model: opus\n", "permission_mode: \"\"\n", "permission_mode: bypass\n"} {
		tmpDir := t.TempDir()
		seedLymConfig(t, tmpDir, "orch", content)
		cfg, err := orchengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "orch")
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", content, err)
		}
		if cfg.PermissionMode != "bypass" {
			t.Errorf("%q: PermissionMode = %q, want bypass", content, cfg.PermissionMode)
		}
	}

	for _, mode := range []string{"prompt", "plan"} {
		tmpDir := t.TempDir()
		seedLymConfig(t, tmpDir, "orch", "permission_mode: "+mode+"\n")
		_, err := orchengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "orch")
		if err == nil {
			t.Fatalf("permission_mode: %s = nil error, want error", mode)
		}
		if !strings.Contains(err.Error(), "set orch.permission_mode to bypass or remove the key") {
			t.Errorf("permission_mode: %s error = %q, want the fix named", mode, err)
		}
	}
}

func TestLoadConfig_RemoteControl(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name      string
		content   string
		wantOn    bool
		wantName  string
		wantError string
	}{
		{"the template is off with the cross-machine address", "model: opus\n", false, "{machine}::{name}", ""},
		{"switched on keeps the template's name", "remote_control: true\n", true, "{machine}::{name}", ""},
		{"off with an empty name loads", "remote_control_name: \"\"\n", false, "", ""},
		{"on with an empty name is refused", "remote_control: true\nremote_control_name: \" \"\n", false, "", "remote_control_name is empty"},
		{"a control character is refused", "remote_control_name: \"a\\tb\"\n", false, "", "control character"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			seedLymConfig(t, tmpDir, "orch", tt.content)
			cfg, err := orchengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "orch")
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Errorf("LoadConfig(%q) error = %v; want it to contain %q", tt.content, err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig(%q): %v", tt.content, err)
			}
			if cfg.RemoteControl != tt.wantOn || cfg.RemoteControlName != tt.wantName {
				t.Errorf("LoadConfig(%q) remote control = %v, %q; want %v, %q", tt.content, cfg.RemoteControl, cfg.RemoteControlName, tt.wantOn, tt.wantName)
			}
		})
	}
}

func TestConfig_RemoteControlSessionName(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	beamOrch := agentname.Name{Shortname: "ly", Slug: "b", Role: "orch"}
	tests := []struct {
		name     string
		template string
		orch     agentname.Name
		want     string
	}{
		{"the template's form is the cross-machine address", "{machine}::{name}", beamOrch, agentname.FormatMachineAddress("neve", beamOrch.String())},
		{"every placeholder fills", "{hub}:{role}[{machine}]", beamOrch, "ly:orch[neve]"},
		{"other brace text stays literal", "{machine} {slug}", beamOrch, "neve {slug}"},
		{"a value is never filled again", "{name}", agentname.Name{Shortname: "ly", Role: "orch"}, "ly:orch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := orchengine.Config{RemoteControlName: tt.template}
			if got := cfg.RemoteControlSessionName("neve", tt.orch); got != tt.want {
				t.Errorf("RemoteControlSessionName(neve, %s) with %q = %q; want %q", tt.orch, tt.template, got, tt.want)
			}
		})
	}
}

func TestConfig_AccessorsFloorNonPositive(t *testing.T) {
	tiergate.Require(t, 0)
	for _, v := range []int{0, -1} {
		cfg := orchengine.Config{ThresholdTokens: v, SoftThresholdTokens: v, SoftIdleS: v, IdleGraceS: v, HandoffTimeoutS: v, PollIntervalMS: v}
		if got := cfg.Threshold(); got != 200000 {
			t.Errorf("Threshold() with %d = %d, want 200000", v, got)
		}
		if got := cfg.SoftThreshold(); got != 150000 {
			t.Errorf("SoftThreshold() with %d = %d, want 150000", v, got)
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
		if got := cfg.PollInterval(); got != time.Second {
			t.Errorf("PollInterval() with %d = %v, want 1s", v, got)
		}
	}
}

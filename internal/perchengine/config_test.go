// config_test.go verifies perch.yaml's template parses, judge_model model-spec strings resolve
// through LoadConfig/LoadConfigWithRegistry, the old split-key (judge_effort) format fails loud,
// and the template-fallback path behaves the way reedengine's and shuttleengine's config tests
// establish the pattern.

package perchengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/perchengine"
)

// seedLyxConfig creates <tmpDir>/_lyx/config/<module>.yaml with content.
func seedLyxConfig(t *testing.T, tmpDir, module, content string) {
	t.Helper()
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx/config: %v", err)
	}
	configFile := configengine.ConfigFile(tmpDir, module)
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

// seedModelsYAML writes a models.yaml registry into tmpDir's _lyx/config.
func seedModelsYAML(t *testing.T, tmpDir, content string) {
	t.Helper()
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx/config: %v", err)
	}
	modelsFile := configengine.ConfigFile(tmpDir, "models")
	if err := os.WriteFile(modelsFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write models.yaml: %v", err)
	}
}

// (a) The template's default judge_model ("haiku") resolves to model "haiku" with an empty effort
// when no models.yaml is present — the built-in fallback registry carries no parameter defaults.
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

// (b) A seeded models.yaml giving sonnet a "defaults: {effort: medium}" entry makes "judge_model:
// sonnet" resolve to effort "medium".
func TestLoadConfig_JudgeModelResolvesRegistryDefaultEffort(t *testing.T) {
	tmpDir := t.TempDir()
	seedModelsYAML(t, tmpDir, "sonnet:\n  engine: claude\n  model: sonnet\n  defaults:\n    effort: medium\n")
	seedLyxConfig(t, tmpDir, "perch", "judge_model: sonnet\nround_caps: [5, 8, 10]\n")

	cfg, err := perchengine.LoadConfig(tmpDir, "perch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JudgeModel != "sonnet" {
		t.Errorf("JudgeModel = %q, want %q", cfg.JudgeModel, "sonnet")
	}
	if cfg.JudgeEffort != "medium" {
		t.Errorf("JudgeEffort = %q, want %q (registry default)", cfg.JudgeEffort, "medium")
	}
}

// (c) A bracket param beats the registry default for the same key.
func TestLoadConfig_JudgeModelBracketBeatsRegistryDefault(t *testing.T) {
	tmpDir := t.TempDir()
	seedModelsYAML(t, tmpDir, "sonnet:\n  engine: claude\n  model: sonnet\n  defaults:\n    effort: medium\n")
	seedLyxConfig(t, tmpDir, "perch", "judge_model: \"sonnet[effort=high]\"\nround_caps: [5, 8, 10]\n")

	cfg, err := perchengine.LoadConfig(tmpDir, "perch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JudgeEffort != "high" {
		t.Errorf("JudgeEffort = %q, want %q (bracket overrides registry default)", cfg.JudgeEffort, "high")
	}
}

// (d) A "version" bracket param fails loud, naming "version" — perch has no field to carry it into,
// so this is a perch-layer rejection, not a modelspec grammar error (version IS a known modelspec
// param).
func TestLoadConfig_JudgeModelVersionParamFailsLoud(t *testing.T) {
	tmpDir := t.TempDir()
	seedLyxConfig(t, tmpDir, "perch", "judge_model: \"sonnet[version=x]\"\nround_caps: [5, 8, 10]\n")

	_, err := perchengine.LoadConfig(tmpDir, "perch")
	if err == nil {
		t.Fatal("expected error for version bracket param, got nil")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("expected error naming %q, got: %v", "version", err)
	}
}

// (e) An old-format perch.yaml still carrying a judge_effort key fails strict validation loud —
// this is the migration's fail-loud enforcement point (configengine.Load's MissingKeys check alone
// would NOT catch this).
func TestLoadConfig_OldSplitKeyFileFailsLoud(t *testing.T) {
	tmpDir := t.TempDir()
	seedLyxConfig(t, tmpDir, "perch", "judge_model: haiku\njudge_effort: \"\"\nround_caps: [5, 8, 10]\n")

	_, err := perchengine.LoadConfig(tmpDir, "perch")
	if err == nil {
		t.Fatal("expected error for old split-key perch.yaml carrying judge_effort, got nil")
	}
	if !strings.Contains(err.Error(), "judge_effort") {
		t.Errorf("expected error naming %q, got: %v", "judge_effort", err)
	}
}

// (f) An unknown alias fails loud.
func TestLoadConfig_JudgeModelUnknownAliasFailsLoud(t *testing.T) {
	tmpDir := t.TempDir()
	seedLyxConfig(t, tmpDir, "perch", "judge_model: nonexistent-alias\nround_caps: [5, 8, 10]\n")

	_, err := perchengine.LoadConfig(tmpDir, "perch")
	if err == nil {
		t.Fatal("expected error for unknown alias, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent-alias") {
		t.Errorf("expected error naming %q, got: %v", "nonexistent-alias", err)
	}
}

func TestLoadConfig_ModuleArgIsThreadedThrough(t *testing.T) {
	tmpDir := t.TempDir()
	// Seed under a non-"perch" module name with a judge_model that differs
	// from the template's "haiku", so a hardcoded module name would be
	// caught either way: this module reads back its seeded model, and the
	// never-seeded "perch" module reads back the template default instead.
	seedLyxConfig(t, tmpDir, "otherperch", "judge_model: sonnet\nround_caps: [5, 8, 10]\n")

	cfg, err := perchengine.LoadConfig(tmpDir, "otherperch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JudgeModel != "sonnet" {
		t.Errorf("JudgeModel = %q, want %q (seeded value)", cfg.JudgeModel, "sonnet")
	}

	// The never-seeded "perch" module must fall back to the template
	// default, not the "otherperch" module's seeded value.
	defaultCfg, err := perchengine.LoadConfig(tmpDir, "perch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultCfg.JudgeModel != "haiku" {
		t.Errorf("JudgeModel = %q, want %q (template default)", defaultCfg.JudgeModel, "haiku")
	}
}

func TestLoadConfig_UninitializedFallsBackToTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/ -- LoadConfig must degrade to the embedded
	// template, resolving judge_model through the fallback registry
	// (builtins()) exactly as modelspec.LoadRegistry's own absent-file
	// fallback does.

	cfg, err := perchengine.LoadConfig(tmpDir, "perch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.JudgeModel != "haiku" {
		t.Errorf("JudgeModel = %q, want %q", cfg.JudgeModel, "haiku")
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

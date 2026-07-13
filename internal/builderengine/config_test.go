//go:build integration

// config_test.go verifies builder.yaml's template parses, defaults resolve
// through LoadConfig, overrides round-trip, and a malformed role model-spec
// fails loud naming the offending key — the perchengine config_test.go
// pattern, seeded via lyxtest.SeedConfig per the lyxtest Leaf Invariant.

package builderengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

func TestLoadConfig_TemplateDefaultsResolve(t *testing.T) {
	fixture := lyxtest.CopyWeft(t)
	// Seed the config file with the template itself: this is exactly the
	// file "lyx config reconcile" would produce, so LoadConfig must accept
	// it verbatim and every default must resolve.
	lyxtest.SeedConfig(t, fixture.WeftPath, map[string]string{
		"builder": builderengine.ConfigTemplate(),
	})

	cfg, err := builderengine.LoadConfig(fixture.WeftPath, "builder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Orchestrator != "sonnet" {
		t.Errorf("Orchestrator = %q, want %q", cfg.Orchestrator, "sonnet")
	}
	if cfg.Implementer != "sonnet" {
		t.Errorf("Implementer = %q, want %q", cfg.Implementer, "sonnet")
	}
	if cfg.ImplementerOversized != "sonnet" {
		t.Errorf("ImplementerOversized = %q, want %q", cfg.ImplementerOversized, "sonnet")
	}
	if cfg.Recovery != "opus[effort=high]" {
		t.Errorf("Recovery = %q, want %q", cfg.Recovery, "opus[effort=high]")
	}
	if cfg.SelfFixCap != 2 {
		t.Errorf("SelfFixCap = %d, want %d", cfg.SelfFixCap, 2)
	}
	if cfg.PollWaitS != 480 {
		t.Errorf("PollWaitS = %d, want %d", cfg.PollWaitS, 480)
	}
	if cfg.BatchTimeoutMin != 60 {
		t.Errorf("BatchTimeoutMin = %d, want %d", cfg.BatchTimeoutMin, 60)
	}
	if cfg.OrchestratorTimeoutMin != 480 {
		t.Errorf("OrchestratorTimeoutMin = %d, want %d", cfg.OrchestratorTimeoutMin, 480)
	}
	if cfg.BatchContextCapTokens != 100000 {
		t.Errorf("BatchContextCapTokens = %d, want %d", cfg.BatchContextCapTokens, 100000)
	}
	if cfg.BatchCardCap != 10 {
		t.Errorf("BatchCardCap = %d, want %d", cfg.BatchCardCap, 10)
	}
}

func TestLoadConfig_OverridesRoundTrip(t *testing.T) {
	fixture := lyxtest.CopyWeft(t)
	override := `orchestrator: opus[effort=high]
implementer: sonnet[effort=high]
implementer_oversized: claude:claude-sonnet-4-5[effort=high]
recovery: opus[effort=max]
self_fix_cap: 5
poll_wait_s: 60
batch_timeout_min: 30
orchestrator_timeout_min: 120
batch_context_cap_tokens: 50000
batch_card_cap: 6
`
	lyxtest.SeedConfig(t, fixture.WeftPath, map[string]string{
		"builder": override,
	})

	cfg, err := builderengine.LoadConfig(fixture.WeftPath, "builder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Orchestrator != "opus[effort=high]" {
		t.Errorf("Orchestrator = %q, want %q", cfg.Orchestrator, "opus[effort=high]")
	}
	if cfg.ImplementerOversized != "claude:claude-sonnet-4-5[effort=high]" {
		t.Errorf("ImplementerOversized = %q, want %q", cfg.ImplementerOversized, "claude:claude-sonnet-4-5[effort=high]")
	}
	if cfg.SelfFixCap != 5 {
		t.Errorf("SelfFixCap = %d, want %d", cfg.SelfFixCap, 5)
	}
	if cfg.BatchCardCap != 6 {
		t.Errorf("BatchCardCap = %d, want %d", cfg.BatchCardCap, 6)
	}
}

func TestLoadConfig_BadRoleGrammarNamesTheKey(t *testing.T) {
	fixture := lyxtest.CopyWeft(t)
	// "sonnet " has a trailing space — Parse rejects whitespace anywhere in
	// a spec string.
	badRole := `orchestrator: sonnet
implementer: "sonnet "
implementer_oversized: sonnet
recovery: opus[effort=high]
self_fix_cap: 2
poll_wait_s: 480
batch_timeout_min: 60
orchestrator_timeout_min: 480
batch_context_cap_tokens: 100000
batch_card_cap: 10
`
	lyxtest.SeedConfig(t, fixture.WeftPath, map[string]string{
		"builder": badRole,
	})

	_, err := builderengine.LoadConfig(fixture.WeftPath, "builder")
	if err == nil {
		t.Fatal("LoadConfig() = nil error; want error naming the offending key")
	}
	if !strings.Contains(err.Error(), "implementer") {
		t.Errorf("LoadConfig() error = %q; want it to name the offending key %q", err.Error(), "implementer")
	}
}

func TestLoadConfig_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	cfg, err := builderengine.LoadConfig(tmpDir, "builder")
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

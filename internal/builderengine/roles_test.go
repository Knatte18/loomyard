// roles_test.go exercises ResolveRoles's pre-flight: an unknown alias fails
// naming the offending role, an escape-form spec resolves with no registry
// entry at all, and a bracket param survives into the resolved Params map.

package builderengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
)

// baseValidConfig returns a Config whose four roles all resolve cleanly
// against reg, for tests that need to isolate a single role's failure.
func baseValidConfig() builderengine.Config {
	return builderengine.Config{
		Orchestrator:         "sonnet",
		Implementer:          "sonnet",
		ImplementerOversized: "sonnet",
		Recovery:             "opus[effort=high]",
	}
}

func TestResolveRoles_AllFourResolve(t *testing.T) {
	reg := modelspec.Registry{
		"sonnet": {Engine: "claude", Model: "sonnet"},
		"opus":   {Engine: "claude", Model: "opus", Defaults: map[string]string{"effort": "high"}},
	}

	resolved, err := builderengine.ResolveRoles(baseValidConfig(), reg)
	if err != nil {
		t.Fatalf("ResolveRoles() = _, %v; want nil error", err)
	}

	for _, role := range []builderengine.Role{
		builderengine.RoleOrchestrator,
		builderengine.RoleImplementer,
		builderengine.RoleImplementerOversized,
		builderengine.RoleRecovery,
	} {
		if _, ok := resolved[role]; !ok {
			t.Errorf("ResolveRoles() result missing role %q", role)
		}
	}

	if got := resolved[builderengine.RoleRecovery]; got.Engine != "claude" || got.Model != "opus" || got.Params["effort"] != "high" {
		t.Errorf("resolved[RoleRecovery] = %+v; want engine claude, model opus, effort high", got)
	}
}

func TestResolveRoles_UnknownAliasNamesTheRole(t *testing.T) {
	cfg := baseValidConfig()
	cfg.Implementer = "typo-alias"
	reg := modelspec.Registry{
		"sonnet": {Engine: "claude", Model: "sonnet"},
		"opus":   {Engine: "claude", Model: "opus"},
	}

	_, err := builderengine.ResolveRoles(cfg, reg)
	if err == nil {
		t.Fatal("ResolveRoles() = nil error; want error naming the offending role")
	}
	if !strings.Contains(err.Error(), string(builderengine.RoleImplementer)) {
		t.Errorf("ResolveRoles() error = %q; want it to name role %q", err.Error(), builderengine.RoleImplementer)
	}
	if !strings.Contains(err.Error(), "typo-alias") {
		t.Errorf("ResolveRoles() error = %q; want it to name the unknown alias %q", err.Error(), "typo-alias")
	}
}

func TestResolveRoles_EscapeFormNeedsNoRegistryEntry(t *testing.T) {
	// Every role is escape form here, since a nil registry never resolves
	// an alias — this isolates the one behavior under test: escape form
	// never consults the registry at all.
	cfg := builderengine.Config{
		Orchestrator:         "claude:sonnet",
		Implementer:          "claude:sonnet",
		ImplementerOversized: "claude:claude-sonnet-4-5[effort=high]",
		Recovery:             "claude:opus",
	}
	var reg modelspec.Registry

	resolved, err := builderengine.ResolveRoles(cfg, reg)
	if err != nil {
		t.Fatalf("ResolveRoles() = _, %v; want nil error (escape form needs no registry entry)", err)
	}

	got := resolved[builderengine.RoleImplementerOversized]
	if got.Engine != "claude" || got.Model != "claude-sonnet-4-5" {
		t.Errorf("resolved[RoleImplementerOversized] = %+v; want engine claude, model claude-sonnet-4-5", got)
	}
	if got.Params["effort"] != "high" {
		t.Errorf("resolved[RoleImplementerOversized].Params[\"effort\"] = %q; want %q", got.Params["effort"], "high")
	}
}

func TestResolveRoles_BracketParamsSurviveIntoResolved(t *testing.T) {
	cfg := baseValidConfig()
	cfg.Recovery = "opus[effort=max]"
	reg := modelspec.Registry{
		"sonnet": {Engine: "claude", Model: "sonnet"},
		"opus":   {Engine: "claude", Model: "opus", Defaults: map[string]string{"effort": "high"}},
	}

	resolved, err := builderengine.ResolveRoles(cfg, reg)
	if err != nil {
		t.Fatalf("ResolveRoles() = _, %v; want nil error", err)
	}

	// bracket param (max) overrides the registry default (high) per the
	// documented "bracket param > registry default" precedence.
	if got := resolved[builderengine.RoleRecovery].Params["effort"]; got != "max" {
		t.Errorf("resolved[RoleRecovery].Params[\"effort\"] = %q; want %q (bracket overrides registry default)", got, "max")
	}
}

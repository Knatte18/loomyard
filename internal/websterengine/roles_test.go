// roles_test.go exercises ResolveRoles's pre-flight: both roles resolve
// cleanly, an unknown alias fails naming the offending role, an escape-form
// spec resolves with no registry entry at all, and a bracket param survives
// into the resolved Params map — the builderengine/roles_test.go pattern,
// narrowed to webster's two roles (no oversized role exists here).

package websterengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// baseValidConfig returns a Config whose two roles both resolve cleanly
// against reg, for tests that need to isolate a single role's failure.
func baseValidConfig() websterengine.Config {
	return websterengine.Config{
		Master:   "sonnet",
		Recovery: "opus[effort=high]",
	}
}

func TestResolveRoles_BothRolesResolve(t *testing.T) {
	reg := modelspec.Registry{
		"sonnet": {Engine: "claude", Model: "sonnet"},
		"opus":   {Engine: "claude", Model: "opus", Defaults: map[string]string{"effort": "high"}},
	}

	resolved, err := websterengine.ResolveRoles(baseValidConfig(), reg)
	if err != nil {
		t.Fatalf("ResolveRoles() = _, %v; want nil error", err)
	}

	for _, role := range []websterengine.Role{
		websterengine.RoleMaster,
		websterengine.RoleRecovery,
	} {
		if _, ok := resolved[role]; !ok {
			t.Errorf("ResolveRoles() result missing role %q", role)
		}
	}

	if got := resolved[websterengine.RoleRecovery]; got.Engine != "claude" || got.Model != "opus" || got.Params["effort"] != "high" {
		t.Errorf("resolved[RoleRecovery] = %+v; want engine claude, model opus, effort high", got)
	}
}

func TestResolveRoles_NoOversizedRole(t *testing.T) {
	reg := modelspec.Registry{
		"sonnet": {Engine: "claude", Model: "sonnet"},
		"opus":   {Engine: "claude", Model: "opus"},
	}

	resolved, err := websterengine.ResolveRoles(baseValidConfig(), reg)
	if err != nil {
		t.Fatalf("ResolveRoles() = _, %v; want nil error", err)
	}
	if len(resolved) != 2 {
		t.Errorf("len(ResolveRoles()) = %d; want exactly 2 roles (master, recovery) — no oversized role", len(resolved))
	}
}

func TestResolveRoles_UnknownAliasNamesTheRole(t *testing.T) {
	cfg := baseValidConfig()
	cfg.Master = "typo-alias"
	reg := modelspec.Registry{
		"sonnet": {Engine: "claude", Model: "sonnet"},
		"opus":   {Engine: "claude", Model: "opus"},
	}

	_, err := websterengine.ResolveRoles(cfg, reg)
	if err == nil {
		t.Fatal("ResolveRoles() = nil error; want error naming the offending role")
	}
	if !strings.Contains(err.Error(), string(websterengine.RoleMaster)) {
		t.Errorf("ResolveRoles() error = %q; want it to name role %q", err.Error(), websterengine.RoleMaster)
	}
	if !strings.Contains(err.Error(), "typo-alias") {
		t.Errorf("ResolveRoles() error = %q; want it to name the unknown alias %q", err.Error(), "typo-alias")
	}
}

func TestResolveRoles_EscapeFormNeedsNoRegistryEntry(t *testing.T) {
	// Every role is escape form here, since a nil registry never resolves an
	// alias — this isolates the one behavior under test: escape form never
	// consults the registry at all.
	cfg := websterengine.Config{
		Master:   "claude:sonnet",
		Recovery: "claude:claude-sonnet-4-5[effort=high]",
	}
	var reg modelspec.Registry

	resolved, err := websterengine.ResolveRoles(cfg, reg)
	if err != nil {
		t.Fatalf("ResolveRoles() = _, %v; want nil error (escape form needs no registry entry)", err)
	}

	got := resolved[websterengine.RoleRecovery]
	if got.Engine != "claude" || got.Model != "claude-sonnet-4-5" {
		t.Errorf("resolved[RoleRecovery] = %+v; want engine claude, model claude-sonnet-4-5", got)
	}
	if got.Params["effort"] != "high" {
		t.Errorf("resolved[RoleRecovery].Params[\"effort\"] = %q; want %q", got.Params["effort"], "high")
	}
}

func TestResolveRoles_BracketParamsSurviveIntoResolved(t *testing.T) {
	cfg := baseValidConfig()
	cfg.Recovery = "opus[effort=max]"
	reg := modelspec.Registry{
		"sonnet": {Engine: "claude", Model: "sonnet"},
		"opus":   {Engine: "claude", Model: "opus", Defaults: map[string]string{"effort": "high"}},
	}

	resolved, err := websterengine.ResolveRoles(cfg, reg)
	if err != nil {
		t.Fatalf("ResolveRoles() = _, %v; want nil error", err)
	}

	// bracket param (max) overrides the registry default (high) per the
	// documented "bracket param > registry default" precedence.
	if got := resolved[websterengine.RoleRecovery].Params["effort"]; got != "max" {
		t.Errorf("resolved[RoleRecovery].Params[\"effort\"] = %q; want %q (bracket overrides registry default)", got, "max")
	}
}

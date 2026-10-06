// roles_test.go exercises ResolveRoles's pre-flight: both roles resolve cleanly, an unknown alias
// fails naming the offending role, an escape-form spec resolves with no registry entry at all, and
// a bracket param survives into the resolved Params map — scoped to webster's two roles
// (no oversized role exists here).

package websterengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

func TestResolveRoles(t *testing.T) {
	t.Parallel()

	sonnetOpusRegistry := modelspec.Registry{
		"sonnet": {Engine: "claude", Model: "sonnet"},
		"opus":   {Engine: "claude", Model: "opus", Defaults: map[string]string{"effort": "high"}},
	}
	tests := []struct {
		name string
		cfg  websterengine.Config
		reg  modelspec.Registry
		// wantErrParts, when non-empty, are the substrings the refusal must carry.
		wantErrParts []string
		wantModel    string
		wantEffort   string
	}{
		{
			name:       "both roles resolve and no oversized role exists",
			cfg:        websterengine.Config{Master: "sonnet", Recovery: "opus[effort=high]"},
			reg:        sonnetOpusRegistry,
			wantModel:  "opus",
			wantEffort: "high",
		},
		{
			name:         "an unknown alias fails naming the role and the alias",
			cfg:          websterengine.Config{Master: "typo-alias", Recovery: "opus[effort=high]"},
			reg:          sonnetOpusRegistry,
			wantErrParts: []string{string(websterengine.RoleMaster), "typo-alias"},
		},
		{
			// Every role is escape form here, since a nil registry never resolves an alias — this
			// isolates the one behavior under test: escape form never consults the registry at all.
			name:       "escape form needs no registry entry",
			cfg:        websterengine.Config{Master: "claude:sonnet", Recovery: "claude:claude-sonnet-4-5[effort=high]"},
			reg:        nil,
			wantModel:  "claude-sonnet-4-5",
			wantEffort: "high",
		},
		{
			// The bracket param (max) overrides the registry default (high) per the documented
			// "bracket param > registry default" precedence.
			name:       "a bracket param overrides the registry default",
			cfg:        websterengine.Config{Master: "sonnet", Recovery: "opus[effort=max]"},
			reg:        sonnetOpusRegistry,
			wantModel:  "opus",
			wantEffort: "max",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolved, err := websterengine.ResolveRoles(tt.cfg, tt.reg)
			if len(tt.wantErrParts) > 0 {
				if err == nil {
					t.Fatal("ResolveRoles() = nil error; want error naming the offending role")
				}
				for _, part := range tt.wantErrParts {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("ResolveRoles() error = %q; want it to name %q", err.Error(), part)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveRoles() = _, %v; want nil error", err)
			}
			for _, role := range []websterengine.Role{websterengine.RoleMaster, websterengine.RoleRecovery} {
				if _, ok := resolved[role]; !ok {
					t.Errorf("ResolveRoles() result missing role %q", role)
				}
			}
			if len(resolved) != 2 {
				t.Errorf("len(ResolveRoles()) = %d; want exactly 2 roles (master, recovery) — no oversized role", len(resolved))
			}
			got := resolved[websterengine.RoleRecovery]
			if got.Engine != "claude" || got.Model != tt.wantModel || got.Params["effort"] != tt.wantEffort {
				t.Errorf("resolved[RoleRecovery] = %+v; want engine claude, model %q, effort %q", got, tt.wantModel, tt.wantEffort)
			}
		})
	}
}

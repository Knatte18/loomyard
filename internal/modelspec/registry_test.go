// registry_test.go table-drives Registry.Resolve: bracket-over-default precedence, whole-entry lookups, unknown-alias/escape-form behaviour, input immutability, and the zero-value Registry's fail-clean shape.

package modelspec

import (
	"maps"
	"strings"
	"testing"
)

func TestRegistry_Resolve(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		registry   Registry
		spec       Spec
		wantEngine string
		wantModel  string
		wantParams map[string]string
	}{
		{
			name:       "registry default only",
			registry:   Registry{"sonnet": {Engine: "claude", Model: "sonnet", Defaults: map[string]string{"effort": "medium"}}},
			spec:       Spec{Alias: "sonnet"},
			wantEngine: "claude",
			wantModel:  "sonnet",
			wantParams: map[string]string{"effort": "medium"},
		},
		{
			name:       "bracket overrides default",
			registry:   Registry{"sonnet": {Engine: "claude", Model: "sonnet", Defaults: map[string]string{"effort": "medium"}}},
			spec:       Spec{Alias: "sonnet", Params: map[string]string{"effort": "high"}},
			wantEngine: "claude",
			wantModel:  "sonnet",
			wantParams: map[string]string{"effort": "high"},
		},
		{
			name:       "bracket adds param absent from defaults",
			registry:   Registry{"sonnet": {Engine: "claude", Model: "sonnet", Defaults: map[string]string{"effort": "medium"}}},
			spec:       Spec{Alias: "sonnet", Params: map[string]string{"version": "4.5"}},
			wantEngine: "claude",
			wantModel:  "sonnet",
			wantParams: map[string]string{"effort": "medium", "version": "4.5"},
		},
		{
			name:       "escape form bypasses registry",
			registry:   Registry{},
			spec:       Spec{Engine: "claude", Model: "claude-sonnet-4-5", Params: map[string]string{"effort": "high"}},
			wantEngine: "claude",
			wantModel:  "claude-sonnet-4-5",
			wantParams: map[string]string{"effort": "high"},
		},
		{
			name:       "escape form no params yields empty map",
			registry:   Registry{},
			spec:       Spec{Engine: "claude", Model: "claude-sonnet-4-5"},
			wantEngine: "claude",
			wantModel:  "claude-sonnet-4-5",
			wantParams: map[string]string{},
		},
		{
			name:       "built-in fallback resolves with zero defaults",
			registry:   builtins(),
			spec:       Spec{Alias: "haiku"},
			wantEngine: "claude",
			wantModel:  "haiku",
			wantParams: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			defaultsBefore := make(map[string]map[string]string, len(tt.registry))
			for alias, entry := range tt.registry {
				defaultsBefore[alias] = maps.Clone(entry.Defaults)
			}
			specParamsBefore := maps.Clone(tt.spec.Params)

			got, err := tt.registry.Resolve(tt.spec)
			if err != nil {
				t.Fatalf("Resolve(%+v) returned unexpected error: %v", tt.spec, err)
			}
			if got.Engine != tt.wantEngine || got.Model != tt.wantModel {
				t.Errorf("Resolve(%+v) = {Engine:%q Model:%q}; want {Engine:%q Model:%q}",
					tt.spec, got.Engine, got.Model, tt.wantEngine, tt.wantModel)
			}
			if got.Params == nil {
				t.Errorf("Resolve(%+v).Params = nil; want a non-nil map", tt.spec)
			}
			if len(got.Params) != len(tt.wantParams) {
				t.Errorf("Resolve(%+v).Params = %v; want %v", tt.spec, got.Params, tt.wantParams)
			}
			for k, v := range tt.wantParams {
				if got.Params[k] != v {
					t.Errorf("Resolve(%+v).Params[%q] = %q; want %q", tt.spec, k, got.Params[k], v)
				}
			}

			// The resolved Params must be a copy: writing through it never reaches the registry or the spec.
			got.Params["effort"] = "mutated"
			for k := range got.Params {
				got.Params[k] = "mutated"
			}
			for alias, entry := range tt.registry {
				if !maps.Equal(entry.Defaults, defaultsBefore[alias]) {
					t.Errorf("Resolve(%+v) mutated registry entry %q Defaults: got %v, want %v", tt.spec, alias, entry.Defaults, defaultsBefore[alias])
				}
			}
			if !maps.Equal(tt.spec.Params, specParamsBefore) {
				t.Errorf("Resolve mutated the input Spec's Params: got %v, want %v", tt.spec.Params, specParamsBefore)
			}
		})
	}
}

func TestRegistry_Resolve_UnknownAlias(t *testing.T) {
	t.Parallel()
	var zeroValueRegistry Registry // nil map, zero value
	tests := []struct {
		name      string
		registry  Registry
		alias     string
		wantNames []string
	}{
		{"unknown alias names it and lists the known ones", Registry{"sonnet": {Engine: "claude", Model: "sonnet"}}, "ghost", []string{`"ghost"`, "sonnet"}},
		{"zero-value registry fails clean", zeroValueRegistry, "sonnet", []string{"sonnet"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.registry.Resolve(Spec{Alias: tt.alias})
			if err == nil {
				t.Fatalf("Resolve(%q) returned nil error; want a clean error naming the alias, not a panic", tt.alias)
			}
			for _, want := range tt.wantNames {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Resolve(%q) error = %q; want it to contain %q", tt.alias, err.Error(), want)
				}
			}
		})
	}
}

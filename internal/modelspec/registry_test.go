// registry_test.go table-drives builtins and Registry.Resolve: bracket-over-
// default precedence, whole-entry lookups, unknown-alias/escape-form
// behaviour, and the zero-value Registry's fail-clean shape.

package modelspec

import (
	"strings"
	"testing"
)

func TestBuiltins(t *testing.T) {
	b := builtins()
	want := map[string]Entry{
		"sonnet": {Engine: "claude", Model: "sonnet"},
		"opus":   {Engine: "claude", Model: "opus"},
		"haiku":  {Engine: "claude", Model: "haiku"},
		"fable":  {Engine: "claude", Model: "fable"},
	}
	if len(b) != len(want) {
		t.Fatalf("builtins() has %d entries; want %d", len(b), len(want))
	}
	for alias, wantEntry := range want {
		gotEntry, ok := b[alias]
		if !ok {
			t.Errorf("builtins() missing alias %q", alias)
			continue
		}
		if gotEntry.Engine != wantEntry.Engine || gotEntry.Model != wantEntry.Model {
			t.Errorf("builtins()[%q] = %+v; want %+v", alias, gotEntry, wantEntry)
		}
		if len(gotEntry.Defaults) != 0 {
			t.Errorf("builtins()[%q].Defaults = %v; want none (built-ins carry no defaults)", alias, gotEntry.Defaults)
		}
	}
}

func TestRegistry_Resolve(t *testing.T) {
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
		})
	}
}

func TestRegistry_Resolve_UnknownAlias(t *testing.T) {
	r := Registry{"sonnet": {Engine: "claude", Model: "sonnet"}}
	_, err := r.Resolve(Spec{Alias: "ghost"})
	if err == nil {
		t.Fatal("Resolve(unknown alias) returned nil error; want an error naming the alias")
	}
	if !strings.Contains(err.Error(), `"ghost"`) {
		t.Errorf("Resolve(unknown alias) error = %q; want it to name the alias %q", err.Error(), "ghost")
	}
	if !strings.Contains(err.Error(), "sonnet") {
		t.Errorf("Resolve(unknown alias) error = %q; want it to list the known alias %q", err.Error(), "sonnet")
	}
}

func TestRegistry_Resolve_ZeroValueRegistry(t *testing.T) {
	var r Registry // nil map, zero value
	_, err := r.Resolve(Spec{Alias: "sonnet"})
	if err == nil {
		t.Fatal("zero-value Registry.Resolve(alias) returned nil error; want a clean error, not a panic")
	}
	if !strings.Contains(err.Error(), "sonnet") {
		t.Errorf("zero-value Registry.Resolve error = %q; want it to name the alias %q", err.Error(), "sonnet")
	}
}

func TestRegistry_Resolve_NeverMutatesInputs(t *testing.T) {
	entry := Entry{Engine: "claude", Model: "sonnet", Defaults: map[string]string{"effort": "medium"}}
	r := Registry{"sonnet": entry}
	specParams := map[string]string{"version": "4.5"}
	spec := Spec{Alias: "sonnet", Params: specParams}

	got, err := r.Resolve(spec)
	if err != nil {
		t.Fatalf("Resolve returned unexpected error: %v", err)
	}
	got.Params["effort"] = "mutated"

	if r["sonnet"].Defaults["effort"] != "medium" {
		t.Errorf("Resolve mutated the registry entry's Defaults: got %q, want %q", r["sonnet"].Defaults["effort"], "medium")
	}
	if len(specParams) != 1 || specParams["version"] != "4.5" {
		t.Errorf("Resolve mutated the input Spec's Params: got %v", specParams)
	}
}

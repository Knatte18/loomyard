// runsettings_test.go — tests for the run-settings migrate-then-validate step.

package configreg

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// TestPrepareRunSettings pins PrepareRunSettings over the real registry: the documents it returns and the refusals that name the module and key.
func TestPrepareRunSettings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		settings     map[string]any
		wantDocs     map[string]string
		wantRefusal  string
		wantModule   string
		wantInvalid  bool
		wantNoModule string
	}{
		{
			name:     "whole number becomes a YAML integer",
			settings: map[string]any{"loom": map[string]any{"review_max_bounces": float64(2)}},
			wantDocs: map[string]string{"loom": "review_max_bounces: 2\n"},
		},
		{
			name:     "a soft and a hard review cap are both settable through run_settings",
			settings: map[string]any{"loom": map[string]any{"review_max_bounces": float64(2), "review_hard_max_bounces": float64(3)}},
			wantDocs: map[string]string{"loom": "review_hard_max_bounces: 3\nreview_max_bounces: 2\n"},
		},
		{
			name:     "a zero per-perch budget keeps its zero-rounds meaning through loom's Migrate",
			settings: map[string]any{"loom": map[string]any{"plan_max_bounces": float64(0)}},
			wantDocs: map[string]string{"loom": "plan_max_bounces: 0\n"},
		},
		{
			name:        "a review hard cap of zero is refused by loom's CheckValues",
			settings:    map[string]any{"loom": map[string]any{"review_hard_max_bounces": float64(0)}},
			wantRefusal: "review_hard_max_bounces",
			wantModule:  "loom",
			wantInvalid: true,
		},
		{
			name:     "below-floor step_idle_timeout_min is raised by loom's Migrate",
			settings: map[string]any{"loom": map[string]any{"step_idle_timeout_min": float64(0)}},
			wantDocs: map[string]string{"loom": "step_idle_timeout_min: 1\n"},
		},
		{
			name:     "models alias the template lacks passes",
			settings: map[string]any{"models": map[string]any{"zephyr": map[string]any{"engine": "claude", "model": "claude-zephyr-1"}}},
			wantDocs: map[string]string{"models": "zephyr:\n    engine: claude\n    model: claude-zephyr-1\n"},
		},
		{
			name:     "nested key of a template alias passes",
			settings: map[string]any{"models": map[string]any{"opus": map[string]any{"defaults": map[string]any{"version": "5"}}}},
			wantDocs: map[string]string{"models": "opus:\n    defaults:\n        version: \"5\"\n"},
		},
		{
			name:     "role under an open map passes",
			settings: map[string]any{"shuttle": map[string]any{"claude_prompt_cache_ttl_roles": map[string]any{"worker": "1h"}}},
			wantDocs: map[string]string{"shuttle": "claude_prompt_cache_ttl_roles:\n    worker: 1h\n"},
		},
		{
			name:         "empty module object is dropped",
			settings:     map[string]any{"loom": map[string]any{}},
			wantDocs:     map[string]string{},
			wantNoModule: "loom",
		},
		{name: "nil settings", settings: nil, wantDocs: map[string]string{}},
		{
			name:        "unknown module",
			settings:    map[string]any{"nope": map[string]any{"a": 1}},
			wantRefusal: "nope",
			wantModule:  "nope",
			wantInvalid: true,
		},
		{
			name:        "non-object module value",
			settings:    map[string]any{"loom": "fast"},
			wantRefusal: "loom",
			wantModule:  "loom",
			wantInvalid: true,
		},
		{
			name:        "key the template lacks outside any open map",
			settings:    map[string]any{"loom": map[string]any{"no_such_key": float64(1)}},
			wantRefusal: "no_such_key",
			wantModule:  "loom",
			wantInvalid: true,
		},
		{
			name:        "mapping where the template holds a scalar",
			settings:    map[string]any{"loom": map[string]any{"plan": map[string]any{"a": "b"}}},
			wantRefusal: "plan",
			wantModule:  "loom",
			wantInvalid: true,
		},
		{
			name:        "D4-closed key of a hub-wide module",
			settings:    map[string]any{"darn": map[string]any{"review_max_bounces": float64(0)}},
			wantRefusal: "review_max_bounces",
			wantModule:  "darn",
			wantInvalid: true,
		},
		{
			name:        "D4-closed key of a per-worktree module",
			settings:    map[string]any{"loom": map[string]any{"parent_review_wait_min": float64(0)}},
			wantRefusal: "parent_review_wait_min",
			wantModule:  "loom",
			wantInvalid: true,
		},
		{
			name:        "guard key skip_alignment is closed to run settings",
			settings:    map[string]any{"loom": map[string]any{"skip_alignment": true}},
			wantRefusal: "skip_alignment",
			wantModule:  "loom",
			wantInvalid: true,
		},
		{
			name:     "alignment_timeout_min is open to run settings",
			settings: map[string]any{"loom": map[string]any{"alignment_timeout_min": float64(10)}},
			wantDocs: map[string]string{"loom": "alignment_timeout_min: 10\n"},
		},
		{
			name:        "driver_kind outside llm and go is refused by loom's CheckValues",
			settings:    map[string]any{"loom": map[string]any{"driver_kind": "fast"}},
			wantRefusal: "driver_kind",
			wantModule:  "loom",
			wantInvalid: true,
		},
		{
			name:     "driver_kind go is open to run settings",
			settings: map[string]any{"loom": map[string]any{"driver_kind": "go"}},
			wantDocs: map[string]string{"loom": "driver_kind: go\n"},
		},
		{
			name:        "negative tackler_max_spawns is refused by loom's CheckValues",
			settings:    map[string]any{"loom": map[string]any{"tackler_max_spawns": float64(-1)}},
			wantRefusal: "tackler_max_spawns",
			wantModule:  "loom",
			wantInvalid: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := PrepareRunSettings(tt.settings)
			if tt.wantRefusal != "" {
				var refusal *RunSettingsError
				if !errors.As(err, &refusal) {
					t.Fatalf("PrepareRunSettings error = %v; want a *RunSettingsError", err)
				}
				if refusal.Module != tt.wantModule {
					t.Errorf("RunSettingsError.Module = %q; want %q", refusal.Module, tt.wantModule)
				}
				if !strings.Contains(err.Error(), tt.wantModule) || !strings.Contains(err.Error(), tt.wantRefusal) {
					t.Errorf("error = %q; want it to name %q and %q", err, tt.wantModule, tt.wantRefusal)
				}
				if errors.Is(err, configengine.ErrInvalid) != tt.wantInvalid {
					t.Errorf("errors.Is(err, ErrInvalid) = %v; want %v", !tt.wantInvalid, tt.wantInvalid)
				}
				return
			}
			if err != nil {
				t.Fatalf("PrepareRunSettings error = %v; want none", err)
			}
			if len(got) != len(tt.wantDocs) {
				t.Fatalf("PrepareRunSettings returned %d modules; want %d", len(got), len(tt.wantDocs))
			}
			for module, want := range tt.wantDocs {
				if string(got[module]) != want {
					t.Errorf("document for %s = %q; want %q", module, got[module], want)
				}
			}
			if _, present := got[tt.wantNoModule]; tt.wantNoModule != "" && present {
				t.Errorf("document for %s present; want it dropped", tt.wantNoModule)
			}
		})
	}
}

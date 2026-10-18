// runsettings_test.go — tests for the run-settings migrate-then-validate step.

package configreg

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/runprofile"
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
			name:         "role under an open map moves from shuttle to claude",
			settings:     map[string]any{"shuttle": map[string]any{"claude_prompt_cache_ttl_roles": map[string]any{"worker": "1h"}}},
			wantDocs:     map[string]string{"claude": "prompt_cache_ttl_roles:\n    worker: 1h\n"},
			wantNoModule: "shuttle",
		},
		{
			name:         "a moved key yields to the value claude already holds",
			settings:     map[string]any{"shuttle": map[string]any{"claude_prompt_cache_ttl": "1h"}, "claude": map[string]any{"prompt_cache_ttl": "5m"}},
			wantDocs:     map[string]string{"claude": "prompt_cache_ttl: 5m\n"},
			wantNoModule: "shuttle",
		},
		{
			name:        "non-object shuttle value is still refused",
			settings:    map[string]any{"shuttle": "fast"},
			wantRefusal: "shuttle",
			wantModule:  "shuttle",
			wantInvalid: true,
		},
		{
			name:        "the legacy python switch is renamed by shuttle's Migrate and refused as a guard key",
			settings:    map[string]any{"shuttle": map[string]any{"claude_deny_python": true}},
			wantRefusal: "deny_python",
			wantModule:  "shuttle",
			wantInvalid: true,
		},
		{
			name:        "a retired loom fan key is renamed by loom's Migrate and refused as a guard key",
			settings:    map[string]any{"loom": map[string]any{"discussion_fan": "standard"}},
			wantRefusal: "discussion_reviewer_fan",
			wantModule:  "loom",
			wantInvalid: true,
		},
		{
			name:     "a retired webster model key is renamed by loom's Migrate and copied onto merge",
			settings: map[string]any{"loom": map[string]any{"webster_review": "sonnet[high]"}},
			wantDocs: map[string]string{"loom": "webster_reviewer_model: sonnet[high] # model-spec for the Webster-Review reviewer; empty takes reviewer_model, a set value (one model-spec, or a per-round list whose last entry serves every later round) replaces it for this segment\nmerge_reviewer_model: sonnet[high] # model-spec for the Merge-Review reviewer; empty takes reviewer_model, a set value (one model-spec, or a per-round list whose last entry serves every later round) replaces it for this segment\n"},
		},
		{
			name:        "a retired darn fan key is renamed by darn's Migrate and refused as a guard key",
			settings:    map[string]any{"darn": map[string]any{"review_fix_fan": "fix-code"}},
			wantRefusal: "darn_fixer_fan",
			wantModule:  "darn",
			wantInvalid: true,
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
			name:     "darn's review budget of zero is open to run settings",
			settings: map[string]any{"darn": map[string]any{"review_max_bounces": float64(0)}},
			wantDocs: map[string]string{"darn": "review_max_bounces: 0\n"},
		},
		{
			name:     "darn's review budget and hard cap are both settable through run_settings",
			settings: map[string]any{"darn": map[string]any{"review_max_bounces": float64(2), "review_hard_max_bounces": float64(3)}},
			wantDocs: map[string]string{"darn": "review_hard_max_bounces: 3\nreview_max_bounces: 2\n"},
		},
		{
			name:        "darn's review hard cap of zero is refused by darn's CheckValues",
			settings:    map[string]any{"darn": map[string]any{"review_hard_max_bounces": float64(0)}},
			wantRefusal: "review_hard_max_bounces",
			wantModule:  "darn",
			wantInvalid: true,
		},
		{
			name:        "D4-closed key of a hub-wide module",
			settings:    map[string]any{"darn": map[string]any{"verify": "true"}},
			wantRefusal: "verify",
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
		{
			name:     "a loom tackler_max_spawns of 4 survives loom's Migrate as 4",
			settings: map[string]any{"loom": map[string]any{"tackler_max_spawns": float64(4)}},
			wantDocs: map[string]string{"loom": "tackler_max_spawns: 4\n"},
		},
		{
			name:     "darn's tackler_max_spawns of 0 is open to run settings",
			settings: map[string]any{"darn": map[string]any{"tackler_max_spawns": float64(0)}},
			wantDocs: map[string]string{"darn": "tackler_max_spawns: 0\n"},
		},
		{
			name:        "darn's negative tackler_max_spawns is refused by darn's CheckValues",
			settings:    map[string]any{"darn": map[string]any{"tackler_max_spawns": float64(-1)}},
			wantRefusal: "tackler_max_spawns",
			wantModule:  "darn",
			wantInvalid: true,
		},
		{
			name:        "a batten driver_kind is refused naming the key",
			settings:    map[string]any{"batten": map[string]any{"driver_kind": "go"}},
			wantRefusal: "driver_kind",
			wantModule:  "batten",
			wantInvalid: true,
		},
		{
			name:     "a batten open_ide switch on is open to run settings",
			settings: map[string]any{"batten": map[string]any{"open_ide": true}},
			wantDocs: map[string]string{"batten": "open_ide: true\n"},
		},
		{
			name:     "a batten open_ide switch off is open to run settings",
			settings: map[string]any{"batten": map[string]any{"open_ide": false}},
			wantDocs: map[string]string{"batten": "open_ide: false\n"},
		},
		{
			name:        "a batten open_ide_command is refused by the local layer's executable category",
			settings:    map[string]any{"batten": map[string]any{"open_ide_command": "x"}},
			wantRefusal: "open_ide_command",
			wantModule:  "batten",
			wantInvalid: true,
		},
		{
			name:     "shed's tackler_max_spawns is open to run settings",
			settings: map[string]any{"shed": map[string]any{"tackler_max_spawns": float64(0)}},
			wantDocs: map[string]string{"shed": "tackler_max_spawns: 0\n"},
		},
		{
			name:        "negative tackler_max_spawns is refused by shed's CheckValues",
			settings:    map[string]any{"shed": map[string]any{"tackler_max_spawns": float64(-1)}},
			wantRefusal: "tackler_max_spawns",
			wantModule:  "shed",
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

// TestLayerRunSettings pins how a task's run settings lie over its run profile's: per key, per module, and with each side's refusal naming its source.
func TestLayerRunSettings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		profile     map[string]any
		settings    map[string]any
		wantDocs    map[string]string
		wantRefusal []string
	}{
		{
			name:     "a task key replaces the profile's and a profile key the task lacks stays",
			profile:  map[string]any{"loom": map[string]any{"plan_max_bounces": float64(1), "merge_max_bounces": float64(1)}},
			settings: map[string]any{"loom": map[string]any{"plan_max_bounces": float64(3)}},
			wantDocs: map[string]string{"loom": "merge_max_bounces: 1\nplan_max_bounces: 3\n"},
		},
		{
			name:     "a module only one side names is written as that side has it",
			profile:  map[string]any{"darn": map[string]any{"review_max_bounces": float64(0)}},
			settings: map[string]any{"loom": map[string]any{"plan_max_bounces": float64(2)}},
			wantDocs: map[string]string{"darn": "review_max_bounces: 0\n", "loom": "plan_max_bounces: 2\n"},
		},
		{
			name:     "no task settings writes the profile alone",
			profile:  map[string]any{"darn": map[string]any{"writer": "sonnet[medium]"}},
			wantDocs: map[string]string{"darn": "writer: sonnet[medium]\n"},
		},
		{
			name:        "a refused profile key names the profile, the module and the key",
			profile:     map[string]any{"loom": map[string]any{"no_such_key": float64(1)}},
			wantRefusal: []string{`run profile "light"`, "loom", "no_such_key"},
		},
		{
			name:        "a profile's batten open_ide_command is refused by the local layer's executable category",
			profile:     map[string]any{"batten": map[string]any{"open_ide_command": "x"}},
			wantRefusal: []string{`run profile "light"`, "batten", "open_ide_command"},
		},
		{
			name:        "a refused task key names the module and the key",
			profile:     map[string]any{"loom": map[string]any{"plan_max_bounces": float64(1)}},
			settings:    map[string]any{"loom": map[string]any{"discussion_lenses": []any{}}},
			wantRefusal: []string{"loom", "discussion_lenses"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := LayerRunSettings("light", tt.profile, tt.settings)
			if tt.wantRefusal != nil {
				var refusal *RunSettingsError
				if !errors.As(err, &refusal) {
					t.Fatalf("LayerRunSettings error = %v; want a *RunSettingsError", err)
				}
				for _, want := range tt.wantRefusal {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error = %q; want it to name %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("LayerRunSettings error = %v; want none", err)
			}
			if len(got) != len(tt.wantDocs) {
				t.Fatalf("LayerRunSettings returned %v; want %d modules", got, len(tt.wantDocs))
			}
			for module, want := range tt.wantDocs {
				if string(got[module]) != want {
					t.Errorf("document for %s = %q; want %q", module, got[module], want)
				}
			}
		})
	}
}

// TestProfilesCheckLoad pins the profiles module's load check: the template's starting profiles pass, and a hub file profile with a key no module takes is refused naming the profile.
func TestProfilesCheckLoad(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		hubFile     string
		wantRefusal []string
	}{
		{name: "the template's starting profiles pass"},
		{name: "a valid hub file passes", hubFile: "light:\n  loom: {plan_max_bounces: 1}\n"},
		{name: "an unknown key is refused naming the profile", hubFile: "light:\n  loom: {plan_max_bounces: 1}\nodd:\n  loom: {no_such_key: 1}\n", wantRefusal: []string{`run profile "odd"`, "no_such_key"}},
		{name: "an unknown module is refused naming the profile", hubFile: "odd:\n  bogus: {x: 1}\n", wantRefusal: []string{`run profile "odd"`, "bogus"}},
	}
	module, _ := Lookup(runprofile.ModuleName)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := configengine.Dirs{}
			if tt.hubFile != "" {
				dirs.Board = t.TempDir()
				if err := os.MkdirAll(configengine.ConfigDir(dirs.Board), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(configengine.ConfigFile(dirs.Board, runprofile.ModuleName), []byte(tt.hubFile), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			err := module.CheckLoad(dirs)
			if tt.wantRefusal == nil {
				if err != nil {
					t.Fatalf("CheckLoad error = %v; want none", err)
				}
				return
			}
			if err == nil {
				t.Fatal("CheckLoad error = nil; want a refusal")
			}
			for _, want := range tt.wantRefusal {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q; want it to name %q", err, want)
				}
			}
		})
	}
}

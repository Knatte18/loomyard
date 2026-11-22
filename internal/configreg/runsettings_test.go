// runsettings_test.go — tests for the run-settings validation step.

package configreg

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/runprofile"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPrepareRunSettings pins PrepareRunSettings over the real registry:
// the documents it returns and the refusals that name the module and key.
func TestPrepareRunSettings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name         string
		settings     map[string]any
		wantDocs     map[string]string
		wantRefusal  string
		wantModule   string
		wantInvalid  bool
		wantNoModule string
		// wantWayForwardOf names the retired key whose RetiredWayForward text the refusal must end with.
		wantWayForwardOf string
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
			name:     "a tapestry plan_max_bounces keeps its zero-rounds meaning",
			settings: map[string]any{"tapestry": map[string]any{"plan_max_bounces": float64(0)}},
			wantDocs: map[string]string{"tapestry": "plan_max_bounces: 0\n"},
		},
		{
			name:             "a loom plan_max_bounces is refused with the tapestry way forward, since it lives in tapestry's config",
			settings:         map[string]any{"loom": map[string]any{"plan_max_bounces": float64(0)}},
			wantRefusal:      "plan_max_bounces",
			wantModule:       "loom",
			wantInvalid:      true,
			wantWayForwardOf: "plan_max_bounces",
		},
		{
			name:        "a review hard cap of zero is refused by loom's CheckValues",
			settings:    map[string]any{"loom": map[string]any{"review_hard_max_bounces": float64(0)}},
			wantRefusal: "review_hard_max_bounces",
			wantModule:  "loom",
			wantInvalid: true,
		},
		{
			name:        "a beam pull_request the lookup refuses is refused at the run settings",
			settings:    map[string]any{"beam": map[string]any{"pull_request": "required"}},
			wantRefusal: "pull_request",
			wantModule:  "beam",
			wantInvalid: true,
		},
		{
			name:        "a beam land_notice the lookup refuses is refused at the run settings",
			settings:    map[string]any{"beam": map[string]any{"land_notice": "per-task"}},
			wantRefusal: "land_notice",
			wantModule:  "beam",
			wantInvalid: true,
		},
		{
			name:     "a beam autonomy boolean is admitted into a beam local document",
			settings: map[string]any{"beam": map[string]any{"autonomy": true}},
			wantDocs: map[string]string{"beam": "autonomy: true\n"},
		},
		{
			name:        "a beam autonomy the lookup refuses is refused at the run settings",
			settings:    map[string]any{"beam": map[string]any{"autonomy": "yes"}},
			wantRefusal: "autonomy",
			wantModule:  "beam",
			wantInvalid: true,
		},
		{
			name:     "hardener packages and llm_driving pass",
			settings: map[string]any{"hardener": map[string]any{"packages": []any{"internal/foo"}, "llm_driving": true}},
			wantDocs: map[string]string{"hardener": "llm_driving: true\npackages:\n    - internal/foo\n"},
		},
		{
			name:        "hardener auto_accept is refused naming the hub file write",
			settings:    map[string]any{"hardener": map[string]any{"auto_accept": true}},
			wantRefusal: `lym config hardener --set auto_accept=<value>`,
			wantModule:  "hardener",
			wantInvalid: true,
		},
		{
			name:        "hardener concurrency below one is refused naming the key",
			settings:    map[string]any{"hardener": map[string]any{"concurrency": float64(0)}},
			wantRefusal: `"concurrency"`,
			wantModule:  "hardener",
			wantInvalid: true,
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
			name:        "non-object shuttle value is refused",
			settings:    map[string]any{"shuttle": "fast"},
			wantRefusal: "shuttle",
			wantModule:  "shuttle",
			wantInvalid: true,
		},
		{
			name:        "the old python switch name is refused naming the key",
			settings:    map[string]any{"shuttle": map[string]any{"claude_deny_python": true}},
			wantRefusal: "claude_deny_python",
			wantModule:  "shuttle",
			wantInvalid: true,
		},
		{
			name:        "a retired shuttle claude binary key is refused as an executable key",
			settings:    map[string]any{"shuttle": map[string]any{"claude": "/x"}},
			wantRefusal: `"claude"`,
			wantModule:  "shuttle",
			wantInvalid: true,
		},
		{
			name:        "a retired loom fan key is refused naming the key",
			settings:    map[string]any{"loom": map[string]any{"discussion_fan": "standard"}},
			wantRefusal: "discussion_fan",
			wantModule:  "loom",
			wantInvalid: true,
		},
		{
			name:        "a retired darn fan key is refused naming the key",
			settings:    map[string]any{"darn": map[string]any{"review_fix_fan": "fix-code"}},
			wantRefusal: "review_fix_fan",
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
			name:        "a module that is not registered is refused, even one that held keys once",
			settings:    map[string]any{"burler": map[string]any{"lenses": map[string]any{"generic": map[string]any{"prose": "x"}}}},
			wantRefusal: "not registered",
			wantModule:  "burler",
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
			settings:    map[string]any{"loom": map[string]any{"discussion": map[string]any{"a": "b"}}},
			wantRefusal: "discussion",
			wantModule:  "loom",
			wantInvalid: true,
		},
		{
			name:     "a solo code_reviewer_fan is admitted as a profile block or run_settings carries it",
			settings: map[string]any{"loom": map[string]any{"code_reviewer_fan": ""}},
			wantDocs: map[string]string{"loom": "code_reviewer_fan: \"\"\n"},
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
			settings:    map[string]any{"loom": map[string]any{"discussion_advisors": []any{}}},
			wantRefusal: "discussion_advisors",
			wantModule:  "loom",
			wantInvalid: true,
		},
		{
			name:     "a task's beam pull_request and land_notice prepare into a beam local document",
			settings: map[string]any{"beam": map[string]any{"pull_request": "merge-in", "land_notice": "on"}},
			wantDocs: map[string]string{"beam": "land_notice: \"on\"\npull_request: merge-in\n"},
		},
		{
			name:     "a task's retired drift skip and verify_max_runs are admitted as retired keys of category none",
			settings: map[string]any{"drift": map[string]any{"skip": []any{"t"}, "verify_max_runs": float64(3)}},
			wantDocs: map[string]string{"drift": "skip:\n    - t\nverify_max_runs: 3\n"},
		},
		{
			name:             "a loom driver_kind is retired and refused with the shed way forward",
			settings:         map[string]any{"loom": map[string]any{"driver_kind": "go"}},
			wantRefusal:      "driver_kind",
			wantModule:       "loom",
			wantInvalid:      true,
			wantWayForwardOf: "driver_kind",
		},
		{
			name:             "a darn arbiter is retired and refused with the shed way forward",
			settings:         map[string]any{"darn": map[string]any{"arbiter": "opus"}},
			wantRefusal:      "arbiter",
			wantModule:       "darn",
			wantInvalid:      true,
			wantWayForwardOf: "arbiter",
		},
		{
			name:        "a drift diff is refused naming the hub's drift.yaml",
			settings:    map[string]any{"drift": map[string]any{"diff": "command"}},
			wantRefusal: "key \"diff\" of module \"drift\" is set only in the hub's config; way forward: set it in the hub's drift.yaml",
			wantModule:  "drift",
			wantInvalid: true,
		},
		{
			name:        "a drift key_patterns is refused naming the hub's drift.yaml",
			settings:    map[string]any{"drift": map[string]any{"key_patterns": []any{}}},
			wantRefusal: "key \"key_patterns\" of module \"drift\" is set only in the hub's config; way forward: set it in the hub's drift.yaml",
			wantModule:  "drift",
			wantInvalid: true,
		},
		{
			name:        "a drift volatile_files is refused naming the hub's drift.yaml",
			settings:    map[string]any{"drift": map[string]any{"volatile_files": []any{"_source.txt"}}},
			wantRefusal: "key \"volatile_files\" of module \"drift\" is set only in the hub's config; way forward: set it in the hub's drift.yaml",
			wantModule:  "drift",
			wantInvalid: true,
		},
		{
			name:        "a drift volatile_keys is refused naming the hub's drift.yaml",
			settings:    map[string]any{"drift": map[string]any{"volatile_keys": []any{"captured_at"}}},
			wantRefusal: "key \"volatile_keys\" of module \"drift\" is set only in the hub's config; way forward: set it in the hub's drift.yaml",
			wantModule:  "drift",
			wantInvalid: true,
		},
		{
			name:     "shed's driver_kind go is open to run settings",
			settings: map[string]any{"shed": map[string]any{"driver_kind": "go"}},
			wantDocs: map[string]string{"shed": "driver_kind: go\n"},
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
				if tt.wantWayForwardOf != "" {
					module, _ := Lookup(tt.wantModule)
					if want := module.Spec().RetiredWayForward[tt.wantWayForwardOf]; want == "" || !strings.HasSuffix(err.Error(), want) {
						t.Errorf("error = %q; want it to end with the module Spec's RetiredWayForward text %q", err, want)
					}
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

// TestLayerRunSettings pins how a task's run settings lie over its run profile's:
// per key, per module, and with each side's refusal naming its source.
func TestLayerRunSettings(t *testing.T) {
	tiergate.Require(t, 0)
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
			profile:  map[string]any{"tapestry": map[string]any{"plan_max_bounces": float64(1)}, "loom": map[string]any{"code_max_bounces": float64(1)}},
			settings: map[string]any{"tapestry": map[string]any{"plan_max_bounces": float64(3)}},
			wantDocs: map[string]string{"loom": "code_max_bounces: 1\n", "tapestry": "plan_max_bounces: 3\n"},
		},
		{
			name:     "a module only one side names is written as that side has it",
			profile:  map[string]any{"darn": map[string]any{"verify_attempts": float64(2)}},
			settings: map[string]any{"tapestry": map[string]any{"plan_max_bounces": float64(2)}},
			wantDocs: map[string]string{"darn": "verify_attempts: 2\n", "tapestry": "plan_max_bounces: 2\n"},
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
			profile:     map[string]any{"tapestry": map[string]any{"plan_max_bounces": float64(1)}},
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

// TestProfilesCheckLoad pins the profiles module's load check: the template's starting profiles pass,
// and a hub file profile with a key no module takes is refused naming the profile.
func TestProfilesCheckLoad(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name        string
		hubFile     string
		wantRefusal []string
	}{
		{name: "the template's starting profiles pass"},
		{name: "a valid hub file passes", hubFile: "light:\n  tapestry:\n    tapestry: {plan_max_bounces: 1}\n  darn:\n    darn: {writer: opus}\n"},
		{name: "a plan key under loom is refused naming the profile and the recipe", hubFile: "odd:\n  tapestry:\n    loom: {plan_max_bounces: 1}\n", wantRefusal: []string{`run profile "odd", recipe "tapestry"`, "plan_max_bounces"}},
		{name: "an unknown key is refused naming the profile and the recipe", hubFile: "light:\n  tapestry:\n    tapestry: {plan_max_bounces: 1}\nodd:\n  tapestry:\n    tapestry: {plan_max_bounces: 1}\n  darn:\n    loom: {no_such_key: 1}\n", wantRefusal: []string{`run profile "odd", recipe "darn"`, "no_such_key"}},
		{name: "an unknown module is refused naming the profile and the recipe", hubFile: "odd:\n  tapestry:\n    bogus: {x: 1}\n", wantRefusal: []string{`run profile "odd", recipe "tapestry"`, "bogus"}},
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

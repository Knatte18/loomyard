//go:build tier0

// runsettings_test.go — tests for the run-settings migrate-then-validate step.

package configreg

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/runprofile"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// renamedCodeMaxBounces is the line loom's Migrate writes for a retired webster_max_bounces of 2, carrying the comment the loom template holds on code_max_bounces.
const renamedCodeMaxBounces = "code_max_bounces: 2 # bounce budget of the Code-Review perch: N gives N judged Burler rounds, 1 by default; null takes review_max_bounces; 0 runs no review round: the perch's Bouncer settles approved at once; a checkpoint above this budget means the perch never rules CIRCLING and reaches the soft-cap rule, or the accept, instead\n"

// TestPrepareRunSettings pins PrepareRunSettings over the real registry: the documents it returns and the refusals that name the module and key.
// It is not parallel: its Warn subtest reads the process-global logger, which a parallel top-level test would also write to.
func TestPrepareRunSettings(t *testing.T) {
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
			name:     "a retired webster model key is renamed to its code key by loom's Migrate",
			settings: map[string]any{"loom": map[string]any{"webster_review": "sonnet[high]"}},
			wantDocs: map[string]string{"loom": "code_reviewer_model: sonnet[high] # model-spec for the Code-Review reviewer, sonnet[medium] by default; empty takes reviewer_model, a set value (one model-spec, or a per-round list whose last entry serves every later round) replaces it for this segment\n"},
		},
		{
			name:     "a retired webster_max_bounces is renamed and a retired merge_max_bounces is dropped by loom's Migrate",
			settings: map[string]any{"loom": map[string]any{"webster_max_bounces": float64(2), "merge_max_bounces": float64(1)}},
			wantDocs: map[string]string{"loom": renamedCodeMaxBounces},
		},
		{
			name:     "a retired merge_max_bounces is dropped by loom's Migrate and never refused",
			settings: map[string]any{"loom": map[string]any{"merge_max_bounces": float64(1), "plan_max_bounces": float64(2)}},
			wantDocs: map[string]string{"loom": "plan_max_bounces: 2\n"},
		},
		{
			name:        "a retired darn fan key moves to loom's code_fixer_fan and is refused as a guard key",
			settings:    map[string]any{"darn": map[string]any{"review_fix_fan": "fix-code"}},
			wantRefusal: "code_fixer_fan",
			wantModule:  "loom",
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
			name:        "a burler object holding lenses is refused with the way forward",
			settings:    map[string]any{"burler": map[string]any{"lenses": map[string]any{"generic": map[string]any{"prose": "x"}}}},
			wantRefusal: "not registered: its keys moved to another module; way forward: remove the burler object from run_settings",
			wantModule:  "burler",
			wantInvalid: true,
		},
		{
			name:        "an empty burler object is refused with the way forward",
			settings:    map[string]any{"burler": map[string]any{}},
			wantRefusal: "not registered: its keys moved to another module; way forward: remove the burler object from run_settings",
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
			settings:    map[string]any{"loom": map[string]any{"plan": map[string]any{"a": "b"}}},
			wantRefusal: "plan",
			wantModule:  "loom",
			wantInvalid: true,
		},
		{
			name:         "a retired darn review budget of zero moves to loom's code_max_bounces",
			settings:     map[string]any{"darn": map[string]any{"review_max_bounces": float64(0)}},
			wantDocs:     map[string]string{"loom": "code_max_bounces: 0\n"},
			wantNoModule: "darn",
		},
		{
			name:     "a retired darn review budget and hard cap move to loom's code keys",
			settings: map[string]any{"darn": map[string]any{"review_max_bounces": float64(2), "review_hard_max_bounces": float64(3)}},
			wantDocs: map[string]string{"loom": "code_hard_max_bounces: 3\ncode_max_bounces: 2\n"},
		},
		{
			name: "a retired darn model and amend key moves to loom's code key",
			settings: map[string]any{"darn": map[string]any{
				"darn_reviewer_model":      "opus[high]",
				"darn_fixer_model":         "opus[low]",
				"review_amend_max_bounces": float64(0),
			}},
			wantDocs: map[string]string{"loom": "code_fixer_model: opus[low]\ncode_reviewer_model: opus[high]\nreview_amend_max_bounces: 0\n"},
		},
		{
			name:         "a retired darn reviewer fan key moves to loom's code_reviewer_fan and is admitted",
			settings:     map[string]any{"darn": map[string]any{"darn_reviewer_fan": "review-code"}},
			wantDocs:     map[string]string{"loom": "code_reviewer_fan: review-code\n"},
			wantNoModule: "darn",
		},
		{
			name:         "a retired darn review_fan moves to loom's code_reviewer_fan and is admitted",
			settings:     map[string]any{"darn": map[string]any{"review_fan": "review-code"}},
			wantDocs:     map[string]string{"loom": "code_reviewer_fan: review-code\n"},
			wantNoModule: "darn",
		},
		{
			name:     "a retired webster_fan is renamed to code_reviewer_fan and admitted by loom's Migrate",
			settings: map[string]any{"loom": map[string]any{"webster_fan": "review-code"}},
			wantDocs: map[string]string{"loom": "code_reviewer_fan: review-code # fan name from lenses.yaml, such as review-code, that turns the lens fan on for Code-Review; one fan name, or a YAML list with one fan name per round, the last entry serving every later round; the default fans round one out over review-code and runs later rounds solo; an empty entry runs that round solo\n"},
		},
		{
			name:     "a retired webster_reviewer_fan is renamed to code_reviewer_fan and admitted by loom's Migrate",
			settings: map[string]any{"loom": map[string]any{"webster_reviewer_fan": "review-code"}},
			wantDocs: map[string]string{"loom": "code_reviewer_fan: review-code # fan name from lenses.yaml, such as review-code, that turns the lens fan on for Code-Review; one fan name, or a YAML list with one fan name per round, the last entry serving every later round; the default fans round one out over review-code and runs later rounds solo; an empty entry runs that round solo\n"},
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
			name:     "a retired loom key migrates away at read time",
			settings: map[string]any{"loom": map[string]any{"skip_alignment": true, "plan_max_bounces": float64(2)}},
			wantDocs: map[string]string{"loom": "plan_max_bounces: 2\n"},
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

	// The logger's output is process-global, so this subtest is not parallel: it runs before the parallel rows above resume.
	t.Run("each migration line is logged at Warn naming the module, the key and the way forward", func(t *testing.T) {
		logs := logcapture.Capture(t)
		settings := map[string]any{"loom": map[string]any{"webster_max_bounces": float64(2), "merge_max_bounces": float64(1)}}
		if _, err := PrepareRunSettings(settings); err != nil {
			t.Fatalf("PrepareRunSettings error = %v; want none", err)
		}
		const wayForward = "way forward: rename or remove the key in profiles.yaml or in the entry's run_settings"
		for _, line := range []string{"webster_max_bounces: renamed to code_max_bounces", "merge_max_bounces: retired, removed"} {
			want := "run_settings for module loom: " + line + "; " + wayForward
			if got := strings.Count(logs.String(), want); got != 1 {
				t.Errorf("log holds %d lines %q; want 1; log: %s", got, want, logs.String())
			}
		}
		if _, err := PrepareRunSettings(map[string]any{"loom": map[string]any{"plan_max_bounces": float64(1)}}); err != nil {
			t.Fatalf("PrepareRunSettings error = %v; want none", err)
		}
		if got := strings.Count(logs.String(), "run_settings for module"); got != 2 {
			t.Errorf("log holds %d migration lines after a document with nothing to migrate; want the 2 from before", got)
		}
		if !strings.Contains(logs.String(), "WARN") {
			t.Errorf("log = %s; want the lines at Warn", logs.String())
		}
	})
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
			profile:  map[string]any{"loom": map[string]any{"plan_max_bounces": float64(1), "code_max_bounces": float64(1)}},
			settings: map[string]any{"loom": map[string]any{"plan_max_bounces": float64(3)}},
			wantDocs: map[string]string{"loom": "code_max_bounces: 1\nplan_max_bounces: 3\n"},
		},
		{
			name:     "a profile's retired merge_max_bounces is dropped rather than refused",
			profile:  map[string]any{"loom": map[string]any{"plan_max_bounces": float64(1), "merge_max_bounces": float64(1)}},
			settings: map[string]any{"loom": map[string]any{"plan_max_bounces": float64(3)}},
			wantDocs: map[string]string{"loom": "plan_max_bounces: 3\n"},
		},
		{
			name:     "a task's retired webster_max_bounces layers as code_max_bounces and its merge_max_bounces is dropped",
			profile:  map[string]any{"loom": map[string]any{"plan_max_bounces": float64(1)}},
			settings: map[string]any{"loom": map[string]any{"webster_max_bounces": float64(2), "merge_max_bounces": float64(1)}},
			wantDocs: map[string]string{"loom": "plan_max_bounces: 1\n" + renamedCodeMaxBounces},
		},
		{
			name:     "a module only one side names is written as that side has it",
			profile:  map[string]any{"darn": map[string]any{"verify_attempts": float64(2)}},
			settings: map[string]any{"loom": map[string]any{"plan_max_bounces": float64(2)}},
			wantDocs: map[string]string{"darn": "verify_attempts: 2\n", "loom": "plan_max_bounces: 2\n"},
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
		{name: "a valid hub file passes", hubFile: "light:\n  loom:\n    loom: {plan_max_bounces: 1}\n  darn:\n    darn: {writer: opus}\n"},
		{name: "a profile holding the retired webster_max_bounces and merge_max_bounces passes", hubFile: "light:\n  loom:\n    loom: {webster_max_bounces: 1, merge_max_bounces: 1}\n"},
		{name: "an unknown key is refused naming the profile and the recipe", hubFile: "light:\n  loom:\n    loom: {plan_max_bounces: 1}\nodd:\n  loom:\n    loom: {plan_max_bounces: 1}\n  darn:\n    loom: {no_such_key: 1}\n", wantRefusal: []string{`run profile "odd", recipe "darn"`, "no_such_key"}},
		{name: "an unknown module is refused naming the profile and the recipe", hubFile: "odd:\n  loom:\n    bogus: {x: 1}\n", wantRefusal: []string{`run profile "odd", recipe "loom"`, "bogus"}},
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

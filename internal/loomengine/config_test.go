// config_test.go — untagged Tier-1 unit tests for loomengine.LoadConfig.
//
// Seeds a bare t.TempDir() with just a _lyx/config/loom.yaml file (no real hub, no SeedConfig, no
// git spawn) since configengine.Load's env-source build tolerates a missing .env — a live fabric
// fixture would be integration-tagged (like websterengine/config_test.go), which is out of scope
// for this package's plain "go test ./internal/loomengine/" run.

package loomengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// seedLoomConfig creates <baseDir>/_lyx/config/loom.yaml with the given contents.
func seedLoomConfig(t *testing.T, baseDir, contents string) {
	t.Helper()
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	cfgPath := filepath.Join(configDir, "loom.yaml")
	if err := os.WriteFile(cfgPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// malformedBurlerYAML is a burler.yaml that fails to parse, for the fan-key rows that pin when LoadConfig reads it.
const malformedBurlerYAML = "fans: [unclosed\n"

// seedBurlerConfig creates <baseDir>/_lyx/config/burler.yaml with the given contents.
func seedBurlerConfig(t *testing.T, baseDir, contents string) {
	t.Helper()
	cfgPath := filepath.Join(baseDir, "_lyx", "config", "burler.yaml")
	if err := os.WriteFile(cfgPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// writeLoomConfigWithKey seeds a loom.yaml built from the shipped template with exactly one key's
// value replaced, so a test can vary a single knob without restating the whole file and drifting
// from the template's key set (which configengine.Load is strict about).
func writeLoomConfigWithKey(t *testing.T, baseDir, key, value string) {
	t.Helper()
	writeLoomConfigWithKeys(t, baseDir, map[string]string{key: value})
}

// writeLoomConfigWithKeys seeds baseDir's loom.yaml with the template's text and each key in values replaced by its value.
func writeLoomConfigWithKeys(t *testing.T, baseDir string, values map[string]string) {
	t.Helper()
	var b strings.Builder
	replaced := map[string]bool{}
	for _, line := range strings.Split(ConfigTemplate(), "\n") {
		matched := false
		for key, value := range values {
			if strings.HasPrefix(line, key+":") {
				b.WriteString(key + ": " + value + "\n")
				replaced[key] = true
				matched = true
				break
			}
		}
		if !matched {
			b.WriteString(line + "\n")
		}
	}
	for key := range values {
		if !replaced[key] {
			t.Fatalf("key %q not present in ConfigTemplate(); the fixture would silently test nothing", key)
		}
	}
	seedLoomConfig(t, baseDir, b.String())
}

func intPtr(v int) *int { return &v }

// templateConfig is the Config the shipped template's own values load to.
func templateConfig() Config {
	return Config{
		Discussion:               "opus[medium]",
		DiscussionTimeoutMin:     480,
		DiscussionInteractive:    false,
		Plan:                     "opus[medium]",
		PlanTimeoutMin:           120,
		Review:                   ModelSpecList{"opus[medium]"},
		Fix:                      ModelSpecList{"opus[medium]"},
		DiscussionReview:         ModelSpecList{""},
		DiscussionFix:            ModelSpecList{""},
		PlanReview:               ModelSpecList{""},
		PlanFix:                  ModelSpecList{""},
		WebsterReview:            ModelSpecList{"sonnet[high]"},
		WebsterFix:               ModelSpecList{""},
		DiscussionFan:            FanList{""},
		PlanFan:                  FanList{""},
		DiscussionFixFan:         FanList{""},
		PlanFixFan:               FanList{""},
		Judge:                    "sonnet[medium]",
		ReviewTimeoutMin:         240,
		Friction:                 "sonnet[medium]",
		FrictionTimeoutMin:       30,
		Driver:                   "sonnet[medium]",
		ParentReviewWaitMin:      60,
		StepIdleTimeoutMin:       60,
		Arbiter:                  "opus[high]",
		ArbiterTimeoutMin:        30,
		ArbiterMaxSpawns:         3,
		DriverKind:               "",
		Tackler:                  "",
		TacklerTimeoutMin:        0,
		TacklerMaxSpawns:         nil,
		Alignment:                "opus[high]",
		AlignmentTimeoutMin:      20,
		SkipAlignment:            false,
		AlignmentPRApprove:       true,
		ReviewCirclingCheckpoint: 3,
		ReviewMaxBounces:         3,
		ReviewHardMaxBounces:     4,
		DiscussionMaxBounces:     nil,
		PlanMaxBounces:           nil,
		WebsterMaxBounces:        nil,
		DiscussionAutoAccept:     false,
		PlanAutoAccept:           false,
		WebsterAutoAccept:        false,
		MergeMaxBounces:          intPtr(1),
		MergeHardMaxBounces:      0,
		MergeAutoAccept:          true,
	}
}

// TestLoadConfig_Loads verifies each loom.yaml shape that must load, against the whole Config it yields:
// the template's own values, an unparseable burler.yaml beside empty fan keys (never read), a retired key ignored, explicit overrides, present-but-empty and null friction and driver values (Tier 2 off, "defer to the provider default"), keys absent from an already-seeded file at their template defaults, zero timeouts (shuttleengine.Spec treats zero as "defer to shuttle's run_timeout_min", so the negative guard must not sweep them up), and a checkpoint above the budget (a run that never rules CIRCLING and reaches the soft-cap rule instead).
func TestLoadConfig_Loads(t *testing.T) {
	t.Parallel()
	// legacyContents is a file seeded before the friction, driver, judge, parent-review and review-budget keys existed.
	const legacyContents = `discussion: opus[effort=high]
discussion_timeout_min: 480
discussion_interactive: false
plan: opus[effort=high]
plan_timeout_min: 120
review: opus[effort=high]
review_timeout_min: 240
`
	tests := []struct {
		name     string
		contents string
		values   map[string]string
		burler   string
		mutate   func(*Config)
		wantSoft map[Perch]int
	}{
		{name: "template defaults", mutate: func(*Config) {}},
		{
			name:   "empty fan keys never read a malformed burler.yaml",
			burler: malformedBurlerYAML,
			mutate: func(*Config) {},
		},
		{
			name:     "retired selfreport key ignored",
			contents: ConfigTemplate() + "selfreport: false\n",
			mutate:   func(*Config) {},
		},
		{
			name:   "review as a per-round list",
			values: map[string]string{"review": "\n  - sonnet[low]\n  - opus[high]"},
			mutate: func(c *Config) { c.Review = ModelSpecList{"sonnet[low]", "opus[high]"} },
		},
		{
			name:   "fix as a per-round list",
			values: map[string]string{"fix": "\n  - opus[low]\n  - opus[high]"},
			mutate: func(c *Config) { c.Fix = ModelSpecList{"opus[low]", "opus[high]"} },
		},
		{
			name:   "plan_review as a per-round list",
			values: map[string]string{"plan_review": "\n  - sonnet[low]\n  - opus[high]"},
			mutate: func(c *Config) { c.PlanReview = ModelSpecList{"sonnet[low]", "opus[high]"} },
		},
		{
			name:   "webster_fix as one model-spec",
			values: map[string]string{"webster_fix": "opus[low]"},
			mutate: func(c *Config) { c.WebsterFix = ModelSpecList{"opus[low]"} },
		},
		{
			name:   "empty segment list takes the run-wide list",
			values: map[string]string{"discussion_fix": "[]"},
			mutate: func(c *Config) { c.DiscussionFix = ModelSpecList{} },
		},
		{
			name:   "each fan key as one fan name",
			values: map[string]string{"discussion_fan": "standard", "plan_fan": "full", "discussion_fix_fan": "fix-discussion", "plan_fix_fan": "fix-plan"},
			mutate: func(c *Config) {
				c.DiscussionFan, c.PlanFan = FanList{"standard"}, FanList{"full"}
				c.DiscussionFixFan, c.PlanFixFan = FanList{"fix-discussion"}, FanList{"fix-plan"}
			},
		},
		{
			name: "each fan key as a per-round list, an empty entry running its round solo",
			values: map[string]string{
				"discussion_fan":     "\n  - \"\"\n  - review-discussion",
				"plan_fan":           "\n  - review-plan\n  - \"\"",
				"discussion_fix_fan": "\n  - \"\"\n  - fix-discussion",
				"plan_fix_fan":       "\n  - fix-plan\n  - \"\"",
			},
			mutate: func(c *Config) {
				c.DiscussionFan, c.PlanFan = FanList{"", "review-discussion"}, FanList{"review-plan", ""}
				c.DiscussionFixFan, c.PlanFixFan = FanList{"", "fix-discussion"}, FanList{"fix-plan", ""}
			},
		},
		{
			name:     "retired fix_start key ignored",
			contents: ConfigTemplate() + "fix_start: after-review\n",
			mutate:   func(*Config) {},
		},
		{
			name:   "discussion_interactive true",
			values: map[string]string{"discussion_interactive": "true"},
			mutate: func(c *Config) { c.DiscussionInteractive = true },
		},
		{
			name:   "discussion_lenses and plan_lenses name the template fans",
			values: map[string]string{"discussion_lenses": "twill-discussion", "plan_lenses": "twill-plan"},
			mutate: func(c *Config) { c.DiscussionLenses, c.PlanLenses = "twill-discussion", "twill-plan" },
		},
		{
			name:   "bare discussion_advisors key is null and means no advisors",
			values: map[string]string{"discussion_advisors": ""},
			mutate: func(c *Config) { c.DiscussionAdvisors = nil },
		},
		{
			name:   "discussion_advisors as an empty list means no advisors",
			values: map[string]string{"discussion_advisors": "[]"},
			mutate: func(c *Config) { c.DiscussionAdvisors = nil },
		},
		{
			name:   "discussion_advisors as one named entry",
			values: map[string]string{"discussion_advisors": "[{name: critic, model: \"opus[low]\"}]"},
			mutate: func(c *Config) { c.DiscussionAdvisors = DiscussionAdvisorList{{Name: "critic", Model: "opus[low]"}} },
		},
		{
			name:   "discussion_advisors as a two-entry block list",
			values: map[string]string{"discussion_advisors": "\n  - name: critic\n    model: sonnet[low]\n  - name: skeptic\n    model: opus[high]"},
			mutate: func(c *Config) {
				c.DiscussionAdvisors = DiscussionAdvisorList{{Name: "critic", Model: "sonnet[low]"}, {Name: "skeptic", Model: "opus[high]"}}
			},
		},
		{
			name:   "empty friction turns Tier 2 off",
			values: map[string]string{"friction": `""`},
			mutate: func(c *Config) { c.Friction = "" },
		},
		{
			name: "judge, seed and rework override keys set",
			values: map[string]string{
				"seed": "haiku[low]", "discussion_judge": "opus[low]", "plan_judge": "opus[high]", "webster_judge": "sonnet[low]",
				"discussion_seed": "haiku[medium]", "plan_seed": "haiku[high]", "webster_seed": "sonnet[high]", "rework": "opus[low]",
			},
			mutate: func(c *Config) {
				c.Seed, c.Rework = "haiku[low]", "opus[low]"
				c.DiscussionJudge, c.PlanJudge, c.WebsterJudge = "opus[low]", "opus[high]", "sonnet[low]"
				c.DiscussionSeed, c.PlanSeed, c.WebsterSeed = "haiku[medium]", "haiku[high]", "sonnet[high]"
			},
		},
		{
			name:   "valid driver spec",
			values: map[string]string{"driver": "opus[effort=high]"},
			mutate: func(c *Config) { c.Driver = "opus[effort=high]" },
		},
		{
			name:   "bare driver key is null",
			values: map[string]string{"driver": ""},
			mutate: func(c *Config) { c.Driver = "" },
		},
		{
			name:   "empty driver string",
			values: map[string]string{"driver": `""`},
			mutate: func(c *Config) { c.Driver = "" },
		},
		{
			name:   "explicit arbiter keys",
			values: map[string]string{"arbiter": "sonnet[low]", "arbiter_timeout_min": "45", "arbiter_max_spawns": "5"},
			mutate: func(c *Config) { c.Arbiter, c.ArbiterTimeoutMin, c.ArbiterMaxSpawns = "sonnet[low]", 45, 5 },
		},
		{
			name:   "zero and empty arbiter keys load as their off and default meanings",
			values: map[string]string{"arbiter": `""`, "arbiter_timeout_min": "0", "arbiter_max_spawns": "0"},
			mutate: func(c *Config) { c.Arbiter, c.ArbiterTimeoutMin, c.ArbiterMaxSpawns = "", 0, 0 },
		},
		{
			name:   "driver_kind go and explicit tackler keys, a tackler_max_spawns of 0 read as a set 0",
			values: map[string]string{"driver_kind": "go", "tackler": "sonnet[low]", "tackler_timeout_min": "5", "tackler_max_spawns": "0"},
			mutate: func(c *Config) {
				c.DriverKind, c.Tackler, c.TacklerTimeoutMin, c.TacklerMaxSpawns = "go", "sonnet[low]", 5, intPtr(0)
			},
		},
		{
			name:   "explicit parent_review_wait_min",
			values: map[string]string{"parent_review_wait_min": "5", "alignment_timeout_min": "4"},
			mutate: func(c *Config) { c.ParentReviewWaitMin, c.AlignmentTimeoutMin = 5, 4 },
		},
		{
			name:   "explicit alignment keys",
			values: map[string]string{"alignment": "sonnet[low]", "alignment_timeout_min": "10", "skip_alignment": "true", "alignment_pr_approve": "false"},
			mutate: func(c *Config) {
				c.Alignment, c.AlignmentTimeoutMin, c.SkipAlignment, c.AlignmentPRApprove = "sonnet[low]", 10, true, false
			},
		},
		{
			name:   "explicit step_idle_timeout_min",
			values: map[string]string{"step_idle_timeout_min": "15"},
			mutate: func(c *Config) { c.StepIdleTimeoutMin = 15 },
		},
		{
			name:   "zero step_idle_timeout_min is floored to one",
			values: map[string]string{"step_idle_timeout_min": "0"},
			mutate: func(c *Config) { c.StepIdleTimeoutMin = 1 },
		},
		{
			name:   "explicit review checkpoint and budget",
			values: map[string]string{"review_circling_checkpoint": "2", "review_max_bounces": "7", "review_hard_max_bounces": "7"},
			mutate: func(c *Config) { c.ReviewCirclingCheckpoint, c.ReviewMaxBounces, c.ReviewHardMaxBounces = 2, 7, 7 },
		},
		{
			name:   "explicit hard caps",
			values: map[string]string{"review_hard_max_bounces": "5", "plan_hard_max_bounces": "6"},
			mutate: func(c *Config) { c.ReviewHardMaxBounces, c.PlanHardMaxBounces = 5, 6 },
		},
		{
			name:   "a hard cap equal to the soft cap loads",
			values: map[string]string{"review_max_bounces": "3", "review_hard_max_bounces": "3"},
			mutate: func(c *Config) { c.ReviewMaxBounces, c.ReviewHardMaxBounces = 3, 3 },
		},
		{
			name: "explicit per-perch budgets and auto-accept flags",
			values: map[string]string{
				"discussion_max_bounces": "1", "plan_max_bounces": "2", "webster_max_bounces": "4",
				"discussion_auto_accept": "true", "plan_auto_accept": "true", "webster_auto_accept": "true",
			},
			mutate: func(c *Config) {
				c.DiscussionMaxBounces, c.PlanMaxBounces, c.WebsterMaxBounces = intPtr(1), intPtr(2), intPtr(4)
				c.DiscussionAutoAccept, c.PlanAutoAccept, c.WebsterAutoAccept = true, true, true
			},
		},
		{
			name:   "explicit merge perch keys",
			values: map[string]string{"merge_max_bounces": "0", "merge_auto_accept": "false"},
			mutate: func(c *Config) { c.MergeMaxBounces, c.MergeAutoAccept = intPtr(0), false },
		},
		{
			name:     "a zero per-perch budget loads as zero rounds, not as null",
			values:   map[string]string{"plan_max_bounces": "0"},
			mutate:   func(c *Config) { c.PlanMaxBounces = intPtr(0) },
			wantSoft: map[Perch]int{PerchPlan: 0, PerchDiscussion: 3},
		},
		{
			name:     "a zero run-wide budget gives every null perch zero rounds",
			values:   map[string]string{"review_max_bounces": "0"},
			mutate:   func(c *Config) { c.ReviewMaxBounces = 0 },
			wantSoft: map[Perch]int{PerchDiscussion: 0, PerchPlan: 0, PerchWebster: 0, PerchMerge: 1},
		},
		{
			name:   "checkpoint above budget",
			values: map[string]string{"review_circling_checkpoint": "9", "review_max_bounces": "2"},
			mutate: func(c *Config) { c.ReviewCirclingCheckpoint, c.ReviewMaxBounces = 9, 2 },
		},
		{
			name:     "keys absent from a seeded file take template defaults",
			contents: legacyContents,
			mutate: func(c *Config) {
				c.Discussion, c.Plan, c.Review = "opus[effort=high]", "opus[effort=high]", ModelSpecList{"opus[effort=high]"}
			},
		},
		{
			name:   "zero discussion timeout",
			values: map[string]string{"discussion_timeout_min": "0"},
			mutate: func(c *Config) { c.DiscussionTimeoutMin = 0 },
		},
		{
			name:   "zero plan timeout",
			values: map[string]string{"plan_timeout_min": "0"},
			mutate: func(c *Config) { c.PlanTimeoutMin = 0 },
		},
		{
			name:   "zero review timeout",
			values: map[string]string{"review_timeout_min": "0"},
			mutate: func(c *Config) { c.ReviewTimeoutMin = 0 },
		},
		{
			name:   "zero friction timeout",
			values: map[string]string{"friction_timeout_min": "0"},
			mutate: func(c *Config) { c.FrictionTimeoutMin = 0 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			switch {
			case tt.contents != "":
				seedLoomConfig(t, baseDir, tt.contents)
			case tt.values != nil:
				writeLoomConfigWithKeys(t, baseDir, tt.values)
			default:
				seedLoomConfig(t, baseDir, ConfigTemplate())
			}
			if tt.burler != "" {
				seedBurlerConfig(t, baseDir, tt.burler)
			}

			want := templateConfig()
			tt.mutate(&want)

			got, err := LoadConfig(configengine.Dirs{Anchor: baseDir}, "loom")
			if err != nil {
				t.Fatalf("LoadConfig(%q, \"loom\") = _, %v; want nil error", baseDir, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("LoadConfig() = %+v; want %+v", got, want)
			}
			for perch, soft := range tt.wantSoft {
				if resolved := got.Caps(perch).Soft; resolved != soft {
					t.Errorf("Caps(%s).Soft = %d; want %d", perch, resolved, soft)
				}
			}
		})
	}

	t.Run("hub stack", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			board      string
			boardExtra string
			worktree   string
			local      string
			mutate     func(*Config)
		}{
			{
				name:   "a local value wins over the repo default",
				board:  "review_max_bounces: 3\n",
				local:  "review_max_bounces: 2\n",
				mutate: func(c *Config) { c.ReviewMaxBounces = 2 },
			},
			{
				name:     "a worktree list replaces the default's whole",
				board:    "review: [\"opus[medium]\"]\n",
				worktree: "review: [\"sonnet[low]\", \"sonnet[high]\"]\n",
				mutate:   func(c *Config) { c.Review = ModelSpecList{"sonnet[low]", "sonnet[high]"} },
			},
			{
				name:       "a retired fan_review in the repo default is ignored",
				boardExtra: "fan_review: sonnet[high]\n",
				mutate:     func(*Config) {},
			},
			{
				name:   "a local driver_kind go is open to the local layer",
				board:  "driver_kind: llm\n",
				local:  "driver_kind: go\n",
				mutate: func(c *Config) { c.DriverKind = "go" },
			},
			{
				name:   "a local alignment_timeout_min is open to the local layer",
				board:  "alignment_timeout_min: 20\n",
				local:  "alignment_timeout_min: 10\n",
				mutate: func(c *Config) { c.AlignmentTimeoutMin = 10 },
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				board, anchor := t.TempDir(), t.TempDir()
				seedLoomConfig(t, board, loomTemplateWith(t, tt.board)+tt.boardExtra)
				if tt.worktree != "" {
					seedLoomConfig(t, anchor, tt.worktree)
				}
				if tt.local != "" {
					writeLocalLoomConfig(t, anchor, tt.local)
				}

				want := templateConfig()
				tt.mutate(&want)
				got, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "loom")
				if err != nil {
					t.Fatalf("LoadConfig() = _, %v; want nil error", err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("LoadConfig() = %+v; want %+v", got, want)
				}
			})
		}

		t.Run("an absent repo default errors naming its path", func(t *testing.T) {
			t.Parallel()
			board, anchor := t.TempDir(), t.TempDir()
			seedLoomConfig(t, anchor, ConfigTemplate())
			if err := os.MkdirAll(configengine.ConfigDir(board), 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) = %v; want nil", configengine.ConfigDir(board), err)
			}

			_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "loom")
			if err == nil {
				t.Fatal("LoadConfig() error = nil; want a refusal for the absent repo default")
			}
			if want := configengine.ConfigFile(board, "loom"); !strings.Contains(err.Error(), want) {
				t.Errorf("LoadConfig() error = %q; want it to name %q", err.Error(), want)
			}
		})
	})
}

// writeLocalLoomConfig writes contents as anchor's local loom override and returns its path.
// anchor gets an empty _lyx/config/ so the layered load sees an initialised worktree.
func writeLocalLoomConfig(t *testing.T, anchor, contents string) string {
	t.Helper()
	for _, dir := range []string{configengine.ConfigDir(anchor), configengine.ScratchDir(anchor)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
		}
	}
	localPath := configengine.LocalFile(anchor, "loom")
	if err := os.WriteFile(localPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", localPath, err)
	}
	return localPath
}

// loomTemplateWith returns the shipped template text with each top-level line of overrides replacing the template's line for the same key.
func loomTemplateWith(t *testing.T, overrides string) string {
	t.Helper()
	replacements := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(overrides), "\n") {
		if line == "" {
			continue
		}
		key, _, _ := strings.Cut(line, ":")
		replacements[key] = line
	}
	var b strings.Builder
	for _, line := range strings.Split(ConfigTemplate(), "\n") {
		key, _, _ := strings.Cut(line, ":")
		if replacement, ok := replacements[key]; ok {
			line = replacement
			delete(replacements, key)
		}
		b.WriteString(line + "\n")
	}
	if len(replacements) != 0 {
		t.Fatalf("overrides %v are not keys of ConfigTemplate(); the fixture would silently test nothing", replacements)
	}
	return b.String()
}

// TestSetDiscussionAdvisors_LoadsBack verifies a discussion_advisors list of name and model mappings written through configengine.Set loads back as those advisors.
func TestSetDiscussionAdvisors_LoadsBack(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()
	seedLoomConfig(t, baseDir, ConfigTemplate())
	dirs := configengine.Dirs{Anchor: baseDir}
	spec := ConfigSpec()
	pairs := []yamlengine.KV{{Key: "discussion_advisors", Value: `[{name: critic, model: "sonnet[high]"}]`}}
	if _, err := configengine.Set(dirs, spec, configengine.BaseLayer(dirs, spec), pairs); err != nil {
		t.Fatalf("Set() = _, %v; want nil error", err)
	}

	got, err := LoadConfig(dirs, "loom")
	if err != nil {
		t.Fatalf("LoadConfig() = _, %v; want nil error", err)
	}
	want := DiscussionAdvisorList{{Name: "critic", Model: "sonnet[high]"}}
	if !reflect.DeepEqual(got.DiscussionAdvisors, want) {
		t.Errorf("DiscussionAdvisors = %+v; want %+v", got.DiscussionAdvisors, want)
	}
}

// TestLoadConfig_Refuses verifies each invalid loom.yaml is refused at load time, naming the offending key:
// an ungrammatical model-spec (rather than being silently carried into a producer's spawn site), a parent_review_wait_min or review key below one (the sibling timeouts' "0 defers to shuttle" carve-out does not apply to them) with the way forward, a negative timeout (which would otherwise surface only when the producer it governs first spawns), and a set fan key beside an unparseable burler.yaml.
func TestLoadConfig_Refuses(t *testing.T) {
	t.Parallel()
	const positiveIntegerForward = "set it to a positive integer in loom.yaml"
	tests := []struct {
		name   string
		key    string
		value  string
		wantIn []string
		burler string
	}{
		{"malformed discussion spec", "discussion", `"opus[effort"`, nil, ""},
		{"malformed plan spec", "plan", `"opus[effort"`, nil, ""},
		{"malformed review spec", "review", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed fix spec", "fix", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed later review entry", "review", "\n  - sonnet[low]\n  - \"opus[effort\"", []string{"entry 2", "a non-empty list of model-specs"}, ""},
		{"empty review list", "review", "[]", []string{"empty", "a non-empty list of model-specs"}, ""},
		{"empty fix list", "fix", "[]", []string{"empty", "a non-empty list of model-specs"}, ""},
		{"malformed discussion_review spec", "discussion_review", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed discussion_fix spec", "discussion_fix", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed plan_review spec", "plan_review", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed plan_fix spec", "plan_fix", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed webster_review spec", "webster_review", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed later webster_fix entry", "webster_fix", "\n  - sonnet[low]\n  - \"opus[effort\"", []string{"entry 2", "a non-empty list of model-specs"}, ""},
		{"unknown discussion_fan", "discussion_fan", "nosuchfan", []string{`"nosuchfan"`, "known fans: ", "standard", "entry 1", `set the entry to ""`}, ""},
		{"unknown plan_fan", "plan_fan", "nosuchfan", []string{`"nosuchfan"`, "known fans: ", "full", "entry 1", `set the entry to ""`}, ""},
		{"unknown second discussion_fix_fan entry", "discussion_fix_fan", "\n  - \"\"\n  - nosuchfan", []string{"entry 2", `"nosuchfan"`, "known fans: ", "define the fan in burler.yaml", `set the entry to ""`}, ""},
		{"empty discussion_fan list", "discussion_fan", "[]", []string{"the list is empty", `write ""`}, ""},
		{"empty plan_fan list", "plan_fan", "[]", []string{"the list is empty", `write ""`}, ""},
		{"empty discussion_fix_fan list", "discussion_fix_fan", "[]", []string{"the list is empty", `write ""`}, ""},
		{"empty plan_fix_fan list", "plan_fix_fan", "[]", []string{"the list is empty", `write ""`}, ""},
		{"mapping discussion_fix_fan value", "discussion_fix_fan", "\n  model: opus", []string{"line ", "neither a fan name nor a list of fan names", "a list of fan names"}, ""},
		{"mapping inside a plan_fan list", "plan_fan", "\n  - full\n  - model: opus", []string{"line ", "neither a fan name nor a list of fan names", "a list of fan names"}, ""},
		{"set discussion_fan with a malformed burler.yaml", "discussion_fan", "standard", []string{"burler: config file ", "parse doc YAML", "fix burler.yaml, or set the key to empty"}, malformedBurlerYAML},
		{"string discussion_advisors entry", "discussion_advisors", `["sonnet[high]"]`, []string{"entry 1", advisorEntryWayForward}, ""},
		{"string discussion_advisors entry after a mapping", "discussion_advisors", `[{name: critic, model: "sonnet[high]"}, "sonnet[low]"]`, []string{"entry 2", advisorEntryWayForward}, ""},
		{"blank discussion_advisors name", "discussion_advisors", `[{name: "", model: "sonnet[high]"}]`, []string{"entry 1", "missing or blank", advisorWayForward}, ""},
		{"discussion_advisors entry without a name", "discussion_advisors", `[{model: "sonnet[high]"}]`, []string{"entry 1", "missing or blank", advisorWayForward}, ""},
		{"duplicate discussion_advisors name", "discussion_advisors", `[{name: critic, model: "sonnet[high]"}, {name: critic, model: "sonnet[low]"}]`, []string{"entry 2", `"critic"`, "repeats entry 1", advisorWayForward}, ""},
		{"numeric discussion_advisors name", "discussion_advisors", `[{name: "2", model: "sonnet[high]"}]`, []string{"entry 1", `"2"`, "no seat name", advisorWayForward}, ""},
		{"malformed discussion_advisors model", "discussion_advisors", `[{name: critic, model: "opus[effort"}]`, []string{"entry 1", advisorWayForward}, ""},
		{"discussion_advisors entry without a model", "discussion_advisors", `[{name: critic}]`, []string{"entry 1", advisorWayForward}, ""},
		{"unknown discussion_advisors alias", "discussion_advisors", `[{name: critic, model: "sonnet[high]"}, {name: skeptic, model: ghost}]`, []string{"entry 2", "ghost", advisorWayForward}, ""},
		{"mapping discussion_advisors value", "discussion_advisors", "\n  model: opus", []string{"line ", advisorWayForward}, ""},
		{"scalar discussion_advisors value", "discussion_advisors", `"sonnet[high]"`, []string{"line ", advisorWayForward}, ""},
		{"mapping review value", "review", "\n  model: opus", []string{"a non-empty list of model-specs"}, ""},
		{"mapping inside a fix list", "fix", "\n  - model: opus", []string{"a non-empty list of model-specs"}, ""},
		{"malformed judge spec", "judge", `"sonnet[medium"`, nil, ""},
		{"malformed seed spec", "seed", `"opus[effort"`, nil, ""},
		{"malformed discussion_judge spec", "discussion_judge", `"opus[effort"`, nil, ""},
		{"malformed plan_judge spec", "plan_judge", `"opus[effort"`, nil, ""},
		{"malformed webster_judge spec", "webster_judge", `"opus[effort"`, nil, ""},
		{"malformed discussion_seed spec", "discussion_seed", `"opus[effort"`, nil, ""},
		{"malformed plan_seed spec", "plan_seed", `"opus[effort"`, nil, ""},
		{"malformed webster_seed spec", "webster_seed", `"opus[effort"`, nil, ""},
		{"malformed rework spec", "rework", `"opus[effort"`, nil, ""},
		{"malformed friction spec", "friction", `"opus[effort"`, nil, ""},
		{"malformed driver spec", "driver", `"opus[effort"`, nil, ""},
		{"malformed arbiter spec", "arbiter", `"opus[effort"`, nil, ""},
		{"malformed alignment spec", "alignment", `"opus[effort"`, nil, ""},
		{"unknown driver_kind", "driver_kind", "fast", []string{"llm or go"}, ""},
		{"malformed tackler spec", "tackler", `"not a spec["`, nil, ""},
		{"negative tackler_timeout_min", "tackler_timeout_min", "-1", []string{"must not be negative", "0 for the 10 minute default"}, ""},
		{"negative tackler_max_spawns", "tackler_max_spawns", "-1", []string{"must not be negative", "0 to turn the tackler off"}, ""},
		{"negative alignment_timeout_min", "alignment_timeout_min", "-1", []string{"must not be negative", "set it to 0 for the 20 minute default, or to a positive integer below parent_review_wait_min"}, ""},
		{"alignment_timeout_min equal to parent_review_wait_min", "alignment_timeout_min", "60", []string{"parent_review_wait_min", "60", "set alignment_timeout_min below parent_review_wait_min"}, ""},
		{"negative arbiter_timeout_min", "arbiter_timeout_min", "-1", []string{"must not be negative", "0 for the 30 minute default"}, ""},
		{"negative arbiter_max_spawns", "arbiter_max_spawns", "-1", []string{"must not be negative", "0 to turn the arbiter off"}, ""},
		{"zero parent_review_wait_min", "parent_review_wait_min", "0", []string{"attempts: 0"}, ""},
		{"negative parent_review_wait_min", "parent_review_wait_min", "-1", []string{"attempts: 0"}, ""},
		{"zero review_circling_checkpoint", "review_circling_checkpoint", "0", []string{positiveIntegerForward}, ""},
		{"negative review_circling_checkpoint", "review_circling_checkpoint", "-1", []string{positiveIntegerForward}, ""},
		{"negative review_max_bounces", "review_max_bounces", "-1", []string{"must not be negative", "set it to 0 for no review round, or to a positive integer"}, ""},
		{"negative discussion_max_bounces", "discussion_max_bounces", "-1", []string{"must not be negative", "set it to 0 for no review round, to a positive integer, or to null to take review_max_bounces"}, ""},
		{"negative plan_max_bounces", "plan_max_bounces", "-1", []string{"must not be negative", "or to null to take review_max_bounces"}, ""},
		{"negative webster_max_bounces", "webster_max_bounces", "-1", []string{"must not be negative", "or to null to take review_max_bounces"}, ""},
		{"negative merge_max_bounces", "merge_max_bounces", "-1", []string{"must not be negative", "or to null to take review_max_bounces"}, ""},
		{"zero review_hard_max_bounces", "review_hard_max_bounces", "0", []string{"must be at least 1", "lyx config loom --set review_hard_max_bounces=<n>", `lyx loom resume`}, ""},
		{"negative discussion_hard_max_bounces", "discussion_hard_max_bounces", "-1", []string{"set it to 0 to take review_hard_max_bounces", "lyx config loom --set discussion_hard_max_bounces=<n>", `lyx loom resume`}, ""},
		{"negative plan_hard_max_bounces", "plan_hard_max_bounces", "-1", []string{"set it to 0 to take review_hard_max_bounces", "lyx config loom --set plan_hard_max_bounces=<n>", `lyx loom resume`}, ""},
		{"negative webster_hard_max_bounces", "webster_hard_max_bounces", "-1", []string{"set it to 0 to take review_hard_max_bounces", "lyx config loom --set webster_hard_max_bounces=<n>", `lyx loom resume`}, ""},
		{"negative merge_hard_max_bounces", "merge_hard_max_bounces", "-1", []string{"set it to 0 to take review_hard_max_bounces", "lyx config loom --set merge_hard_max_bounces=<n>", `lyx loom resume`}, ""},
		{"review_max_bounces above the default hard cap", "review_max_bounces", "5", []string{"review_hard_max_bounces", "lyx config loom --set review_hard_max_bounces=<n>", "in the task worktree", `lyx loom resume`}, ""},
		{"webster_max_bounces above the default hard cap", "webster_max_bounces", "5", []string{"review_hard_max_bounces", "webster_max_bounces"}, ""},
		{"negative discussion timeout", "discussion_timeout_min", "-1", []string{"must not be negative"}, ""},
		{"negative plan timeout", "plan_timeout_min", "-1", []string{"must not be negative"}, ""},
		{"negative review timeout", "review_timeout_min", "-1", []string{"must not be negative"}, ""},
		{"negative friction timeout", "friction_timeout_min", "-1", []string{"must not be negative"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeLoomConfigWithKey(t, dir, tt.key, tt.value)
			if tt.burler != "" {
				seedBurlerConfig(t, dir, tt.burler)
			}

			_, err := LoadConfig(configengine.Dirs{Anchor: dir}, "loom")
			if err == nil {
				t.Fatalf("LoadConfig() error = nil; want a refusal for %s: %s", tt.key, tt.value)
			}
			for _, want := range append([]string{tt.key}, tt.wantIn...) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
				}
			}
		})
	}

	t.Run("a zero alignment_timeout_min resolving to 20 is refused at a 20-minute wait", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeLoomConfigWithKeys(t, dir, map[string]string{"alignment_timeout_min": "0", "parent_review_wait_min": "20"})

		_, err := LoadConfig(configengine.Dirs{Anchor: dir}, "loom")
		if err == nil {
			t.Fatal("LoadConfig() error = nil; want the coherence refusal for a resolved 20-minute timeout")
		}
		for _, want := range []string{"alignment_timeout_min", "parent_review_wait_min", "20"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
			}
		}
	})

	t.Run("a local skip_alignment is refused naming the local file and the key", func(t *testing.T) {
		t.Parallel()
		board, anchor := t.TempDir(), t.TempDir()
		seedLoomConfig(t, board, ConfigTemplate())
		localPath := writeLocalLoomConfig(t, anchor, "skip_alignment: true\n")

		_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "loom")
		if !errors.Is(err, configengine.ErrInvalid) {
			t.Fatalf("LoadConfig() error = %v; want ErrInvalid", err)
		}
		for _, want := range []string{localPath, "skip_alignment"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
			}
		}
	})

	t.Run("a local fan key is refused naming the local file and the key", func(t *testing.T) {
		t.Parallel()
		for _, key := range []string{"discussion_fan", "plan_fan", "discussion_fix_fan", "plan_fix_fan"} {
			t.Run(key, func(t *testing.T) {
				t.Parallel()
				board, anchor := t.TempDir(), t.TempDir()
				seedLoomConfig(t, board, ConfigTemplate())
				localPath := writeLocalLoomConfig(t, anchor, key+": standard\n")

				_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "loom")
				if !errors.Is(err, configengine.ErrInvalid) {
					t.Fatalf("LoadConfig() error = %v; want ErrInvalid", err)
				}
				for _, want := range []string{localPath, key} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
					}
				}
			})
		}
	})

	t.Run("a local alignment_timeout_min at the hub's wait is refused by the coherence check", func(t *testing.T) {
		t.Parallel()
		board, anchor := t.TempDir(), t.TempDir()
		seedLoomConfig(t, board, ConfigTemplate())
		writeLocalLoomConfig(t, anchor, "alignment_timeout_min: 60\n")

		_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "loom")
		if err == nil {
			t.Fatal("LoadConfig() error = nil; want the coherence refusal for a local timeout at the hub's 60-minute wait")
		}
		for _, want := range []string{"alignment_timeout_min", "parent_review_wait_min"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
			}
		}
	})

	t.Run("a mapping discussion_advisors value names no entry index", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeLoomConfigWithKey(t, dir, "discussion_advisors", "\n  model: opus")

		_, err := LoadConfig(configengine.Dirs{Anchor: dir}, "loom")
		if err == nil || strings.Contains(err.Error(), "entry") {
			t.Errorf("LoadConfig() error = %v; want a refusal that names no entry", err)
		}
	})

	t.Run("a malformed models.yaml refuses advisors and is never read for none", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeLoomConfigWithKey(t, dir, "discussion_advisors", `[{name: critic, model: "sonnet[high]"}]`)
		modelsPath := filepath.Join(dir, "_lyx", "config", "models.yaml")
		if err := os.WriteFile(modelsPath, []byte("sonnet: [unclosed\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", modelsPath, err)
		}

		_, err := LoadConfig(configengine.Dirs{Anchor: dir}, "loom")
		if err == nil {
			t.Fatal("LoadConfig() error = nil; want the models.yaml refusal for discussion_advisors")
		}
		for _, want := range []string{"discussion_advisors", "fix models.yaml, or set the key to an empty list for no advisors"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
			}
		}

		writeLoomConfigWithKey(t, dir, "discussion_advisors", "[]")
		if _, err := LoadConfig(configengine.Dirs{Anchor: dir}, "loom"); err != nil {
			t.Errorf("LoadConfig() with no advisors and a malformed models.yaml = %v; want nil", err)
		}
	})

	t.Run("a hard cap below the soft cap names the route of the layer each key comes from", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name     string
			worktree string
			local    string
			wantIn   []string
		}{
			{
				name:   "a local soft cap over the board default's hard cap",
				local:  "review_max_bounces: 5\n",
				wantIn: []string{"lyx config loom --layer local --set review_max_bounces=<n>", "lyx board upsert", "lyx config loom --layer default --set review_hard_max_bounces=<n>", "prime"},
			},
			{
				name:     "a worktree hard cap under the board default's soft cap",
				worktree: "review_hard_max_bounces: 2\n",
				wantIn:   []string{"lyx config loom --set review_hard_max_bounces=<n>", "in the task worktree"},
			},
			{
				name:   "a local hard cap of zero",
				local:  "review_hard_max_bounces: 0\n",
				wantIn: []string{"must be at least 1", "lyx config loom --layer local --set review_hard_max_bounces=<n>", "lyx board upsert", `lyx loom resume`},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				board, anchor := t.TempDir(), t.TempDir()
				seedLoomConfig(t, board, loomTemplateWith(t, "review_max_bounces: 3"))
				if tt.worktree != "" {
					seedLoomConfig(t, anchor, tt.worktree)
				}
				if tt.local != "" {
					writeLocalLoomConfig(t, anchor, tt.local)
				}

				_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "loom")
				if err == nil {
					t.Fatal("LoadConfig() error = nil; want a refusal naming the route of the layer each key comes from")
				}
				for _, want := range tt.wantIn {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
					}
				}
			})
		}
	})

	t.Run("a local parent_review_wait_min is refused naming the local file and the key", func(t *testing.T) {
		t.Parallel()
		board, anchor := t.TempDir(), t.TempDir()
		seedLoomConfig(t, board, ConfigTemplate())
		localPath := writeLocalLoomConfig(t, anchor, "parent_review_wait_min: 5\n")

		_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "loom")
		if !errors.Is(err, configengine.ErrInvalid) {
			t.Fatalf("LoadConfig() error = %v; want ErrInvalid", err)
		}
		for _, want := range []string{localPath, "parent_review_wait_min"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
			}
		}
	})
}

// TestLoadConfig_NotInitialized verifies uninitialized baseDir yields recovery hint.
func TestLoadConfig_NotInitialized(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()

	_, err := LoadConfig(configengine.Dirs{Anchor: baseDir}, "loom")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for uninitialized baseDir")
	}
	want := `not initialized here; run "lyx fabric reconcile"; a session lyx refuses the verb from reports status: FAILED and the orch runs it`
	if err.Error() != want {
		t.Errorf("LoadConfig() error = %q; want %q", err.Error(), want)
	}
}

// oldFormatLoomYAML returns the shipped template with its two lens keys swapped for the retired producer keys in producerLines, and advisorsLine, when non-empty, as its discussion_advisors line.
func oldFormatLoomYAML(advisorsLine, producerLines string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(ConfigTemplate(), "\n"), "\n") {
		key, _, _ := strings.Cut(line, ":")
		switch {
		case key == "discussion_lenses" || key == "plan_lenses":
			continue
		case key == "discussion_advisors" && advisorsLine != "":
			line = advisorsLine
		}
		b.WriteString(line + "\n")
	}
	return b.String() + producerLines
}

// TestMigrateConfig pins the loom.yaml migration: each retired producer value becomes its lens key, discussion_advisors is rewritten or emptied to match, comments and other keys are carried, the step_idle_timeout_min floor still applies, a migrated or producer-free document is returned unchanged, and a migrated stack of layers loads with the advisors each layer's migration left.
func TestMigrateConfig(t *testing.T) {
	t.Parallel()
	const (
		replacedDiscussion = `discussion_producer: %s replaced by discussion_lenses: %q`
		replacedPlan       = `plan_producer: %s replaced by plan_lenses: %q`
		renamed            = "discussion_advisors: %d string entries rewritten as {name: advisor-<n>, model: <entry>}; rename each advisor to its seat name"
		lowerLayer         = "discussion_producer was seats and discussion_advisors is not set here, so its advisors come from a lower layer whose migration may have emptied them; set discussion_advisors to a list of {name, model} entries"
	)
	tests := []struct {
		name         string
		input        string
		want         string
		wantRewrites []string
	}{
		{
			name:  "twill sets the lens fan and, with no list, turns the advisors off",
			input: "discussion_producer: twill\n",
			want:  "discussion_lenses: twill-discussion\ndiscussion_advisors: []\n",
			wantRewrites: []string{
				`discussion_producer: twill replaced by discussion_lenses: "twill-discussion"`,
				"discussion_advisors: set to [] because discussion_producer was twill",
			},
		},
		{
			name:  "single empties its advisors",
			input: "discussion_producer: single\ndiscussion_advisors: [\"sonnet[high]\"]\n",
			want:  "discussion_lenses: \"\"\ndiscussion_advisors: []\n",
			wantRewrites: []string{
				`discussion_producer: single replaced by discussion_lenses: ""`,
				"discussion_advisors: emptied because discussion_producer was single",
			},
		},
		{
			name:  "single with an already empty list reports only the key",
			input: "discussion_producer: single\ndiscussion_advisors: []\n",
			want:  "discussion_lenses: \"\"\ndiscussion_advisors: []\n",
			wantRewrites: []string{
				`discussion_producer: single replaced by discussion_lenses: ""`,
			},
		},
		{
			name:  "an unknown producer value reads as single",
			input: "discussion_producer: sideways\n",
			want:  "discussion_lenses: \"\"\ndiscussion_advisors: []\n",
			wantRewrites: []string{
				`discussion_producer: sideways replaced by discussion_lenses: ""`,
				"discussion_advisors: set to [] because discussion_producer was sideways",
			},
		},
		{
			name:  "seats names each string entry advisor-n",
			input: "discussion_producer: seats\ndiscussion_advisors:\n  - sonnet[high]\n  - opus[low]\n",
			want:  "discussion_lenses: \"\"\ndiscussion_advisors:\n  - name: advisor-1\n    model: sonnet[high]\n  - name: advisor-2\n    model: opus[low]\n",
			wantRewrites: []string{
				`discussion_producer: seats replaced by discussion_lenses: ""`,
				fmt.Sprintf(renamed, 2),
			},
		},
		{
			name:  "seats keeps the quoting of a flow entry",
			input: "discussion_producer: seats\ndiscussion_advisors: [\"sonnet[high]\"]\n",
			want:  "discussion_lenses: \"\"\ndiscussion_advisors:\n  - name: advisor-1\n    model: \"sonnet[high]\"\n",
			wantRewrites: []string{
				`discussion_producer: seats replaced by discussion_lenses: ""`,
				fmt.Sprintf(renamed, 1),
			},
		},
		{
			name:  "seats with an empty list leaves it empty",
			input: "discussion_producer: seats\ndiscussion_advisors: []\n",
			want:  "discussion_lenses: \"\"\ndiscussion_advisors: []\n",
			wantRewrites: []string{
				`discussion_producer: seats replaced by discussion_lenses: ""`,
			},
		},
		{
			name:  "seats with no list of its own says where its advisors come from",
			input: "discussion_producer: seats\n",
			want:  "discussion_lenses: \"\"\n",
			wantRewrites: []string{
				`discussion_producer: seats replaced by discussion_lenses: ""`,
				lowerLayer,
			},
		},
		{
			name:         "plan twill sets the plan lens fan",
			input:        "plan_producer: twill\n",
			want:         "plan_lenses: twill-plan\n",
			wantRewrites: []string{fmt.Sprintf(replacedPlan, "twill", "twill-plan")},
		},
		{
			name:         "plan single and empty turn the forks off",
			input:        "plan_producer: single\n",
			want:         "plan_lenses: \"\"\n",
			wantRewrites: []string{fmt.Sprintf(replacedPlan, "single", "")},
		},
		{
			name:         "an empty plan producer turns the forks off",
			input:        "plan_producer: \"\"\n",
			want:         "plan_lenses: \"\"\n",
			wantRewrites: []string{fmt.Sprintf(replacedPlan, "", "")},
		},
		{
			name:  "comments and other keys are carried and the floor still applies",
			input: "# head\nstep_idle_timeout_min: 0\ndiscussion_producer: twill # the writer\nplan: opus[medium]\n",
			want:  "# head\nstep_idle_timeout_min: 1\ndiscussion_lenses: twill-discussion # the writer\nplan: opus[medium]\ndiscussion_advisors: []\n",
			wantRewrites: []string{
				"step_idle_timeout_min: raised from 0 to 1",
				fmt.Sprintf(replacedDiscussion, "twill", "twill-discussion"),
				"discussion_advisors: set to [] because discussion_producer was twill",
			},
		},
		{
			name:  "a migrated document is unchanged on a second pass",
			input: "discussion_lenses: twill-discussion\ndiscussion_advisors:\n  - name: advisor-1\n    model: sonnet[high]\nplan_lenses: \"\"\n",
			want:  "discussion_lenses: twill-discussion\ndiscussion_advisors:\n  - name: advisor-1\n    model: sonnet[high]\nplan_lenses: \"\"\n",
		},
		{
			name:  "a sparse document with string entries and no producer is left for the load to refuse",
			input: "discussion_advisors: [\"sonnet[high]\"]\n",
			want:  "discussion_advisors: [\"sonnet[high]\"]\n",
		},
		{name: "an empty document is unchanged", input: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, rewrites, err := MigrateConfig([]byte(tt.input))
			if err != nil {
				t.Fatalf("MigrateConfig() = _, _, %v; want nil error", err)
			}
			if string(got) != tt.want {
				t.Errorf("MigrateConfig() output:\n%s\nwant:\n%s", got, tt.want)
			}
			if !slices.Equal(rewrites, tt.wantRewrites) {
				t.Errorf("MigrateConfig() rewrites = %q; want %q", rewrites, tt.wantRewrites)
			}
			again, againRewrites, err := MigrateConfig(got)
			if err != nil {
				t.Fatalf("MigrateConfig(second pass) = _, _, %v; want nil error", err)
			}
			if string(again) != string(got) || againRewrites != nil {
				t.Errorf("MigrateConfig(second pass) = %q, %q; want the bytes unchanged and no rewrites", again, againRewrites)
			}
		})
	}

	t.Run("a document that does not parse is marked invalid", func(t *testing.T) {
		t.Parallel()
		_, _, err := MigrateConfig([]byte("discussion_producer: [unclosed\n"))
		if !errors.Is(err, configengine.ErrInvalid) {
			t.Errorf("MigrateConfig() error = %v; want it marked configengine.ErrInvalid", err)
		}
	})

	t.Run("migrated layers load with the advisors each layer's migration left", func(t *testing.T) {
		t.Parallel()
		hub, _, err := MigrateConfig([]byte(oldFormatLoomYAML(`discussion_advisors: ["sonnet[high]"]`, "discussion_producer: seats\nplan_producer: twill\n")))
		if err != nil {
			t.Fatalf("MigrateConfig(hub) = _, _, %v; want nil error", err)
		}
		worktree, _, err := MigrateConfig([]byte("discussion_producer: twill\n"))
		if err != nil {
			t.Fatalf("MigrateConfig(worktree) = _, _, %v; want nil error", err)
		}

		alone := t.TempDir()
		seedLoomConfig(t, alone, string(hub))
		got, err := LoadConfig(configengine.Dirs{Anchor: alone}, "loom")
		if err != nil {
			t.Fatalf("LoadConfig(migrated hub file) = _, %v; want nil error", err)
		}
		if want := (DiscussionAdvisorList{{Name: "advisor-1", Model: "sonnet[high]"}}); !reflect.DeepEqual(got.DiscussionAdvisors, want) || got.DiscussionLenses != "" || got.PlanLenses != "twill-plan" {
			t.Errorf("LoadConfig(migrated hub file) advisors/lenses/plan lenses = %v/%q/%q; want %v, no discussion lenses and twill-plan", got.DiscussionAdvisors, got.DiscussionLenses, got.PlanLenses, want)
		}

		board, anchor := t.TempDir(), t.TempDir()
		seedLoomConfig(t, board, string(hub))
		seedLoomConfig(t, anchor, string(worktree))
		got, err = LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "loom")
		if err != nil {
			t.Fatalf("LoadConfig(migrated stack) = _, %v; want nil error", err)
		}
		if got.DiscussionAdvisors != nil || got.DiscussionLenses != "twill-discussion" || got.PlanLenses != "twill-plan" {
			t.Errorf("LoadConfig(migrated stack) advisors/lenses/plan lenses = %v/%q/%q; want no advisors, twill-discussion and twill-plan", got.DiscussionAdvisors, got.DiscussionLenses, got.PlanLenses)
		}
	})
}

// TestLoomDriverLogAndBootstrapLock covers LoomDriverLog and LoomBootstrapLock at both an unanchored and a subpath-anchored *lyxcwd.Location, hand-built rather than spawned.
//
// It also proves the two survive card 12's status/run-lock relocation onto shedrun.RunDir:
// LoomBootstrapLock must never collide with shedrun's own StatusLock or RunLock for the same location and shedrun.SelfRunID, exactly as it never collided with loomengine's own now-deleted LoomStatusLock/LoomRunLock before the relocation -- this is what makes the overview's loomDirName-survives-the-status-relocation decision checkable rather than asserted.
//
// LoomScratchDir names exactly the directory the driver log and the bootstrap lock share, so the three never drift apart, and that directory is loom's own scratch tree under .lyx/loom/, distinct from the run directory's .lyx/shed/<runID>/ mirror that shedrun.ScratchDir names.
func TestLoomDriverLogAndBootstrapLock(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		anchorRel string
	}{
		{"unanchored", "."},
		{"subpath-anchored", filepath.Join("sub", "dir")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := &lyxcwd.Location{
				HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
				WorktreeName: "repo",
				AnchorRel:    tt.anchorRel,
			}

			wantDriverLog := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "driver.log")
			if got := LoomDriverLog(l); got != wantDriverLog {
				t.Errorf("LoomDriverLog() = %q; want %q", got, wantDriverLog)
			}

			wantBootstrapLock := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "bootstrap.lock")
			if got := LoomBootstrapLock(l); got != wantBootstrapLock {
				t.Errorf("LoomBootstrapLock() = %q; want %q", got, wantBootstrapLock)
			}

			// Three distinct lock files is a correctness property, not a coincidence:
			// LoomBootstrapLock must never collide with either the per-persist status
			// lock or the whole-run lock, both now on shedrun's run directory.
			if got := LoomBootstrapLock(l); got == shedrun.StatusLock(l, shedrun.SelfRunID) {
				t.Errorf("LoomBootstrapLock() = %q; must differ from shedrun.StatusLock() = %q", got, shedrun.StatusLock(l, shedrun.SelfRunID))
			}
			if got := LoomBootstrapLock(l); got == shedrun.RunLock(l, shedrun.SelfRunID) {
				t.Errorf("LoomBootstrapLock() = %q; must differ from shedrun.RunLock() = %q", got, shedrun.RunLock(l, shedrun.SelfRunID))
			}

			got := LoomScratchDir(l)
			if want := filepath.Dir(LoomDriverLog(l)); got != want {
				t.Errorf("LoomScratchDir() = %q; want %q (filepath.Dir(LoomDriverLog()))", got, want)
			}
			if want := filepath.Dir(LoomBootstrapLock(l)); got != want {
				t.Errorf("LoomScratchDir() = %q; want %q (filepath.Dir(LoomBootstrapLock()))", got, want)
			}
			if got == shedrun.ScratchDir(l, shedrun.SelfRunID) {
				t.Errorf("LoomScratchDir() = %q; must differ from shedrun.ScratchDir() = %q", got, shedrun.ScratchDir(l, shedrun.SelfRunID))
			}
		})
	}
}

// TestLandingAndApprovalAccessors pins the landing directory, its commit-pathspec form and the
// approval record path for a hand-built location.
func TestLandingAndApprovalAccessors(t *testing.T) {
	t.Parallel()
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
		WorktreeName: "repo",
	}

	if got, want := LandingDirRel(), filepath.Join("_lyx", "landing"); got != want {
		t.Errorf("LandingDirRel() = %q; want %q", got, want)
	}
	if got, want := LandingDir(l), filepath.Join(l.AnchorPath(), "_lyx", "landing"); got != want {
		t.Errorf("LandingDir() = %q; want %q", got, want)
	}
	if got, want := LoomApprovalPath(l), filepath.Join(l.AnchorPath(), ".lyx", "loom", "approval.json"); got != want {
		t.Errorf("LoomApprovalPath() = %q; want %q", got, want)
	}
	if got, want := LoomRejectionPath(l), filepath.Join(l.AnchorPath(), ".lyx", "loom", "rejection.json"); got != want {
		t.Errorf("LoomRejectionPath() = %q; want %q", got, want)
	}
	if got, want := LoomReworkCoveragePath(l), filepath.Join(l.AnchorPath(), ".lyx", "loom", "rework-coverage.md"); got != want {
		t.Errorf("LoomReworkCoveragePath() = %q; want %q", got, want)
	}
	if got, want := LoomReworkDirRel(), filepath.Join("_lyx", "loom", "rework"); got != want {
		t.Errorf("LoomReworkDirRel() = %q; want %q", got, want)
	}
	if got, want := LoomReworkDir(l), filepath.Join(l.AnchorPath(), "_lyx", "loom", "rework"); got != want {
		t.Errorf("LoomReworkDir() = %q; want %q", got, want)
	}

	if slug, ok := LandingCheckpointSlug(LandingCheckpointSubject("my-slug")); slug != "my-slug" || !ok {
		t.Errorf("LandingCheckpointSlug(LandingCheckpointSubject(my-slug)) = %q, %v; want my-slug, true", slug, ok)
	}
	for _, subject := range []string{"loom: run records checkpoint for landing ", "batten: Land -> running"} {
		if slug, ok := LandingCheckpointSlug(subject); ok || slug != "" {
			t.Errorf("LandingCheckpointSlug(%q) = %q, %v; want empty, false", subject, slug, ok)
		}
	}
}

// TestConfigTemplate_ContainsEveryConfigYAMLTag walks Config's fields via reflection and asserts every yaml tag appears in the template text -- so a struct field added without a matching template line is caught mechanically rather than relying on review to notice the gap.
// The Config Strictness Invariant makes a struct field with no matching template key a silent hole rather than a load error, which is exactly what this check guards against.
//
//testtiming:keep pins that every Config yaml tag has a template line, a reflection check no LoadConfig case makes
func TestConfigTemplate_ContainsEveryConfigYAMLTag(t *testing.T) {
	t.Parallel()
	text := ConfigTemplate()

	typ := reflect.TypeOf(Config{})
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("yaml")
		if tag == "" {
			t.Fatalf("Config field %q has no yaml tag", typ.Field(i).Name)
		}
		if !containsConfigKey(text, tag) {
			t.Errorf("ConfigTemplate() does not contain key %q for field %q", tag, typ.Field(i).Name)
		}
	}
}

// TestPathsOutsideRunRecords pins that PathsOutsideRunRecords drops every path under the run directory, the loom durable directory and the reviews directory, and keeps a code path and a path that only shares a records directory's name as a prefix.
func TestPathsOutsideRunRecords(t *testing.T) {
	t.Parallel()

	l := &lyxcwd.Location{HubPath: filepath.Join("home", "user", "repo-LYXHUB"), WorktreeName: "repo", AnchorRel: "."}
	runID := shedrun.SelfRunID
	runDir := shedrun.RunDir(l, runID)
	codePath := filepath.Join(l.AnchorPath(), "internal", "x.go")
	prefixOnly := LoomDurableDir(l) + "-extra"

	got := PathsOutsideRunRecords(l, runID, []string{
		filepath.Join(runDir, "status.json"),
		filepath.Join(LoomDurableDir(l), "friction", "note.md"),
		filepath.Join(LoomReviewsDir(l), "round.yaml"),
		codePath,
		prefixOnly,
	})

	if want := []string{codePath, prefixOnly}; !reflect.DeepEqual(got, want) {
		t.Errorf("PathsOutsideRunRecords() = %q; want %q", got, want)
	}
}

// containsConfigKey reports whether text contains a "<key>:" line-start token, the shape every one
// of this template's keys takes.
func containsConfigKey(text, key string) bool {
	needle := key + ":"
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), needle) {
			return true
		}
	}
	return false
}

// TestMigrateConfig_RetiresSkipMergeReview pins what MigrateConfig rewrites for skip_merge_review and removes for fan_review: an old-schema document, recognised by its skip_merge_review key, has each zero per-perch budget turned to null and skip_merge_review true turned to merge_max_bounces: 0 before the key goes;
// a document without the key keeps its zeros as the zero-rounds meaning; a second run changes nothing, a below-floor step_idle_timeout_min is still raised, and an unparseable document is refused.
func TestMigrateConfig_RetiresSkipMergeReview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		document  string
		wantLines []string
		wantKeys  map[string]any
		wantGone  []string
	}{
		{
			name:      "an old-schema zero becomes null and the skip key goes",
			document:  "skip_merge_review: false\nplan_max_bounces: 0\nwebster_max_bounces: 2\n",
			wantLines: []string{"plan_max_bounces: 0 rewritten to null, which takes review_max_bounces", "skip_merge_review: removed"},
			wantKeys:  map[string]any{"plan_max_bounces": nil, "webster_max_bounces": 2},
			wantGone:  []string{"skip_merge_review"},
		},
		{
			name:      "skip_merge_review true becomes a merge budget of 0",
			document:  "skip_merge_review: true\nmerge_max_bounces: 1\n",
			wantLines: []string{"skip_merge_review: true rewritten to merge_max_bounces: 0", "skip_merge_review: removed"},
			wantKeys:  map[string]any{"merge_max_bounces": 0},
			wantGone:  []string{"skip_merge_review"},
		},
		{
			name:      "skip_merge_review true over a zero merge budget rewrites it twice",
			document:  "skip_merge_review: true\nmerge_max_bounces: 0\n",
			wantLines: []string{"merge_max_bounces: 0 rewritten to null, which takes review_max_bounces", "skip_merge_review: true rewritten to merge_max_bounces: 0", "skip_merge_review: removed"},
			wantKeys:  map[string]any{"merge_max_bounces": 0},
			wantGone:  []string{"skip_merge_review"},
		},
		{
			name:     "a document without skip_merge_review keeps its zero",
			document: "plan_max_bounces: 0\n",
			wantKeys: map[string]any{"plan_max_bounces": 0},
		},
		{
			name:      "fan_review goes with its reconcile line",
			document:  "fan_review: sonnet[high]\nplan_max_bounces: 2\n",
			wantLines: []string{"fan_review: removed; a fanned round's reviewer now runs on its segment's own reviewer list"},
			wantKeys:  map[string]any{"plan_max_bounces": 2},
			wantGone:  []string{"fan_review"},
		},
		{
			name:      "a below-floor step_idle_timeout_min is still raised",
			document:  "step_idle_timeout_min: 0\n",
			wantLines: []string{"step_idle_timeout_min: raised from 0 to 1"},
			wantKeys:  map[string]any{"step_idle_timeout_min": 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, lines, err := MigrateConfig([]byte(tt.document))
			if err != nil {
				t.Fatalf("MigrateConfig() error = %v; want nil", err)
			}
			if !reflect.DeepEqual(lines, tt.wantLines) {
				t.Errorf("MigrateConfig() lines = %q; want %q", lines, tt.wantLines)
			}
			var decoded map[string]any
			if err := yaml.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("decode migrated %q: %v", got, err)
			}
			for key, want := range tt.wantKeys {
				if value, present := decoded[key]; !present || value != want {
					t.Errorf("migrated %s = %v, present %v; want %v", key, value, present, want)
				}
			}
			for _, key := range tt.wantGone {
				if _, present := decoded[key]; present {
					t.Errorf("migrated document still holds %s", key)
				}
			}

			again, againLines, err := MigrateConfig(got)
			if err != nil || len(againLines) != 0 || string(again) != string(got) {
				t.Errorf("second MigrateConfig() = %q, %q, %v; want the same bytes and no lines", again, againLines, err)
			}
		})
	}

	t.Run("an unparseable document is refused", func(t *testing.T) {
		t.Parallel()
		if _, _, err := MigrateConfig([]byte("plan_max_bounces: [\n")); !errors.Is(err, configengine.ErrInvalid) {
			t.Errorf("MigrateConfig() error = %v; want configengine.ErrInvalid", err)
		}
	})
}

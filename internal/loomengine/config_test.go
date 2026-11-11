// config_test.go holds untagged Tier-1 unit tests for loomengine.LoadConfig.
//
// Each test seeds a bare t.TempDir() with only a _lym/config/loom.yaml file, with no real hub and no git spawn.
// configengine.Load's env-source build tolerates a missing .env.
// A live fabric fixture would need an integration tag.
// The plain "go test ./internal/loomengine/" run leaves it out.

package loomengine

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// seedLoomConfig creates <baseDir>/_lym/config/loom.yaml with the given contents.
func seedLoomConfig(t *testing.T, baseDir, contents string) {
	t.Helper()
	configDir := filepath.Join(baseDir, "_lym", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	cfgPath := filepath.Join(configDir, "loom.yaml")
	if err := os.WriteFile(cfgPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// malformedLensesYAML is a lenses.yaml that fails to parse, for tests that show when LoadConfig reads the file.
const malformedLensesYAML = "fans: [unclosed\n"

// seedLensesConfig creates <baseDir>/_lym/config/lenses.yaml with the given contents.
func seedLensesConfig(t *testing.T, baseDir, contents string) {
	t.Helper()
	cfgPath := filepath.Join(baseDir, "_lym", "config", "lenses.yaml")
	if err := os.WriteFile(cfgPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// writeLoomConfigWithKey seeds a loom.yaml from the shipped template with one key's value replaced.
// It lets a test vary a single knob without restating the whole file.
// The key set must match the template, since configengine.Load is strict about it.
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

// templateDefaults maps a Config field name to the value the shipped template's own line for that key loads to.
// It pins how the template's values parse; a key added to the template needs no entry here.
var templateDefaults = map[string]any{
	"Discussion":                    "opus[medium]",
	"DiscussionTimeoutMin":          480,
	"DiscussionInteractive":         false,
	"Raddle":                        "opus[high]",
	"RaddleAutoApprove":             false,
	"ReviewerModel":                 burlerengine.ModelSpecList{"opus[medium]"},
	"FixerModel":                    burlerengine.ModelSpecList{"opus[medium]"},
	"DiscussionLenses":              "twill-discussion",
	"DiscussionGateFixer":           "sonnet[medium]",
	"SubagentModel":                 "sonnet",
	"DiscussionReviewerModel":       burlerengine.ModelSpecList{"opus[medium]", "opus[medium]", "sonnet[high]"},
	"DiscussionFixerModel":          burlerengine.ModelSpecList{""},
	"CodeReviewerModel":             burlerengine.ModelSpecList{"sonnet[medium]"},
	"CodeFixerModel":                burlerengine.ModelSpecList{"sonnet[medium]"},
	"SplitReviewerModel":            burlerengine.ModelSpecList{""},
	"SplitFixerModel":               burlerengine.ModelSpecList{""},
	"SplitReviewerFan":              lenses.FanList{""},
	"SplitFixerFan":                 lenses.FanList{""},
	"DiscussionReviewerFan":         lenses.FanList{"review-discussion", ""},
	"CodeReviewerFan":               lenses.FanList{"review-code", ""},
	"DiscussionFixerFan":            lenses.FanList{"fix-discussion"},
	"CodeFixerFan":                  lenses.FanList{"fix-code"},
	"DiscussionAssistReviewerModel": burlerengine.ModelSpecList{""},
	"CodeAssistReviewerModel":       burlerengine.ModelSpecList{""},
	"DiscussionAssistReviewerFan":   lenses.FanList{""},
	"CodeAssistReviewerFan":         lenses.FanList{""},
	"AssistReviewerTimeoutMin":      60,
	"Judge":                         "sonnet[medium]",
	"ReviewTimeoutMin":              240,
	"Friction":                      "sonnet[medium]",
	"FrictionTimeoutMin":            30,
	"Driver":                        "haiku[medium]",
	"StepIdleTimeoutMin":            60,
	"ReviewCirclingCheckpoint":      3,
	"ReviewMaxBounces":              3,
	"ReviewHardMaxBounces":          4,
	"ReviewAmendMaxBounces":         1,
	"DiscussionMaxBounces":          intPtr(2),
	"CodeMaxBounces":                intPtr(1),
	"SplitMaxBounces":               intPtr(2),
	"DiscussionAutoAccept":          false,
	"CodeAutoAccept":                true,

	"DiscussionReviewAskParent":        false,
	"DiscussionReviewParentTimeoutMin": 30,
}

// loadTemplateConfig returns the Config the shipped template loads to, read from an anchor that holds only the template.
func loadTemplateConfig(t *testing.T) Config {
	t.Helper()
	baseDir := t.TempDir()
	seedLoomConfig(t, baseDir, ConfigTemplate())
	config, err := LoadConfig(configengine.Dirs{Anchor: baseDir}, "loom")
	if err != nil {
		t.Fatalf("LoadConfig(template) = _, %v; want nil error", err)
	}
	return config
}

// TestLoadConfig_Loads verifies each loom.yaml shape that must load.
// Each case names only the fields its file changes, and every other field must equal the template's own load,
// so a key added to the template leaves the table alone.
// Zero timeouts must load as zero, since shuttleengine.Spec reads zero as "defer to shuttle's run_timeout_min",
// so the negative guard must not sweep them up.
func TestLoadConfig_Loads(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	templateLoad := loadTemplateConfig(t)

	t.Run("template defaults", func(t *testing.T) {
		t.Parallel()
		loaded := reflect.ValueOf(templateLoad)
		for _, name := range slices.Sorted(maps.Keys(templateDefaults)) {
			field := loaded.FieldByName(name)
			if !field.IsValid() {
				t.Errorf("Config has no field %q; rename or drop its templateDefaults entry", name)
				continue
			}
			if got, want := field.Interface(), templateDefaults[name]; !reflect.DeepEqual(got, want) {
				t.Errorf("LoadConfig(template).%s = %#v; want %#v", name, got, want)
			}
		}
	})

	// legacyContents is a file seeded before the friction, driver, judge, parent-review and review-budget keys existed.
	const legacyContents = `discussion: opus[effort=high]
discussion_timeout_min: 480
discussion_interactive: false
reviewer_model: opus[effort=high]
review_timeout_min: 240
`
	tests := []struct {
		name       string
		contents   string
		values     map[string]string
		lensesYAML string
		mutate     func(*Config)
		wantSoft   map[Perch]int
	}{
		{
			name:       "empty fan keys never read a malformed lenses.yaml",
			lensesYAML: malformedLensesYAML,
			values: map[string]string{
				"discussion_reviewer_fan": `""`, "code_reviewer_fan": `""`,
				"discussion_fixer_fan": `""`, "code_fixer_fan": `""`,
			},
			mutate: func(c *Config) {
				c.DiscussionReviewerFan, c.CodeReviewerFan = lenses.FanList{""}, lenses.FanList{""}
				c.DiscussionFixerFan, c.CodeFixerFan = lenses.FanList{""}, lenses.FanList{""}
			},
		},
		{
			name:     "retired selfreport key ignored",
			contents: ConfigTemplate() + "selfreport: false\n",
			mutate:   func(*Config) {},
		},
		{
			name:   "raddle keys overridden",
			values: map[string]string{"raddle": "sonnet[high]", "raddle_auto_approve": "true"},
			mutate: func(c *Config) { c.Raddle, c.RaddleAutoApprove = "sonnet[high]", true },
		},
		{
			name:   "pr_review_ask_parent overridden",
			values: map[string]string{"pr_review_ask_parent": "true"},
			mutate: func(c *Config) { c.PRReviewAskParent = true },
		},
		{
			name:   "Discussion-Review parent-review keys overridden",
			values: map[string]string{"discussion_review_ask_parent": "true", "discussion_review_parent_timeout_min": "1"},
			mutate: func(c *Config) { c.DiscussionReviewAskParent, c.DiscussionReviewParentTimeoutMin = true, 1 },
		},
		{
			name:   "reviewer_model as a per-round list",
			values: map[string]string{"reviewer_model": "\n  - sonnet[low]\n  - opus[high]"},
			mutate: func(c *Config) { c.ReviewerModel = burlerengine.ModelSpecList{"sonnet[low]", "opus[high]"} },
		},
		{
			name:   "fixer_model as a per-round list",
			values: map[string]string{"fixer_model": "\n  - opus[low]\n  - opus[high]"},
			mutate: func(c *Config) { c.FixerModel = burlerengine.ModelSpecList{"opus[low]", "opus[high]"} },
		},
		{
			name:   "code_fixer_model as one model-spec",
			values: map[string]string{"code_fixer_model": "opus[low]"},
			mutate: func(c *Config) { c.CodeFixerModel = burlerengine.ModelSpecList{"opus[low]"} },
		},
		{
			name:   "empty segment list takes the shared list",
			values: map[string]string{"discussion_fixer_model": "[]"},
			mutate: func(c *Config) { c.DiscussionFixerModel = burlerengine.ModelSpecList{} },
		},
		{
			name:   "each fan key as one fan name",
			values: map[string]string{"discussion_reviewer_fan": "standard", "code_reviewer_fan": "review-code", "discussion_fixer_fan": "fix-discussion", "code_fixer_fan": "fix-code"},
			mutate: func(c *Config) {
				c.DiscussionReviewerFan = lenses.FanList{"standard"}
				c.CodeReviewerFan = lenses.FanList{"review-code"}
				c.DiscussionFixerFan = lenses.FanList{"fix-discussion"}
				c.CodeFixerFan = lenses.FanList{"fix-code"}
			},
		},
		{
			name: "each fan key as a per-round list, an empty entry running its round solo",
			values: map[string]string{
				"discussion_reviewer_fan": "\n  - \"\"\n  - review-discussion",
				"code_reviewer_fan":       "\n  - review-code\n  - \"\"",
				"discussion_fixer_fan":    "\n  - \"\"\n  - fix-discussion",
				"code_fixer_fan":          "\n  - fix-code\n  - \"\"",
			},
			mutate: func(c *Config) {
				c.DiscussionReviewerFan = lenses.FanList{"", "review-discussion"}
				c.CodeReviewerFan = lenses.FanList{"review-code", ""}
				c.DiscussionFixerFan = lenses.FanList{"", "fix-discussion"}
				c.CodeFixerFan = lenses.FanList{"fix-code", ""}
			},
		},
		{
			name: "assistant model keys as a per-round list, an empty entry running no assistant that round",
			values: map[string]string{
				"discussion_assist_reviewer_model": `["luna[low]", ""]`,
				"code_assist_reviewer_model":       "\n  - \"\"\n  - luna[high]",
			},
			mutate: func(c *Config) {
				c.DiscussionAssistReviewerModel = burlerengine.ModelSpecList{"luna[low]", ""}
				c.CodeAssistReviewerModel = burlerengine.ModelSpecList{"", "luna[high]"}
			},
		},
		{
			name: "assistant fan keys as one fan name and as a per-round list",
			values: map[string]string{
				"discussion_assist_reviewer_fan": "standard",
				"code_assist_reviewer_fan":       `["review-code", ""]`,
			},
			mutate: func(c *Config) {
				c.DiscussionAssistReviewerFan = lenses.FanList{"standard"}
				c.CodeAssistReviewerFan = lenses.FanList{"review-code", ""}
			},
		},
		{
			name:   "explicit assist_reviewer_timeout_min",
			values: map[string]string{"assist_reviewer_timeout_min": "15"},
			mutate: func(c *Config) { c.AssistReviewerTimeoutMin = 15 },
		},
		{
			name:   "a zero review_timeout_min loads beside the assistant's timeout",
			values: map[string]string{"review_timeout_min": "0", "assist_reviewer_timeout_min": "60"},
			mutate: func(c *Config) { c.ReviewTimeoutMin, c.AssistReviewerTimeoutMin = 0, 60 },
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
			name:   "empty discussion_lenses turns the writer forks off",
			values: map[string]string{"discussion_lenses": `""`},
			mutate: func(c *Config) { c.DiscussionLenses = "" },
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
			name:   "empty discussion_gate_fixer turns the fixer off",
			values: map[string]string{"discussion_gate_fixer": `""`},
			mutate: func(c *Config) { c.DiscussionGateFixer = "" },
		},
		{
			name:   "empty friction turns Tier 2 off",
			values: map[string]string{"friction": `""`},
			mutate: func(c *Config) { c.Friction = "" },
		},
		{
			name: "judge and seed override keys set",
			values: map[string]string{
				"seed": "haiku[low]", "discussion_judge": "opus[low]", "code_judge": "sonnet[low]",
				"discussion_seed": "haiku[medium]", "code_seed": "sonnet[high]",
			},
			mutate: func(c *Config) {
				c.Seed = "haiku[low]"
				c.DiscussionJudge, c.CodeJudge = "opus[low]", "sonnet[low]"
				c.DiscussionSeed, c.CodeSeed = "haiku[medium]", "sonnet[high]"
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
			values: map[string]string{"review_hard_max_bounces": "5", "discussion_hard_max_bounces": "6"},
			mutate: func(c *Config) { c.ReviewHardMaxBounces, c.DiscussionHardMaxBounces = 5, 6 },
		},
		{
			name:   "a hard cap equal to the soft cap loads",
			values: map[string]string{"review_max_bounces": "3", "review_hard_max_bounces": "3"},
			mutate: func(c *Config) { c.ReviewMaxBounces, c.ReviewHardMaxBounces = 3, 3 },
		},
		{
			name: "explicit per-perch budgets and auto-accept flags",
			values: map[string]string{
				"discussion_max_bounces": "1", "code_max_bounces": "4",
				"discussion_auto_accept": "true", "code_auto_accept": "true",
			},
			mutate: func(c *Config) {
				c.DiscussionMaxBounces, c.CodeMaxBounces = intPtr(1), intPtr(4)
				c.DiscussionAutoAccept, c.CodeAutoAccept = true, true
			},
		},
		{
			name:     "a zero per-perch budget loads as zero rounds, not as null",
			values:   map[string]string{"discussion_max_bounces": "0"},
			mutate:   func(c *Config) { c.DiscussionMaxBounces = intPtr(0) },
			wantSoft: map[Perch]int{PerchDiscussion: 0, PerchCode: 1},
		},
		{
			name:     "a zero run-wide budget gives every null perch zero rounds",
			values:   map[string]string{"review_max_bounces": "0"},
			mutate:   func(c *Config) { c.ReviewMaxBounces = 0 },
			wantSoft: map[Perch]int{PerchDiscussion: 2, PerchPlan: 0, PerchCode: 1},
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
				c.Discussion, c.ReviewerModel = "opus[effort=high]", burlerengine.ModelSpecList{"opus[effort=high]"}
			},
		},
		{
			name:   "zero discussion timeout",
			values: map[string]string{"discussion_timeout_min": "0"},
			mutate: func(c *Config) { c.DiscussionTimeoutMin = 0 },
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
			if tt.lensesYAML != "" {
				seedLensesConfig(t, baseDir, tt.lensesYAML)
			}

			want := templateLoad
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
				board:    "reviewer_model: [\"opus[medium]\"]\n",
				worktree: "reviewer_model: [\"sonnet[low]\", \"sonnet[high]\"]\n",
				mutate:   func(c *Config) { c.ReviewerModel = burlerengine.ModelSpecList{"sonnet[low]", "sonnet[high]"} },
			},
			{
				name:       "a retired fan_review in the repo default is ignored",
				boardExtra: "fan_review: sonnet[high]\n",
				mutate:     func(*Config) {},
			},
			{
				name:       "a retired pr_review in the repo default is ignored",
				boardExtra: "pr_review: true\n",
				mutate:     func(*Config) {},
			},
			{
				name:   "a local code_reviewer_fan of \"\" is open to the local layer",
				local:  "code_reviewer_fan: \"\"\n",
				mutate: func(c *Config) { c.CodeReviewerFan = lenses.FanList{""} },
			},
			{
				name:   "a local code_assist_reviewer_model is open to the local layer",
				local:  "code_assist_reviewer_model: \"luna[low]\"\n",
				mutate: func(c *Config) { c.CodeAssistReviewerModel = burlerengine.ModelSpecList{"luna[low]"} },
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

				want := templateLoad
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
// anchor gets an empty _lym/config/ so the layered load sees an initialised worktree.
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

// loomTemplateWith returns the shipped template text with overrides applied.
// Each top-level line of overrides replaces the template's line for the same key.
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

// TestSetDiscussionAdvisors_LoadsBack verifies the discussion_advisors round trip.
// A list of name and model mappings written through configengine.Set must load back as the same advisors.
func TestSetDiscussionAdvisors_LoadsBack(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestLoadConfig_Refuses verifies that each invalid loom.yaml is refused at load time, naming the offending key.
// The refused cases are an ungrammatical model-spec, which would otherwise reach a producer's spawn site;
// a review key below one, refused with the way forward, since zero does not defer to shuttle for it;
// a negative timeout, which would otherwise surface only when the producer it governs first spawns;
// and a set fan key beside an unparseable lenses.yaml.
func TestLoadConfig_Refuses(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const positiveIntegerForward = "set it to a positive integer in loom.yaml"
	tests := []struct {
		name       string
		key        string
		value      string
		wantIn     []string
		lensesYAML string
	}{
		{"malformed discussion spec", "discussion", `"opus[effort"`, nil, ""},
		{"malformed raddle spec", "raddle", `"opus[effort"`, []string{`loom config key "raddle"`}, ""},
		{"empty subagent_model", "subagent_model", `""`, []string{"must not be empty", "lym config loom --set subagent_model=sonnet"}, ""},
		{"malformed reviewer_model spec", "reviewer_model", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed fixer_model spec", "fixer_model", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed later reviewer_model entry", "reviewer_model", "\n  - sonnet[low]\n  - \"opus[effort\"", []string{"entry 2", "a non-empty list of model-specs"}, ""},
		{"empty reviewer_model list", "reviewer_model", "[]", []string{"empty", "a non-empty list of model-specs"}, ""},
		{"empty fixer_model list", "fixer_model", "[]", []string{"empty", "a non-empty list of model-specs"}, ""},
		{"malformed discussion_reviewer_model spec", "discussion_reviewer_model", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed discussion_fixer_model spec", "discussion_fixer_model", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed code_reviewer_model spec", "code_reviewer_model", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed later code_fixer_model entry", "code_fixer_model", "\n  - sonnet[low]\n  - \"opus[effort\"", []string{"entry 2", "a non-empty list of model-specs"}, ""},
		{"malformed discussion_assist_reviewer_model entry", "discussion_assist_reviewer_model", `"opus[effort"`, []string{`loom config key "discussion_assist_reviewer_model" entry 1`, "no assistant that round"}, ""},
		{"malformed later code_assist_reviewer_model entry", "code_assist_reviewer_model", `["luna[low]", "opus[effort"]`, []string{`loom config key "code_assist_reviewer_model" entry 2`, "no assistant that round"}, ""},
		{"malformed entry after an empty one", "discussion_assist_reviewer_model", `["", "opus[effort"]`, []string{"entry 2"}, ""},
		{"unknown discussion_assist_reviewer_fan", "discussion_assist_reviewer_fan", "nosuchfan", []string{`"nosuchfan"`, "known fans: ", "entry 1"}, ""},
		{"negative assist_reviewer_timeout_min", "assist_reviewer_timeout_min", "-1", []string{"must not be negative"}, ""},
		{"unknown discussion_reviewer_fan", "discussion_reviewer_fan", "nosuchfan", []string{`"nosuchfan"`, "known fans: ", "standard", "entry 1", `set the entry to ""`}, ""},
		{"unknown second discussion_fixer_fan entry", "discussion_fixer_fan", "\n  - \"\"\n  - nosuchfan", []string{"entry 2", `"nosuchfan"`, "known fans: ", "define the fan in lenses.yaml", `set the entry to ""`}, ""},
		{"empty discussion_reviewer_fan list", "discussion_reviewer_fan", "[]", []string{"the list is empty", `write ""`}, ""},
		{"empty discussion_fixer_fan list", "discussion_fixer_fan", "[]", []string{"the list is empty", `write ""`}, ""},
		{"unknown code_reviewer_fan", "code_reviewer_fan", "nosuchfan", []string{`loom config key "code_reviewer_fan" entry 1`, `"nosuchfan"`, "known fans: ", `set the entry to ""`}, ""},
		{"mapping code_fixer_fan value", "code_fixer_fan", "\n  model: opus", []string{`loom config key "code_fixer_fan"`, "neither a fan name nor a list of fan names"}, ""},
		{"mapping discussion_fixer_fan value", "discussion_fixer_fan", "\n  model: opus", []string{"line ", "neither a fan name nor a list of fan names", "a list of fan names"}, ""},
		{"set discussion_reviewer_fan with a malformed lenses.yaml", "discussion_reviewer_fan", "standard", []string{"lenses: config file ", "parse doc YAML", "fix lenses.yaml, or set the key to empty"}, malformedLensesYAML},
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
		{"mapping reviewer_model value", "reviewer_model", "\n  model: opus", []string{"a non-empty list of model-specs"}, ""},
		{"mapping inside a fixer_model list", "fixer_model", "\n  - model: opus", []string{"a non-empty list of model-specs"}, ""},
		{"malformed judge spec", "judge", `"sonnet[medium"`, nil, ""},
		{"malformed seed spec", "seed", `"opus[effort"`, nil, ""},
		{"malformed discussion_judge spec", "discussion_judge", `"opus[effort"`, nil, ""},
		{"malformed code_judge spec", "code_judge", `"opus[effort"`, nil, ""},
		{"malformed discussion_seed spec", "discussion_seed", `"opus[effort"`, nil, ""},
		{"malformed code_seed spec", "code_seed", `"opus[effort"`, nil, ""},
		{"malformed discussion_gate_fixer spec", "discussion_gate_fixer", `"opus[effort"`, []string{"lym config loom"}, ""},
		{"malformed friction spec", "friction", `"opus[effort"`, nil, ""},
		{"malformed driver spec", "driver", `"opus[effort"`, nil, ""},
		{"zero review_circling_checkpoint", "review_circling_checkpoint", "0", []string{positiveIntegerForward}, ""},
		{"negative review_circling_checkpoint", "review_circling_checkpoint", "-1", []string{positiveIntegerForward}, ""},
		{"zero discussion_review_parent_timeout_min", "discussion_review_parent_timeout_min", "0", []string{"must be at least 1", positiveIntegerForward}, ""},
		{"negative discussion_review_parent_timeout_min", "discussion_review_parent_timeout_min", "-1", []string{"must be at least 1", positiveIntegerForward}, ""},
		{"negative review_max_bounces", "review_max_bounces", "-1", []string{"must not be negative", "set it to 0 for no review round, or to a positive integer"}, ""},
		{"negative discussion_max_bounces", "discussion_max_bounces", "-1", []string{"must not be negative", "set it to 0 for no review round, to a positive integer, or to null to take review_max_bounces"}, ""},
		{"negative code_max_bounces", "code_max_bounces", "-1", []string{"must not be negative", "or to null to take review_max_bounces"}, ""},
		{"negative review_amend_max_bounces", "review_amend_max_bounces", "-1", []string{"must not be negative", "set it to 0 to settle every amendment with no review round", "lym config loom --set review_amend_max_bounces=<n>", `lym loom resume`}, ""},
		{"zero review_hard_max_bounces", "review_hard_max_bounces", "0", []string{"must be at least 1", "lym config loom --set review_hard_max_bounces=<n>", `lym loom resume`}, ""},
		{"negative discussion_hard_max_bounces", "discussion_hard_max_bounces", "-1", []string{"set it to 0 to take review_hard_max_bounces", "lym config loom --set discussion_hard_max_bounces=<n>", `lym loom resume`}, ""},
		{"negative code_hard_max_bounces", "code_hard_max_bounces", "-1", []string{"set it to 0 to take review_hard_max_bounces", "lym config loom --set code_hard_max_bounces=<n>", `lym loom resume`}, ""},
		{"negative split_max_bounces", "split_max_bounces", "-1", []string{"must not be negative", "or to null to take review_max_bounces"}, ""},
		{"negative split_hard_max_bounces", "split_hard_max_bounces", "-1", []string{"set it to 0 to take review_hard_max_bounces", "lym config loom --set split_hard_max_bounces=<n>", `lym loom resume`}, ""},
		{"review_max_bounces above the default hard cap", "review_max_bounces", "5", []string{"review_hard_max_bounces", "lym config loom --set review_hard_max_bounces=<n>", "in the task worktree", `lym loom resume`}, ""},
		{"code_max_bounces above the default hard cap", "code_max_bounces", "5", []string{"review_hard_max_bounces", "code_max_bounces"}, ""},
		{"negative discussion timeout", "discussion_timeout_min", "-1", []string{"must not be negative"}, ""},
		{"negative review timeout", "review_timeout_min", "-1", []string{"must not be negative"}, ""},
		{"negative friction timeout", "friction_timeout_min", "-1", []string{"must not be negative"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeLoomConfigWithKey(t, dir, tt.key, tt.value)
			if tt.lensesYAML != "" {
				seedLensesConfig(t, dir, tt.lensesYAML)
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

	t.Run("a local fan key is refused naming the local file and the key", func(t *testing.T) {
		t.Parallel()
		for _, key := range []string{"discussion_reviewer_fan", "discussion_fixer_fan", "code_fixer_fan"} {
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

	t.Run("a shed key is refused in a local file naming the file, the key and shed.yaml, and dropped from a hub base file", func(t *testing.T) {
		t.Parallel()
		for _, key := range shedOnlyKeys {
			t.Run(key, func(t *testing.T) {
				t.Parallel()
				board, anchor := t.TempDir(), t.TempDir()
				seedLoomConfig(t, board, ConfigTemplate()+key+": 1\n")
				if _, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "loom"); err != nil {
					t.Errorf("LoadConfig() with %s in the hub base file = %v; want nil, the value dropped", key, err)
				}

				localPath := writeLocalLoomConfig(t, anchor, key+": 1\n")
				_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "loom")
				if !errors.Is(err, configengine.ErrInvalid) {
					t.Fatalf("LoadConfig() with %s in a local file error = %v; want ErrInvalid", key, err)
				}
				for _, want := range []string{localPath, key, "shed.yaml"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
					}
				}
			})
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
		modelsPath := filepath.Join(dir, "_lym", "config", "models.yaml")
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
				wantIn: []string{"lym config loom --layer local --set review_max_bounces=<n>", "lym board upsert", "lym config loom --layer default --set review_hard_max_bounces=<n>", "prime"},
			},
			{
				name:     "a worktree hard cap under the board default's soft cap",
				worktree: "review_hard_max_bounces: 2\n",
				wantIn:   []string{"lym config loom --set review_hard_max_bounces=<n>", "in the task worktree"},
			},
			{
				name:   "a local hard cap of zero",
				local:  "review_hard_max_bounces: 0\n",
				wantIn: []string{"must be at least 1", "lym config loom --layer local --set review_hard_max_bounces=<n>", "lym board upsert", `lym loom resume`},
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

}

// TestLoadConfig_NotInitialized verifies uninitialized baseDir yields recovery hint.
func TestLoadConfig_NotInitialized(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	baseDir := t.TempDir()

	_, err := LoadConfig(configengine.Dirs{Anchor: baseDir}, "loom")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for uninitialized baseDir")
	}
	want := `not initialized here; run "lym fabric reconcile"; a session lym refuses the verb from reports status: FAILED and the orch runs it`
	if err.Error() != want {
		t.Errorf("LoadConfig() error = %q; want %q", err.Error(), want)
	}
}

// oldFormatLoomYAML returns the shipped template with its lens keys dropped and producerLines appended.
// advisorsLine, when non-empty, replaces the discussion_advisors line.
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

// TestMigrateConfig pins the loom.yaml migration.
// Each old producer value becomes its lens key, and discussion_advisors is rewritten or emptied to match.
// Comments and other keys are carried, and the step_idle_timeout_min floor still applies.
// A migrated or producer-free document is returned unchanged.
// A migrated stack of layers loads with the advisors each layer's migration left.
func TestMigrateConfig(t *testing.T) {
	tiergate.Require(t, 0)
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
		if want := (DiscussionAdvisorList{{Name: "advisor-1", Model: "sonnet[high]"}}); !reflect.DeepEqual(got.DiscussionAdvisors, want) || got.DiscussionLenses != "" {
			t.Errorf("LoadConfig(migrated hub file) advisors/lenses = %v/%q; want %v and no discussion lenses", got.DiscussionAdvisors, got.DiscussionLenses, want)
		}

		board, anchor := t.TempDir(), t.TempDir()
		seedLoomConfig(t, board, string(hub))
		seedLoomConfig(t, anchor, string(worktree))
		got, err = LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "loom")
		if err != nil {
			t.Fatalf("LoadConfig(migrated stack) = _, %v; want nil error", err)
		}
		if got.DiscussionAdvisors != nil || got.DiscussionLenses != "twill-discussion" {
			t.Errorf("LoadConfig(migrated stack) advisors/lenses = %v/%q; want no advisors and twill-discussion", got.DiscussionAdvisors, got.DiscussionLenses)
		}
	})
}

// TestLoomDriverLogAndBootstrapLock covers LoomDriverLog and LoomBootstrapLock.
// It runs each at an unanchored and at a subpath-anchored *lymcwd.Location, hand-built rather than spawned.
//
// It also checks that LoomBootstrapLock differs from shedrun's StatusLock and RunLock.
// Each is computed for the same location and shedrun.SelfRunID.
//
// LoomScratchDir names the directory the driver log and the bootstrap lock share, so the three never drift apart.
// That directory is loom's own scratch tree under .lym/loom/.
// It is distinct from the run directory's .lym/shed/<runID>/ mirror, which shedrun.ScratchDir names.
func TestLoomDriverLogAndBootstrapLock(t *testing.T) {
	tiergate.Require(t, 0)
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
			l := &lymcwd.Location{
				HubPath:      filepath.Join("home", "user", "repo-LYMHUB"),
				WorktreeName: "repo",
				AnchorRel:    tt.anchorRel,
			}

			wantDriverLog := filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, "loom", "driver.log")
			if got := LoomDriverLog(l); got != wantDriverLog {
				t.Errorf("LoomDriverLog() = %q; want %q", got, wantDriverLog)
			}

			wantBootstrapLock := filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, "loom", "bootstrap.lock")
			if got := LoomBootstrapLock(l); got != wantBootstrapLock {
				t.Errorf("LoomBootstrapLock() = %q; want %q", got, wantBootstrapLock)
			}

			// Three distinct lock files is a correctness property, not a coincidence:
			// LoomBootstrapLock must never collide with the per-persist status lock or the whole-run lock.
			// Both of those live on shedrun's run directory.
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

// TestLandingAndApprovalAccessors pins the landing directory, its commit-pathspec form and the decision record paths.
// The location is hand-built.
func TestLandingAndApprovalAccessors(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	l := &lymcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYMHUB"),
		WorktreeName: "repo",
	}

	if got, want := LandingDirRel(), filepath.Join("_lym", "landing"); got != want {
		t.Errorf("LandingDirRel() = %q; want %q", got, want)
	}
	if got, want := LandingDir(l), filepath.Join(l.AnchorPath(), "_lym", "landing"); got != want {
		t.Errorf("LandingDir() = %q; want %q", got, want)
	}
	if got, want := BeamLandingPath(l), filepath.Join(l.AnchorPath(), "_lym", "landing", "beam-landing.json"); got != want {
		t.Errorf("BeamLandingPath() = %q; want %q", got, want)
	}
	if got, want := BeamLandingLockPath(l), filepath.Join(l.AnchorPath(), ".lym", "locks", "_lym", "landing", "beam-landing.json.lock"); got != want || got == BeamLandingPath(l)+".lock" {
		t.Errorf("BeamLandingLockPath() = %q; want %q, never beside the tracked record", got, want)
	}
	if got, want := LoomReworkCoveragePath(l), filepath.Join(l.AnchorPath(), ".lym", "loom", "rework-coverage.md"); got != want {
		t.Errorf("LoomReworkCoveragePath() = %q; want %q", got, want)
	}
	if got, want := LoomReworkDirRel(), filepath.Join("_lym", "loom", "rework"); got != want {
		t.Errorf("LoomReworkDirRel() = %q; want %q", got, want)
	}
	if got, want := LoomReworkDir(l), filepath.Join(l.AnchorPath(), "_lym", "loom", "rework"); got != want {
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

// TestConfigTemplate_ContainsEveryConfigYAMLTag walks the fields of Config and TapestryConfig via reflection.
// It asserts every yaml tag appears in its module's template text, so a field without a template line fails here.
// A struct field with no matching template key is a silent hole, not a load error.
// This check guards against that.
//
//testtiming:keep pins that every Config and TapestryConfig yaml tag has a template line, a reflection check no LoadConfig case makes
//lym:seam internal/loomengine#Config
//lym:seam internal/loomengine#TapestryConfig
func TestConfigTemplate_ContainsEveryConfigYAMLTag(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	for _, tt := range []struct {
		name     string
		template string
		typ      reflect.Type
	}{
		{"Config", ConfigTemplate(), reflect.TypeOf(Config{})},
		{"TapestryConfig", TapestryConfigTemplate(), reflect.TypeOf(TapestryConfig{})},
	} {
		for i := 0; i < tt.typ.NumField(); i++ {
			tag := tt.typ.Field(i).Tag.Get("yaml")
			if tag == "" {
				t.Fatalf("%s field %q has no yaml tag", tt.name, tt.typ.Field(i).Name)
			}
			// A "-" tag marks a Config field that tapestry.yaml supplies, checked through TapestryConfig.
			if tag == "-" {
				continue
			}
			if !containsConfigKey(tt.template, tag) {
				t.Errorf("the %s template does not contain key %q for field %q", tt.name, tag, tt.typ.Field(i).Name)
			}
		}
	}
}

// TestConfigTemplate_ImplementerDefaultsToWebsterAndAcceptsALocalValue pins the implementer key.
// No config struct carries it, since the recipe reads it where it builds the Implementer row.
// tapestry.yaml's template defaults it to webster.
// TapestryConfigSpec leaves it unguarded, so the local layer may set it.
func TestConfigTemplate_ImplementerDefaultsToWebsterAndAcceptsALocalValue(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	var template map[string]any
	if err := yaml.Unmarshal([]byte(TapestryConfigTemplate()), &template); err != nil {
		t.Fatalf("parse TapestryConfigTemplate(): %v", err)
	}
	if got := template["implementer"]; got != "webster" {
		t.Errorf("TapestryConfigTemplate() implementer = %v; want webster", got)
	}
	// CategoryNone is the zero Category, so a dropped entry reads the same as a declared one unless presence is checked.
	got, declared := TapestryConfigSpec().Categories["implementer"]
	if !declared || got != configengine.CategoryNone {
		t.Errorf("TapestryConfigSpec() category of implementer = %v, declared %v; want %v, true", got, declared, configengine.CategoryNone)
	}
}

// TestPathsOutsideRunRecords pins what PathsOutsideRunRecords drops:
// every path under the run directory, the loom durable directory or the reviews directory.
// It keeps a code path and a path that only shares a records directory's name as a prefix.
// Every file of a raddle run is kept too, since none of those directories holds it.
func TestPathsOutsideRunRecords(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	l := &lymcwd.Location{HubPath: filepath.Join("home", "user", "repo-LYMHUB"), WorktreeName: "repo", AnchorRel: "."}
	runID := shedrun.SelfRunID
	runDir := shedrun.RunDir(l, runID)
	codePath := filepath.Join(l.AnchorPath(), "internal", "x.go")
	prefixOnly := LoomDurableDir(l) + "-extra"
	raddlePaths := []string{
		RaddleProposal(l), RaddleColdProposal(l), RaddleReturnedProposal(l), RaddleApproval(l),
		RaddleStatePath(l), RaddleStageDir(l), RaddlePartRecord(l, "part-a"),
	}

	got := PathsOutsideRunRecords(l, runID, append([]string{
		filepath.Join(runDir, "status.json"),
		filepath.Join(LoomDurableDir(l), "friction", "note.md"),
		filepath.Join(LoomReviewsDir(l), "round.yaml"),
		codePath,
		prefixOnly,
	}, raddlePaths...))

	if want := append([]string{codePath, prefixOnly}, raddlePaths...); !reflect.DeepEqual(got, want) {
		t.Errorf("PathsOutsideRunRecords() = %q; want %q", got, want)
	}
}

// containsConfigKey reports whether a line of text starts with "<key>:", the form each config template key takes.
func containsConfigKey(text, key string) bool {
	needle := key + ":"
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), needle) {
			return true
		}
	}
	return false
}

// TestMigrateConfig_RetiredKeys pins how MigrateConfig handles the retired config keys.
// An old-schema document, recognised by its skip_merge_review key, has each zero per-perch budget turned to null.
// Its skip_merge_review true is turned to merge_max_bounces: 0, and then the key goes.
// A document without the key keeps its zeros as the zero-rounds meaning.
// A retired model or fan key takes its new name and the template's line comment unless the new name is already set.
// Each webster_* review-perch key then takes its code_* name.
// Webster's retired models are copied onto unset Merge keys, and every Merge-Review key is then dropped.
// The fan_review key is removed.
// A second run changes nothing.
// A below-floor step_idle_timeout_min is still raised, and an unparseable document is refused.
func TestMigrateConfig_RetiredKeys(t *testing.T) {
	tiergate.Require(t, 0)
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
			wantLines: []string{"plan_max_bounces: 0 rewritten to null, which takes review_max_bounces", "skip_merge_review: removed", "webster_max_bounces: renamed to code_max_bounces"},
			wantKeys:  map[string]any{"plan_max_bounces": nil, "code_max_bounces": 2},
			wantGone:  []string{"skip_merge_review", "webster_max_bounces"},
		},
		{
			name:      "skip_merge_review true runs through to no merge key at all",
			document:  "skip_merge_review: true\nmerge_max_bounces: 1\n",
			wantLines: []string{"skip_merge_review: true rewritten to merge_max_bounces: 0", "skip_merge_review: removed", "merge_max_bounces: retired, removed"},
			wantGone:  []string{"skip_merge_review", "merge_max_bounces"},
		},
		{
			name: "each retired merge key is dropped",
			document: "merge_reviewer_model: sonnet[low]\nmerge_fixer_model: sonnet[low]\nmerge_reviewer_fan: review-code\nmerge_fixer_fan: fix-code\n" +
				"merge_max_bounces: 1\nmerge_hard_max_bounces: 0\nmerge_auto_accept: true\nplan_max_bounces: 2\n",
			wantLines: []string{
				"merge_reviewer_model: retired, removed",
				"merge_fixer_model: retired, removed",
				"merge_reviewer_fan: retired, removed",
				"merge_fixer_fan: retired, removed",
				"merge_max_bounces: retired, removed",
				"merge_hard_max_bounces: retired, removed",
				"merge_auto_accept: retired, removed",
			},
			wantKeys: map[string]any{"plan_max_bounces": 2},
			wantGone: []string{
				"merge_reviewer_model", "merge_fixer_model", "merge_reviewer_fan", "merge_fixer_fan",
				"merge_max_bounces", "merge_hard_max_bounces", "merge_auto_accept",
			},
		},
		{
			name:      "the retired rework key is dropped",
			document:  "rework: opus[low]\nplan_max_bounces: 2\n",
			wantLines: []string{"rework: retired, removed"},
			wantKeys:  map[string]any{"plan_max_bounces": 2},
			wantGone:  []string{"rework"},
		},
		{
			name:      "the retired alignment_pr_approve key is dropped",
			document:  "alignment_pr_approve: false\nplan_max_bounces: 2\n",
			wantLines: []string{"alignment_pr_approve: retired, removed"},
			wantKeys:  map[string]any{"plan_max_bounces": 2},
			wantGone:  []string{"alignment_pr_approve"},
		},
		{
			name:     "each retired alignment and parent-review key is dropped",
			document: "alignment: opus[high]\nalignment_timeout_min: 20\nskip_alignment: true\nparent_review_wait_min: 60\nplan_max_bounces: 2\n",
			wantLines: []string{
				"alignment: retired, removed",
				"alignment_timeout_min: retired, removed",
				"skip_alignment: retired, removed",
				"parent_review_wait_min: retired, removed",
			},
			wantKeys: map[string]any{"plan_max_bounces": 2},
			wantGone: []string{"alignment", "alignment_timeout_min", "skip_alignment", "parent_review_wait_min"},
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
		{
			name:     "the retired reviewer, fixer and fan keys are renamed with their values",
			document: "review: opus[high]\nfix: sonnet[low]\ndiscussion_review: \"\"\nplan_fix: opus[low]\ndiscussion_fan: standard\nmerge_fix_fan: fix-code\n",
			wantLines: []string{
				"review: renamed to reviewer_model",
				"fix: renamed to fixer_model",
				"discussion_review: renamed to discussion_reviewer_model",
				"plan_fix: renamed to plan_fixer_model",
				"discussion_fan: renamed to discussion_reviewer_fan",
				"merge_fix_fan: renamed to merge_fixer_fan",
				"merge_fixer_fan: retired, removed",
			},
			wantKeys: map[string]any{
				"reviewer_model": "opus[high]", "fixer_model": "sonnet[low]", "discussion_reviewer_model": "", "plan_fixer_model": "opus[low]",
				"discussion_reviewer_fan": "standard",
			},
			wantGone: []string{"review", "fix", "discussion_review", "plan_fix", "discussion_fan", "merge_fix_fan", "merge_fixer_fan"},
		},
		{
			name:     "webster's models are copied onto the unset merge keys and the drop then removes them",
			document: "webster_review: sonnet[medium]\nwebster_fix: sonnet[low]\n",
			wantLines: []string{
				"merge_reviewer_model: set to webster_review's value, the models Merge-Review ran on",
				"merge_fixer_model: set to webster_fix's value, the models Merge-Review ran on",
				"webster_review: renamed to webster_reviewer_model",
				"webster_fix: renamed to webster_fixer_model",
				"webster_reviewer_model: renamed to code_reviewer_model",
				"webster_fixer_model: renamed to code_fixer_model",
				"merge_reviewer_model: retired, removed",
				"merge_fixer_model: retired, removed",
			},
			wantKeys: map[string]any{"code_reviewer_model": "sonnet[medium]", "code_fixer_model": "sonnet[low]"},
			wantGone: []string{"webster_review", "webster_fix", "webster_reviewer_model", "webster_fixer_model", "merge_reviewer_model", "merge_fixer_model"},
		},
		{
			name:     "a set new webster key is the value copied onto merge before the drop",
			document: "webster_review: opus[high]\nwebster_reviewer_model: sonnet[low]\n",
			wantLines: []string{
				"merge_reviewer_model: set to webster_reviewer_model's value, the models Merge-Review ran on",
				"webster_review: removed, webster_reviewer_model already set",
				"webster_reviewer_model: renamed to code_reviewer_model",
				"merge_reviewer_model: retired, removed",
			},
			wantKeys: map[string]any{"code_reviewer_model": "sonnet[low]"},
			wantGone: []string{"webster_review", "webster_reviewer_model", "merge_reviewer_model"},
		},
		{
			name:      "a set merge key is dropped with the rest",
			document:  "webster_review: sonnet[medium]\nmerge_reviewer_model: opus[high]\n",
			wantLines: []string{"webster_review: renamed to webster_reviewer_model", "webster_reviewer_model: renamed to code_reviewer_model", "merge_reviewer_model: retired, removed"},
			wantKeys:  map[string]any{"code_reviewer_model": "sonnet[medium]"},
			wantGone:  []string{"webster_review", "webster_reviewer_model", "merge_reviewer_model", "merge_fixer_model"},
		},
		{
			name:     "each webster review-perch key is renamed to its code successor with its value",
			document: "webster_reviewer_model: sonnet[low]\nwebster_fixer_model: sonnet[high]\nwebster_reviewer_fan: review-code\nwebster_fixer_fan: fix-code\nwebster_judge: opus[low]\nwebster_seed: haiku[low]\nwebster_max_bounces: 2\nwebster_hard_max_bounces: 5\nwebster_auto_accept: false\n",
			wantLines: []string{
				"webster_reviewer_model: renamed to code_reviewer_model",
				"webster_fixer_model: renamed to code_fixer_model",
				"webster_reviewer_fan: renamed to code_reviewer_fan",
				"webster_fixer_fan: renamed to code_fixer_fan",
				"webster_judge: renamed to code_judge",
				"webster_seed: renamed to code_seed",
				"webster_max_bounces: renamed to code_max_bounces",
				"webster_hard_max_bounces: renamed to code_hard_max_bounces",
				"webster_auto_accept: renamed to code_auto_accept",
			},
			wantKeys: map[string]any{
				"code_reviewer_model": "sonnet[low]", "code_fixer_model": "sonnet[high]", "code_reviewer_fan": "review-code", "code_fixer_fan": "fix-code",
				"code_judge": "opus[low]", "code_seed": "haiku[low]", "code_max_bounces": 2, "code_hard_max_bounces": 5, "code_auto_accept": false,
			},
			wantGone: []string{
				"webster_reviewer_model", "webster_fixer_model", "webster_reviewer_fan", "webster_fixer_fan", "webster_judge",
				"webster_seed", "webster_max_bounces", "webster_hard_max_bounces", "webster_auto_accept",
			},
		},
		{
			name:      "a set code key wins over its webster one",
			document:  "webster_max_bounces: 2\ncode_max_bounces: 4\n",
			wantLines: []string{"webster_max_bounces: removed, code_max_bounces already set"},
			wantKeys:  map[string]any{"code_max_bounces": 4},
			wantGone:  []string{"webster_max_bounces"},
		},
		{
			name:      "a set new key wins over its retired one",
			document:  "review: opus[high]\nreviewer_model: sonnet[low]\n",
			wantLines: []string{"review: removed, reviewer_model already set"},
			wantKeys:  map[string]any{"reviewer_model": "sonnet[low]"},
			wantGone:  []string{"review"},
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

	t.Run("a renamed key carries the template's line comment", func(t *testing.T) {
		t.Parallel()
		got, _, err := MigrateConfig([]byte("webster_fan: review-code # the retired comment\n"))
		if err != nil {
			t.Fatalf("MigrateConfig() error = %v; want nil", err)
		}
		var document, template yaml.Node
		if err := yaml.Unmarshal(got, &document); err != nil {
			t.Fatalf("decode migrated %q: %v", got, err)
		}
		if err := yaml.Unmarshal([]byte(ConfigTemplate()), &template); err != nil {
			t.Fatalf("decode template: %v", err)
		}
		const key = "code_reviewer_fan"
		if got, want := mappingValueNode(document.Content[0], key).LineComment, mappingValueNode(template.Content[0], key).LineComment; got != want {
			t.Errorf("migrated %s line comment = %q; want the template's %q", key, got, want)
		}
	})

	t.Run("an unparseable document is refused", func(t *testing.T) {
		t.Parallel()
		if _, _, err := MigrateConfig([]byte("plan_max_bounces: [\n")); !errors.Is(err, configengine.ErrInvalid) {
			t.Errorf("MigrateConfig() error = %v; want configengine.ErrInvalid", err)
		}
	})
}

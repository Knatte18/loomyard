// config_test.go holds untagged Tier-1 unit tests for loomengine.LoadConfig.
//
// Each test seeds a bare t.TempDir() with only a _lym/config/loom.yaml file, with no real hub and no git spawn.
// configengine.Load's env-source build tolerates a missing .env.
// A live fabric fixture would need an integration tag.
// The plain "go test ./internal/loomengine/" run leaves it out.

package loomengine

import (
	"errors"
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

// writeLoomConfigWithKey seeds a loom.yaml from the shipped template with one dotted path's value replaced.
// It lets a test vary a single knob without restating the whole file.
// The path must be one the template holds, since configengine.Load is strict about it.
func writeLoomConfigWithKey(t *testing.T, baseDir, path, value string) {
	t.Helper()
	seedLoomConfig(t, baseDir, loomTemplateWith(t, map[string]string{path: value}))
}

// loomTemplateWith returns the shipped template with each given dotted path's value replaced by its YAML text.
func loomTemplateWith(t *testing.T, values map[string]string) string {
	t.Helper()
	return templateWith(t, ConfigTemplate(), values)
}

// templateWith returns template with the value at each dotted path replaced by the YAML text given for it.
// A path the template lacks fails the test, since the fixture would silently test nothing.
func templateWith(t *testing.T, template string, values map[string]string) string {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(template), &document); err != nil {
		t.Fatalf("parse the template: %v", err)
	}
	for path, value := range values {
		node := document.Content[0]
		for _, step := range strings.Split(path, ".") {
			var next *yaml.Node
			for i := 0; i+1 < len(node.Content); i += 2 {
				if node.Content[i].Value == step {
					next = node.Content[i+1]
				}
			}
			if next == nil {
				t.Fatalf("path %q is not in the template", path)
			}
			node = next
		}
		var replacement yaml.Node
		if err := yaml.Unmarshal([]byte("key: "+value), &replacement); err != nil {
			t.Fatalf("parse the value %q of %q: %v", value, path, err)
		}
		*node = *replacement.Content[0].Content[1]
	}
	contents, err := yaml.Marshal(&document)
	if err != nil {
		t.Fatalf("marshal the template: %v", err)
	}
	return string(contents)
}

// requireMovesIntoTemplate fails unless template holds every new path of moves and no old name.
// An old name that is also the area its new path lies under is held as that area's mapping.
func requireMovesIntoTemplate(t *testing.T, moves map[string]string, template string) {
	t.Helper()
	var document map[string]any
	if err := yaml.Unmarshal([]byte(template), &document); err != nil {
		t.Fatalf("parse the template: %v", err)
	}
	if len(moves) == 0 {
		t.Fatal("the move table is empty")
	}
	for oldKey, newPath := range moves {
		area, leaf, nested := strings.Cut(newPath, ".")
		value, held := document[area]
		if nested {
			mapping, isMapping := value.(map[string]any)
			if isMapping {
				_, held = mapping[leaf]
			}
			held = held && isMapping
		}
		if !held {
			t.Errorf("the template lacks %q, the new path of %q", newPath, oldKey)
		}
		if old, stillHeld := document[oldKey]; stillHeld {
			if _, isArea := old.(map[string]any); !isArea || !strings.HasPrefix(newPath, oldKey+".") {
				t.Errorf("the template still holds the old key %q", oldKey)
			}
		}
	}
}

func intPtr(v int) *int { return &v }

// templateDefaults maps a Config field path to the value the shipped template's own line for that key loads to.
// It pins how the template's values parse; a key added to the template needs no entry here.
var templateDefaults = map[string]any{
	"Discussion.Model":                  "opus[medium]",
	"Discussion.TimeoutMin":             480,
	"Discussion.Interactive":            false,
	"Raddle.Model":                      "opus[high]",
	"Raddle.AutoApprove":                false,
	"Review.ReviewerModel":              burlerengine.ModelSpecList{"opus[medium]"},
	"Review.FixerModel":                 burlerengine.ModelSpecList{"opus[medium]"},
	"Discussion.Lenses":                 "twill-discussion",
	"Discussion.GateFixer":              "sonnet[medium]",
	"SubagentModel":                     "sonnet",
	"Discussion.ReviewerModel":          burlerengine.ModelSpecList{"opus[medium]", "opus[medium]", "sonnet[high]"},
	"Discussion.FixerModel":             burlerengine.ModelSpecList{""},
	"Code.ReviewerModel":                burlerengine.ModelSpecList{"sonnet[medium]"},
	"Code.FixerModel":                   burlerengine.ModelSpecList{"sonnet[medium]"},
	"Split.ReviewerModel":               burlerengine.ModelSpecList{""},
	"Split.FixerModel":                  burlerengine.ModelSpecList{""},
	"Split.ReviewerFan":                 lenses.FanList{""},
	"Split.FixerFan":                    lenses.FanList{""},
	"Discussion.ReviewerFan":            lenses.FanList{"review-discussion", ""},
	"Code.ReviewerFan":                  lenses.FanList{"review-code", ""},
	"Discussion.FixerFan":               lenses.FanList{"fix-discussion"},
	"Code.FixerFan":                     lenses.FanList{"fix-code"},
	"Discussion.AssistReviewerModel":    burlerengine.ModelSpecList{""},
	"Code.AssistReviewerModel":          burlerengine.ModelSpecList{""},
	"Discussion.AssistReviewerFan":      lenses.FanList{""},
	"Code.AssistReviewerFan":            lenses.FanList{""},
	"Review.AssistReviewerTimeoutMin":   60,
	"Review.Judge":                      "sonnet[medium]",
	"Review.TimeoutMin":                 240,
	"Friction.Model":                    "sonnet[medium]",
	"Friction.TimeoutMin":               30,
	"Driver":                            "haiku[medium]",
	"StepIdleTimeoutMin":                60,
	"Review.CirclingCheckpoint":         3,
	"Review.StalemateRounds":            2,
	"Review.MaxBounces":                 3,
	"Review.HardMaxBounces":             4,
	"Review.AmendMaxBounces":            1,
	"Discussion.MaxBounces":             intPtr(2),
	"Code.MaxBounces":                   intPtr(1),
	"Split.MaxBounces":                  intPtr(2),
	"Discussion.AutoAccept":             false,
	"Code.AutoAccept":                   true,
	"PR.ReviewSmallMax":                 3,
	"PR.ReviewParentTimeoutMin":         60,
	"Discussion.ReviewAskParent":        false,
	"Discussion.ReviewParentTimeoutMin": 30,
	"Code.ReviewAskParent":              false,
	"Code.ReviewParentTimeoutMin":       30,
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

	t.Run("every moved key's new path is in the template and its old name is gone", func(t *testing.T) {
		t.Parallel()
		requireMovesIntoTemplate(t, loomMovedKeys(), ConfigTemplate())
	})

	t.Run("the move table holds every renamed key", func(t *testing.T) {
		t.Parallel()
		want := map[string]string{
			"discussion":                           "discussion.model",
			"discussion_timeout_min":               "discussion.timeout_min",
			"discussion_interactive":               "discussion.interactive",
			"discussion_advisors":                  "discussion.advisors",
			"discussion_lenses":                    "discussion.lenses",
			"discussion_gate_fixer":                "discussion.gate_fixer",
			"discussion_reviewer_model":            "discussion.reviewer_model",
			"discussion_fixer_model":               "discussion.fixer_model",
			"discussion_reviewer_fan":              "discussion.reviewer_fan",
			"discussion_fixer_fan":                 "discussion.fixer_fan",
			"discussion_assist_reviewer_model":     "discussion.assist_reviewer_model",
			"discussion_assist_reviewer_fan":       "discussion.assist_reviewer_fan",
			"discussion_judge":                     "discussion.judge",
			"discussion_seed":                      "discussion.seed",
			"discussion_max_bounces":               "discussion.max_bounces",
			"discussion_hard_max_bounces":          "discussion.hard_max_bounces",
			"discussion_auto_accept":               "discussion.auto_accept",
			"discussion_review_ask_parent":         "discussion.review_ask_parent",
			"discussion_review_parent_timeout_min": "discussion.review_parent_timeout_min",
			"code_reviewer_model":                  "code.reviewer_model",
			"code_fixer_model":                     "code.fixer_model",
			"code_reviewer_fan":                    "code.reviewer_fan",
			"code_fixer_fan":                       "code.fixer_fan",
			"code_assist_reviewer_model":           "code.assist_reviewer_model",
			"code_assist_reviewer_fan":             "code.assist_reviewer_fan",
			"code_judge":                           "code.judge",
			"code_seed":                            "code.seed",
			"code_max_bounces":                     "code.max_bounces",
			"code_hard_max_bounces":                "code.hard_max_bounces",
			"code_auto_accept":                     "code.auto_accept",
			"code_review_ask_parent":               "code.review_ask_parent",
			"code_review_parent_timeout_min":       "code.review_parent_timeout_min",
			"split_reviewer_model":                 "split.reviewer_model",
			"split_fixer_model":                    "split.fixer_model",
			"split_reviewer_fan":                   "split.reviewer_fan",
			"split_fixer_fan":                      "split.fixer_fan",
			"split_judge":                          "split.judge",
			"split_max_bounces":                    "split.max_bounces",
			"split_hard_max_bounces":               "split.hard_max_bounces",
			"split_auto_accept":                    "split.auto_accept",
			"raddle":                               "raddle.model",
			"raddle_auto_approve":                  "raddle.auto_approve",
			"friction":                             "friction.model",
			"friction_timeout_min":                 "friction.timeout_min",
			"pr_review_ask_parent":                 "pr.review_ask_parent",
			"pr_review_parent_timeout_min":         "pr.review_parent_timeout_min",
			"pr_review_small_max":                  "pr.review_small_max",
			"review_timeout_min":                   "review.timeout_min",
			"review_circling_checkpoint":           "review.circling_checkpoint",
			"review_stalemate_rounds":              "review.stalemate_rounds",
			"review_max_bounces":                   "review.max_bounces",
			"review_hard_max_bounces":              "review.hard_max_bounces",
			"review_amend_max_bounces":             "review.amend_max_bounces",
			"reviewer_model":                       "review.reviewer_model",
			"fixer_model":                          "review.fixer_model",
			"judge":                                "review.judge",
			"seed":                                 "review.seed",
			"assist_reviewer_timeout_min":          "review.assist_reviewer_timeout_min",
		}
		if got := loomMovedKeys(); !maps.Equal(got, want) {
			t.Errorf("loomMovedKeys() = %v; want %v", got, want)
		}
	})

	t.Run("template defaults", func(t *testing.T) {
		t.Parallel()
		for _, path := range slices.Sorted(maps.Keys(templateDefaults)) {
			field := reflect.ValueOf(templateLoad)
			for _, name := range strings.Split(path, ".") {
				if field = field.FieldByName(name); !field.IsValid() {
					break
				}
			}
			if !field.IsValid() {
				t.Errorf("Config has no field %q; rename or drop its templateDefaults entry", path)
				continue
			}
			if got, want := field.Interface(), templateDefaults[path]; !reflect.DeepEqual(got, want) {
				t.Errorf("LoadConfig(template).%s = %#v; want %#v", path, got, want)
			}
		}
	})

	// legacyContents is a file seeded before the friction, driver, judge, parent-review and review-budget keys existed.
	const legacyContents = `discussion:
  model: opus[effort=high]
  timeout_min: 480
  interactive: false
review:
  reviewer_model: opus[effort=high]
  timeout_min: 240
`
	tests := []struct {
		name     string
		contents string
		values   map[string]string
		// hubBase is written as the hub base loom.yaml under a board dir beside the worktree anchor.
		hubBase    string
		lensesYAML string
		mutate     func(*Config)
		wantSoft   map[Perch]int
	}{
		{
			name:    "a nested key of a hub base file",
			hubBase: "discussion: {review_ask_parent: true}\n",
			mutate:  func(c *Config) { c.Discussion.ReviewAskParent = true },
		},
		{
			name:     "a perch's own budget wins",
			hubBase:  "code: {max_bounces: 2}\n",
			mutate:   func(c *Config) { c.Code.MaxBounces = intPtr(2) },
			wantSoft: map[Perch]int{PerchDiscussion: 2, PerchCode: 2, PerchSplit: 2},
		},
		{
			name:    "a perch with a null budget takes the run-wide one",
			hubBase: "review: {max_bounces: 4}\ndiscussion: {max_bounces: ~}\ncode: {max_bounces: ~}\nsplit: {max_bounces: ~}\n",
			mutate: func(c *Config) {
				c.Review.MaxBounces = 4
				c.Discussion.MaxBounces, c.Code.MaxBounces, c.Split.MaxBounces = nil, nil, nil
			},
			wantSoft: map[Perch]int{PerchDiscussion: 4, PerchCode: 4, PerchSplit: 4},
		},
		{
			name:       "empty fan keys never read a malformed lenses.yaml",
			lensesYAML: malformedLensesYAML,
			values: map[string]string{
				"discussion.reviewer_fan": `""`, "code.reviewer_fan": `""`,
				"discussion.fixer_fan": `""`, "code.fixer_fan": `""`,
			},
			mutate: func(c *Config) {
				c.Discussion.ReviewerFan, c.Code.ReviewerFan = lenses.FanList{""}, lenses.FanList{""}
				c.Discussion.FixerFan, c.Code.FixerFan = lenses.FanList{""}, lenses.FanList{""}
			},
		},
		{
			name:     "retired selfreport key ignored",
			contents: ConfigTemplate() + "selfreport: false\n",
			mutate:   func(*Config) {},
		},
		{
			name:   "raddle keys overridden",
			values: map[string]string{"raddle.model": "sonnet[high]", "raddle.auto_approve": "true"},
			mutate: func(c *Config) { c.Raddle.Model, c.Raddle.AutoApprove = "sonnet[high]", true },
		},
		{
			name:   "pr.review_ask_parent overridden",
			values: map[string]string{"pr.review_ask_parent": "true"},
			mutate: func(c *Config) { c.PR.ReviewAskParent = true },
		},
		{
			name:   "pr.review_small_max overridden",
			values: map[string]string{"pr.review_small_max": "5"},
			mutate: func(c *Config) { c.PR.ReviewSmallMax = 5 },
		},
		{
			name:   "pr.review_parent_timeout_min overridden",
			values: map[string]string{"pr.review_parent_timeout_min": "5"},
			mutate: func(c *Config) { c.PR.ReviewParentTimeoutMin = 5 },
		},
		{
			name:   "Discussion-Review parent-review keys overridden",
			values: map[string]string{"discussion.review_ask_parent": "true", "discussion.review_parent_timeout_min": "1"},
			mutate: func(c *Config) { c.Discussion.ReviewAskParent, c.Discussion.ReviewParentTimeoutMin = true, 1 },
		},
		{
			name:   "Code-Review parent-review keys overridden",
			values: map[string]string{"code.review_ask_parent": "true", "code.review_parent_timeout_min": "1"},
			mutate: func(c *Config) { c.Code.ReviewAskParent, c.Code.ReviewParentTimeoutMin = true, 1 },
		},
		{
			name:   "notify_parent_on overridden",
			values: map[string]string{"notify_parent_on": `["Drift-Verify"]`},
			mutate: func(c *Config) { c.NotifyParentOn = []string{"Drift-Verify"} },
		},
		{
			name:   "review.reviewer_model as a per-round list",
			values: map[string]string{"review.reviewer_model": `["sonnet[low]", "opus[high]"]`},
			mutate: func(c *Config) { c.Review.ReviewerModel = burlerengine.ModelSpecList{"sonnet[low]", "opus[high]"} },
		},
		{
			name:   "review.fixer_model as a per-round list",
			values: map[string]string{"review.fixer_model": `["opus[low]", "opus[high]"]`},
			mutate: func(c *Config) { c.Review.FixerModel = burlerengine.ModelSpecList{"opus[low]", "opus[high]"} },
		},
		{
			name:   "code.fixer_model as one model-spec",
			values: map[string]string{"code.fixer_model": "opus[low]"},
			mutate: func(c *Config) { c.Code.FixerModel = burlerengine.ModelSpecList{"opus[low]"} },
		},
		{
			name:   "empty segment list takes the shared list",
			values: map[string]string{"discussion.fixer_model": "[]"},
			mutate: func(c *Config) { c.Discussion.FixerModel = burlerengine.ModelSpecList{} },
		},
		{
			name:   "each fan key as one fan name",
			values: map[string]string{"discussion.reviewer_fan": "standard", "code.reviewer_fan": "review-code", "discussion.fixer_fan": "fix-discussion", "code.fixer_fan": "fix-code"},
			mutate: func(c *Config) {
				c.Discussion.ReviewerFan = lenses.FanList{"standard"}
				c.Code.ReviewerFan = lenses.FanList{"review-code"}
				c.Discussion.FixerFan = lenses.FanList{"fix-discussion"}
				c.Code.FixerFan = lenses.FanList{"fix-code"}
			},
		},
		{
			name: "each fan key as a per-round list, an empty entry running its round solo",
			values: map[string]string{
				"discussion.reviewer_fan": `["", review-discussion]`,
				"code.reviewer_fan":       `[review-code, ""]`,
				"discussion.fixer_fan":    `["", fix-discussion]`,
				"code.fixer_fan":          `[fix-code, ""]`,
			},
			mutate: func(c *Config) {
				c.Discussion.ReviewerFan = lenses.FanList{"", "review-discussion"}
				c.Code.ReviewerFan = lenses.FanList{"review-code", ""}
				c.Discussion.FixerFan = lenses.FanList{"", "fix-discussion"}
				c.Code.FixerFan = lenses.FanList{"fix-code", ""}
			},
		},
		{
			name: "assistant model keys as a per-round list, an empty entry running no assistant that round",
			values: map[string]string{
				"discussion.assist_reviewer_model": `["luna[low]", ""]`,
				"code.assist_reviewer_model":       `["", "luna[high]"]`,
			},
			mutate: func(c *Config) {
				c.Discussion.AssistReviewerModel = burlerengine.ModelSpecList{"luna[low]", ""}
				c.Code.AssistReviewerModel = burlerengine.ModelSpecList{"", "luna[high]"}
			},
		},
		{
			name: "assistant fan keys as one fan name and as a per-round list",
			values: map[string]string{
				"discussion.assist_reviewer_fan": "standard",
				"code.assist_reviewer_fan":       `["review-code", ""]`,
			},
			mutate: func(c *Config) {
				c.Discussion.AssistReviewerFan = lenses.FanList{"standard"}
				c.Code.AssistReviewerFan = lenses.FanList{"review-code", ""}
			},
		},
		{
			name:   "explicit review.assist_reviewer_timeout_min",
			values: map[string]string{"review.assist_reviewer_timeout_min": "15"},
			mutate: func(c *Config) { c.Review.AssistReviewerTimeoutMin = 15 },
		},
		{
			name:   "a zero review.timeout_min loads beside the assistant's timeout",
			values: map[string]string{"review.timeout_min": "0", "review.assist_reviewer_timeout_min": "60"},
			mutate: func(c *Config) { c.Review.TimeoutMin, c.Review.AssistReviewerTimeoutMin = 0, 60 },
		},
		{
			name:     "retired fix_start key ignored",
			contents: ConfigTemplate() + "fix_start: after-review\n",
			mutate:   func(*Config) {},
		},
		{
			name:   "discussion.interactive true",
			values: map[string]string{"discussion.interactive": "true"},
			mutate: func(c *Config) { c.Discussion.Interactive = true },
		},
		{
			name:   "empty discussion.lenses turns the writer forks off",
			values: map[string]string{"discussion.lenses": `""`},
			mutate: func(c *Config) { c.Discussion.Lenses = "" },
		},
		{
			name:   "bare discussion.advisors key is null and means no advisors",
			values: map[string]string{"discussion.advisors": ""},
			mutate: func(c *Config) { c.Discussion.Advisors = nil },
		},
		{
			name:   "discussion.advisors as an empty list means no advisors",
			values: map[string]string{"discussion.advisors": "[]"},
			mutate: func(c *Config) { c.Discussion.Advisors = nil },
		},
		{
			name:   "discussion.advisors as one named entry",
			values: map[string]string{"discussion.advisors": "[{name: critic, model: \"opus[low]\"}]"},
			mutate: func(c *Config) { c.Discussion.Advisors = DiscussionAdvisorList{{Name: "critic", Model: "opus[low]"}} },
		},
		{
			name:   "discussion.advisors as a two-entry list",
			values: map[string]string{"discussion.advisors": `[{name: critic, model: "sonnet[low]"}, {name: skeptic, model: "opus[high]"}]`},
			mutate: func(c *Config) {
				c.Discussion.Advisors = DiscussionAdvisorList{{Name: "critic", Model: "sonnet[low]"}, {Name: "skeptic", Model: "opus[high]"}}
			},
		},
		{
			name:   "empty discussion.gate_fixer turns the fixer off",
			values: map[string]string{"discussion.gate_fixer": `""`},
			mutate: func(c *Config) { c.Discussion.GateFixer = "" },
		},
		{
			name:   "empty friction.model turns Tier 2 off",
			values: map[string]string{"friction.model": `""`},
			mutate: func(c *Config) { c.Friction.Model = "" },
		},
		{
			name: "judge and seed override keys set",
			values: map[string]string{
				"review.seed": "haiku[low]", "discussion.judge": "opus[low]", "code.judge": "sonnet[low]",
				"discussion.seed": "haiku[medium]", "code.seed": "sonnet[high]",
			},
			mutate: func(c *Config) {
				c.Review.Seed = "haiku[low]"
				c.Discussion.Judge, c.Code.Judge = "opus[low]", "sonnet[low]"
				c.Discussion.Seed, c.Code.Seed = "haiku[medium]", "sonnet[high]"
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
			values: map[string]string{"review.circling_checkpoint": "2", "review.max_bounces": "7", "review.hard_max_bounces": "7"},
			mutate: func(c *Config) { c.Review.CirclingCheckpoint, c.Review.MaxBounces, c.Review.HardMaxBounces = 2, 7, 7 },
		},
		{
			name:   "explicit hard caps",
			values: map[string]string{"review.hard_max_bounces": "5", "discussion.hard_max_bounces": "6"},
			mutate: func(c *Config) { c.Review.HardMaxBounces, c.Discussion.HardMaxBounces = 5, 6 },
		},
		{
			name:   "a hard cap equal to the soft cap loads",
			values: map[string]string{"review.max_bounces": "3", "review.hard_max_bounces": "3"},
			mutate: func(c *Config) { c.Review.MaxBounces, c.Review.HardMaxBounces = 3, 3 },
		},
		{
			name: "explicit per-perch budgets and auto-accept flags",
			values: map[string]string{
				"discussion.max_bounces": "1", "code.max_bounces": "4",
				"discussion.auto_accept": "true", "code.auto_accept": "true",
			},
			mutate: func(c *Config) {
				c.Discussion.MaxBounces, c.Code.MaxBounces = intPtr(1), intPtr(4)
				c.Discussion.AutoAccept, c.Code.AutoAccept = true, true
			},
		},
		{
			name:     "a zero per-perch budget loads as zero rounds, not as null",
			values:   map[string]string{"discussion.max_bounces": "0"},
			mutate:   func(c *Config) { c.Discussion.MaxBounces = intPtr(0) },
			wantSoft: map[Perch]int{PerchDiscussion: 0, PerchCode: 1},
		},
		{
			name:     "a zero run-wide budget gives every null perch zero rounds",
			values:   map[string]string{"review.max_bounces": "0"},
			mutate:   func(c *Config) { c.Review.MaxBounces = 0 },
			wantSoft: map[Perch]int{PerchDiscussion: 2, PerchPlan: 0, PerchCode: 1},
		},
		{
			name:   "checkpoint above budget",
			values: map[string]string{"review.circling_checkpoint": "9", "review.max_bounces": "2"},
			mutate: func(c *Config) { c.Review.CirclingCheckpoint, c.Review.MaxBounces = 9, 2 },
		},
		{
			name:   "zero review.stalemate_rounds turns the check off",
			values: map[string]string{"review.stalemate_rounds": "0"},
			mutate: func(c *Config) { c.Review.StalemateRounds = 0 },
		},
		{
			name:     "keys absent from a seeded file take template defaults",
			contents: legacyContents,
			mutate: func(c *Config) {
				c.Discussion.Model, c.Review.ReviewerModel = "opus[effort=high]", burlerengine.ModelSpecList{"opus[effort=high]"}
			},
		},
		{
			name:   "zero discussion timeout",
			values: map[string]string{"discussion.timeout_min": "0"},
			mutate: func(c *Config) { c.Discussion.TimeoutMin = 0 },
		},
		{
			name:   "zero review timeout",
			values: map[string]string{"review.timeout_min": "0"},
			mutate: func(c *Config) { c.Review.TimeoutMin = 0 },
		},
		{
			name:   "zero friction timeout",
			values: map[string]string{"friction.timeout_min": "0"},
			mutate: func(c *Config) { c.Friction.TimeoutMin = 0 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			dirs := configengine.Dirs{Anchor: baseDir}
			switch {
			case tt.hubBase != "":
				dirs.Board = t.TempDir()
				seedLoomConfig(t, dirs.Board, tt.hubBase)
			case tt.contents != "":
				seedLoomConfig(t, baseDir, tt.contents)
			default:
				seedLoomConfig(t, baseDir, loomTemplateWith(t, tt.values))
			}
			if tt.lensesYAML != "" {
				seedLensesConfig(t, baseDir, tt.lensesYAML)
			}

			want := templateLoad
			tt.mutate(&want)

			got, err := LoadConfig(dirs, "loom")
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
			board      map[string]string
			boardExtra string
			worktree   string
			local      string
			mutate     func(*Config)
		}{
			{
				name:   "a local value wins over the repo default",
				board:  map[string]string{"review.max_bounces": "3"},
				local:  "review: {max_bounces: 2}\n",
				mutate: func(c *Config) { c.Review.MaxBounces = 2 },
			},
			{
				name:     "a worktree list replaces the default's whole",
				board:    map[string]string{"review.reviewer_model": `["opus[medium]"]`},
				worktree: "review: {reviewer_model: [\"sonnet[low]\", \"sonnet[high]\"]}\n",
				mutate:   func(c *Config) { c.Review.ReviewerModel = burlerengine.ModelSpecList{"sonnet[low]", "sonnet[high]"} },
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
				name:   "a local code.reviewer_fan of \"\" is open to the local layer",
				local:  "code: {reviewer_fan: \"\"}\n",
				mutate: func(c *Config) { c.Code.ReviewerFan = lenses.FanList{""} },
			},
			{
				name:   "a local code.assist_reviewer_model is open to the local layer",
				local:  "code: {assist_reviewer_model: \"luna[low]\"}\n",
				mutate: func(c *Config) { c.Code.AssistReviewerModel = burlerengine.ModelSpecList{"luna[low]"} },
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

// TestSetDiscussionAdvisors_LoadsBack verifies the discussion.advisors round trip.
// A list of name and model mappings written through configengine.Set must load back as the same advisors.
func TestSetDiscussionAdvisors_LoadsBack(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	baseDir := t.TempDir()
	seedLoomConfig(t, baseDir, ConfigTemplate())
	dirs := configengine.Dirs{Anchor: baseDir}
	spec := ConfigSpec()
	pairs := []yamlengine.KV{{Key: "discussion.advisors", Value: `[{name: critic, model: "sonnet[high]"}]`}}
	if _, err := configengine.Set(dirs, spec, configengine.BaseLayer(dirs, spec), pairs); err != nil {
		t.Fatalf("Set() = _, %v; want nil error", err)
	}

	got, err := LoadConfig(dirs, "loom")
	if err != nil {
		t.Fatalf("LoadConfig() = _, %v; want nil error", err)
	}
	want := DiscussionAdvisorList{{Name: "critic", Model: "sonnet[high]"}}
	if !reflect.DeepEqual(got.Discussion.Advisors, want) {
		t.Errorf("Discussion.Advisors = %+v; want %+v", got.Discussion.Advisors, want)
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
		{"malformed discussion spec", "discussion.model", `"opus[effort"`, nil, ""},
		{"malformed raddle spec", "raddle.model", `"opus[effort"`, []string{`config key "loom.raddle.model"`}, ""},
		{"empty subagent_model", "subagent_model", `""`, []string{"must not be empty", "lym config loom --set subagent_model=sonnet"}, ""},
		{"malformed review.reviewer_model spec", "review.reviewer_model", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed review.fixer_model spec", "review.fixer_model", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed later review.reviewer_model entry", "review.reviewer_model", `["sonnet[low]", "opus[effort"]`, []string{"entry 2", "a non-empty list of model-specs"}, ""},
		{"empty review.reviewer_model list", "review.reviewer_model", "[]", []string{"empty", "a non-empty list of model-specs"}, ""},
		{"empty review.fixer_model list", "review.fixer_model", "[]", []string{"empty", "a non-empty list of model-specs"}, ""},
		{"malformed discussion.reviewer_model spec", "discussion.reviewer_model", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed discussion.fixer_model spec", "discussion.fixer_model", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed code.reviewer_model spec", "code.reviewer_model", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed later code.fixer_model entry", "code.fixer_model", `["sonnet[low]", "opus[effort"]`, []string{"entry 2", "a non-empty list of model-specs"}, ""},
		{"malformed discussion.assist_reviewer_model entry", "discussion.assist_reviewer_model", `"opus[effort"`, []string{`config key "loom.discussion.assist_reviewer_model" entry 1`, "no assistant that round"}, ""},
		{"malformed later code.assist_reviewer_model entry", "code.assist_reviewer_model", `["luna[low]", "opus[effort"]`, []string{`config key "loom.code.assist_reviewer_model" entry 2`, "no assistant that round"}, ""},
		{"malformed entry after an empty one", "discussion.assist_reviewer_model", `["", "opus[effort"]`, []string{"entry 2"}, ""},
		{"unknown discussion.assist_reviewer_fan", "discussion.assist_reviewer_fan", "nosuchfan", []string{`"nosuchfan"`, "known fans: ", "entry 1"}, ""},
		{"negative review.assist_reviewer_timeout_min", "review.assist_reviewer_timeout_min", "-1", []string{"must not be negative"}, ""},
		{"unknown discussion.reviewer_fan", "discussion.reviewer_fan", "nosuchfan", []string{`"nosuchfan"`, "known fans: ", "standard", "entry 1", `set the entry to ""`}, ""},
		{"unknown second discussion.fixer_fan entry", "discussion.fixer_fan", `["", nosuchfan]`, []string{"entry 2", `"nosuchfan"`, "known fans: ", "define the fan in lenses.yaml", `set the entry to ""`}, ""},
		{"empty discussion.reviewer_fan list", "discussion.reviewer_fan", "[]", []string{"the list is empty", `write ""`}, ""},
		{"empty discussion.fixer_fan list", "discussion.fixer_fan", "[]", []string{"the list is empty", `write ""`}, ""},
		{"unknown code.reviewer_fan", "code.reviewer_fan", "nosuchfan", []string{`config key "loom.code.reviewer_fan" entry 1`, `"nosuchfan"`, "known fans: ", `set the entry to ""`}, ""},
		{"mapping code.fixer_fan value", "code.fixer_fan", "{model: opus}", []string{`config key "loom.code.fixer_fan"`, "neither a fan name nor a list of fan names"}, ""},
		{"mapping discussion.fixer_fan value", "discussion.fixer_fan", "{model: opus}", []string{"line ", "neither a fan name nor a list of fan names", "a list of fan names"}, ""},
		{"set discussion.reviewer_fan with a malformed lenses.yaml", "discussion.reviewer_fan", "standard", []string{"lenses: config file ", "parse doc YAML", "fix lenses.yaml, or set the key to empty"}, malformedLensesYAML},
		{"string discussion.advisors entry", "discussion.advisors", `["sonnet[high]"]`, []string{"entry 1", advisorEntryWayForward}, ""},
		{"string discussion.advisors entry after a mapping", "discussion.advisors", `[{name: critic, model: "sonnet[high]"}, "sonnet[low]"]`, []string{"entry 2", advisorEntryWayForward}, ""},
		{"blank discussion.advisors name", "discussion.advisors", `[{name: "", model: "sonnet[high]"}]`, []string{"entry 1", "missing or blank", advisorWayForward}, ""},
		{"discussion.advisors entry without a name", "discussion.advisors", `[{model: "sonnet[high]"}]`, []string{"entry 1", "missing or blank", advisorWayForward}, ""},
		{"duplicate discussion.advisors name", "discussion.advisors", `[{name: critic, model: "sonnet[high]"}, {name: critic, model: "sonnet[low]"}]`, []string{"entry 2", `"critic"`, "repeats entry 1", advisorWayForward}, ""},
		{"numeric discussion.advisors name", "discussion.advisors", `[{name: "2", model: "sonnet[high]"}]`, []string{"entry 1", `"2"`, "no seat name", advisorWayForward}, ""},
		{"malformed discussion.advisors model", "discussion.advisors", `[{name: critic, model: "opus[effort"}]`, []string{"entry 1", advisorWayForward}, ""},
		{"discussion.advisors entry without a model", "discussion.advisors", `[{name: critic}]`, []string{"entry 1", advisorWayForward}, ""},
		{"unknown discussion.advisors alias", "discussion.advisors", `[{name: critic, model: "sonnet[high]"}, {name: skeptic, model: ghost}]`, []string{"entry 2", "ghost", advisorWayForward}, ""},
		{"mapping discussion.advisors value", "discussion.advisors", "{model: opus}", []string{"line ", advisorWayForward}, ""},
		{"scalar discussion.advisors value", "discussion.advisors", `"sonnet[high]"`, []string{"line ", advisorWayForward}, ""},
		{"mapping review.reviewer_model value", "review.reviewer_model", "{model: opus}", []string{"a non-empty list of model-specs"}, ""},
		{"mapping inside a review.fixer_model list", "review.fixer_model", "[{model: opus}]", []string{"a non-empty list of model-specs"}, ""},
		{"malformed judge spec", "review.judge", `"sonnet[medium"`, nil, ""},
		{"malformed seed spec", "review.seed", `"opus[effort"`, nil, ""},
		{"malformed discussion.judge spec", "discussion.judge", `"opus[effort"`, nil, ""},
		{"malformed code.judge spec", "code.judge", `"opus[effort"`, nil, ""},
		{"malformed discussion.seed spec", "discussion.seed", `"opus[effort"`, nil, ""},
		{"malformed code.seed spec", "code.seed", `"opus[effort"`, nil, ""},
		{"malformed discussion.gate_fixer spec", "discussion.gate_fixer", `"opus[effort"`, []string{"lym config loom"}, ""},
		{"malformed friction spec", "friction.model", `"opus[effort"`, nil, ""},
		{"malformed driver spec", "driver", `"opus[effort"`, nil, ""},
		{"zero review.circling_checkpoint", "review.circling_checkpoint", "0", []string{positiveIntegerForward}, ""},
		{"negative review.circling_checkpoint", "review.circling_checkpoint", "-1", []string{positiveIntegerForward}, ""},
		{"negative review.stalemate_rounds", "review.stalemate_rounds", "-1", []string{"must not be negative", "set it to 0 to turn the check off", "lym config loom --set review.stalemate_rounds=<n>", "in the task worktree"}, ""},
		{"zero discussion.review_parent_timeout_min", "discussion.review_parent_timeout_min", "0", []string{"must be at least 1", positiveIntegerForward}, ""},
		{"negative discussion.review_parent_timeout_min", "discussion.review_parent_timeout_min", "-1", []string{"must be at least 1", positiveIntegerForward}, ""},
		{"zero code.review_parent_timeout_min", "code.review_parent_timeout_min", "0", []string{"must be at least 1", positiveIntegerForward}, ""},
		{"zero pr.review_parent_timeout_min", "pr.review_parent_timeout_min", "0", []string{"must be at least 1", positiveIntegerForward}, ""},
		{"negative pr.review_parent_timeout_min", "pr.review_parent_timeout_min", "-1", []string{"must be at least 1", positiveIntegerForward}, ""},
		{"negative review.max_bounces", "review.max_bounces", "-1", []string{"must not be negative", "set it to 0 for no review round, or to a positive integer"}, ""},
		{"negative discussion.max_bounces", "discussion.max_bounces", "-1", []string{"must not be negative", "set it to 0 for no review round, to a positive integer, or to null to take loom.review.max_bounces"}, ""},
		{"negative code.max_bounces", "code.max_bounces", "-1", []string{"must not be negative", "or to null to take loom.review.max_bounces"}, ""},
		{"negative review.amend_max_bounces", "review.amend_max_bounces", "-1", []string{"must not be negative", "set it to 0 to settle every amendment with no review round", "lym config loom --set review.amend_max_bounces=<n>", `lym loom resume`}, ""},
		{"zero review.hard_max_bounces", "review.hard_max_bounces", "0", []string{"must be at least 1", "lym config loom --set review.hard_max_bounces=<n>", `lym loom resume`}, ""},
		{"negative discussion.hard_max_bounces", "discussion.hard_max_bounces", "-1", []string{"set it to 0 to take loom.review.hard_max_bounces", "lym config loom --set discussion.hard_max_bounces=<n>", `lym loom resume`}, ""},
		{"negative code.hard_max_bounces", "code.hard_max_bounces", "-1", []string{"set it to 0 to take loom.review.hard_max_bounces", "lym config loom --set code.hard_max_bounces=<n>", `lym loom resume`}, ""},
		{"negative split.max_bounces", "split.max_bounces", "-1", []string{"must not be negative", "or to null to take loom.review.max_bounces"}, ""},
		{"negative split.hard_max_bounces", "split.hard_max_bounces", "-1", []string{"set it to 0 to take loom.review.hard_max_bounces", "lym config loom --set split.hard_max_bounces=<n>", `lym loom resume`}, ""},
		{"review.max_bounces above the default hard cap", "review.max_bounces", "5", []string{"review.hard_max_bounces", "lym config loom --set review.hard_max_bounces=<n>", "in the task worktree", `lym loom resume`}, ""},
		{"code.max_bounces above the default hard cap", "code.max_bounces", "5", []string{"review.hard_max_bounces", "code.max_bounces"}, ""},
		{"negative discussion timeout", "discussion.timeout_min", "-1", []string{"must not be negative"}, ""},
		{"negative review timeout", "review.timeout_min", "-1", []string{"must not be negative"}, ""},
		{"negative friction timeout", "friction.timeout_min", "-1", []string{"must not be negative"}, ""},
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

	t.Run("an old key name in each file layer is judged by the move table", func(t *testing.T) {
		t.Parallel()
		movedNames := []string{"loom.code_max_bounces", "loom.code.max_bounces"}
		layers := []struct {
			name     string
			layer    configengine.Layer
			document string
			// wantIn is empty for a document that loads.
			wantIn []string
		}{
			{"an old key in the hub base file", configengine.LayerDefault, "code_max_bounces: 2\n", append([]string{`lym config reconcile --layer default --apply`}, movedNames...)},
			{"an old key in the worktree file", configengine.LayerWorktree, "code_max_bounces: 2\n", append([]string{`run "lym config reconcile --apply" in this worktree`}, movedNames...)},
			{"an old key in the local file", configengine.LayerLocal, "code_max_bounces: 2\n", append([]string{`run "lym config reconcile --apply" in this worktree`}, movedNames...)},
			{"a guard key under an old key's area is refused by its new path", configengine.LayerLocal, "discussion: {reviewer_fan: \"\"}\n", []string{"discussion.reviewer_fan", "guard"}},
			{"an open key under an old key's area loads", configengine.LayerLocal, "discussion: {timeout_min: 60}\n", nil},
			{"a plan key stays retired and names its tapestry path", configengine.LayerLocal, "plan_max_bounces: 1\n", []string{"tapestry.plan.max_bounces"}},
		}
		for _, tt := range layers {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				board, anchor := t.TempDir(), t.TempDir()
				seedLoomConfig(t, board, ConfigTemplate())
				switch tt.layer {
				case configengine.LayerDefault:
					seedLoomConfig(t, board, tt.document)
				case configengine.LayerWorktree:
					seedLoomConfig(t, anchor, tt.document)
				case configengine.LayerLocal:
					writeLocalLoomConfig(t, anchor, tt.document)
				}

				_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "loom")
				if len(tt.wantIn) == 0 {
					if err != nil {
						t.Fatalf("LoadConfig() error = %v; want nil", err)
					}
					return
				}
				if err == nil {
					t.Fatalf("LoadConfig() error = nil; want a refusal holding %q", tt.wantIn)
				}
				for _, want := range tt.wantIn {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
					}
				}
			})
		}
	})

	t.Run("a hub base mapping where a model-spec belongs is refused naming the deepest key", func(t *testing.T) {
		t.Parallel()
		board, anchor := t.TempDir(), t.TempDir()
		seedLoomConfig(t, board, "code: {reviewer_model: {x: 1}}\n")

		_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "loom")
		if err == nil || !strings.Contains(err.Error(), `config key "loom.code.reviewer_model"`) {
			t.Errorf("LoadConfig() error = %v; want a refusal naming code.reviewer_model", err)
		}
	})

	t.Run("a local guarded key is refused naming the local file and the key", func(t *testing.T) {
		t.Parallel()
		for key, contents := range map[string]string{
			"discussion.reviewer_fan": "discussion: {reviewer_fan: standard}\n",
			"discussion.fixer_fan":    "discussion: {fixer_fan: standard}\n",
			"code.fixer_fan":          "code: {fixer_fan: standard}\n",
			"discussion.advisors":     "discussion: {advisors: []}\n",
		} {
			t.Run(key, func(t *testing.T) {
				t.Parallel()
				board, anchor := t.TempDir(), t.TempDir()
				seedLoomConfig(t, board, ConfigTemplate())
				localPath := writeLocalLoomConfig(t, anchor, contents)

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

	t.Run("a mapping discussion.advisors value names no entry index", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeLoomConfigWithKey(t, dir, "discussion.advisors", "{model: opus}")

		_, err := LoadConfig(configengine.Dirs{Anchor: dir}, "loom")
		if err == nil || strings.Contains(err.Error(), "entry") {
			t.Errorf("LoadConfig() error = %v; want a refusal that names no entry", err)
		}
	})

	t.Run("a malformed models.yaml refuses advisors and is never read for none", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeLoomConfigWithKey(t, dir, "discussion.advisors", `[{name: critic, model: "sonnet[high]"}]`)
		modelsPath := filepath.Join(dir, "_lym", "config", "models.yaml")
		if err := os.WriteFile(modelsPath, []byte("sonnet: [unclosed\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", modelsPath, err)
		}

		_, err := LoadConfig(configengine.Dirs{Anchor: dir}, "loom")
		if err == nil {
			t.Fatal("LoadConfig() error = nil; want the models.yaml refusal for discussion.advisors")
		}
		for _, want := range []string{"discussion.advisors", "fix models.yaml, or set the key to an empty list for no advisors"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
			}
		}

		writeLoomConfigWithKey(t, dir, "discussion.advisors", "[]")
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
				local:  "review: {max_bounces: 5}\n",
				wantIn: []string{"lym config loom --layer local --set review.max_bounces=<n>", "lym board upsert", "lym config loom --layer default --set review.hard_max_bounces=<n>", "prime"},
			},
			{
				name:     "a worktree hard cap under the board default's soft cap",
				worktree: "review: {hard_max_bounces: 2}\n",
				wantIn:   []string{"lym config loom --set review.hard_max_bounces=<n>", "in the task worktree"},
			},
			{
				name:   "a local hard cap of zero",
				local:  "review: {hard_max_bounces: 0}\n",
				wantIn: []string{"must be at least 1", "lym config loom --layer local --set review.hard_max_bounces=<n>", "lym board upsert", `lym loom resume`},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				board, anchor := t.TempDir(), t.TempDir()
				seedLoomConfig(t, board, loomTemplateWith(t, map[string]string{"review.max_bounces": "3"}))
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

// TestApplyAutonomy pins what the run's autonomy forces on the resolved config.
// It also pins what the setting lookup answers for it.
// Autonomy on forces three keys from their layered values and keeps the two review ask keys as layered.
// Autonomy off keeps every layered value.
func TestApplyAutonomy(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	layered := Config{Discussion: DiscussionConfig{Interactive: true}, Code: CodeConfig{ReviewAskParent: true}}

	want := layered
	want.PR.ReviewAskParent, want.Discussion.Interactive, want.Raddle.AutoApprove = true, false, true
	if got := ApplyAutonomy(layered, true); !reflect.DeepEqual(got, want) {
		t.Errorf("ApplyAutonomy(layered, true) = %+v; want %+v", got, want)
	}
	if got := ApplyAutonomy(layered, false); !reflect.DeepEqual(got, layered) {
		t.Errorf("ApplyAutonomy(layered, false) = %+v; want %+v", got, layered)
	}

	settings := []struct {
		key        string
		wantValue  bool
		wantForced bool
	}{
		{"pr.review_ask_parent", true, true},
		{"discussion.interactive", false, true},
		{"raddle.auto_approve", true, true},
		{"code.review_ask_parent", false, false},
		{"discussion.review_ask_parent", false, false},
		{"raddle.model", false, false},
	}
	for _, setting := range settings {
		if value, forced := AutonomyForcedSetting(setting.key); value != setting.wantValue || forced != setting.wantForced {
			t.Errorf("AutonomyForcedSetting(%q) = %v, %v; want %v, %v", setting.key, value, forced, setting.wantValue, setting.wantForced)
		}
	}
}

// TestConfigTemplate_ContainsEveryConfigYAMLTag reflects over Config and TapestryConfig, their areas included.
// It asserts every yaml tag appears in its module's template, so a field without a template line fails here.
// A struct field with no matching template key is a silent hole, not a load error.
// This check guards against that.
//
//testtiming:keep pins that every Config and TapestryConfig yaml tag has a template line, a reflection check no LoadConfig case makes
//lym:seam internal/loomengine#Config
//lym:seam internal/loomengine#TapestryConfig
//lym:seam internal/loomengine#DiscussionConfig
//lym:seam internal/loomengine#CodeConfig
//lym:seam internal/loomengine#SplitConfig
//lym:seam internal/loomengine#RaddleConfig
//lym:seam internal/loomengine#FrictionConfig
//lym:seam internal/loomengine#PRConfig
//lym:seam internal/loomengine#ReviewConfig
//lym:seam internal/loomengine#PlanConfig
//lym:seam internal/loomengine#PartsConfig
func TestConfigTemplate_ContainsEveryConfigYAMLTag(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	var check func(name string, typ reflect.Type, mapping map[string]any)
	check = func(name string, typ reflect.Type, mapping map[string]any) {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			tag := field.Tag.Get("yaml")
			if tag == "" {
				t.Fatalf("%s field %q has no yaml tag", name, field.Name)
			}
			// A "-" tag marks a Config field that tapestry.yaml supplies, checked through TapestryConfig.
			if tag == "-" {
				continue
			}
			value, held := mapping[tag]
			if !held {
				t.Errorf("the %s template does not contain key %q for field %q", name, tag, field.Name)
				continue
			}
			if field.Type.Kind() == reflect.Struct {
				area, _ := value.(map[string]any)
				check(name+"."+field.Name, field.Type, area)
			}
		}
	}
	for _, tt := range []struct {
		name     string
		template string
		typ      reflect.Type
	}{
		{"Config", ConfigTemplate(), reflect.TypeOf(Config{})},
		{"TapestryConfig", TapestryConfigTemplate(), reflect.TypeOf(TapestryConfig{})},
	} {
		var template map[string]any
		if err := yaml.Unmarshal([]byte(tt.template), &template); err != nil {
			t.Fatalf("parse the %s template: %v", tt.name, err)
		}
		check(tt.name, tt.typ, template)
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

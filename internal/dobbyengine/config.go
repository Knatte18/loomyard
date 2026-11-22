// config.go declares the beam config module, beam.yaml: its template, its layered-load spec and LoadConfig.

package dobbyengine

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/configengine"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns beam.yaml's default YAML template.
func ConfigTemplate() string {
	return configTemplate
}

// The keys of beam.yaml's per-task settings, which only the per-task lookup reads.
// They are declared once, in beamsetting.
const (
	// PullRequestKey decides whether a task's landing goes through a GitHub pull request.
	PullRequestKey = beamsetting.PullRequestKey
	// LandNoticeKey decides whether dobby posts a line to the orch once a task's run lands or ends without landing.
	LandNoticeKey = beamsetting.LandNoticeKey
)

// The values of beam.yaml's pull_request, declared once in beamsetting.
const (
	// PullRequestOn lands through a pull request when the origin allows one.
	PullRequestOn = beamsetting.PullRequestOn
	// PullRequestOff lands with no pull request unless GitHub's rules for the parent branch require one.
	PullRequestOff = beamsetting.PullRequestOff
	// PullRequestMerge lands through a pull request squash-merged on GitHub at once, with no review.
	PullRequestMerge = beamsetting.PullRequestMerge
	// PullRequestMergeIn lands a task that merges its parent's parent branch into its parent with no pull request.
	PullRequestMergeIn = beamsetting.PullRequestMergeIn
)

// The values of beam.yaml's land_notice, declared once in beamsetting.
const (
	// LandNoticeOn sends the landing notice.
	LandNoticeOn = beamsetting.LandNoticeOn
	// LandNoticeOff sends none.
	LandNoticeOff = beamsetting.LandNoticeOff
)

// Config is the resolved beam.yaml's worktree-wide keys.
// The per-task keys, pull_request and land_notice, are read through LookupPullRequest and LookupLandNotice alone.
type Config struct {
	// MaxRuns is the most runs dobby drives at once; 0 starts none.
	MaxRuns int `yaml:"max_runs"`
	// BoardTidyEvery is the landings between board tidies; 0 never tidies.
	BoardTidyEvery int `yaml:"board_tidy_every"`
	// BoardTidyTimeoutMin is the minutes the board-tidy agent may run.
	BoardTidyTimeoutMin int `yaml:"board_tidy_timeout_min"`
	// TaskHeadroom is the fewest free tasks under the cgroup task limit for dobby to start a run; 0 never holds a start.
	TaskHeadroom int `yaml:"task_headroom"`
	// AdoptFixTries is the most fix tasks a failed adopt check files for the worktree's subject between two hand adopts; 0 files none.
	AdoptFixTries int `yaml:"adopt_fix_tries"`
}

// ConfigSpec returns beam's layered-load spec: a per-worktree module whose keys are all open to every layer.
// pull_request decides how a task lands, never what lands, so a task's run settings may set it like land_notice.
// test_map_upkeep, test_map_every and test_map_batch are retired: a layer that still holds one loads as if it did not,
// and a reconcile prunes it.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     beamsetting.Module,
		Template: []byte(ConfigTemplate()),
		Categories: map[string]configengine.Category{
			"max_runs":               configengine.CategoryNone,
			"board_tidy_every":       configengine.CategoryNone,
			"board_tidy_timeout_min": configengine.CategoryNone,
			"task_headroom":          configengine.CategoryNone,
			PullRequestKey:           configengine.CategoryNone,
			LandNoticeKey:            configengine.CategoryNone,
			"adopt_fix_tries":        configengine.CategoryNone,
		},
		RetiredKeys: map[string]configengine.Category{
			"test_map_upkeep": configengine.CategoryNone,
			"test_map_every":  configengine.CategoryNone,
			"test_map_batch":  configengine.CategoryNone,
		},
	}
}

// LoadConfig resolves beam.yaml strictly from the layer stack under dirs, since dobby only ever runs inside a hub.
// A negative value is refused, naming its key.
// So is a resolved pull_request other than on, off, merge or merge-in, and a land_notice other than on or off,
// so a write the per-task lookup would refuse is refused when it is made.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.Load(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal beam config: %w", err))
	}
	for _, key := range []struct {
		name  string
		value int
	}{
		{"max_runs", cfg.MaxRuns},
		{"board_tidy_every", cfg.BoardTidyEvery},
		{"board_tidy_timeout_min", cfg.BoardTidyTimeoutMin},
		{"task_headroom", cfg.TaskHeadroom},
		{"adopt_fix_tries", cfg.AdoptFixTries},
	} {
		if key.value < 0 {
			return Config{}, configengine.MarkInvalid(fmt.Errorf("beam config key %q is %d: must not be negative; way forward: fix it with `lym config beam --set %s=<value>`", key.name, key.value, key.name))
		}
	}
	var perTask map[string]any
	if err := yaml.Unmarshal(resolved, &perTask); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal beam config: %w", err))
	}
	for _, key := range []string{PullRequestKey, LandNoticeKey} {
		if _, err := checkTaskSetting(key, perTask[key], "the resolved beam.yaml"); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

// CheckLocalValues refuses a local-layer beam document holding a value its key does not take.
// It checks only pull_request and land_notice.
// The refusal is marked [configengine.ErrInvalid] and names the key, the values it takes and the way forward.
func CheckLocalValues(document []byte) error {
	var keys map[string]any
	if err := yaml.Unmarshal(document, &keys); err != nil {
		return configengine.MarkInvalid(fmt.Errorf("unmarshal beam run settings: %w", err))
	}
	for _, key := range []string{PullRequestKey, LandNoticeKey} {
		value, set := keys[key]
		if !set {
			continue
		}
		if _, err := checkTaskSetting(key, value, "the task's run_settings"); err != nil {
			return err
		}
	}
	return nil
}

// checkTaskSetting returns the per-task key's value as text, or refuses a value its key does not take.
// where names the file or layer the value was read from, for the refusal.
// A refusal is marked [configengine.ErrInvalid] and names the key, the values it takes and the way forward.
func checkTaskSetting(key string, value any, where string) (string, error) {
	allowed := beamsetting.LandNoticeValues()
	if key == PullRequestKey {
		allowed = beamsetting.PullRequestValues()
	}
	text, isText := value.(string)
	if isText {
		for _, candidate := range allowed {
			if text == candidate {
				return text, nil
			}
		}
	}
	choices := fmt.Sprint(allowed)
	return "", configengine.MarkInvalid(fmt.Errorf("beam config key %q in %s is %v: must be one of %s; way forward: fix it with `lym config beam --set %s=<value>` in that worktree, or in the task's run_settings for its next run", key, where, value, choices, key))
}

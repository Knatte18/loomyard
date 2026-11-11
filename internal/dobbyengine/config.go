// config.go declares the beam config module, beam.yaml: its template, its layered-load spec and LoadConfig.

package dobbyengine

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns beam.yaml's default YAML template.
func ConfigTemplate() string {
	return configTemplate
}

// The values of beam.yaml's land_notice.
const (
	// LandNoticeAll sends a landing notice for every run the dobby drives.
	LandNoticeAll = "all"
	// LandNoticePerTask sends one only for a run whose board entry carried notify_on_land.
	LandNoticePerTask = "per-task"
	// LandNoticeOff sends none.
	LandNoticeOff = "off"
)

// Config is the resolved beam.yaml.
type Config struct {
	// MaxRuns is the most runs dobby drives at once; 0 starts none.
	MaxRuns int `yaml:"max_runs"`
	// BoardTidyEvery is the landings between board tidies; 0 never tidies.
	BoardTidyEvery int `yaml:"board_tidy_every"`
	// BoardTidyTimeoutMin is the minutes the board-tidy agent may run.
	BoardTidyTimeoutMin int `yaml:"board_tidy_timeout_min"`
	// TestMapEvery is the landings between the prime dobby's test-map refreshes; 0 never refreshes.
	TestMapEvery int `yaml:"test_map_every"`
	// TestMapBatch is the most queued packages one test-map refresh measures, its `--max`; at least 1.
	TestMapBatch int `yaml:"test_map_batch"`
	// TaskHeadroom is the fewest free tasks under the cgroup task limit for dobby to start a run; 0 never holds a start.
	TaskHeadroom int `yaml:"task_headroom"`
	// PullRequest is the pull-request mode: "on", "off", "required" or "merge" (squash-merged at once, no review).
	PullRequest string `yaml:"pull_request"`
	// LandNotice decides which runs the dobby sends a landing notice for: LandNoticeAll, LandNoticePerTask or LandNoticeOff.
	LandNotice string `yaml:"land_notice"`
	// AdoptFixTries is the most fix tasks a failed adopt check files for the worktree's subject between two hand adopts; 0 files none.
	AdoptFixTries int `yaml:"adopt_fix_tries"`
}

// ConfigSpec returns beam's layered-load spec: a per-worktree module whose keys are open to every layer but pull_request.
// pull_request decides what lands on the parent, so it is cross-pair and the local layer refuses it.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "beam",
		Template: []byte(ConfigTemplate()),
		Categories: map[string]configengine.Category{
			"max_runs":               configengine.CategoryNone,
			"board_tidy_every":       configengine.CategoryNone,
			"board_tidy_timeout_min": configengine.CategoryNone,
			"test_map_every":         configengine.CategoryNone,
			"test_map_batch":         configengine.CategoryNone,
			"task_headroom":          configengine.CategoryNone,
			"pull_request":           configengine.CategoryCrossPair,
			"land_notice":            configengine.CategoryNone,
			"adopt_fix_tries":        configengine.CategoryNone,
		},
	}
}

// LoadConfig resolves beam.yaml strictly from the layer stack under dirs, since dobby only ever runs inside a hub.
// A negative value is refused, naming its key, and so is a test_map_batch below 1.
// A pull_request other than on, off, required or merge is refused too.
// So is a land_notice other than all, per-task or off.
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
		{"test_map_every", cfg.TestMapEvery},
		{"task_headroom", cfg.TaskHeadroom},
		{"adopt_fix_tries", cfg.AdoptFixTries},
	} {
		if key.value < 0 {
			return Config{}, configengine.MarkInvalid(fmt.Errorf("beam config key %q is %d: must not be negative; way forward: fix it with `lym config beam --set %s=<value>`", key.name, key.value, key.name))
		}
	}
	if cfg.TestMapBatch < 1 {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("beam config key %q is %d: must be at least 1; way forward: fix it with `lym config beam --set test_map_batch=<value>`", "test_map_batch", cfg.TestMapBatch))
	}
	switch cfg.PullRequest {
	case "on", "off", "required", "merge":
	default:
		return Config{}, configengine.MarkInvalid(fmt.Errorf("beam config key %q is %q: must be on, off, required or merge; way forward: fix it with lym config beam --set pull_request=<on|off|required|merge>", "pull_request", cfg.PullRequest))
	}
	switch cfg.LandNotice {
	case LandNoticeAll, LandNoticePerTask, LandNoticeOff:
	default:
		return Config{}, configengine.MarkInvalid(fmt.Errorf("beam config key %q is %q: must be all, per-task or off; way forward: fix it with lym config beam --set land_notice=<all|per-task|off>", "land_notice", cfg.LandNotice))
	}
	return cfg, nil
}

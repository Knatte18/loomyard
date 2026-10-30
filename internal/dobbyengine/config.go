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

// Config is the resolved beam.yaml.
type Config struct {
	// MaxRuns is the most runs dobby drives at once; 0 starts none.
	MaxRuns int `yaml:"max_runs"`
	// BoardTidyEvery is the landings between board tidies; 0 never tidies.
	BoardTidyEvery int `yaml:"board_tidy_every"`
	// BoardTidyTimeoutMin is the minutes the board-tidy agent may run.
	BoardTidyTimeoutMin int `yaml:"board_tidy_timeout_min"`
	// TaskHeadroom is the fewest free tasks under the cgroup task limit for dobby to start a run; 0 never holds a start.
	TaskHeadroom int `yaml:"task_headroom"`
	// PullRequest is the receiving branch's pull-request mode: "on", "off" or "required".
	PullRequest string `yaml:"pull_request"`
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
			"task_headroom":          configengine.CategoryNone,
			"pull_request":           configengine.CategoryCrossPair,
		},
	}
}

// LoadConfig resolves beam.yaml strictly from the layer stack under dirs, since dobby only ever runs inside a hub.
// A negative value is refused naming its key, and so is a pull_request other than on, off or required.
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
	} {
		if key.value < 0 {
			return Config{}, configengine.MarkInvalid(fmt.Errorf("beam config key %q is %d: must not be negative; way forward: fix it with `lym config beam --set %s=<value>`", key.name, key.value, key.name))
		}
	}
	switch cfg.PullRequest {
	case "on", "off", "required":
	default:
		return Config{}, configengine.MarkInvalid(fmt.Errorf("beam config key %q is %q: must be on, off or required; way forward: fix it with lym config beam --set pull_request=<on|off|required>", "pull_request", cfg.PullRequest))
	}
	return cfg, nil
}

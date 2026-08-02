// config.go — configuration for the loom module.
//
// Defines the Config type mirroring loom.yaml's keys and LoadConfig, which
// uses internal/configengine.Load with ConfigTemplate() to strictly
// validate and resolve loom's config file, then validates the discussion
// and plan role model-specs' grammar via modelspec.Parse so a typo'd spec
// fails loud at load time rather than hours into a run when the
// discussion or plan producer first spawns.

package loomengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved loom.yaml configuration: role model-specs and timeout knobs.
type Config struct {
	Discussion           string `yaml:"discussion"`
	DiscussionTimeoutMin int    `yaml:"discussion_timeout_min"`
	Plan                 string `yaml:"plan"`
	PlanTimeoutMin       int    `yaml:"plan_timeout_min"`
}

// LoadConfig loads and unmarshals configuration for the loom module.
// It validates model-spec grammar at load time.
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx fabric reconcile\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal loom config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Discussion); err != nil {
		return Config{}, fmt.Errorf("loom config key %q: %w", "discussion", err)
	}

	if _, err := modelspec.Parse(cfg.Plan); err != nil {
		return Config{}, fmt.Errorf("loom config key %q: %w", "plan", err)
	}

	return cfg, nil
}

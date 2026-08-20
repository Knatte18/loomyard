// config.go — configuration for the landing module.
//
// Defines the Config type mirroring landing.yaml's keys and LoadConfig, which uses
// internal/configengine.Load with ConfigTemplate() to strictly validate and resolve landing's
// config file -- the strict entry point, where an absent config file is an error. Strict is
// required by the membership rule rather than chosen: neither producer in this package has a
// standalone entry point, both are reachable only inside a hub, and there an absent config means
// the hub is broken. It then validates the conflict key's model-spec grammar at load time via
// modelspec.Parse, so a typo fails loudly at load rather than hours into a run when the conflict
// session first spawns.

package landingshed

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved landing.yaml configuration.
type Config struct {
	// RequirePRToBase is the list of base branch names Publish requires a pull request against.
	RequirePRToBase []string `yaml:"require_pr_to_base"`
	// Squash selects whether Finalize's parent-side merge squashes.
	Squash bool `yaml:"squash"`
	// Conflict is the model-spec string selecting the model both producers' conflict-resolution
	// session runs under.
	Conflict string `yaml:"conflict"`
	// ConflictTimeoutMin is the conflict-resolution session's wall-clock budget in minutes.
	ConflictTimeoutMin int `yaml:"conflict_timeout_min"`
}

// LoadConfig loads and unmarshals configuration for the landing module.
// It validates the conflict key's model-spec grammar at load time.
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
		return Config{}, fmt.Errorf("unmarshal landing config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Conflict); err != nil {
		return Config{}, fmt.Errorf("landing config key %q: %w", "conflict", err)
	}

	return cfg, nil
}

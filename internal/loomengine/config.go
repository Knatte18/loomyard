// config.go — configuration for the loom module.
//
// Defines the Config type mirroring loom.yaml's keys and LoadConfig, which
// uses internal/configengine.Load with ConfigTemplate() to strictly
// validate and resolve loom's config file, then validates the discussion
// role model-spec's grammar via modelspec.Parse so a typo'd spec fails
// loud at load time rather than hours into a run when the discussion
// producer first spawns.

package loomengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved loom.yaml configuration: the discussion
// role model-spec (see docs/reference/model-spec.md's "Roles that use this
// notation" section) and the timeout knob the discussion producer consults.
type Config struct {
	// Discussion is the model-spec for the discussion-phase interview
	// agent.
	Discussion string `yaml:"discussion"`
	// DiscussionTimeoutMin is the number of minutes the discussion
	// agent's shuttle run is allowed to run (interactive interviews run
	// long).
	DiscussionTimeoutMin int `yaml:"discussion_timeout_min"`
}

// LoadConfig loads and unmarshals configuration for the loom module.
//
// Calls configengine.Load with loom's ConfigTemplate() to strictly
// validate the config file against the template, resolve environment
// variables, and return resolved bytes. Unmarshals the resolved bytes into
// a Config struct. The module name is threaded through by the caller
// (never hardcoded to "loom" here), mirroring builderengine.LoadConfig.
//
// After unmarshal, the discussion role string is checked against
// modelspec.Parse for grammar only (registry resolution is a separate
// factory-time step — see the discussion producer's spawn-site mapping); a
// grammar error is wrapped naming the offending config key, so a
// hand-edited loom.yaml with a malformed spec fails here rather than at
// spawn time.
//
// If <baseDir>/_lyx/ does not exist, returns an error containing
// "not initialized here; run \"lyx init\"".
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		// Wrap the generic "not initialized" error with the loom-specific
		// hint, matching builderengine/perchengine/muxengine/shuttleengine's
		// shape so every module surfaces the same recovery instruction.
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx init\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal loom config: %w", err)
	}

	// Validate the discussion role's model-spec grammar now, naming the
	// offending key, so a malformed spec is caught at load time rather
	// than silently carried into the discussion producer's spawn site.
	if _, err := modelspec.Parse(cfg.Discussion); err != nil {
		return Config{}, fmt.Errorf("loom config key %q: %w", "discussion", err)
	}

	return cfg, nil
}

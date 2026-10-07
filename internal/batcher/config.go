// config.go — configuration for the batcher module.
//
// Defines the package-local config type mirroring batcher.yaml's keys and Active, which uses
// internal/configengine.LoadOrTemplate with ConfigTemplate() to resolve batcher's own config file,
// degrading to the embedded template on proven absence, then hands the resolved batchifier name to
// the existing Select unchanged.

package batcher

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// moduleName is batcher's own configreg module name, passed to configengine.LoadOrTemplate — never
// a parameter, since Active always loads batcher's own config file.
const moduleName = "batcher"

// config represents the resolved batcher.yaml configuration: the name of the active batchifier and
// the named profiles.
type config struct {
	Active   string             `yaml:"active"`
	Profiles map[string]profile `yaml:"profiles"`
}

// profile is one named entry under batcher.yaml's profiles: key.
type profile struct {
	Weights map[string]float64 `yaml:"weights"`
}

// Active loads the configured batchifier name from batcher.yaml under baseDir and resolves it via
// Select.
// An absent <baseDir>/_lyx/ directory or an absent batcher.yaml both resolve the embedded
// ConfigTemplate() instead of erroring;
// a config file that exists but is invalid still errors.
// baseDir must already be resolved by the caller — Active never resolves cwd itself (see
// PATTERN-cwd-resolution).
func Active(baseDir string) (Batcher, error) {
	cfg, err := loadConfig(baseDir)
	if err != nil {
		return nil, err
	}

	return Select(cfg.Active)
}

// loadConfig loads and unmarshals batcher.yaml under baseDir, degrading to the embedded template on
// proven absence.
func loadConfig(baseDir string) (config, error) {
	resolved, err := configengine.LoadOrTemplate(baseDir, moduleName, []byte(ConfigTemplate()))
	if err != nil {
		return config{}, err
	}

	var cfg config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return config{}, fmt.Errorf("unmarshal batcher config: %w", err)
	}
	return cfg, nil
}

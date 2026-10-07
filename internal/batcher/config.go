// config.go — configuration for the batcher module.
//
// Defines the package-local config type mirroring batcher.yaml's keys, ConfigOpenMaps, and Active, which uses internal/configengine.LoadOrTemplate with ConfigTemplate() to resolve batcher's own config file, degrading to the embedded template on proven absence, then builds the batchifier the active profile names.

package batcher

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// moduleName is batcher's own configreg module name, passed to configengine.LoadOrTemplate — never
// a parameter, since Active always loads batcher's own config file.
const moduleName = "batcher"

// config represents the resolved batcher.yaml configuration: the name of the active profile and the named profiles.
type config struct {
	Active   string             `yaml:"active"`
	Profiles map[string]profile `yaml:"profiles"`
}

// profile is one named entry under batcher.yaml's profiles: key.
// The cost parameters are pointers so a missing key is told apart from a zero.
type profile struct {
	Batchifier string             `yaml:"batchifier"`
	AloneAbove *float64           `yaml:"alone_above"`
	Budget     *float64           `yaml:"budget"`
	MaxCards   *int               `yaml:"max_cards"`
	Weights    map[string]float64 `yaml:"weights"`
}

// ConfigOpenMaps returns the batcher.yaml keys whose entries are the operator's own, which configengine carries whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{"profiles"}
}

// Active loads batcher.yaml under baseDir and builds the batchifier its active: profile names.
// An empty active: and active: identity resolve to the identity batchifier even when no profile of that name is configured;
// a configured profile named identity wins over that default.
// An absent <baseDir>/_lyx/ directory or an absent batcher.yaml both resolve the embedded
// ConfigTemplate() instead of erroring;
// a config file that exists but is invalid still errors, and so does an active: naming no profile, a profile of an unknown batchifier kind, or a cost profile with a missing or invalid parameter, each naming batcher.yaml.
// baseDir must already be resolved by the caller — Active never resolves cwd itself (see
// PATTERN-cwd-resolution).
func Active(baseDir string) (Batcher, error) {
	cfg, err := loadConfig(baseDir)
	if err != nil {
		return nil, err
	}

	name := cfg.Active
	if name == "" {
		name = DefaultName
	}
	prof, ok := cfg.Profiles[name]
	if !ok {
		if name == DefaultName {
			return Identity(), nil
		}
		return nil, fmt.Errorf("batcher.yaml: active %q names no profile under profiles:", name)
	}
	construct, ok := constructors[prof.Batchifier]
	if !ok {
		return nil, fmt.Errorf("batcher.yaml profile %q has unknown batchifier %q", name, prof.Batchifier)
	}
	return construct(name, prof)
}

// loadConfig loads and unmarshals batcher.yaml under baseDir, degrading to the embedded template on proven absence.
func loadConfig(baseDir string) (config, error) {
	resolved, err := configengine.LoadOrTemplate(baseDir, moduleName, []byte(ConfigTemplate()), ConfigOpenMaps()...)
	if err != nil {
		return config{}, err
	}

	var cfg config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return config{}, fmt.Errorf("unmarshal batcher config: %w", err)
	}
	return cfg, nil
}

// config.go — configuration for the batcher module.
//
// Defines the package-local config type mirroring batcher.yaml's single key and Active, which uses
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

// config represents the resolved batcher.yaml configuration: the name of the active batchifier.
type config struct {
	Active string `yaml:"active"`
}

// Active loads the configured batchifier name from batcher.yaml under baseDir and resolves it via
// Select.
// An absent <baseDir>/_lyx/ directory or an absent batcher.yaml both resolve the embedded
// ConfigTemplate() instead of erroring;
// a config file that exists but is invalid still errors.
// baseDir must already be resolved by the caller — Active never resolves cwd itself (see the Cwd
// Resolution Invariant in CONSTRAINTS.md).
func Active(baseDir string) (Batcher, error) {
	resolved, err := configengine.LoadOrTemplate(baseDir, moduleName, []byte(ConfigTemplate()))
	if err != nil {
		return nil, err
	}

	var cfg config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal batcher config: %w", err)
	}

	return Select(cfg.Active)
}

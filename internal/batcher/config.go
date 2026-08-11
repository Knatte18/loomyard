// config.go — configuration for the batcher module.
//
// Defines the package-local config type mirroring batcher.yaml's single key and Active, which uses
// internal/configengine.Load with ConfigTemplate() to strictly validate and resolve batcher's own
// config file, then hands the resolved batchifier name to the existing Select unchanged.

package batcher

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// moduleName is batcher's own configreg module name, passed to configengine.Load — never a
// parameter, since Active always loads batcher's own config file.
const moduleName = "batcher"

// config represents the resolved batcher.yaml configuration: the name of the active batchifier.
type config struct {
	Active string `yaml:"active"`
}

// Active loads the configured batchifier name from batcher.yaml under baseDir and resolves it via
// Select.
// If <baseDir>/_lyx/ does not exist, returns an error containing "not initialized here;
// run \"lyx fabric reconcile\"".
// If <baseDir>/_lyx/ exists but batcher.yaml does not, returns configengine.Load's own
// "config file ... not found; run \"lyx config reconcile\"" error unchanged.
// baseDir must already be resolved by the caller — Active never resolves cwd itself (see the Cwd
// Resolution Invariant in CONSTRAINTS.md).
func Active(baseDir string) (Batcher, error) {
	resolved, err := configengine.Load(baseDir, moduleName, []byte(ConfigTemplate()))
	if err != nil {
		// Wrap the generic "not initialized" error with the fabric-specific
		// hint, matching websterengine.LoadConfig's shape so every module
		// surfaces the same recovery instruction.
		if strings.Contains(err.Error(), "not initialized") {
			return nil, fmt.Errorf("not initialized here; run \"lyx fabric reconcile\"")
		}
		return nil, err
	}

	var cfg config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal batcher config: %w", err)
	}

	return Select(cfg.Active)
}

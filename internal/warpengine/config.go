// config.go — configuration for the warp module.
//
// Defines the Config type with a single field BranchPrefix and LoadConfig.
// LoadConfig uses internal/configengine.Load with the ConfigTemplate() to strictly
// validate and resolve the warp config file; the warp module never reads
// config files or knows their layout itself.

package warpengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// Config represents the configuration for the warp module.
type Config struct {
	BranchPrefix string `yaml:"branch_prefix"`
}

// LoadConfig loads and unmarshals configuration for the warp module.
//
// Calls configengine.Load with the warp ConfigTemplate() to strictly validate
// the config file against the template, resolve environment variables, and
// return resolved bytes. Unmarshals the resolved bytes into a Config struct.
//
// If <baseDir>/_lyx/ does not exist, returns an error containing
// "not initialized here; run \"lyx init\"".
func LoadConfig(baseDir, module string) (Config, error) {
	// Load and resolve the config file using the template
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		// Wrap the generic error with a warp-specific message
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx init\"")
		}
		return Config{}, err
	}

	// Unmarshal resolved bytes into Config struct
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal warp config: %w", err)
	}

	return cfg, nil
}

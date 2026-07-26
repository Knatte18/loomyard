// config.go — configuration for the fabric module.
//
// Defines the Config type carrying both settings fabric replaces from warp.yaml
// (BranchPrefix) and weft.yaml (Pathspec) in one fabric.yaml file, plus LoadConfig.
// LoadConfig uses internal/configengine.Load with ConfigTemplate() to strictly
// validate and resolve the fabric config file; the fabric module never reads
// config files or knows their layout itself.

package fabricengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// Config represents the configuration for the fabric module: the host branch
// prefix (warp.yaml's BranchPrefix equivalent) and the weft-sync pathspec
// (weft.yaml's Pathspec equivalent), unified into one config file since fabric
// unifies both modules.
type Config struct {
	BranchPrefix string `yaml:"branch_prefix"`
	Pathspec     string `yaml:"pathspec"`
}

// Dirs returns the pathspec as a slice of directory names, split on whitespace.
func (c Config) Dirs() []string {
	return strings.Fields(c.Pathspec)
}

// LoadConfig loads and unmarshals configuration for the fabric module from baseDir.
//
// Calls configengine.Load with the fabric ConfigTemplate() to strictly validate
// the config file against the template, resolve environment variables, and
// return resolved bytes. Unmarshals the resolved bytes into a Config struct.
//
// If <baseDir>/_lyx/ does not exist, returns an error containing
// "not initialized here; run \"lyx init\"".
func LoadConfig(baseDir string) (Config, error) {
	// Load and resolve the config file using the template
	resolved, err := configengine.Load(baseDir, "fabric", []byte(ConfigTemplate()))
	if err != nil {
		// Wrap the generic error with a fabric-specific, actionable message.
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx init\"")
		}
		return Config{}, err
	}

	// Unmarshal resolved bytes into Config struct
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal fabric config: %w", err)
	}

	return cfg, nil
}

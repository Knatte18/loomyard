// config.go — configuration for the fabric module.
//
// Defines the Config type carrying the warp branch prefix (BranchPrefix) and the weft-sync pathspec
// (Pathspec) in one fabric.yaml file, unified from fabric's two predecessor config schemas into one
// file, plus LoadConfig.
// LoadConfig uses internal/configengine.Load with ConfigTemplate() to strictly validate and resolve
// the fabric config file;
// the fabric module never reads config files or knows their layout itself.

package fabricengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// Config represents the fabric configuration: warp branch prefix and weft-sync pathspec.
type Config struct {
	BranchPrefix string `yaml:"branch_prefix"`
	Pathspec     string `yaml:"pathspec"`
}

// Dirs returns the pathspec as a slice of directory names, split on whitespace.
func (c Config) Dirs() []string {
	return strings.Fields(c.Pathspec)
}

// LoadConfig loads and unmarshals fabric configuration from baseDir, returning an error if not
// initialized (no _lyx/ directory).
func LoadConfig(baseDir string) (Config, error) {
	resolved, err := configengine.Load(baseDir, "fabric", []byte(ConfigTemplate()))
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx fabric reconcile\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal fabric config: %w", err)
	}

	return cfg, nil
}

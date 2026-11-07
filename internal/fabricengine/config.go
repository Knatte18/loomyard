// config.go holds fabric's configuration: the Config type, its layered-load spec and LoadConfig.
// Config carries the warp branch prefix, the weft-sync pathspec and the warp exclude list, all in fabric.yaml.
// LoadConfig resolves the fabric config file strictly through internal/configengine.Load and ConfigTemplate().
// The fabric module never reads config files or knows their layout itself.

package fabricengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// Config holds the settings every pair shares: the warp branch prefix, the weft-sync pathspec and the warp exclude list.
type Config struct {
	BranchPrefix string `yaml:"branch_prefix"`
	Pathspec     string `yaml:"pathspec"`
	WarpExclude  string `yaml:"warp_exclude"`
}

// Dirs returns the pathspec as a slice of directory names, split on whitespace.
func (c Config) Dirs() []string {
	return strings.Fields(c.Pathspec)
}

// WarpExcludePaths returns the warp exclude list as paths relative to a warp worktree's anchor, split on whitespace.
// They are excluded through the warp repository's info/exclude and are never wired as junctions.
func (c Config) WarpExcludePaths() []string {
	return strings.Fields(c.WarpExclude)
}

// ConfigSpec returns fabric's layered-load spec.
// Each key either shapes a pair at creation or reaches every worktree through the repository-wide exclude file.
// Every pair converges to the same value for each key, so none may be set in the local layer.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "fabric",
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			"branch_prefix": configengine.CategoryCrossPair,
			"pathspec":      configengine.CategoryCrossPair,
			"warp_exclude":  configengine.CategoryCrossPair,
		},
	}
}

// LoadConfig loads and unmarshals fabric configuration strictly through the hub-wide layer stack under dirs.
// It returns an error if the hub is not initialized, that is, if there is no _lym/ directory.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.Load(dirs, ConfigSpec())
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lym fabric reconcile\"; a session lym refuses the verb from reports status: FAILED and the orch runs it")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal fabric config: %w", err)
	}

	return cfg, nil
}

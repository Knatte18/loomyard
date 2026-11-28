// config.go holds fabric's configuration: the Config type, its layered-load spec and LoadConfig.
// Config carries the keys of fabric.yaml.
// They are the warp branch prefix, the weft-sync pathspec, the warp exclude list and the hub's secret-file patterns.
// LoadConfig resolves the fabric config file strictly through internal/configengine.Load and ConfigTemplate().
// The fabric module never reads config files or knows their layout itself.

package fabricengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/secretpath"
	"gopkg.in/yaml.v3"
)

// ExcludeListKey is fabric.yaml's key holding the warp exclude list.
const ExcludeListKey = "warp_exclude"

// SecretPathsKey is fabric.yaml's key holding the secret-file patterns a hub adds to the ones lym ships.
const SecretPathsKey = "secret_paths"

// Config holds the settings every pair shares:
// the warp branch prefix, the weft-sync pathspec, the warp exclude list and the hub's secret-file patterns.
type Config struct {
	BranchPrefix string `yaml:"branch_prefix"`
	Pathspec     string `yaml:"pathspec"`
	WarpExclude  string `yaml:"warp_exclude"`
	SecretPaths  string `yaml:"secret_paths"`
}

// SecretPathMatcher returns the matcher over lym's shipped secret-file patterns and the ones SecretPaths adds.
// SecretPaths is split on whitespace.
// A malformed added pattern is an error wrapping configengine.ErrInvalid that names the key.
func (c Config) SecretPathMatcher() (secretpath.Matcher, error) {
	m, err := secretpath.New(strings.Fields(c.SecretPaths))
	if err != nil {
		return secretpath.Matcher{}, fmt.Errorf("%w: fabric.yaml's %s: %w", configengine.ErrInvalid, SecretPathsKey, err)
	}
	return m, nil
}

// LoadSecretPathMatcher loads fabric configuration under dirs and returns its SecretPathMatcher.
// It serves a module that refuses a secret file in commits fabric does not make.
func LoadSecretPathMatcher(dirs configengine.Dirs) (secretpath.Matcher, error) {
	cfg, err := LoadConfig(dirs)
	if err != nil {
		return secretpath.Matcher{}, err
	}
	return cfg.SecretPathMatcher()
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
// A key shapes a pair at creation, or it reaches every worktree through the repository-wide exclude file.
// The secret-file key configures the guard on every commit.
// Every pair converges to the same value for each key, so none may be set in the local layer.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "fabric",
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			"branch_prefix": configengine.CategoryCrossPair,
			"pathspec":      configengine.CategoryCrossPair,
			ExcludeListKey:  configengine.CategoryCrossPair,
			SecretPathsKey:  configengine.CategoryGuard,
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

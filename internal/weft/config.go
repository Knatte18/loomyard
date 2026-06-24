// config.go — configuration for the weft module.
//
// Defines the Config type with a Pathspec field and LoadConfig.
// LoadConfig uses internal/config.Load with the ConfigTemplate() to strictly
// validate and resolve the weft config file; the weft module never reads
// config files or knows their layout itself.

package weft

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/config"
	"gopkg.in/yaml.v3"
)

// Config represents the configuration for the weft module.
type Config struct {
	Pathspec string `yaml:"pathspec"`
}

// Dirs returns the pathspec as a slice of directory names, split on whitespace.
func (c Config) Dirs() []string {
	return strings.Fields(c.Pathspec)
}

// LoadConfig loads and unmarshals configuration for the weft module from weftBaseDir.
//
// Calls config.Load with the weft ConfigTemplate() to strictly validate
// the config file against the template, resolve environment variables, and
// return resolved bytes. Unmarshals the resolved bytes into a Config struct.
//
// The weftBaseDir should be constructed as filepath.Join(layout.WeftWorktree(), layout.RelPath),
// so it reads the real _lyx/config/weft.yaml in the weft worktree (never via a host junction).
//
// If weftBaseDir/_lyx does not exist, returns an error wrapped as
// "weft worktree or its _lyx is missing at <weftBaseDir>".
func LoadConfig(weftBaseDir string) (Config, error) {
	// Load and resolve the config file using the template
	resolved, err := config.Load(weftBaseDir, "weft", []byte(ConfigTemplate()))
	if err != nil {
		// Wrap the generic error with a weft-specific message
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("weft worktree or its _lyx is missing at %s", weftBaseDir)
		}
		return Config{}, err
	}

	// Unmarshal resolved bytes into Config struct
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal weft config: %w", err)
	}

	return cfg, nil
}

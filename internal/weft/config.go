// config.go — configuration for the weft module.
//
// Defines the Config type with a Pathspec field, plus DefaultConfig and LoadConfig.
// LoadConfig delegates to internal/config for resolution and applies weft-specific
// error wrapping. The weft module never reads config files or knows their layout itself.

package weft

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/config"
)

// Config represents the configuration for the weft module.
type Config struct {
	Pathspec string `yaml:"pathspec"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Pathspec: "_lyx",
	}
}

// Dirs returns the pathspec as a slice of directory names, split on whitespace.
func (c Config) Dirs() []string {
	return strings.Fields(c.Pathspec)
}

// LoadConfig loads configuration for the weft module from weftBaseDir.
//
// The weftBaseDir should be constructed as filepath.Join(layout.WeftWorktree(), layout.RelPath),
// so it reads the real _lyx/config/weft.yaml in the weft worktree (never via a host junction).
//
// If weftBaseDir/_lyx does not exist, returns an error wrapped as
// "weft worktree or its _lyx is missing at <weftBaseDir>".
// Otherwise, returns a typed Config built from DefaultConfig()'s defaults and the
// loaded configuration.
func LoadConfig(weftBaseDir string) (Config, error) {
	// Build defaults map
	defaults := map[string]string{
		"pathspec": DefaultConfig().Pathspec,
	}

	// Load configuration using internal/config
	raw, err := config.Load(weftBaseDir, "weft", defaults)
	if err != nil {
		// Wrap the generic error with a weft-specific message
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("weft worktree or its _lyx is missing at %s", weftBaseDir)
		}
		return Config{}, err
	}

	// Map to typed struct
	cfg := Config{
		Pathspec: raw["pathspec"],
	}

	return cfg, nil
}

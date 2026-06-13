// config.go — configuration system for worktree module.
//
// Defines the Config type with a single field BranchPrefix, plus functions for
// loading configuration from YAML files. Mirrors the structure of internal/board/config.go.

package worktree

import (
	"fmt"
	"strings"

	"github.com/Knatte18/mhgo/internal/config"
)

// Config represents the configuration for a worktree module.
type Config struct {
	BranchPrefix string `yaml:"branch_prefix"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		BranchPrefix: "",
	}
}

// LoadConfig loads configuration for a module from configuration files.
//
// If <baseDir>/_mhgo/ does not exist, returns an error containing
// "not initialized here; run \"mhgo init\"".
//
// Otherwise, loads configuration using internal/config.Load with defaults from
// DefaultConfig(), and returns the result as a typed Config struct.
func LoadConfig(baseDir, module string) (Config, error) {
	// Build defaults map
	defaults := map[string]string{
		"branch_prefix": DefaultConfig().BranchPrefix,
	}

	// Load configuration using internal/config
	// config.Load checks _mhgo/ existence and returns appropriate error
	raw, err := config.Load(baseDir, module, defaults)
	if err != nil {
		// Wrap the generic error with a worktree-specific message
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"mhgo init\"")
		}
		return Config{}, err
	}

	// Map to typed struct
	cfg := Config{
		BranchPrefix: raw["branch_prefix"],
	}

	return cfg, nil
}

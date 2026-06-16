// config.go — configuration for the worktree module.
//
// Defines the Config type with a single field BranchPrefix, plus DefaultConfig
// and LoadConfig. LoadConfig delegates entirely to internal/config for resolution;
// the worktree module never reads config files or knows their layout itself.
// Mirrors the structure of internal/board/config.go.

package worktree

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/config"
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

// LoadConfig resolves the worktree config for baseDir via internal/config.Load,
// returning a typed Config built from DefaultConfig()'s defaults.
//
// If internal/config reports that baseDir is not an initialized Loomyard base, the
// error is rewrapped to "not initialized here; run \"lyx init\"".
func LoadConfig(baseDir, module string) (Config, error) {
	// Build defaults map
	defaults := map[string]string{
		"branch_prefix": DefaultConfig().BranchPrefix,
	}

	// Load configuration using internal/config
	// config.Load checks _lyx/ existence and returns appropriate error
	raw, err := config.Load(baseDir, module, defaults)
	if err != nil {
		// Wrap the generic error with a worktree-specific message
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx init\"")
		}
		return Config{}, err
	}

	// Map to typed struct
	cfg := Config{
		BranchPrefix: raw["branch_prefix"],
	}

	return cfg, nil
}

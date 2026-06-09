// config.go — layered configuration system for board modules.
//
// Defines the Config and Outputs types, plus functions for loading configuration
// from YAML files organized in layers. The system supports environment variable
// expansion and path resolution.

package board

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/mhgo/internal/config"
)

// Config represents the configuration for a board module.
type Config struct {
	Path           string `yaml:"path"`
	Home           string `yaml:"home"`
	Sidebar        string `yaml:"sidebar"`
	ProposalPrefix string `yaml:"proposal_prefix"`
}

// Outputs represents the output configuration values derived from Config.
type Outputs struct {
	Home           string
	Sidebar        string
	ProposalPrefix string
}

// Outputs returns the Outputs derived from a Config.
func (c Config) Outputs() Outputs {
	return Outputs{
		Home:           c.Home,
		Sidebar:        c.Sidebar,
		ProposalPrefix: c.ProposalPrefix,
	}
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Path:           "../_board",
		Home:           "Home.md",
		Sidebar:        "_Sidebar.md",
		ProposalPrefix: "proposal-",
	}
}

// DefaultOutputs returns the Outputs derived from DefaultConfig.
func DefaultOutputs() Outputs {
	return DefaultConfig().Outputs()
}

// LoadConfig loads configuration for a module from configuration files.
//
// If <baseDir>/_mhgo/ does not exist, returns an error containing
// "not initialized here; run \"mhgo init\"".
//
// Otherwise, loads configuration using internal/config.Load with defaults from
// DefaultConfig(), and returns the result as a typed Config struct.
func LoadConfig(baseDir, module string) (Config, error) {
	// Check if _mhgo/ directory exists
	mhgoDir := filepath.Join(baseDir, "_mhgo")
	_, err := os.Stat(mhgoDir)
	if os.IsNotExist(err) {
		return Config{}, fmt.Errorf("not initialized here; run \"mhgo init\"")
	}

	// Build defaults map
	defaults := map[string]string{
		"path":              DefaultConfig().Path,
		"home":              DefaultConfig().Home,
		"sidebar":           DefaultConfig().Sidebar,
		"proposal_prefix":   DefaultConfig().ProposalPrefix,
	}

	// Load configuration using internal/config
	raw, err := config.Load(baseDir, module, defaults)
	if err != nil {
		return Config{}, err
	}

	// Map to typed struct
	cfg := Config{
		Path:           raw["path"],
		Home:           raw["home"],
		Sidebar:        raw["sidebar"],
		ProposalPrefix: raw["proposal_prefix"],
	}

	// Resolve relative path
	if !filepath.IsAbs(cfg.Path) {
		cfg.Path = filepath.Join(baseDir, cfg.Path)
	}

	return cfg, nil
}

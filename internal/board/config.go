// config.go — configuration for the board module.
//
// Defines the Config and Outputs types, plus DefaultConfig and LoadConfig.
// LoadConfig delegates entirely to internal/config for resolution; the board
// module never reads config files or knows their layout itself.

package board

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/config"
)

// Config represents the configuration for a board module.
type Config struct {
	Path           string `yaml:"path"`
	Home           string `yaml:"home"`
	Sidebar        string `yaml:"sidebar"`
	ProposalPrefix string `yaml:"proposal_prefix"`
	// SkipGit and SkipPush are populated from BOARD_SKIP_* env at the CLI entry.
	SkipGit  bool
	SkipPush bool
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
// If <baseDir>/_lyx/ does not exist, returns an error containing
// "not initialized here; run \"lyx init\"".
//
// Otherwise, loads configuration using internal/config.Load with defaults from
// DefaultConfig(), and returns the result as a typed Config struct.
func LoadConfig(baseDir, module string) (Config, error) {
	// Build defaults map
	defaults := map[string]string{
		"path":            DefaultConfig().Path,
		"home":            DefaultConfig().Home,
		"sidebar":         DefaultConfig().Sidebar,
		"proposal_prefix": DefaultConfig().ProposalPrefix,
	}

	// Load configuration using internal/config
	// config.Load checks _lyx/ existence and returns appropriate error
	raw, err := config.Load(baseDir, module, defaults)
	if err != nil {
		// Wrap the generic error with a board-specific message
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx init\"")
		}
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

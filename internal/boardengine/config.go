// config.go — configuration for the boardengine module.
//
// Defines the Config and Outputs types and LoadConfig.
// LoadConfig uses internal/configengine.Load with the ConfigTemplate() to strictly
// validate and resolve the board config file; the boardengine module never reads
// config files or knows their layout itself.

package boardengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
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

// LoadConfig loads and unmarshals configuration for the board module.
//
// Calls configengine.Load with the board ConfigTemplate() to strictly validate
// the config file against the template, resolve environment variables, and
// return resolved bytes. Unmarshals the resolved bytes into a Config struct.
//
// If <baseDir>/_lyx/ does not exist, returns an error containing
// "not initialized here; run \"lyx init\"".
//
// Preserves relative-Path resolution (if !filepath.IsAbs(cfg.Path), resolves
// relative to baseDir).
func LoadConfig(baseDir, module string) (Config, error) {
	// Load and resolve the config file using the template
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		// Wrap the generic error with a board-specific message
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx init\"")
		}
		return Config{}, err
	}

	// Unmarshal resolved bytes into Config struct
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal board config: %w", err)
	}

	// Resolve relative path
	if !filepath.IsAbs(cfg.Path) {
		cfg.Path = filepath.Join(baseDir, cfg.Path)
	}

	return cfg, nil
}

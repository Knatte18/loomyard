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
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
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

// LoadConfig loads configuration for a module from layered configuration files.
//
// If <baseDir>/_mhgo/ does not exist, returns an error containing
// "not initialized here; run \"mhgo init\"".
//
// Otherwise, starts from DefaultConfig(), then overlays values from:
// 1. <baseDir>/_mhgo/<module>.yaml (if present)
// 2. <baseDir>/.mhgo/<module>.yaml (if present)
//
// Each layer is unmarshalled as a map[string]string, and only known keys
// (path, home, sidebar, proposal_prefix) are applied. Unknown keys are ignored.
//
// After merging all layers, environment variable expansion is performed on
// all string values using the $env:NAME syntax. After expansion, relative
// paths are resolved against baseDir.
//
// Malformed YAML or unset environment variables result in errors.
func LoadConfig(baseDir, module string) (Config, error) {
	// Check if _mhgo/ directory exists
	mhgoDir := filepath.Join(baseDir, "_mhgo")
	_, err := os.Stat(mhgoDir)
	if os.IsNotExist(err) {
		return Config{}, fmt.Errorf("not initialized here; run \"mhgo init\"")
	}

	// Start with default config
	cfg := DefaultConfig()

	// Layer files to process in order
	layerFiles := []string{
		filepath.Join(baseDir, "_mhgo", module+".yaml"),
		filepath.Join(baseDir, ".mhgo", module+".yaml"),
	}

	// Merge each layer
	for _, layerFile := range layerFiles {
		_, err := os.Stat(layerFile)
		if os.IsNotExist(err) {
			// File is absent, skip (no error)
			continue
		}
		if err != nil {
			return Config{}, fmt.Errorf("error reading layer file %s: %w", layerFile, err)
		}

		// Read and unmarshal the layer file
		content, err := os.ReadFile(layerFile)
		if err != nil {
			return Config{}, fmt.Errorf("error reading layer file %s: %w", layerFile, err)
		}

		layerMap := make(map[string]string)
		err = yaml.Unmarshal(content, layerMap)
		if err != nil {
			return Config{}, fmt.Errorf("error parsing YAML in layer file %s: %w", layerFile, err)
		}

		// Overlay known keys from this layer
		if val, ok := layerMap["path"]; ok {
			cfg.Path = val
		}
		if val, ok := layerMap["home"]; ok {
			cfg.Home = val
		}
		if val, ok := layerMap["sidebar"]; ok {
			cfg.Sidebar = val
		}
		if val, ok := layerMap["proposal_prefix"]; ok {
			cfg.ProposalPrefix = val
		}
	}

	// Expand environment variables in all string fields
	var expandErr error
	cfg.Path, expandErr = expandEnv(cfg.Path)
	if expandErr != nil {
		return Config{}, expandErr
	}
	cfg.Home, expandErr = expandEnv(cfg.Home)
	if expandErr != nil {
		return Config{}, expandErr
	}
	cfg.Sidebar, expandErr = expandEnv(cfg.Sidebar)
	if expandErr != nil {
		return Config{}, expandErr
	}
	cfg.ProposalPrefix, expandErr = expandEnv(cfg.ProposalPrefix)
	if expandErr != nil {
		return Config{}, expandErr
	}

	// Resolve Path: if relative, join with baseDir; if absolute, use as-is
	if !filepath.IsAbs(cfg.Path) {
		cfg.Path = filepath.Join(baseDir, cfg.Path)
	}

	return cfg, nil
}

// expandEnv expands environment variables in the form $env:NAME within a string.
// NAME must match [A-Za-z_][A-Za-z0-9_]*. Returns an error if a referenced
// variable is not set.
func expandEnv(s string) (string, error) {
	// Regex pattern: $env: followed by a valid env var name
	re := regexp.MustCompile(`\$env:([A-Za-z_][A-Za-z0-9_]*)`)

	var firstUnsetVar string

	result := re.ReplaceAllStringFunc(s, func(match string) string {
		// Extract the variable name
		parts := strings.SplitN(match, ":", 2)
		varName := parts[1]

		// Look up the environment variable
		value, ok := os.LookupEnv(varName)
		if !ok {
			if firstUnsetVar == "" {
				firstUnsetVar = varName
			}
			return match // Return original if not found, we'll error later
		}
		return value
	})

	if firstUnsetVar != "" {
		return "", fmt.Errorf("referenced env var %q is not set", firstUnsetVar)
	}

	return result, nil
}

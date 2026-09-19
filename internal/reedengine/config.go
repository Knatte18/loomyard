// config.go — configuration for the reed module.
//
// Defines the Config type mirroring reed.yaml's keys and LoadConfig, which uses
// internal/configengine.LoadOrTemplate with ConfigTemplate() to resolve the reed config file,
// degrading to the embedded template on proven absence; reed never reads config files or knows
// their on-disk layout itself.

package reedengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved reed.yaml configuration.
type Config struct {
	Tmux               string `yaml:"tmux"`
	Shell              string `yaml:"shell"`
	Width              int    `yaml:"width"`
	Height             int    `yaml:"height"`
	CollapsedStripRows int    `yaml:"collapsed_strip_rows"`
	MinFullRows        int    `yaml:"min_full_rows"`
	StrandName         string `yaml:"strand_name"`

	DebugLog string `yaml:"debug_log"`

	Mouse string `yaml:"mouse"`

	Watchdog string `yaml:"watchdog"`

	StatusLine StatusLineConfig `yaml:"status_line"`
	Selvage    SelvageConfig    `yaml:"selvage"`
}

// StatusLineConfig configures the tmux status-line's rendered text.
// This is a distinct mechanism from SelvageConfig: text in a tmux option, versus a row budget for a
// pane -- one block holding both would be misleading.
type StatusLineConfig struct {
	Template string `yaml:"template"`
}

// SelvageConfig configures the Selvage pane's fixed row budget.
type SelvageConfig struct {
	HeightRows int `yaml:"height_rows"`
}

// LoadConfig loads and unmarshals configuration for the reed module.
// An absent <baseDir>/_lyx/ directory or an absent config file resolves the embedded template;
// a config file that exists but is invalid still errors.
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal reed config: %w", err)
	}

	return cfg, nil
}

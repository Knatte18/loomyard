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
	Tmux          string `yaml:"tmux"`
	Shell         string `yaml:"shell"`
	Width         int    `yaml:"width"`
	Height        int    `yaml:"height"`
	CollapsedRows int    `yaml:"collapsed_rows"`
	MinFullRows   int    `yaml:"min_full_rows"`

	DebugLog string `yaml:"debug_log"`

	Mouse string `yaml:"mouse"`

	Watchdog string `yaml:"watchdog"`

	// OperatorQuietS is the seconds since an attached client's last activity on a strand's pane within which lyx types nothing into that pane.
	OperatorQuietS int `yaml:"operator_quiet_s"`

	// WindowShareMax is the largest declared group size whose strands share one window; a larger group gives each strand a window of its own.
	WindowShareMax int `yaml:"window_share_max"`

	// WindowSplit is the direction a shared window splits in: "width" or "height".
	WindowSplit string `yaml:"window_split"`

	TmuxConf string `yaml:"tmux_conf"`

	Selvage SelvageConfig `yaml:"selvage"`

	// SegmentColors maps a loom segment key to a palette color name.
	SegmentColors map[string]string `yaml:"segment_colors"`
}

// SelvageConfig configures the Selvage pane's fixed row budget.
type SelvageConfig struct {
	HeightRows int `yaml:"height_rows"`
}

// ConfigSpec returns reed's layered-load spec.
// It carries the GOOS template and the per-key classification that decides which keys a local override may hold.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "reed",
		Template: []byte(ConfigTemplate()),
		Categories: map[string]configengine.Category{
			"tmux":             configengine.CategoryExecutable,
			"shell":            configengine.CategoryExecutable,
			"width":            configengine.CategoryNone,
			"height":           configengine.CategoryNone,
			"collapsed_rows":   configengine.CategoryNone,
			"min_full_rows":    configengine.CategoryNone,
			"debug_log":        configengine.CategoryCrossPair,
			"mouse":            configengine.CategoryCrossPair,
			"watchdog":         configengine.CategoryNone,
			"operator_quiet_s": configengine.CategoryNone,
			"window_share_max": configengine.CategoryNone,
			"window_split":     configengine.CategoryNone,
			"tmux_conf":        configengine.CategorySourcedFile,
			"selvage":          configengine.CategoryNone,
			"segment_colors":   configengine.CategoryNone,
		},
	}
}

// windowSplitLayout maps a window_split value to the tmux split flag and layout a shared window uses.
// The empty string reads as "width", the template default, so a Config built in code without the key keeps working.
func windowSplitLayout(raw string) (splitFlag, layout string, err error) {
	switch raw {
	case "", "width":
		return "-h", "even-horizontal", nil
	case "height":
		return "-v", "even-vertical", nil
	}
	return "", "", fmt.Errorf("invalid window_split value %q: want \"width\" or \"height\"", raw)
}

// validateWindowConfig refuses a window_split other than "width" or "height" and a negative window_share_max.
// Each message names its key.
func validateWindowConfig(cfg Config) error {
	if _, _, err := windowSplitLayout(cfg.WindowSplit); err != nil {
		return err
	}
	if cfg.WindowShareMax < 0 {
		return fmt.Errorf("invalid window_share_max value %d: want an integer of at least 0", cfg.WindowShareMax)
	}
	return nil
}

// LoadConfig loads and unmarshals configuration for the reed module through the layer stack under dirs.
// An absent config file in every layer resolves the embedded template;
// a layer file that exists but is invalid still errors.
// module names the config file to read, so a caller may load a sibling file with reed's shape.
func LoadConfig(dirs configengine.Dirs, module string) (Config, error) {
	spec := ConfigSpec()
	spec.Name = module
	resolved, err := configengine.LoadOrTemplate(dirs, spec)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal reed config: %w", err)
	}

	return cfg, nil
}

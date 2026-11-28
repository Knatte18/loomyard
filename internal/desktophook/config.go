// config.go defines desktop.yaml: the Config type, the template and spec accessors, and LoadConfig.

package desktophook

import (
	_ "embed"
	"fmt"
	"slices"

	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for the desktop config module.
func ConfigTemplate() string {
	return configTemplate
}

// configModule is the config module name desktop.yaml is loaded under.
const configModule = "desktop"

// The placement keys of desktop.yaml, which a refused value names.
const (
	windowPlacementKey = "window_placement"
	minTileWidthKey    = "tiling.min_tile_width"
	minTileHeightKey   = "tiling.min_tile_height"
)

// Config represents the resolved desktop.yaml configuration.
type Config struct {
	// WindowCommand is the shell command run after lym opens a window; empty runs nothing.
	WindowCommand string `yaml:"window_command"`
	// WindowWaitMs is the milliseconds to wait after opening a window before the window command runs.
	WindowWaitMs int `yaml:"window_wait_ms"`

	// Buttons is the machine's additional status-bar buttons, a mapping of the groups task, beam and prime.
	// The node is decoded and validated only where the status bar renders it.
	Buttons yaml.Node `yaml:"buttons"`

	// WindowPlacement is the machine's default mode for the desktop a new terminal window lands on: off, same or next.
	WindowPlacement string `yaml:"window_placement"`
	// Tiling is the smallest tile of a desktop's grid, in logical pixels.
	Tiling struct {
		// MinTileWidth is the narrowest a tile may be.
		MinTileWidth int `yaml:"min_tile_width"`
		// MinTileHeight is the lowest a tile may be.
		MinTileHeight int `yaml:"min_tile_height"`
	} `yaml:"tiling"`
}

// ConfigSpec returns the desktop module's layered-load spec.
// Every key describes the machine's desktop,
// so each machine sets them in its own machine override and the local layer refuses them.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     configModule,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			"window_command": configengine.CategoryMachine,
			"window_wait_ms": configengine.CategoryMachine,
			"buttons":        configengine.CategoryMachine,

			windowPlacementKey: configengine.CategoryMachine,
			minTileWidthKey:    configengine.CategoryMachine,
			minTileHeightKey:   configengine.CategoryMachine,
		},
	}
}

// LoadConfig loads and unmarshals the desktop module's configuration through the hub-wide layer stack under dirs.
// A hub whose base file is absent resolves the template under the machine and local layers present.
// Opening a window therefore never fails on a hub not yet reconciled.
// It refuses a negative desktop.window_wait_ms, naming the key.
// It also refuses a desktop.window_placement other than off, same or next, and a tile size below 1.
// Each of those two refusals names the key and the command that sets it.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal desktop config: %w", err))
	}
	if cfg.WindowWaitMs < 0 {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("config key %q holds %d; want an integer of at least 0", configengine.KeyName(configModule, "window_wait_ms"), cfg.WindowWaitMs))
	}
	if modes := beamsetting.WindowPlacementModes(); !slices.Contains(modes, cfg.WindowPlacement) {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("config key %q holds %q; must be one of %s; way forward: run `lym config desktop --layer machine --set %s=<value>` from the prime", configengine.KeyName(configModule, windowPlacementKey), cfg.WindowPlacement, modes, windowPlacementKey))
	}
	for _, tile := range []struct {
		key  string
		size int
	}{
		{minTileWidthKey, cfg.Tiling.MinTileWidth},
		{minTileHeightKey, cfg.Tiling.MinTileHeight},
	} {
		if tile.size < 1 {
			return Config{}, configengine.MarkInvalid(fmt.Errorf("config key %q holds %d; want an integer of at least 1; way forward: run `lym config desktop --layer machine --set %s=<value>` from the prime", configengine.KeyName(configModule, tile.key), tile.size, tile.key))
		}
	}
	return cfg, nil
}

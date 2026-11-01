// config.go defines desktop.yaml: the Config type, the template and spec accessors, and LoadConfig.

package desktophook

import (
	_ "embed"
	"fmt"

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

// Config represents the resolved desktop.yaml configuration.
type Config struct {
	// WindowCommand is the shell command run after lym opens a window; empty runs nothing.
	WindowCommand string `yaml:"window_command"`
	// WindowWaitMs is the milliseconds lym waits after opening a window before it runs WindowCommand.
	WindowWaitMs int `yaml:"window_wait_ms"`

	// Buttons is the machine's additional status-bar buttons, a mapping of groups task, beam and prime shaped as reed.yaml's buttons.
	// The node is decoded and validated only where the status bar renders it.
	Buttons yaml.Node `yaml:"buttons"`
}

// ConfigSpec returns the desktop module's layered-load spec.
// Every key describes the machine's desktop, so each machine sets them in its own machine override and the local layer refuses them.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     configModule,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			"window_command": configengine.CategoryMachine,
			"window_wait_ms": configengine.CategoryMachine,
			"buttons":        configengine.CategoryMachine,
		},
	}
}

// LoadConfig loads and unmarshals the desktop module's configuration through the hub-wide layer stack under dirs.
// A hub whose base file is absent resolves the template under the machine and local layers present, so opening a window never fails on a hub not yet reconciled.
// It refuses a negative window_wait_ms, naming the key.
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
		return Config{}, configengine.MarkInvalid(fmt.Errorf("desktop config key %q holds %d; want an integer of at least 0", "window_wait_ms", cfg.WindowWaitMs))
	}
	return cfg, nil
}

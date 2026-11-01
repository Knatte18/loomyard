// config.go defines heddle.yaml: the Config type, the template and spec accessors, and LoadConfig.

package heddleengine

import (
	_ "embed"
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for the heddle config module.
func ConfigTemplate() string {
	return configTemplate
}

// configModule is the config module name heddle.yaml is loaded under.
const configModule = "heddle"

// LymOverrideKey is the heddle.yaml key that switches the hub's lym overrides on.
const LymOverrideKey = "lym_override"

// Config represents the resolved heddle.yaml configuration.
type Config struct {
	// LymOverride reports whether renders use the hub's overrides of shipped entries in place of the shipped texts.
	LymOverride bool `yaml:"lym_override"`
}

// ConfigSpec returns the heddle module's layered-load spec.
// The module is hub-wide, and its one key changes what every worktree of the hub renders, so the local layer refuses it.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     configModule,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			LymOverrideKey: configengine.CategoryCrossPair,
		},
	}
}

// LoadConfig loads and unmarshals the heddle module's configuration through the hub-wide layer stack under dirs.
// A hub whose base file is absent resolves the template, so a render never fails on a hub not yet reconciled.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal heddle config: %w", err))
	}
	return cfg, nil
}

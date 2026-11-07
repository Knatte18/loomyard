// config.go defines slack.yaml: the Config type, the template and spec accessors, and LoadConfig.

package slackengine

import (
	_ "embed"
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for the slack config module.
func ConfigTemplate() string {
	return configTemplate
}

// configModule is the config module name slack.yaml is loaded under.
const configModule = "slack"

// remindAfterMinKey is the key holding the minutes between dobby's reminders of a run waiting on the operator.
// The reminders apply while presence is slack.
const remindAfterMinKey = "remind_after_min"

// Config represents the resolved slack.yaml configuration.
type Config struct {
	// RemindAfterMin is the minutes between dobby's reminders of a run waiting on the operator while presence is slack.
	RemindAfterMin int `yaml:"remind_after_min"`
}

// ConfigSpec returns the slack module's layered-load spec.
// The module is hub-wide, and its one key bounds no resource and switches off no guard, so the local layer may hold it.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     configModule,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			remindAfterMinKey: configengine.CategoryNone,
		},
	}
}

// LoadConfig loads and unmarshals the slack module's configuration through the hub-wide layer stack under dirs.
// A hub whose base file is absent resolves the template.
// It refuses a remind_after_min under 1, naming the key and the way forward.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal slack config: %w", err))
	}
	if cfg.RemindAfterMin < 1 {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("slack config key %q holds %d; want an integer of at least 1; way forward: set `%s` to 1 or more with `lym config slack`", remindAfterMinKey, cfg.RemindAfterMin, remindAfterMinKey))
	}
	return cfg, nil
}

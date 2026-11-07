// config.go defines creel.yaml: the Config type, the template and spec accessors, and LoadConfig.

package creelconfig

import (
	_ "embed"
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for the creel config module.
func ConfigTemplate() string {
	return configTemplate
}

// configModule is the config module name creel.yaml is loaded under.
const configModule = "creel"

// trustedHubsKey is the creel.yaml key listing the hubs whose orch this hub obeys.
const trustedHubsKey = "trusted_hubs"

// Config represents the resolved creel.yaml configuration.
type Config struct {
	// TrustedHubs lists the repository names, owner/repo, of the hubs whose orch this hub obeys.
	// No entry is validated: a malformed one matches no sender.
	TrustedHubs []string `yaml:"trusted_hubs"`
}

// ConfigSpec returns the creel module's layered-load spec.
// The module is hub-wide, and its keys grant authority over this hub's agents, so the local layer refuses it.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     configModule,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			trustedHubsKey: configengine.CategoryGuard,
		},
	}
}

// LoadConfig loads and unmarshals the creel module's configuration through the hub-wide layer stack under dirs.
// A hub whose base file is absent resolves the template, which trusts no hub.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal creel config: %w", err))
	}
	return cfg, nil
}

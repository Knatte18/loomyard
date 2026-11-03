// config.go defines hibernate.yaml: the Config type, the template and spec accessors, and LoadConfig.

package hibernateconfig

import (
	_ "embed"
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for the hibernate config module.
func ConfigTemplate() string {
	return configTemplate
}

// configModule is the config module name hibernate.yaml is loaded under.
const configModule = "hibernate"

// resumeOnWakeKey is the key that makes the wake lift the request's hub pause.
const resumeOnWakeKey = "resume_on_wake"

// Config represents the resolved hibernate.yaml configuration.
type Config struct {
	// ResumeOnWake reports whether the wake from a hibernate request lifts the hub pause the request added, for a request that does not choose for itself.
	ResumeOnWake bool `yaml:"resume_on_wake"`
}

// ConfigSpec returns the hibernate module's layered-load spec.
// The module is hub-wide, and its one key decides whether every pair's run goes on after a wake, so the local layer refuses it.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     configModule,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			resumeOnWakeKey: configengine.CategoryCrossPair,
		},
	}
}

// LoadConfig loads and unmarshals the hibernate module's configuration through the hub-wide layer stack under dirs.
// A hub whose base file is absent resolves the template, so a hibernate request never fails on a hub not yet reconciled.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal hibernate config: %w", err))
	}
	return cfg, nil
}

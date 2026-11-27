// config.go holds the configuration of the sampler recipe.
//
// It defines the Config type, which mirrors sampler.yaml's keys.
// It also defines the ConfigTemplate and ConfigSpec accessors, and LoadConfig, which loads the file strictly.

package samplerengine

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for the sampler config module.
func ConfigTemplate() string {
	return configTemplate
}

// configModule is the config module name sampler.yaml is loaded under.
const configModule = "sampler"

// Config represents the resolved sampler.yaml configuration.
type Config struct {
	// Agent is the model-spec string selecting the model the sampler agent's session runs under.
	Agent string `yaml:"agent"`
	// CodeReview reports whether the Code-Review perch runs on a sampler run.
	CodeReview bool `yaml:"code_review"`
}

// ConfigSpec returns the sampler module's layered-load spec.
// Both keys are open to the local layer, so a task's run_settings or a profile block may set code_review.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     configModule,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			"agent":       configengine.CategoryNone,
			"code_review": configengine.CategoryNone,
		},
	}
}

// LoadConfig loads and unmarshals the sampler module's configuration strictly.
// The configuration is resolved through the hub-wide layer stack under dirs.
// It refuses an agent value that fails the model-spec grammar, naming the key.
func LoadConfig(dirs configengine.Dirs, module string) (Config, error) {
	spec := ConfigSpec()
	spec.Name = module
	resolved, err := configengine.Load(dirs, spec)
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lym fabric reconcile\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal sampler config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Agent); err != nil {
		return Config{}, fmt.Errorf("sampler config key %q: %w", "agent", err)
	}
	return cfg, nil
}

// config.go declares batten's config module, batten.yaml: its template, its layered-load spec and LoadConfig.

package battenshed

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns batten.yaml's default YAML template.
func ConfigTemplate() string {
	return configTemplate
}

// Config is the resolved batten.yaml: the flags of the switched rows a batten run passes through.
type Config struct {
	// OpenWindow switches the Open-Window row.
	OpenWindow bool `yaml:"open_window"`
	// SeedIDE switches the Seed-IDE row.
	SeedIDE bool `yaml:"seed_ide"`
	// OpenIDE switches the Open-IDE row.
	OpenIDE bool `yaml:"open_ide"`
}

// ConfigSpec returns batten's layered-load spec: a per-worktree module whose every key a local override, and so a task's run settings, may set.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "batten",
		Template: []byte(ConfigTemplate()),
		Categories: map[string]configengine.Category{
			"open_window": configengine.CategoryNone,
			"seed_ide":    configengine.CategoryNone,
			"open_ide":    configengine.CategoryNone,
		},
	}
}

// LoadConfig resolves batten.yaml from the layer stack under dirs.
// An absent file in every layer resolves the template; a layer file that exists but is invalid is an error.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal batten config: %w", err))
	}
	return cfg, nil
}

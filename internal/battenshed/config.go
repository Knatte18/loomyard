// config.go declares batten's config module, batten.yaml: its template, its layered-load spec and LoadConfig.

package battenshed

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
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
	// Tackler, TacklerTimeoutMin and TacklerMaxSpawns override shed.yaml's keys of the same names for a batten run;
	// an empty string, a zero and a null inherit shed.yaml, and a set TacklerMaxSpawns of 0 turns the tackler off.
	Tackler           string `yaml:"tackler"`
	TacklerTimeoutMin int    `yaml:"tackler_timeout_min"`
	TacklerMaxSpawns  *int   `yaml:"tackler_max_spawns"`
}

// ShedOverride returns the tackler keys of batten.yaml as the override of shed.yaml's.
// DriverKind stays empty: batten's own driver is always go, so batten.yaml holds no driver_kind.
func (c Config) ShedOverride() sheddriver.Override {
	return sheddriver.Override{
		Tackler:           c.Tackler,
		TacklerTimeoutMin: c.TacklerTimeoutMin,
		TacklerMaxSpawns:  c.TacklerMaxSpawns,
	}
}

// ValidateLocalValues checks the tackler keys a local-layer document sets, and leaves every other key to LoadConfig.
// A refusal is marked configengine.ErrInvalid and names the key.
func ValidateLocalValues(document []byte) error {
	return sheddriver.CheckLocalOverride("batten", document)
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

			"tackler":             configengine.CategoryNone,
			"tackler_timeout_min": configengine.CategoryNone,
			"tackler_max_spawns":  configengine.CategoryNone,
		},
	}
}

// LoadConfig resolves batten.yaml from the layer stack under dirs.
// An absent file in every layer resolves the template; a layer file that exists but is invalid is an error.
// A resolved document holding a driver_kind key is refused, since batten's own driver is always go.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var keys map[string]any
	if err := yaml.Unmarshal(resolved, &keys); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal batten config: %w", err))
	}
	if _, present := keys["driver_kind"]; present {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("batten config key %q: batten's own driver is always go, so batten.yaml holds no driver_kind; remove the key from batten.yaml", "driver_kind"))
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal batten config: %w", err))
	}
	if err := sheddriver.CheckOverride("batten", cfg.ShedOverride()); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

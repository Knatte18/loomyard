// config.go defines report.yaml: the Config type, the template and spec accessors, and LoadConfig.

package reportconfig

import (
	_ "embed"
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for the report config module.
func ConfigTemplate() string {
	return configTemplate
}

// configModule is the config module name report.yaml is loaded under.
const configModule = "report"

// costMinRunsKey is the report.yaml key holding the run count a cost cell needs before it shows figures.
const costMinRunsKey = "cost_min_runs"

// Config represents the resolved report.yaml configuration.
type Config struct {
	// CostMinRuns is the number of runs a cell of the cost report needs before it shows its figures instead of "too few runs".
	CostMinRuns int `yaml:"cost_min_runs"`
}

// ConfigSpec returns the report module's layered-load spec.
// The module is hub-wide, and its one key decides which cost figures the raddle agent prices a proposal from, so the local layer refuses it.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     configModule,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			costMinRunsKey: configengine.CategoryCrossPair,
		},
	}
}

// LoadConfig loads and unmarshals the report module's configuration through the hub-wide layer stack under dirs.
// A hub whose base file is absent resolves the template, so a report never fails on a hub not yet reconciled.
// It refuses a cost_min_runs under 1, naming the key and the way forward.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal report config: %w", err))
	}
	if cfg.CostMinRuns < 1 {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("report config key %q holds %d; want an integer of at least 1; way forward: run `lym config report --set %s=<runs>`", costMinRunsKey, cfg.CostMinRuns, costMinRunsKey))
	}
	return cfg, nil
}

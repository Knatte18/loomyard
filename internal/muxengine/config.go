// config.go — configuration for the mux module.
//
// Defines the Config type mirroring mux.yaml's keys and LoadConfig, which
// uses internal/configengine.Load with ConfigTemplate() to strictly
// validate and resolve the mux config file; mux never reads config files
// or knows their on-disk layout itself.

package muxengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved mux.yaml configuration: the machine tool
// paths psmux/pwsh/claude spawn, the window dimensions, the height-policy
// knobs render.Params carries, and the strand-name template.
type Config struct {
	Psmux              string `yaml:"psmux"`
	Pwsh               string `yaml:"pwsh"`
	Claude             string `yaml:"claude"`
	Width              int    `yaml:"width"`
	Height             int    `yaml:"height"`
	CollapsedStripRows int    `yaml:"collapsed_strip_rows"`
	TopBandRows        int    `yaml:"top_band_rows"`
	MinFullRows        int    `yaml:"min_full_rows"`
	StrandName         string `yaml:"strand_name"`
}

// LoadConfig loads and unmarshals configuration for the mux module.
//
// Calls configengine.Load with mux's ConfigTemplate() to strictly validate
// the config file against the template, resolve environment variables, and
// return resolved bytes. Unmarshals the resolved bytes into a Config
// struct. The module name is threaded through by the caller (never
// hardcoded to "mux" here), mirroring warpengine.LoadConfig.
//
// If <baseDir>/_lyx/ does not exist, returns an error containing
// "not initialized here; run \"lyx init\"".
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		// Wrap the generic "not initialized" error with the mux-specific hint,
		// matching warpengine's shape so every module surfaces the same
		// recovery instruction.
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx init\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal mux config: %w", err)
	}

	return cfg, nil
}

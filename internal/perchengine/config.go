// config.go — configuration for the perch module.
//
// Defines the Config type mirroring perch.yaml's keys and LoadConfig, which
// uses internal/configengine.Load with ConfigTemplate() to strictly
// validate and resolve the perch config file; perch never reads config
// files or knows their on-disk layout itself.

package perchengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved perch.yaml configuration: the model and
// effort the progress judge and the asking-triage call default to, and the
// engine-wide default milestone cap ladder a profile may override.
type Config struct {
	JudgeModel  string `yaml:"judge_model"`
	JudgeEffort string `yaml:"judge_effort"`
	RoundCaps   []int  `yaml:"round_caps"`
}

// LoadConfig loads and unmarshals configuration for the perch module.
//
// Calls configengine.Load with perch's ConfigTemplate() to strictly
// validate the config file against the template, resolve environment
// variables, and return resolved bytes. Unmarshals the resolved bytes into a
// Config struct. The module name is threaded through by the caller (never
// hardcoded to "perch" here), mirroring muxengine.LoadConfig and
// shuttleengine.LoadConfig.
//
// If <baseDir>/_lyx/ does not exist, returns an error containing
// "not initialized here; run \"lyx init\"".
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		// Wrap the generic "not initialized" error with the perch-specific
		// hint, matching muxengine/shuttleengine's shape so every module
		// surfaces the same recovery instruction.
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx init\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal perch config: %w", err)
	}

	return cfg, nil
}

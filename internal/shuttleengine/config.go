// config.go — configuration for the shuttle module.
//
// Defines the Config type mirroring shuttle.yaml's keys and LoadConfig, which uses
// internal/configengine.Load with ConfigTemplate() to strictly validate and resolve the shuttle
// config file;
// shuttle never reads config files or knows their on-disk layout itself.

package shuttleengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved shuttle.yaml configuration: run directories, timeouts, poll knobs,
// and claude engine denies.
type Config struct {
	RunDir         string `yaml:"run_dir"`
	PollIntervalMS int    `yaml:"poll_interval_ms"` // Wait loop's tick interval; non-positive is floored to template default.

	LivenessEveryNPolls int `yaml:"liveness_every_n_polls"`
	RunTimeoutMin       int `yaml:"run_timeout_min"` // Fallback run deadline in minutes; 0 means deadline equals start time, not "unlimited".

	StartupTimeoutS int `yaml:"startup_timeout_s"` // Startup probe timeout; 0 fast-fails as died and zeroes orphan sweep's protection window.

	Claude                    string `yaml:"claude"`
	ClaudeDenyAgentTool       bool   `yaml:"claude_deny_agent_tool"`
	ClaudeDenyAskUserQuestion bool   `yaml:"claude_deny_ask_user_question"`
}

// LoadConfig loads and unmarshals shuttle module configuration.
// It validates against the template, resolves environment variables, and unmarshals into Config.
// If <baseDir>/_lyx/ does not exist, it returns an error with recovery instructions.
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		// Wrap "not initialized" error with shuttle-specific recovery hint.
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx fabric reconcile\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal shuttle config: %w", err)
	}

	return cfg, nil
}

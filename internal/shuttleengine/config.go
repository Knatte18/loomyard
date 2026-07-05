// config.go — configuration for the shuttle module.
//
// Defines the Config type mirroring shuttle.yaml's keys and LoadConfig, which
// uses internal/configengine.Load with ConfigTemplate() to strictly
// validate and resolve the shuttle config file; shuttle never reads config
// files or knows their on-disk layout itself.

package shuttleengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved shuttle.yaml configuration: where run
// directories live, the poll/liveness/timeout knobs a run loop consumes, the
// claude executable path, and which PreToolUse denies the claude engine
// emits.
type Config struct {
	RunDir                    string `yaml:"run_dir"`
	PollIntervalMS            int    `yaml:"poll_interval_ms"`
	LivenessEveryNPolls       int    `yaml:"liveness_every_n_polls"`
	RunTimeoutMin             int    `yaml:"run_timeout_min"`
	StartupTimeoutS           int    `yaml:"startup_timeout_s"`
	Claude                    string `yaml:"claude"`
	ClaudeDenyAgentTool       bool   `yaml:"claude_deny_agent_tool"`
	ClaudeDenyAskUserQuestion bool   `yaml:"claude_deny_ask_user_question"`
}

// LoadConfig loads and unmarshals configuration for the shuttle module.
//
// Calls configengine.Load with shuttle's ConfigTemplate() to strictly
// validate the config file against the template, resolve environment
// variables, and return resolved bytes. Unmarshals the resolved bytes into a
// Config struct. The module name is threaded through by the caller (never
// hardcoded to "shuttle" here), mirroring muxengine.LoadConfig.
//
// If <baseDir>/_lyx/ does not exist, returns an error containing
// "not initialized here; run \"lyx init\"".
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		// Wrap the generic "not initialized" error with the shuttle-specific
		// hint, matching muxengine's shape so every module surfaces the same
		// recovery instruction.
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx init\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal shuttle config: %w", err)
	}

	return cfg, nil
}

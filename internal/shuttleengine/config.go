// config.go — configuration for the shuttle module.
//
// Defines the Config type mirroring shuttle.yaml's keys and LoadConfig, which uses
// internal/configengine.LoadOrTemplate with ConfigTemplate() to resolve the shuttle config file,
// degrading to the embedded template on proven absence;
// shuttle never reads config files or knows their on-disk layout itself.

package shuttleengine

import (
	"fmt"

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

	BackgroundShellWaitMin int `yaml:"background_shell_wait_min"` // Minutes a turn end waits on an outstanding background shell; LoadConfig refuses non-positive.

	SubmitSettleMS int `yaml:"submit_settle_ms"` // Pause between typed text and its submitting Enter; LoadConfig refuses negative.

	SubmitRedrawSettleMS int `yaml:"submit_redraw_settle_ms"` // Wait after an Enter before the input box is read to confirm the send; 0 reads at once, LoadConfig refuses negative.

	Claude                    string `yaml:"claude"`
	ClaudeDenyAgentTool       bool   `yaml:"claude_deny_agent_tool"`
	ClaudeDenyAskUserQuestion bool   `yaml:"claude_deny_ask_user_question"`

	// ClaudePromptCacheTTL is Claude Code's prompt-cache TTL for a role without a map entry; the claude engine validates it, not LoadConfig.
	ClaudePromptCacheTTL string `yaml:"claude_prompt_cache_ttl"`

	// ClaudePromptCacheTTLRoles maps a strand's role segment (Spec.Role) to Claude Code's prompt-cache TTL; the claude engine validates it, not LoadConfig.
	ClaudePromptCacheTTLRoles map[string]string `yaml:"claude_prompt_cache_ttl_roles"`
}

// ConfigOpenMaps returns the shuttle.yaml keys whose entries are the operator's own, which configengine carries whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{"claude_prompt_cache_ttl_roles"}
}

// LoadConfig loads and unmarshals shuttle module configuration.
// It validates against the template, resolves environment variables, and unmarshals into Config.
// An absent <baseDir>/_lyx/ directory or an absent config file resolves the embedded template;
// a config file that exists but is invalid still errors.
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(baseDir, module, []byte(ConfigTemplate()), ConfigOpenMaps()...)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal shuttle config: %w", err)
	}

	if cfg.BackgroundShellWaitMin <= 0 {
		return Config{}, fmt.Errorf("shuttle config: background_shell_wait_min must be positive, got %d", cfg.BackgroundShellWaitMin)
	}

	if cfg.SubmitSettleMS < 0 {
		return Config{}, fmt.Errorf("shuttle config: submit_settle_ms must not be negative, got %d", cfg.SubmitSettleMS)
	}

	if cfg.SubmitRedrawSettleMS < 0 {
		return Config{}, fmt.Errorf("shuttle config: submit_redraw_settle_ms must not be negative, got %d", cfg.SubmitRedrawSettleMS)
	}

	return cfg, nil
}

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

	BackgroundShellWaitMin int `yaml:"background_shell_wait_min"` // Minutes a turn end waits on an outstanding transcript-reported background shell; a shell the turn-end payload reports never expires. LoadConfig refuses non-positive.

	SubmitSettleMS int `yaml:"submit_settle_ms"` // Interval between the two input-box reads that must agree before a verified send's Enter, and the pause before a slash command's Enter; 0 reads back to back, LoadConfig refuses negative.

	SubmitRedrawSettleMS int `yaml:"submit_redraw_settle_ms"` // First step of the doubling interval at which the input box is read after an Enter to confirm the send; 0 reads at once, LoadConfig refuses negative.

	SendReadyTimeoutS int `yaml:"send_ready_timeout_s"` // Seconds a verified send waits for an idle session before it fails busy, ending at the run's deadline for a gate send; binds only an engine with the SessionCycler idle reading. LoadConfig refuses non-positive.

	SubmitConfirmTimeoutS int `yaml:"submit_confirm_timeout_s"` // Seconds from the moment a verified send begins typing within which it must be seen to land before it fails, ending at the run's deadline for a gate send; binds only an engine with InputBoxReader. LoadConfig refuses non-positive.

	Claude                    string `yaml:"claude"`
	ClaudeDenyAgentTool       bool   `yaml:"claude_deny_agent_tool"`
	ClaudeDenyAskUserQuestion bool   `yaml:"claude_deny_ask_user_question"`
	ClaudeDenyPython          bool   `yaml:"claude_deny_python"`

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

	if cfg.SendReadyTimeoutS <= 0 {
		return Config{}, fmt.Errorf("shuttle config: send_ready_timeout_s must be positive, got %d", cfg.SendReadyTimeoutS)
	}

	if cfg.SubmitConfirmTimeoutS <= 0 {
		return Config{}, fmt.Errorf("shuttle config: submit_confirm_timeout_s must be positive, got %d", cfg.SubmitConfirmTimeoutS)
	}

	return cfg, nil
}

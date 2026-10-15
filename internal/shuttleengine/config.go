// config.go — configuration for the shuttle module.
//
// Defines the Config type mirroring shuttle.yaml's keys and LoadConfig, which uses
// internal/configengine.LoadOrTemplate with ConfigTemplate() to resolve the shuttle config file,
// degrading to the embedded template on proven absence;
// shuttle never reads config files or knows their on-disk layout itself.

package shuttleengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved shuttle.yaml configuration: run directories, timeouts, poll knobs,
// and claude engine denies.
type Config struct {
	RunDir         string `yaml:"run_dir"`
	PollIntervalMS int    `yaml:"poll_interval_ms"` // Wait loop's tick interval; a value below one second is floored to it.

	LivenessEveryNPolls int `yaml:"liveness_every_n_polls"`
	RunTimeoutMin       int `yaml:"run_timeout_min"` // Fallback run deadline in minutes; 0 means deadline equals start time, not "unlimited".

	StartupTimeoutS int `yaml:"startup_timeout_s"` // Startup probe timeout; 0 fast-fails as died and zeroes orphan sweep's protection window.

	BackgroundShellWaitMin int `yaml:"background_shell_wait_min"` // Minutes an outstanding background shell of either signal may run before the wait logs it and shows it in the wait marker; it expires nothing. LoadConfig refuses non-positive.

	SubmitSettleMS int `yaml:"submit_settle_ms"` // Interval between the two input-box reads that must agree before a verified send's Enter, and the pause before the /model command's Enter; 0 reads back to back, LoadConfig refuses negative.

	SubmitRedrawSettleMS int `yaml:"submit_redraw_settle_ms"` // First step of the doubling interval at which the input box is read after an Enter to confirm the send; 0 reads at once, LoadConfig refuses negative.

	SendReadyTimeoutS int `yaml:"send_ready_timeout_s"` // Seconds a verified send waits for an idle session before it fails busy, ending at the run's deadline for a gate send; binds only an engine with the SessionCycler idle reading. LoadConfig refuses non-positive.

	SubmitConfirmTimeoutS int `yaml:"submit_confirm_timeout_s"` // Seconds from the moment a verified send begins typing within which it must be seen to land before it fails, ending at the run's deadline for a gate send; binds only an engine with InputBoxReader. LoadConfig refuses non-positive.

	// Engine names the engine of a spec that names none; it must be a registered engine name, checked where the registry builds the set.
	Engine string `yaml:"engine"`

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

// ConfigSpec returns shuttle's layered-load spec.
// It carries the template, the open maps and the per-key classification that decides which keys a local override may hold.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "shuttle",
		Template: []byte(ConfigTemplate()),
		OpenMaps: ConfigOpenMaps(),
		Categories: map[string]configengine.Category{
			"run_dir":                       configengine.CategoryCrossPair,
			"poll_interval_ms":              configengine.CategoryNone,
			"liveness_every_n_polls":        configengine.CategoryNone,
			"run_timeout_min":               configengine.CategoryNone,
			"startup_timeout_s":             configengine.CategoryNone,
			"background_shell_wait_min":     configengine.CategoryNone,
			"send_ready_timeout_s":          configengine.CategoryNone,
			"submit_confirm_timeout_s":      configengine.CategoryNone,
			"submit_settle_ms":              configengine.CategoryNone,
			"submit_redraw_settle_ms":       configengine.CategoryNone,
			"engine":                        configengine.CategoryNone,
			"claude":                        configengine.CategoryExecutable,
			"claude_deny_agent_tool":        configengine.CategoryGuard,
			"claude_deny_ask_user_question": configengine.CategoryGuard,
			"claude_deny_python":            configengine.CategoryGuard,
			"claude_prompt_cache_ttl":       configengine.CategoryNone,
			"claude_prompt_cache_ttl_roles": configengine.CategoryNone,
		},
	}
}

// LoadConfig loads and unmarshals shuttle module configuration through the layer stack under dirs.
// It validates against the template, resolves environment variables, and unmarshals into Config.
// An absent config file in every layer resolves the embedded template;
// a layer file that exists but is invalid still errors.
// module names the config file to read, so a caller may load a sibling file with shuttle's shape.
func LoadConfig(dirs configengine.Dirs, module string) (Config, error) {
	spec := ConfigSpec()
	spec.Name = module
	resolved, err := configengine.LoadOrTemplate(dirs, spec)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal shuttle config: %w", err)
	}

	if time.Duration(cfg.PollIntervalMS)*time.Millisecond < pollFloor {
		logger.Warn("shuttle config: poll_interval_ms is below the floor and is raised to it", "key", "poll_interval_ms", "value", cfg.PollIntervalMS, "floor_ms", pollFloor.Milliseconds())
	}

	if cfg.Engine == "" {
		return Config{}, fmt.Errorf("shuttle config: engine must name a registered engine, got an empty value")
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

// MigrateConfig raises a below-floor or null poll_interval_ms in a shuttle.yaml document to the poll floor, for lyx config reconcile, and returns the rewritten bytes with one line describing the rewrite.
// Everything else is carried through with its comments;
// the loader never calls it.
// A document that does not parse returns an error marked configengine.ErrInvalid.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	rewritten, line, err := yamlengine.RaiseIntFloor(existing, "poll_interval_ms", int(pollFloor.Milliseconds()))
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate shuttle config: %w", err))
	}
	if line == "" {
		return rewritten, nil, nil
	}
	return rewritten, []string{line}, nil
}

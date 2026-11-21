// config.go — configuration for the shuttle module.
//
// Defines the Config type, which mirrors shuttle.yaml's keys, and LoadConfig.
// LoadConfig resolves the shuttle config file through internal/configengine.LoadOrTemplate with ConfigTemplate(),
// and it degrades to the embedded template on proven absence.
// shuttle never reads config files or knows their on-disk layout itself.

package shuttleengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved shuttle.yaml configuration.
// It holds run directories, timeouts, poll knobs, and the python guard switch.
type Config struct {
	RunDir         string `yaml:"run_dir"`
	PollIntervalMS int    `yaml:"poll_interval_ms"` // Wait loop's tick interval; a value below one second is floored to it.

	LivenessEveryNPolls int `yaml:"liveness_every_n_polls"`
	RunTimeoutMin       int `yaml:"run_timeout_min"` // Fallback run deadline in minutes; 0 means deadline equals start time, not "unlimited".

	StartupTimeoutS int `yaml:"startup_timeout_s"` // Startup probe timeout; 0 fast-fails as died and zeroes orphan sweep's protection window.

	// StartRetries is how many extra launches a start gets after an attempt that fails this way:
	// its provider died before taking its first input, and its strand was removed.
	// 0 turns the retry off; a negative value is rejected when the config loads.
	StartRetries int `yaml:"start_retries"`

	// KeepWarmMaxReruns is how many keep-warm re-runs one long gate wait is worth.
	// That count is the break-even against a single cache write.
	// 0 turns the long wait off; a negative value is rejected when the config loads.
	KeepWarmMaxReruns int `yaml:"keep_warm_max_reruns"`

	BackgroundShellWaitMin int `yaml:"background_shell_wait_min"` // Minutes an outstanding background shell of either signal may run before the wait logs it and shows it in the wait marker; it also expires a waiting turn end whose every outstanding task is a non-awaited shell, an ungated one with an output file missing into a held turn end and a gated one with every output file present into a Done. LoadConfig refuses non-positive.

	SubmitSettleMS int `yaml:"submit_settle_ms"` // Interval between the two input-box reads that must agree before a verified send's Enter; 0 reads back to back, LoadConfig refuses negative.

	SubmitRedrawSettleMS int `yaml:"submit_redraw_settle_ms"` // First step of the doubling interval at which the input box is read after an Enter to confirm the send; 0 reads at once, LoadConfig refuses negative.

	SendReadyTimeoutS int `yaml:"send_ready_timeout_s"` // Seconds a verified send waits for an idle session before it fails busy, ending at the run's deadline for a gate send; binds only an engine with the SessionCycler idle reading. LoadConfig refuses non-positive.

	SubmitConfirmTimeoutS int `yaml:"submit_confirm_timeout_s"` // Seconds from the moment a verified send begins typing within which it must be seen to land before it fails, ending at the run's deadline for a gate send; binds only an engine with InputBoxReader. LoadConfig refuses non-positive.

	// Engine names the engine of a spec that names none.
	// It must be a registered engine name, checked where the registry builds the set.
	Engine string `yaml:"engine"`

	// DenyPython switches the python guard of the shell-guard table for every engine.
	// Each engine installs the guard's hook in every session,
	// and the hook reads this key from the hub's config on each matching call,
	// so a change reaches running sessions; the system-prompt sentence follows the value at launch.
	// Leave it off for a hub whose work needs Python.
	DenyPython bool `yaml:"deny_python"`

	// EditLoopEdits is how many repeat edits of one file by one agent within EditLoopWindowS make an edit loop.
	// A repeat is an edit that leaves the file with content an earlier edit of it in the window left.
	// An agent that makes progress leaves new content with each edit, however often it edits the file.
	// An autonomous run's Wait reports the loop to its parent.
	// 0 turns the watch off, and an engine then installs no edit hook.
	// A negative value is rejected when the config loads.
	EditLoopEdits int `yaml:"edit_loop_edits"`
	// EditLoopWindowS is the window, in seconds, the edits of an edit loop fall within.
	// LoadConfig refuses a non-positive value while EditLoopEdits is above 0.
	EditLoopWindowS int `yaml:"edit_loop_window_s"`

	// RefuseRaw lists the command prefixes the shell guard refuses to run raw, from the hub's gate config.
	// No config file sets it: an engine fills it on its own copy of the config, so it has no yaml key.
	RefuseRaw []string `yaml:"-"`

	// Agents is the hub's agents.yaml, loaded from the same directories as shuttle.yaml.
	// It answers the skills of a spec's config role and the efforts an engine gives a launch and its subagents.
	// No shuttle.yaml key sets it.
	Agents agentconfig.Config `yaml:"-"`
}

// StartAllowance returns the longest a start may take:
// every launch's startup_timeout_s, the first and each of start_retries retries, with the wait between attempts.
func (c Config) StartAllowance() time.Duration {
	launches := time.Duration(1+c.StartRetries) * time.Duration(c.StartupTimeoutS) * time.Second
	return launches + time.Duration(c.StartRetries)*startRetryDelay
}

// ConfigSpec returns shuttle's layered-load spec.
// It carries the template and the per-key classification, which decides which keys a local override may hold.
// It also carries the retired keys, which a local layer is refused by their category.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "shuttle",
		Template: []byte(ConfigTemplate()),
		Categories: map[string]configengine.Category{
			"run_dir":                   configengine.CategoryCrossPair,
			"poll_interval_ms":          configengine.CategoryNone,
			"liveness_every_n_polls":    configengine.CategoryNone,
			"run_timeout_min":           configengine.CategoryNone,
			"startup_timeout_s":         configengine.CategoryNone,
			"start_retries":             configengine.CategoryNone,
			"keep_warm_max_reruns":      configengine.CategoryNone,
			"background_shell_wait_min": configengine.CategoryNone,
			"send_ready_timeout_s":      configengine.CategoryNone,
			"submit_confirm_timeout_s":  configengine.CategoryNone,
			"submit_settle_ms":          configengine.CategoryNone,
			"submit_redraw_settle_ms":   configengine.CategoryNone,
			"engine":                    configengine.CategoryNone,
			"deny_python":               configengine.CategoryGuard,
			"edit_loop_edits":           configengine.CategoryNone,
			"edit_loop_window_s":        configengine.CategoryNone,
		},
		RetiredKeys: map[string]configengine.Category{
			"claude":                        configengine.CategoryExecutable,
			"claude_deny_agent_tool":        configengine.CategoryGuard,
			"claude_deny_ask_user_question": configengine.CategoryGuard,
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
// It also loads agents.yaml under dirs into Agents, and an agents.yaml that fails to load fails the call.
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

	if cfg.StartRetries < 0 {
		return Config{}, fmt.Errorf("shuttle config: start_retries must not be negative, got %d", cfg.StartRetries)
	}

	if cfg.KeepWarmMaxReruns < 0 {
		return Config{}, fmt.Errorf("shuttle config: keep_warm_max_reruns must not be negative, got %d; way forward: set it to 0 or more with lym config shuttle", cfg.KeepWarmMaxReruns)
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

	if cfg.EditLoopEdits < 0 {
		return Config{}, fmt.Errorf("shuttle config: edit_loop_edits must not be negative, got %d; way forward: set it to 0 to turn the edit-loop watch off, or above 0, with lym config shuttle", cfg.EditLoopEdits)
	}

	if cfg.EditLoopEdits > 0 && cfg.EditLoopWindowS <= 0 {
		return Config{}, fmt.Errorf("shuttle config: edit_loop_window_s must be positive while edit_loop_edits is above 0, got %d; way forward: set it above 0 with lym config shuttle", cfg.EditLoopWindowS)
	}

	agents, err := agentconfig.Load(dirs)
	if err != nil {
		return Config{}, err
	}
	cfg.Agents = agents

	return cfg, nil
}

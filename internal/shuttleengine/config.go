// config.go — configuration for the shuttle module.
//
// Defines the Config type mirroring shuttle.yaml's keys and LoadConfig, which uses
// internal/configengine.LoadOrTemplate with ConfigTemplate() to resolve the shuttle config file,
// degrading to the embedded template on proven absence;
// shuttle never reads config files or knows their on-disk layout itself.

package shuttleengine

import (
	"bytes"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved shuttle.yaml configuration: run directories, timeouts, poll knobs,
// and the python guard switch.
type Config struct {
	RunDir         string `yaml:"run_dir"`
	PollIntervalMS int    `yaml:"poll_interval_ms"` // Wait loop's tick interval; a value below one second is floored to it.

	LivenessEveryNPolls int `yaml:"liveness_every_n_polls"`
	RunTimeoutMin       int `yaml:"run_timeout_min"` // Fallback run deadline in minutes; 0 means deadline equals start time, not "unlimited".

	StartupTimeoutS int `yaml:"startup_timeout_s"` // Startup probe timeout; 0 fast-fails as died and zeroes orphan sweep's protection window.

	BackgroundShellWaitMin int `yaml:"background_shell_wait_min"` // Minutes an outstanding background shell of either signal may run before the wait logs it and shows it in the wait marker; it expires nothing. LoadConfig refuses non-positive.

	SubmitSettleMS int `yaml:"submit_settle_ms"` // Interval between the two input-box reads that must agree before a verified send's Enter; 0 reads back to back, LoadConfig refuses negative.

	SubmitRedrawSettleMS int `yaml:"submit_redraw_settle_ms"` // First step of the doubling interval at which the input box is read after an Enter to confirm the send; 0 reads at once, LoadConfig refuses negative.

	SendReadyTimeoutS int `yaml:"send_ready_timeout_s"` // Seconds a verified send waits for an idle session before it fails busy, ending at the run's deadline for a gate send; binds only an engine with the SessionCycler idle reading. LoadConfig refuses non-positive.

	SubmitConfirmTimeoutS int `yaml:"submit_confirm_timeout_s"` // Seconds from the moment a verified send begins typing within which it must be seen to land before it fails, ending at the run's deadline for a gate send; binds only an engine with InputBoxReader. LoadConfig refuses non-positive.

	// Engine names the engine of a spec that names none; it must be a registered engine name, checked where the registry builds the set.
	Engine string `yaml:"engine"`

	// DenyPython switches the python guard of the shell-guard table for every engine; off in a Python target repo.
	DenyPython bool `yaml:"deny_python"`
}

// ConfigSpec returns shuttle's layered-load spec.
// It carries the template, the per-key classification that decides which keys a local override may hold,
// and the retired keys an unreconciled layer may still hold.
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
			"background_shell_wait_min": configengine.CategoryNone,
			"send_ready_timeout_s":      configengine.CategoryNone,
			"submit_confirm_timeout_s":  configengine.CategoryNone,
			"submit_settle_ms":          configengine.CategoryNone,
			"submit_redraw_settle_ms":   configengine.CategoryNone,
			"engine":                    configengine.CategoryNone,
			"deny_python":               configengine.CategoryGuard,
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

// MigrateConfig raises a below-floor or null poll_interval_ms in a shuttle.yaml document to the poll floor, for lyx config reconcile.
// It also renames a legacy claude_deny_python to deny_python, and returns the rewritten bytes with one line per rewrite.
// Everything else is carried through with its comments;
// the loader never calls it.
// A document that does not parse returns an error marked configengine.ErrInvalid.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	rewritten, line, err := yamlengine.RaiseIntFloor(existing, "poll_interval_ms", int(pollFloor.Milliseconds()))
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate shuttle config: %w", err))
	}
	var lines []string
	if line != "" {
		lines = append(lines, line)
	}
	rewritten, renameLine, err := renameDenyPython(rewritten)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate shuttle config: %w", err))
	}
	if renameLine != "" {
		lines = append(lines, renameLine)
	}
	return rewritten, lines, nil
}

// renameDenyPython renames a top-level claude_deny_python key to deny_python in place, keeping its value, position and line comment, and returns the rewritten document with one line describing the rewrite.
// A document already holding deny_python beside claude_deny_python keeps deny_python and drops the legacy key, and the line reports the dropped value.
// A document holding neither key, or no content, comes back unchanged with an empty line.
// A document that does not parse, or whose root is not a mapping, returns an error.
func renameDenyPython(existing []byte) ([]byte, string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, "", fmt.Errorf("unmarshal yaml: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return existing, "", nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, "", fmt.Errorf("yaml root is not a mapping")
	}

	legacyIndex, currentIndex := -1, -1
	for i := 0; i+1 < len(root.Content); i += 2 {
		switch root.Content[i].Value {
		case "claude_deny_python":
			legacyIndex = i
		case "deny_python":
			currentIndex = i
		}
	}
	if legacyIndex < 0 {
		return existing, "", nil
	}

	var line string
	if currentIndex >= 0 {
		line = fmt.Sprintf("claude_deny_python: %s dropped, deny_python already set", root.Content[legacyIndex+1].Value)
		root.Content = append(root.Content[:legacyIndex], root.Content[legacyIndex+2:]...)
	} else {
		root.Content[legacyIndex].Value = "deny_python"
		line = "claude_deny_python: renamed to deny_python"
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, "", fmt.Errorf("encode yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, "", fmt.Errorf("encode yaml: %w", err)
	}
	return out.Bytes(), line, nil
}

// config.go — configuration for the orch module.
//
// Defines the Config type mirroring orch.yaml's keys and LoadConfig, which uses internal/configengine.LoadOrTemplate with ConfigTemplate() to resolve the orch config file, degrading to the embedded template on proven absence;
// orch never reads config files or knows their on-disk layout itself.

package orchengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"gopkg.in/yaml.v3"
)

// Template defaults; the accessors floor a non-positive value back to them, except the poll interval, which pollFloor floors.
const (
	defaultThresholdTokens     = 400000
	defaultSoftThresholdTokens = 300000
	defaultSoftIdleS           = 300
	defaultIdleGraceS          = 30
	defaultHandoffTimeoutS     = 600
	defaultPollIntervalMS      = 2000
)

// pollFloor is the shortest watcher tick interval; a configured value below it is floored to it.
const pollFloor = time.Second

// Fails to compile when the template default falls below pollFloor, since the template would then load with a warning.
const _ = uint(defaultPollIntervalMS*time.Millisecond - pollFloor)

// The accepted cycle_mode values.
const (
	// CycleClear runs the handoff, /clear and resume cycle.
	CycleClear = "clear"
	// CycleCompact compacts the session in place, keeping its session id and Remote Control link.
	CycleCompact = "compact"
)

// permissionBypass is the only accepted permission_mode.
const permissionBypass = "bypass"

// Config represents the resolved orch.yaml configuration.
// Read the numeric knobs through their accessors, which floor a non-positive value.
type Config struct {
	Model  string `yaml:"model"`  // Model alias; empty is the provider default.
	Effort string `yaml:"effort"` // Effort level in shuttle's engine vocabulary; empty is the provider default.

	PermissionMode string `yaml:"permission_mode"` // Only bypass is accepted; LoadConfig resolves an empty value to bypass and refuses any other.
	CycleMode      string `yaml:"cycle_mode"`      // CycleClear or CycleCompact; read it through Mode, which resolves empty to CycleCompact.

	ThresholdTokens     int `yaml:"threshold_tokens"`      // Hard cap: context tokens at which the watcher cycles the session.
	SoftThresholdTokens int `yaml:"soft_threshold_tokens"` // Context tokens at which the watcher may cycle at a natural break.
	SoftIdleS           int `yaml:"soft_idle_s"`           // Seconds the newest turn end must be quiet before a soft cycle, and a DEFER's hold.
	IdleGraceS          int `yaml:"idle_grace_s"`          // Seconds idle after the newest event before acting.
	HandoffTimeoutS     int `yaml:"handoff_timeout_s"`     // Seconds the session gets to write its handoff.
	PollIntervalMS      int `yaml:"poll_interval_ms"`      // Watcher tick interval in milliseconds.
}

// LoadConfig loads and unmarshals orch module configuration.
// An absent <baseDir>/_lyx/ directory or an absent config file resolves the embedded template;
// a config file that exists but is invalid still errors.
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal orch config: %w", err)
	}

	switch cfg.CycleMode {
	case "", CycleClear, CycleCompact:
	default:
		return Config{}, fmt.Errorf("orch config: cycle_mode %q is not accepted; set cycle_mode to %q or %q", cfg.CycleMode, CycleClear, CycleCompact)
	}
	switch cfg.PermissionMode {
	case "":
		cfg.PermissionMode = permissionBypass
	case permissionBypass:
	default:
		return Config{}, fmt.Errorf("orch config: permission_mode %q is not accepted; set permission_mode: bypass or remove the key", cfg.PermissionMode)
	}

	if time.Duration(cfg.PollIntervalMS)*time.Millisecond < pollFloor {
		logger.Warn("orch config: poll_interval_ms is below the floor and is raised to it", "key", "poll_interval_ms", "value", cfg.PollIntervalMS, "floor_ms", pollFloor.Milliseconds())
	}

	return cfg, nil
}

// Mode returns the cycle mode, resolving an empty value to CycleCompact.
func (c Config) Mode() string {
	if c.CycleMode == "" {
		return CycleCompact
	}
	return c.CycleMode
}

// Threshold returns the context-token count that triggers a cycle, flooring a non-positive value to the template default so a zero can never cycle on every turn.
func (c Config) Threshold() int {
	if c.ThresholdTokens <= 0 {
		return defaultThresholdTokens
	}
	return c.ThresholdTokens
}

// SoftThreshold returns the context-token count at which the watcher may cycle at a natural break, flooring a non-positive value to the template default.
// A value at or above Threshold never fires.
func (c Config) SoftThreshold() int {
	if c.SoftThresholdTokens <= 0 {
		return defaultSoftThresholdTokens
	}
	return c.SoftThresholdTokens
}

// SoftIdle returns how long the newest turn end must be quiet before a soft cycle, and how long a DEFER reply holds the next soft attempt, flooring a non-positive value to the template default.
func (c Config) SoftIdle() time.Duration {
	if c.SoftIdleS <= 0 {
		return defaultSoftIdleS * time.Second
	}
	return time.Duration(c.SoftIdleS) * time.Second
}

// IdleGrace returns how long the session must stay idle after its newest event, flooring a non-positive value to the template default.
func (c Config) IdleGrace() time.Duration {
	if c.IdleGraceS <= 0 {
		return defaultIdleGraceS * time.Second
	}
	return time.Duration(c.IdleGraceS) * time.Second
}

// HandoffTimeout returns how long the session gets to write its handoff, flooring a non-positive value to the template default.
func (c Config) HandoffTimeout() time.Duration {
	if c.HandoffTimeoutS <= 0 {
		return defaultHandoffTimeoutS * time.Second
	}
	return time.Duration(c.HandoffTimeoutS) * time.Second
}

// PollInterval returns the watcher's tick interval, flooring a value below pollFloor, non-positive included, to pollFloor so a zero can never busy-spin.
// It floors silently; LoadConfig logs the one warning.
func (c Config) PollInterval() time.Duration {
	interval := time.Duration(c.PollIntervalMS) * time.Millisecond
	if interval < pollFloor {
		return pollFloor
	}
	return interval
}

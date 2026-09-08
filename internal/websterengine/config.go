// config.go — configuration for the webster module.
//
// Defines the Config type mirroring webster.yaml's keys and LoadConfig, which uses
// internal/configengine.LoadOrTemplate with ConfigTemplate() to resolve webster's config file,
// degrading to the embedded template on proven absence, then validates each role model-spec's
// grammar via modelspec.Parse so a typo'd spec fails loud at load time rather than hours into a run
// when that role first spawns.

package websterengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved webster.yaml configuration: the two role model-specs (see
// contracts/specs/llm-model-spec.md's "Roles that use this notation" section) and the numeric knobs the
// Master session's bracket verbs consult.
type Config struct {
	// Master is the model-spec for the long-lived Master session that reads
	// the plan once and forks one implementer per batch in-session.
	Master string `yaml:"master"`
	// Recovery is the model-spec for the cold, fresh recovery strand
	// recover-batch spawns when a fork reports stuck or writes no report.
	Recovery string `yaml:"recovery"`

	// SelfFixCap is the maximum number of self-fix attempts a forked
	// implementer makes before reporting stuck.
	SelfFixCap int `yaml:"self_fix_cap"`
	// MasterTimeoutMin is the number of minutes the Master session's own
	// shuttle spawn is allowed to run — the WHOLE-RUN timeout (the
	// orchestrator_timeout_min analog, spanning every batch of the plan),
	// never a per-batch timeout.
	MasterTimeoutMin int `yaml:"master_timeout_min"`
	// RecoveryTimeoutMin is the number of minutes since spawn after which
	// the cold recovery strand with no report and no live strand classifies
	// dead (dead_reason: timeout); applies only to recover-batch.
	RecoveryTimeoutMin int `yaml:"recovery_timeout_min"`
	// PollWaitS is the number of seconds a single recover-batch call blocks
	// watching the recovery strand for a terminal state before returning a
	// running snapshot.
	PollWaitS int `yaml:"poll_wait_s"`
}

// LoadConfig loads configuration from the webster module's config file, validates role model-spec
// grammar, and returns a Config struct.
// An absent <baseDir>/_lyx/ directory or an absent config file resolves the embedded template;
// a config file that exists but is invalid still errors.
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal webster config: %w", err)
	}

	roles := []struct {
		key   string
		value string
	}{
		{"master", cfg.Master},
		{"recovery", cfg.Recovery},
	}
	for _, role := range roles {
		if _, err := modelspec.Parse(role.value); err != nil {
			return Config{}, fmt.Errorf("webster config key %q: %w", role.key, err)
		}
	}

	// The numeric knobs are validated for the same reason the role specs are: LoadOrTemplate
	// degrades to the embedded template only on PROVEN ABSENCE of the file, so a hand-written
	// webster.yaml carrying only the two role keys leaves every one of these at Go's zero value —
	// and the zero values are not merely conservative, they break the run silently.
	// RecoveryTimeoutMin at 0 makes classify's `Elapsed > BatchTimeout` true on the very first poll,
	// so EVERY recovery batch classifies dead/timeout immediately; PollWaitS at 0 gives recover-batch
	// no wait budget at all; MasterTimeoutMin at 0 does the same to the Master spawn. Nothing
	// anywhere reported why. Failing at load names the key instead.
	knobs := []struct {
		key   string
		value int
	}{
		{"self_fix_cap", cfg.SelfFixCap},
		{"master_timeout_min", cfg.MasterTimeoutMin},
		{"recovery_timeout_min", cfg.RecoveryTimeoutMin},
		{"poll_wait_s", cfg.PollWaitS},
	}
	for _, knob := range knobs {
		if knob.value <= 0 {
			return Config{}, fmt.Errorf("webster config key %q: must be a positive integer, got %d — a missing or zero value silently classifies every recovery batch dead on its first poll rather than degrading", knob.key, knob.value)
		}
	}

	return cfg, nil
}

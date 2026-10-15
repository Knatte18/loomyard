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
	// VerifyGateAttempts is the number of attempts the plan-level verify gate on Merriam's strand gives Merriam before the gate stops the run.
	VerifyGateAttempts int `yaml:"verify_gate_attempts"`
	// KickstartPack true renders the batch's resolved member glyphs into the fork prompt;
	// false renders the fork prompt as it was before the pack.
	KickstartPack bool `yaml:"kickstart_pack"`
	// KickstartPackFile true renders the pack in its own file, `<batch>-pack.md`, with the fork prompt ending on the `## Start here` read line;
	// false renders the pack inline in the fork prompt and writes no pack file.
	// It has an effect only when a pack is built.
	KickstartPackFile bool `yaml:"kickstart_pack_file"`
	// KickstartPackPerCard true renders the per-card layout, in which the fork fetches each card's pack with `lyx loom card-pack` at that card's start and no pack file is written.
	// It overrides kickstart_pack_file, which picks between the file and inline layouts only when this key is false.
	// Like the other pack keys, it has an effect only when a pack is built.
	// The per-card layout applies to a hub run;
	// a standalone run keeps the layout kickstart_pack_file picks, because `lyx loom card-pack` cannot run there.
	// Every begin-batch reads the key afresh.
	KickstartPackPerCard bool `yaml:"kickstart_pack_per_card"`
}

// ConfigSpec returns webster's layered-load spec.
// It carries the template and the per-key classification that decides which keys a local override may hold.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "webster",
		Template: []byte(ConfigTemplate()),
		Categories: map[string]configengine.Category{
			"master":                  configengine.CategoryNone,
			"recovery":                configengine.CategoryNone,
			"self_fix_cap":            configengine.CategoryNone,
			"master_timeout_min":      configengine.CategoryNone,
			"recovery_timeout_min":    configengine.CategoryNone,
			"verify_gate_attempts":    configengine.CategoryNone,
			"kickstart_pack":          configengine.CategoryNone,
			"kickstart_pack_file":     configengine.CategoryNone,
			"kickstart_pack_per_card": configengine.CategoryNone,
		},
	}
}

// LoadConfig loads configuration from the webster module's layer stack under dirs, validates role model-spec
// grammar, and returns a Config struct.
// An absent config file in every layer resolves the embedded template;
// a layer file that exists but is invalid still errors.
// module names the config file to read, so a caller may load a sibling file with webster's shape.
func LoadConfig(dirs configengine.Dirs, module string) (Config, error) {
	spec := ConfigSpec()
	spec.Name = module
	resolved, err := configengine.LoadOrTemplate(dirs, spec)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal webster config: %w", err))
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
			return Config{}, configengine.MarkInvalid(fmt.Errorf("webster config key %q: %w", role.key, err))
		}
	}

	// The numeric knobs are validated for the same reason the role specs are.
	// The template fill supplies a knob a file omits, so only an explicit non-positive value reaches this check.
	// Such a value is not merely conservative, it breaks the run silently.
	// RecoveryTimeoutMin at 0 makes classify's `Elapsed > BatchTimeout` true on the very first poll,
	// so EVERY recovery batch classifies dead/timeout immediately.
	// MasterTimeoutMin at 0 does the same to the Master spawn.
	// VerifyGateAttempts at 0 turns the must-pass verify gate off, since shuttle skips an entry with no attempts, so the plan-level verify never runs.
	// Nothing anywhere reported why.
	// Failing at load names the key instead.
	knobs := []struct {
		key   string
		value int
	}{
		{"self_fix_cap", cfg.SelfFixCap},
		{"master_timeout_min", cfg.MasterTimeoutMin},
		{"recovery_timeout_min", cfg.RecoveryTimeoutMin},
		{"verify_gate_attempts", cfg.VerifyGateAttempts},
	}
	for _, knob := range knobs {
		if knob.value <= 0 {
			return Config{}, configengine.MarkInvalid(fmt.Errorf("webster config key %q: must be a positive integer, got %d — a missing or zero value silently classifies every recovery batch dead on its first poll rather than degrading", knob.key, knob.value))
		}
	}

	return cfg, nil
}

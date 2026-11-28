// config.go defines the Config type, which mirrors the keys of webster.yaml, and LoadConfig, which loads it.
// LoadConfig resolves the file through configengine.LoadOrTemplate with ConfigTemplate().
// An absent file in every layer resolves to the embedded template.
// It validates each role model-spec with modelspec.Parse,
// so a malformed spec fails at load time rather than when its role first spawns.

package websterengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// Config is the resolved webster.yaml configuration.
// It holds the two role model-specs, whose notation contracts/specs/llm-model-spec.md describes,
// and the numeric knobs the Master session's bracket verbs consult.
type Config struct {
	// Master is the model-spec for the long-lived Master session.
	// It reads the plan once and forks one implementer per batch in-session.
	Master string `yaml:"master"`
	// Recovery is the model-spec for the cold, fresh strand that recover-batch spawns.
	// It runs when a fork reports stuck or writes no report.
	Recovery string `yaml:"recovery"`

	// SelfFixCap is the maximum number of self-fix attempts a forked implementer makes.
	// Once the cap is used up, it reports stuck.
	SelfFixCap int `yaml:"self_fix_cap"`
	// MasterTimeoutMin is the number of minutes the Master session's own shuttle spawn may run.
	// It is the whole-run timeout.
	// It is never a per-batch timeout.
	// It spans every batch of the plan, so each batch runs inside this window.
	MasterTimeoutMin int `yaml:"master_timeout_min"`
	// RecoveryTimeoutMin is the number of minutes after spawn at which the cold recovery strand classifies dead.
	// It applies only while the strand has no report and no live strand.
	// Its dead reason is timeout, and it applies only to recover-batch.
	RecoveryTimeoutMin int `yaml:"recovery_timeout_min"`
	// VerifyGateAttempts is how many attempts the plan-level verify gate gives the Master session before the run stops.
	VerifyGateAttempts int `yaml:"verify_gate_attempts"`
}

// ConfigSpec returns webster's layered-load spec.
// It carries the template and the per-key classification that decides which keys a local override may hold.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "webster",
		Template: []byte(ConfigTemplate()),
		Categories: map[string]configengine.Category{
			"master":               configengine.CategoryNone,
			"recovery":             configengine.CategoryNone,
			"self_fix_cap":         configengine.CategoryNone,
			"master_timeout_min":   configengine.CategoryNone,
			"recovery_timeout_min": configengine.CategoryNone,
			"verify_gate_attempts": configengine.CategoryNone,
		},
		RetiredKeys: map[string]configengine.Category{
			"kickstart_pack":          configengine.CategoryNone,
			"kickstart_pack_per_card": configengine.CategoryNone,
			"kickstart_pack_file":     configengine.CategoryNone,
		},
	}
}

// LoadConfig loads configuration from the webster module's layer stack under dirs.
// It validates the role model-spec grammar and returns the resulting Config.
// An absent config file in every layer resolves the embedded template.
// A layer file that exists but is invalid still returns an error.
// module names the config file to read, so a caller can load a sibling file with the same shape.
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
	// Such a value is not merely conservative; it breaks the run silently.
	// A zero RecoveryTimeoutMin makes classify's `Elapsed > BatchTimeout` true on the first poll,
	// so every recovery batch classifies dead with reason timeout immediately.
	// A zero MasterTimeoutMin does the same to the Master spawn.
	// A zero VerifyGateAttempts turns the must-pass verify gate off, since shuttle skips an entry with no attempts,
	// so the plan-level verify never runs.
	// Without this check, nothing reports why.
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

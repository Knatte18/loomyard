// config.go defines Config, which mirrors the keys of weaver.yaml, and the loading and ladder resolution over it.
// LoadConfig resolves the file through configengine.LoadOrTemplate with ConfigTemplate().
// An absent file in every layer resolves to the embedded template.

package weaverengine

import (
	_ "embed"
	"fmt"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"gopkg.in/yaml.v3"
)

//go:embed template.yaml
var configTemplate string

// Config is the resolved weaver.yaml configuration.
type Config struct {
	// Ladder is the model-specs a batch attempt climbs in order, one rung per failed attempt.
	Ladder []string `yaml:"ladder"`
	// AttemptTimeoutMin is the number of minutes at which a batch attempt's strand times out.
	// The minutes count from the strand's spawn, or from an attach for a strand a crashed call left.
	// The waits on the attempt's gates and their fixers count against it.
	AttemptTimeoutMin int `yaml:"attempt_timeout_min"`
	// FixerTimeoutMin is the number of minutes after spawn at which a plan-level verify fixer strand classifies dead.
	FixerTimeoutMin int `yaml:"fixer_timeout_min"`
	// VerifyGate is the gate key of the plan-level verify Weaver's Done rests on.
	VerifyGate string `yaml:"verify_gate"`
	// BatchProfile names the batcher.yaml profile Weaver groups its cards with; empty means one card per batch.
	BatchProfile string `yaml:"batch_profile"`
}

// BatchProfileKeyName returns the full name of the weaver.yaml key naming Weaver's batch profile.
func BatchProfileKeyName() string {
	return keyName(batcher.ChoiceKey)
}

// ConfigTemplate returns the default YAML template for weaver configuration.
func ConfigTemplate() string {
	return configTemplate
}

// ConfigOpenMaps returns the weaver.yaml keys a layer replaces whole, the weaver.ladder list.
func ConfigOpenMaps() []string {
	return []string{"ladder"}
}

// ConfigSpec returns weaver's layered-load spec.
// It carries the template and the per-key classification that decides which keys a local override may hold.
// weaver.verify_gate is a guard key: a narrower gate key would hollow the plan-level verify Weaver's Done rests on.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "weaver",
		Template: []byte(ConfigTemplate()),
		Categories: map[string]configengine.Category{
			"ladder":              configengine.CategoryNone,
			"attempt_timeout_min": configengine.CategoryNone,
			"fixer_timeout_min":   configengine.CategoryNone,
			"verify_gate":         configengine.CategoryGuard,
			batcher.ChoiceKey:     configengine.CategoryNone,
		},
		OpenMaps: ConfigOpenMaps(),
	}
}

// LoadConfig loads configuration from the weaver module's layer stack under dirs.
// It refuses a ladder spec modelspec.Parse rejects and an empty `weaver.verify_gate`.
// It also refuses a non-positive `weaver.attempt_timeout_min` or `weaver.fixer_timeout_min`.
// Each refusal names the key and is marked configengine.MarkInvalid.
// module names the config file to read.
func LoadConfig(dirs configengine.Dirs, module string) (Config, error) {
	spec := ConfigSpec()
	spec.Name = module
	resolved, err := configengine.LoadOrTemplate(dirs, spec)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal weaver config: %w", err))
	}
	for i, rung := range cfg.Ladder {
		if _, err := modelspec.Parse(rung); err != nil {
			return Config{}, configengine.MarkInvalid(fmt.Errorf("config key %q rung %d: %w", keyName(ladderKey), i+1, err))
		}
	}
	if cfg.AttemptTimeoutMin <= 0 {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("config key %q: must be a positive integer, got %d", keyName(attemptTimeoutMinKey), cfg.AttemptTimeoutMin))
	}
	if cfg.FixerTimeoutMin <= 0 {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("config key %q: must be a positive integer, got %d", keyName(fixerTimeoutMinKey), cfg.FixerTimeoutMin))
	}
	if cfg.VerifyGate == "" {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("config key %q: must name a gate key, got none", keyName(verifyGateKey)))
	}
	return cfg, nil
}

// ResolveLadder resolves cfg's ladder against reg and returns one ModelChoice per rung, in order.
// It refuses an empty ladder and a spec the registry cannot resolve, each naming the key and `lym config weaver`.
func ResolveLadder(cfg Config, reg modelspec.Registry) ([]shuttleengine.ModelChoice, error) {
	if len(cfg.Ladder) == 0 {
		return nil, fmt.Errorf("weaver: %s is empty; way forward: list at least one model-spec under it with `lym config weaver`", keyName(ladderKey))
	}
	choices := make([]shuttleengine.ModelChoice, 0, len(cfg.Ladder))
	for i, rung := range cfg.Ladder {
		parsed, err := modelspec.Parse(rung)
		if err != nil {
			return nil, fmt.Errorf("weaver: rung %d of %s: %w; way forward: fix the spec with `lym config weaver`", i+1, keyName(ladderKey), err)
		}
		resolved, err := reg.Resolve(parsed)
		if err != nil {
			return nil, fmt.Errorf("weaver: rung %d of %s: %w; way forward: name a known alias with `lym config weaver`", i+1, keyName(ladderKey), err)
		}
		choices = append(choices, shuttleengine.NewModelChoice(resolved))
	}
	return choices, nil
}

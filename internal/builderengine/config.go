// config.go — configuration for the builder module.
//
// Defines the Config type mirroring builder.yaml's keys and LoadConfig,
// which uses internal/configengine.Load with ConfigTemplate() to strictly
// validate and resolve builder's config file, then validates each role
// model-spec's grammar via modelspec.Parse so a typo'd spec fails loud at
// load time rather than hours into a run when that role first spawns.

package builderengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved builder.yaml configuration: the four
// role model-specs (see docs/reference/model-spec.md's "Roles that use this
// notation" section) and the numeric knobs the batch loop and its
// validation gate consult.
type Config struct {
	// Orchestrator is the model-spec for the long-lived orchestrator
	// session that drives the batch loop.
	Orchestrator string `yaml:"orchestrator"`
	// Implementer is the model-spec for a normal-sized batch's
	// implementer spawn.
	Implementer string `yaml:"implementer"`
	// ImplementerOversized is the model-spec for a batch whose frontmatter
	// carries oversized: true.
	ImplementerOversized string `yaml:"implementer_oversized"`
	// Recovery is the model-spec for the fresh escalated recovery spawn
	// the orchestrator triggers after a batch reports stuck.
	Recovery string `yaml:"recovery"`

	// SelfFixCap is the maximum number of self-fix attempts an
	// implementer makes before reporting stuck.
	SelfFixCap int `yaml:"self_fix_cap"`
	// PollWaitS is the number of seconds `poll --wait` blocks watching
	// for the in-flight batch's terminal state before returning a
	// running snapshot.
	PollWaitS int `yaml:"poll_wait_s"`
	// BatchTimeoutMin is the number of minutes since spawn after which a
	// batch with no report and no live strand classifies dead
	// (dead_reason: timeout).
	BatchTimeoutMin int `yaml:"batch_timeout_min"`
	// OrchestratorTimeoutMin is the number of minutes the orchestrator's
	// own shuttle spawn is allowed to run.
	OrchestratorTimeoutMin int `yaml:"orchestrator_timeout_min"`
	// BatchContextCapTokens is validation check 5's context-estimate cap:
	// a batch whose estimated context exceeds this without oversized:
	// true fails validation.
	BatchContextCapTokens int `yaml:"batch_context_cap_tokens"`
	// BatchCardCap is validation check 5's card-count cap per batch.
	BatchCardCap int `yaml:"batch_card_cap"`
}

// LoadConfig loads and unmarshals configuration for the builder module.
//
// Calls configengine.Load with builder's ConfigTemplate() to strictly
// validate the config file against the template, resolve environment
// variables, and return resolved bytes. Unmarshals the resolved bytes into
// a Config struct. The module name is threaded through by the caller
// (never hardcoded to "builder" here), mirroring perchengine.LoadConfig.
//
// After unmarshal, each of the four role strings is checked against
// modelspec.Parse for grammar only (registry resolution is a separate
// pre-flight — see ResolveRoles); a grammar error is wrapped naming the
// offending config key, so a hand-edited builder.yaml with a malformed spec
// fails here rather than at spawn time.
//
// If <baseDir>/_lyx/ does not exist, returns an error containing
// "not initialized here; run \"lyx init\"".
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		// Wrap the generic "not initialized" error with the builder-specific
		// hint, matching perchengine/muxengine/shuttleengine's shape so every
		// module surfaces the same recovery instruction.
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx init\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal builder config: %w", err)
	}

	// Validate every role's model-spec grammar now, naming the offending
	// key, so a malformed spec is caught at load time rather than silently
	// carried into ResolveRoles or a spawn site.
	roles := []struct {
		key   string
		value string
	}{
		{"orchestrator", cfg.Orchestrator},
		{"implementer", cfg.Implementer},
		{"implementer_oversized", cfg.ImplementerOversized},
		{"recovery", cfg.Recovery},
	}
	for _, role := range roles {
		if _, err := modelspec.Parse(role.value); err != nil {
			return Config{}, fmt.Errorf("builder config key %q: %w", role.key, err)
		}
	}

	return cfg, nil
}

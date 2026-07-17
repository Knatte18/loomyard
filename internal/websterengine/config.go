// config.go — configuration for the webster module.
//
// Defines the Config type mirroring webster.yaml's keys and LoadConfig,
// which uses internal/configengine.Load with ConfigTemplate() to strictly
// validate and resolve webster's config file, then validates each role
// model-spec's grammar via modelspec.Parse so a typo'd spec fails loud at
// load time rather than hours into a run when that role first spawns.

package websterengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved webster.yaml configuration: the three
// role model-specs (see docs/reference/model-spec.md's "Roles that use this
// notation" section) and the numeric knobs the Master session's bracket
// verbs and validation gate consult.
type Config struct {
	// Master is the model-spec for the long-lived Master session that reads
	// the plan once and forks one implementer per batch in-session.
	Master string `yaml:"master"`
	// MasterOversized is the model-spec begin-batch injects into Master's
	// pane for a batch flagged oversized: true; forks always inherit the
	// session's current model, so this is what an oversized batch's fork
	// actually runs at.
	MasterOversized string `yaml:"master_oversized"`
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
	// BatchContextCapTokens is validation check 5's context-estimate cap:
	// a batch whose estimated context exceeds this without oversized:
	// true fails validation.
	BatchContextCapTokens int `yaml:"batch_context_cap_tokens"`
	// BatchCardCap is validation check 5's card-count cap per batch.
	BatchCardCap int `yaml:"batch_card_cap"`
}

// LoadConfig loads and unmarshals configuration for the webster module.
//
// Calls configengine.Load with webster's ConfigTemplate() to strictly
// validate the config file against the template, resolve environment
// variables, and return resolved bytes. Unmarshals the resolved bytes into
// a Config struct. The module name is threaded through by the caller
// (never hardcoded to "webster" here), mirroring builderengine.LoadConfig.
//
// After unmarshal, each of the three role strings is checked against
// modelspec.Parse for grammar only (registry resolution is a separate
// pre-flight — see ResolveRoles); a grammar error is wrapped naming the
// offending config key, so a hand-edited webster.yaml with a malformed spec
// fails here rather than at spawn time.
//
// If <baseDir>/_lyx/ does not exist, returns an error containing
// "not initialized here; run \"lyx init\"".
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		// Wrap the generic "not initialized" error with the webster-specific
		// hint, matching builderengine/perchengine/muxengine/shuttleengine's
		// shape so every module surfaces the same recovery instruction.
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx init\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal webster config: %w", err)
	}

	// Validate every role's model-spec grammar now, naming the offending
	// key, so a malformed spec is caught at load time rather than silently
	// carried into ResolveRoles or a spawn site.
	roles := []struct {
		key   string
		value string
	}{
		{"master", cfg.Master},
		{"master_oversized", cfg.MasterOversized},
		{"recovery", cfg.Recovery},
	}
	for _, role := range roles {
		if _, err := modelspec.Parse(role.value); err != nil {
			return Config{}, fmt.Errorf("webster config key %q: %w", role.key, err)
		}
	}

	return cfg, nil
}

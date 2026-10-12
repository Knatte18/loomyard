// config.go — configuration for the landing module.
//
// Defines the Config type mirroring landing.yaml's keys and LoadConfig, which uses
// internal/configengine.Load with ConfigTemplate() to strictly validate and resolve landing's
// config file -- the strict entry point, where an absent config file is an error. Strict is
// required by the membership rule rather than chosen: neither producer in this package has a
// standalone entry point, both are reachable only inside a hub, and there an absent config means
// the hub is broken. It then validates the conflict key's model-spec grammar at load time via
// modelspec.Parse, so a typo fails loudly at load rather than hours into a run when the conflict
// session first spawns.

package landingshed

import (
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved landing.yaml configuration.
type Config struct {
	// RequirePRToBase is the list of base branch names Publish requires a pull request against.
	RequirePRToBase []string `yaml:"require_pr_to_base"`
	// Squash selects whether Finalize's parent-side merge squashes.
	Squash bool `yaml:"squash"`
	// Conflict is the model-spec string selecting the model both producers' conflict-resolution
	// session runs under.
	Conflict string `yaml:"conflict"`
	// ConflictTimeoutMin is the conflict-resolution session's wall-clock budget in minutes.
	ConflictTimeoutMin int `yaml:"conflict_timeout_min"`
	// Describe is the model-spec string selecting the model the Describe row's session runs under.
	Describe string `yaml:"describe"`
	// DescribeTimeoutMin is the Describe session's wall-clock budget in minutes.
	DescribeTimeoutMin int `yaml:"describe_timeout_min"`
	// CoAuthoredBy is the value of the landing commit's single Co-Authored-By trailer.
	CoAuthoredBy string `yaml:"co_authored_by"`
	// PublishVerify is the shell command Publish runs in the task worktree after the plan's verify passes.
	// Empty runs nothing; Finalize never runs it.
	PublishVerify string `yaml:"publish_verify"`
	// Deploy is the shell command batten's Land row runs in the prime after a landing into the prime's branch.
	// Empty runs nothing.
	Deploy string `yaml:"deploy"`
	// ParentLockIdleReleaseMin is the minutes a waiting run, or an undecided PR-Gate hand-off, keeps its parent lock before the lock is released.
	// A value below 1 reads as the default.
	ParentLockIdleReleaseMin int `yaml:"parent_lock_idle_release_min"`
}

// defaultIdleReleaseMin is the idle bound in minutes that a missing or non-positive parent_lock_idle_release_min reads as.
const defaultIdleReleaseMin = 10

// IdleRelease returns the parent lock's idle bound.
// A parent_lock_idle_release_min below 1, an absent key included, returns the default and logs one Warn naming the key.
func (c Config) IdleRelease() time.Duration {
	if c.ParentLockIdleReleaseMin < 1 {
		logger.Warn("landingshed: parent_lock_idle_release_min is below 1 or absent, using the default", "key", "parent_lock_idle_release_min", "value", c.ParentLockIdleReleaseMin, "defaultMin", defaultIdleReleaseMin)
		return defaultIdleReleaseMin * time.Minute
	}
	return time.Duration(c.ParentLockIdleReleaseMin) * time.Minute
}

// LoadConfig loads and unmarshals configuration for the landing module.
// It validates the conflict key's model-spec grammar at load time.
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx fabric reconcile\"; a session lyx refuses the verb from reports status: FAILED and the orch runs it")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal landing config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Conflict); err != nil {
		return Config{}, fmt.Errorf("landing config key %q: %w", "conflict", err)
	}

	if _, err := modelspec.Parse(cfg.Describe); err != nil {
		return Config{}, fmt.Errorf("landing config key %q: %w", "describe", err)
	}
	if strings.TrimSpace(cfg.CoAuthoredBy) == "" {
		return Config{}, fmt.Errorf("landing config key %q: must not be empty", "co_authored_by")
	}

	return cfg, nil
}

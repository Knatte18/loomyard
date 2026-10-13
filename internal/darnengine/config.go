// config.go — configuration for the darn recipe.
//
// It defines the Config type mirroring darn.yaml's keys and the ConfigTemplate accessor.
// LoadConfig loads the file strictly.
// VerifyCommand reads the one verify command the darn gate, Publish and Finalize all run.
// The module is hub-wide, so every path here is told by the caller.

package darnengine

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for darn's config module.
func ConfigTemplate() string {
	return configTemplate
}

// configModule is the config module name darn.yaml is loaded under.
const configModule = "darn"

// verifyWayForward is the way forward a missing or empty verify command names.
const verifyWayForward = "run \"lyx config darn --set verify=<command>\" from the prime, then \"lyx loom resume\""

// hardCapSetCommand and softCapSetCommand are the commands the hard-cap refusals name; darn's budget keys are barred from the local layer, so the hub file is the only route.
const (
	hardCapSetCommand = "run \"lyx config darn --set review_hard_max_bounces=<n>\" from the prime"
	softCapSetCommand = "run \"lyx config darn --set review_max_bounces=<n>\" from the prime"
)

// Config represents the resolved darn.yaml configuration.
type Config struct {
	// Writer is the model-spec string selecting the model the darn writer's session runs under.
	Writer string `yaml:"writer"`
	// WriterTimeoutMin is the writer session's wall-clock budget in minutes.
	WriterTimeoutMin int `yaml:"writer_timeout_min"`
	// Verify is the shell command the darn verify gate, Publish and Finalize run in the task worktree.
	// It runs any shell command in the task worktree, which is why the module is hub-wide.
	Verify string `yaml:"verify"`
	// VerifyAttempts is how many times a verify gate, the writer's or a Darn-Review fix round's, re-prompts its session before it halts the run.
	VerifyAttempts int `yaml:"verify_attempts"`
	// ReviewMaxBounces is the bounce budget of the Darn-Review perch that reviews the writer's diff before Publish; 0 runs neither the review nor Describe.
	ReviewMaxBounces int `yaml:"review_max_bounces"`
	// ReviewHardMaxBounces is the round number at or past which the Darn-Review perch never continues a round.
	ReviewHardMaxBounces int `yaml:"review_hard_max_bounces"`
}

// ConfigSpec returns darn's layered-load spec.
// verify runs a shell command, review_max_bounces at 0 skips Darn-Review and Describe, and a raised review_hard_max_bounces lets the review run more rounds, so none of the three may be set in the local layer.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     configModule,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			"writer":                  configengine.CategoryNone,
			"writer_timeout_min":      configengine.CategoryNone,
			"verify":                  configengine.CategoryExecutable,
			"verify_attempts":         configengine.CategoryNone,
			"review_max_bounces":      configengine.CategoryGuard,
			"review_hard_max_bounces": configengine.CategoryGuard,
		},
	}
}

// LoadConfig loads and unmarshals the darn module's configuration strictly through the hub-wide layer stack under dirs.
// It validates the writer key's model-spec grammar at load time and leaves verify, verify_attempts and review_max_bounces to their readers.
// review_hard_max_bounces, default 4, is refused below 1, and below review_max_bounces while that is above 0; each refusal names the command that fixes it.
func LoadConfig(dirs configengine.Dirs, module string) (Config, error) {
	spec := ConfigSpec()
	spec.Name = module
	resolved, err := configengine.Load(dirs, spec)
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx fabric reconcile\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal darn config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Writer); err != nil {
		return Config{}, fmt.Errorf("darn config key %q: %w", "writer", err)
	}

	if cfg.ReviewHardMaxBounces < 1 {
		return Config{}, fmt.Errorf("darn config key %q: must be at least 1, got %d; way forward: %s, then \"lyx loom resume\"", "review_hard_max_bounces", cfg.ReviewHardMaxBounces, hardCapSetCommand)
	}

	// A review_max_bounces of 0 skips the review, so it carries no soft cap for the hard cap to sit under.
	if cfg.ReviewMaxBounces > 0 && cfg.ReviewHardMaxBounces < cfg.ReviewMaxBounces {
		return Config{}, fmt.Errorf("darn config key %q: hard cap %d is below %q %d; way forward: raise the hard cap: %s, or lower the soft cap: %s, then \"lyx loom resume\"", "review_hard_max_bounces", cfg.ReviewHardMaxBounces, "review_max_bounces", cfg.ReviewMaxBounces, hardCapSetCommand, softCapSetCommand)
	}

	return cfg, nil
}

// VerifyCommand returns darn.yaml's verify command, loaded through the layer stack under dirs at each call.
// A missing file and an empty value are errors naming the way forward, never "", so the darn gate, Publish and Finalize stop at call time instead of skipping their verify.
func VerifyCommand(dirs configengine.Dirs) (string, error) {
	cfg, err := LoadConfig(dirs, configModule)
	if err != nil {
		return "", fmt.Errorf("darn verify command: %w; %s", err, verifyWayForward)
	}
	if strings.TrimSpace(cfg.Verify) == "" {
		return "", fmt.Errorf("darn config key %q is empty; %s", "verify", verifyWayForward)
	}
	return cfg.Verify, nil
}

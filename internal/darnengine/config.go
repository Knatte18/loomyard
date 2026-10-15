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
	"github.com/Knatte18/loomyard/internal/reviewbudget"
	"github.com/Knatte18/loomyard/internal/sheddriver"
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
	// It is settable per run through the task's run_settings.
	ReviewMaxBounces int `yaml:"review_max_bounces"`
	// ReviewHardMaxBounces is the round number at or past which the Darn-Review perch never continues a round; it is at least 1 and not below ReviewMaxBounces, and settable per run.
	ReviewHardMaxBounces int `yaml:"review_hard_max_bounces"`
	// DriverKind, Tackler, TacklerTimeoutMin and TacklerMaxSpawns override shed.yaml's keys of the same names for a darn run;
	// an empty string, a zero and a null inherit shed.yaml, and a set TacklerMaxSpawns of 0 turns the tackler off.
	DriverKind        string `yaml:"driver_kind"`
	Tackler           string `yaml:"tackler"`
	TacklerTimeoutMin int    `yaml:"tackler_timeout_min"`
	TacklerMaxSpawns  *int   `yaml:"tackler_max_spawns"`
}

// ShedOverride returns the driver and tackler keys of darn.yaml as the override of shed.yaml's.
func (c Config) ShedOverride() sheddriver.Override {
	return sheddriver.Override{
		DriverKind:        c.DriverKind,
		Tackler:           c.Tackler,
		TacklerTimeoutMin: c.TacklerTimeoutMin,
		TacklerMaxSpawns:  c.TacklerMaxSpawns,
	}
}

// ConfigSpec returns darn's layered-load spec.
// verify runs a shell command, so it may not be set in the local layer.
// The two budget keys come from reviewbudget and are ordinary keys, so a task's local layer, and its run_settings, set them.
func ConfigSpec() configengine.Spec {
	categories := map[string]configengine.Category{
		"writer":             configengine.CategoryNone,
		"writer_timeout_min": configengine.CategoryNone,
		"verify":             configengine.CategoryExecutable,
		"verify_attempts":    configengine.CategoryNone,

		"driver_kind":         configengine.CategoryNone,
		"tackler":             configengine.CategoryNone,
		"tackler_timeout_min": configengine.CategoryNone,
		"tackler_max_spawns":  configengine.CategoryNone,
	}
	for key, category := range reviewbudget.Categories(nil) {
		categories[key] = category
	}
	return configengine.Spec{
		Name:       configModule,
		Template:   []byte(ConfigTemplate()),
		HubWide:    true,
		Categories: categories,
	}
}

// ValidateLocalValues checks the review budget keys and the shed override keys a local-layer document sets against their bounds, and leaves every other key to LoadConfig.
// A refusal is marked configengine.ErrInvalid and names the key.
func ValidateLocalValues(document []byte) error {
	if err := sheddriver.CheckLocalOverride(configModule, document); err != nil {
		return err
	}
	var set map[string]any
	if err := yaml.Unmarshal(document, &set); err != nil {
		return configengine.MarkInvalid(fmt.Errorf("unmarshal darn local values: %w", err))
	}
	return reviewbudget.CheckLocal(set, nil)
}

// LoadConfig loads and unmarshals the darn module's configuration strictly through the hub-wide layer stack under dirs.
// It validates the writer key's model-spec grammar at load time and leaves verify, verify_attempts and review_max_bounces to their readers.
// review_max_bounces is refused below 0, and review_hard_max_bounces, default 4, below 1 and below review_max_bounces;
// each refusal names the command that fixes it for the layer the value comes from.
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

	if err := sheddriver.CheckOverride(configModule, cfg.ShedOverride()); err != nil {
		return Config{}, err
	}

	if err := reviewbudget.CheckResolved(dirs, spec, reviewbudget.Values{RunSoft: cfg.ReviewMaxBounces, RunHard: cfg.ReviewHardMaxBounces}); err != nil {
		return Config{}, err
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

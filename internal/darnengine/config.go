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

// Config represents the resolved darn.yaml configuration.
type Config struct {
	// Writer is the model-spec string selecting the model the darn writer's session runs under.
	Writer string `yaml:"writer"`
	// WriterTimeoutMin is the writer session's wall-clock budget in minutes.
	WriterTimeoutMin int `yaml:"writer_timeout_min"`
	// Verify is the shell command the darn verify gate, Publish and Finalize run in the task worktree.
	// It runs any shell command in the task worktree, which is why the module is hub-wide.
	Verify string `yaml:"verify"`
	// VerifyAttempts is how many times the verify gate re-prompts the writer's session before it halts the run.
	VerifyAttempts int `yaml:"verify_attempts"`
}

// LoadConfig loads and unmarshals the darn module's configuration from baseDir.
// It is strict: an absent file is an error.
// It validates the writer key's model-spec grammar at load time and leaves verify and verify_attempts to their readers.
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
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

	return cfg, nil
}

// VerifyCommand returns darn.yaml's verify command, loaded from baseDir at each call.
// A missing file and an empty value are errors naming the way forward, never "", so the darn gate, Publish and Finalize stop at call time instead of skipping their verify.
func VerifyCommand(baseDir string) (string, error) {
	cfg, err := LoadConfig(baseDir, configModule)
	if err != nil {
		return "", fmt.Errorf("darn verify command: %w; %s", err, verifyWayForward)
	}
	if strings.TrimSpace(cfg.Verify) == "" {
		return "", fmt.Errorf("darn config key %q is empty; %s", "verify", verifyWayForward)
	}
	return cfg.Verify, nil
}

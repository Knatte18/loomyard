// config.go — configuration for the drift module.
//
// It defines the Config and CommandStep types mirroring drift.yaml's keys, the ConfigTemplate and ConfigSpec accessors, and LoadConfig, which loads the file strictly.
// The module is hub-wide, so every path here is told by the caller.

package driftengine

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

// ConfigTemplate returns the default YAML template for the drift config module.
func ConfigTemplate() string {
	return configTemplate
}

// configModule is the config module name drift.yaml is loaded under.
const configModule = "drift"

// baselineDirPlaceholder is the placeholder a declared command holds for the directory a capture writes to and a compare reads from.
const baselineDirPlaceholder = "{baseline_dir}"

// Step names of the two command steps.
const (
	stepCapture = "capture"
	stepCompare = "compare"
)

// CommandStep is one declared drift command: the shell command, its run directory and the environment it runs with.
// The zero CommandStep is an undeclared step.
type CommandStep struct {
	// Command is the shell command the step runs, holding the {baseline_dir} placeholder.
	Command string `yaml:"command"`
	// Dir is the run directory, absolute or relative to the worktree root.
	Dir string `yaml:"dir"`
	// Env is the environment the command runs with, over the process environment.
	Env map[string]string `yaml:"env"`
	// TimeoutMin is the minutes the command may run.
	TimeoutMin int `yaml:"timeout_min"`
}

// Config represents the resolved drift.yaml configuration.
type Config struct {
	// Enabled switches the Drift rows on through their entry gate.
	Enabled bool
	// Capture is the declared capture step, or the zero CommandStep.
	Capture CommandStep
	// Compare is the declared compare step, or the zero CommandStep.
	Compare CommandStep
	// BaselineModel is the model-spec of the Drift-Baseline agent.
	BaselineModel string
	// VerifyModel is the model-spec of the Drift-Verify agent.
	VerifyModel string
	// VerifyMaxRuns is how many compare or side-capture runs one Drift-Verify agent session may take.
	VerifyMaxRuns int
}

// configFile is drift.yaml as it is spelled, before defaults and validation.
type configFile struct {
	Enabled       bool     `yaml:"enabled"`
	Capture       stepFile `yaml:"capture"`
	Compare       stepFile `yaml:"compare"`
	BaselineModel string   `yaml:"baseline_model"`
	VerifyModel   string   `yaml:"verify_model"`
	VerifyMaxRuns int      `yaml:"verify_max_runs"`
}

// stepFile is a command step as drift.yaml spells it; TimeoutMin is a pointer so a missing key reads apart from a zero.
type stepFile struct {
	Command    string            `yaml:"command"`
	Dir        string            `yaml:"dir"`
	Env        map[string]string `yaml:"env"`
	TimeoutMin *int              `yaml:"timeout_min"`
}

// ConfigOpenMaps returns the drift.yaml keys whose value is the operator's own mapping, which configengine carries whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{stepCapture, stepCompare}
}

// ConfigSpec returns drift's layered-load spec.
// capture and compare run commands, so they are executable keys and closed to the local layer.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     configModule,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		OpenMaps: ConfigOpenMaps(),
		Categories: map[string]configengine.Category{
			"enabled":         configengine.CategoryNone,
			stepCapture:       configengine.CategoryExecutable,
			stepCompare:       configengine.CategoryExecutable,
			"baseline_model":  configengine.CategoryNone,
			"verify_model":    configengine.CategoryNone,
			"verify_max_runs": configengine.CategoryNone,
		},
	}
}

// LoadConfig loads and validates the drift module's configuration strictly through the hub-wide layer stack under dirs.
// A declared step needs a command holding {baseline_dir} and a timeout_min of at least 1, and its dir defaults to ".";
// an all-empty step is undeclared and loads as the zero CommandStep.
// Every refusal names the key and lym config drift as the way forward.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.Load(dirs, ConfigSpec())
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lym fabric reconcile\"")
		}
		return Config{}, err
	}

	var file configFile
	if err := yaml.Unmarshal(resolved, &file); err != nil {
		return Config{}, fmt.Errorf("unmarshal drift config: %w", err)
	}

	capture, err := decodeStep(stepCapture, file.Capture)
	if err != nil {
		return Config{}, err
	}
	compare, err := decodeStep(stepCompare, file.Compare)
	if err != nil {
		return Config{}, err
	}
	if file.VerifyMaxRuns < 1 {
		return Config{}, refuseKey("verify_max_runs", fmt.Sprintf("%d; want at least 1", file.VerifyMaxRuns))
	}
	for key, spec := range map[string]string{"baseline_model": file.BaselineModel, "verify_model": file.VerifyModel} {
		if _, err := modelspec.Parse(spec); err != nil {
			return Config{}, refuseKey(key, err.Error())
		}
	}
	return Config{
		Enabled:       file.Enabled,
		Capture:       capture,
		Compare:       compare,
		BaselineModel: file.BaselineModel,
		VerifyModel:   file.VerifyModel,
		VerifyMaxRuns: file.VerifyMaxRuns,
	}, nil
}

// refuseKey builds the refusal of a drift config key, naming the key and its way forward.
func refuseKey(key, want string) error {
	return fmt.Errorf("drift config key %q: %s; way forward: fix it with lym config drift", key, want)
}

// decodeStep validates one declared command step and fills its defaults; a step with no key set is undeclared and decodes to the zero CommandStep.
func decodeStep(name string, file stepFile) (CommandStep, error) {
	if file.Command == "" && file.Dir == "" && len(file.Env) == 0 && file.TimeoutMin == nil {
		return CommandStep{}, nil
	}
	if strings.TrimSpace(file.Command) == "" {
		return CommandStep{}, refuseKey(name+".command", "missing; want the shell command the step runs")
	}
	if !strings.Contains(file.Command, baselineDirPlaceholder) {
		return CommandStep{}, refuseKey(name+".command", "holds no "+baselineDirPlaceholder+"; want the placeholder where the command reads or writes the baseline")
	}
	if file.TimeoutMin == nil {
		return CommandStep{}, refuseKey(name+".timeout_min", "missing; want at least 1")
	}
	if *file.TimeoutMin < 1 {
		return CommandStep{}, refuseKey(name+".timeout_min", fmt.Sprintf("%d; want at least 1", *file.TimeoutMin))
	}
	step := CommandStep{Command: file.Command, Dir: file.Dir, Env: file.Env, TimeoutMin: *file.TimeoutMin}
	if step.Dir == "" {
		step.Dir = "."
	}
	return step, nil
}

// Step returns the declared capture or compare step.
// An undeclared step is refused with the way to declare it, so no row or verb runs an empty command.
func (c Config) Step(name string) (CommandStep, error) {
	var step CommandStep
	switch name {
	case stepCapture:
		step = c.Capture
	case stepCompare:
		step = c.Compare
	default:
		return CommandStep{}, fmt.Errorf("drift: %q is not a command step; want %s or %s", name, stepCapture, stepCompare)
	}
	if step.Command == "" {
		return CommandStep{}, fmt.Errorf("drift: %s is not declared; way forward: declare it with lym config drift --set '%s={command: ..., timeout_min: N}'", name, name)
	}
	return step, nil
}

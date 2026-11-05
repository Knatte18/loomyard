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
	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/yamlengine"
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

// The values of the `diff` key.
const (
	// DiffLines compares the two capture directories line by line in Go, against the expectation the expect agent writes.
	DiffLines = "lines"
	// DiffCommand runs the declared compare command and judges its result.
	DiffCommand = "command"
)

// hubOnlyWayForward is the way forward a task's run settings are refused with for a key only the hub's drift.yaml sets.
const hubOnlyWayForward = "set it in the hub's drift.yaml with lym config drift --set <key>=<value> from the prime"

// CommandStep is one declared drift command: the shell command, its run directory and the environment it runs with.
// The zero CommandStep is an undeclared step.
type CommandStep struct {
	// Command is the shell command the step runs, holding the {baseline_dir} placeholder.
	Command string `yaml:"command"`
	// Dir is the run directory, absolute or relative to the worktree root; empty runs the step in the worktree's anchored directory.
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
	// Diff is how a run compares a capture with the baseline: DiffLines or DiffCommand.
	Diff string
	// KeyPatterns are the regular expressions, tried in order, that key a captured line; each holds the named groups `key` and `value`.
	KeyPatterns []string
	// ExpectModel is the model-spec of the expect agent.
	ExpectModel string
	// JudgeModel is the model-spec of the judge.
	JudgeModel string
	// JudgeInteractive makes the judge a session with the operator in its pane, who decides each ruling.
	JudgeInteractive bool
	// Scope is the task's regression scope, ScopeOff or ScopeFull, which Drift-Baseline records with no agent; empty records full.
	// A stored partial reads as ScopeFull.
	Scope string
	// ScopeWasPartial is true when the stored scope was partial, which Scope reads as full.
	ScopeWasPartial bool
}

// configFile is drift.yaml as it is spelled, before defaults and validation.
type configFile struct {
	Enabled          bool     `yaml:"enabled"`
	Capture          stepFile `yaml:"capture"`
	Compare          stepFile `yaml:"compare"`
	Diff             string   `yaml:"diff"`
	KeyPatterns      []string `yaml:"key_patterns"`
	ExpectModel      string   `yaml:"expect_model"`
	JudgeModel       string   `yaml:"judge_model"`
	JudgeInteractive bool     `yaml:"judge_interactive"`
	Scope            string   `yaml:"scope"`
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
// diff and key_patterns decide what counts as drift, so they are guard keys, and a task's run settings are refused them with the hub's drift.yaml as the way forward.
// Both steps carry the shape their decode gives them, so `--set` writes a step whole, such as `capture={command: ..., timeout_min: 30}`, or reaches one key inside it, such as `capture.env.NAME`, and refuses a key a step lacks.
// skip and verify_max_runs are retired keys: a task's run settings that still carry one seed, the load drops it, and a reconcile prunes it from a hub base.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     configModule,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		OpenMaps: ConfigOpenMaps(),
		OpenMapShapes: map[string]*yamlengine.Shape{
			stepCapture: yamlengine.ShapeOf(stepFile{}),
			stepCompare: yamlengine.ShapeOf(stepFile{}),
		},
		Categories: map[string]configengine.Category{
			"enabled":           configengine.CategoryNone,
			stepCapture:         configengine.CategoryExecutable,
			stepCompare:         configengine.CategoryExecutable,
			"diff":              configengine.CategoryGuard,
			"key_patterns":      configengine.CategoryGuard,
			"expect_model":      configengine.CategoryNone,
			"judge_model":       configengine.CategoryNone,
			"judge_interactive": configengine.CategoryNone,
			"scope":             configengine.CategoryNone,
		},
		RetiredKeys: map[string]configengine.Category{
			"verify_max_runs": configengine.CategoryNone,
			"skip":            configengine.CategoryNone,
		},
		HubOnlyWayForward: map[string]string{
			"diff":         hubOnlyWayForward,
			"key_patterns": hubOnlyWayForward,
		},
	}
}

// LoadConfig loads and validates the drift module's configuration strictly through the hub-wide layer stack under dirs.
// A declared step needs a command holding {baseline_dir} and a timeout_min of at least 1;
// an all-empty step is undeclared and loads as the zero CommandStep.
// A scope must be empty, off, partial or full; a stored partial reads as full and sets ScopeWasPartial.
// A diff must be lines or command, every key_patterns entry must compile with the named groups key and value, and diff command needs a declared compare.
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
	if file.Diff != DiffLines && file.Diff != DiffCommand {
		return Config{}, refuseKey("diff", fmt.Sprintf("%q; want %s or %s", file.Diff, DiffLines, DiffCommand))
	}
	if _, err := CompileKeyPatterns(file.KeyPatterns); err != nil {
		return Config{}, refuseKey("key_patterns", err.Error())
	}
	if file.Diff == DiffCommand && compare.Command == "" {
		return Config{}, refuseKey(stepCompare, "not declared under diff: command; want the command that compares a capture with the baseline")
	}
	for key, spec := range map[string]string{"expect_model": file.ExpectModel, "judge_model": file.JudgeModel} {
		if _, err := modelspec.Parse(spec); err != nil {
			return Config{}, refuseKey(key, err.Error())
		}
	}
	if err := checkScope(file.Scope); err != nil {
		return Config{}, err
	}
	scope := file.Scope
	if scope == ScopePartial {
		scope = ScopeFull
	}
	return Config{
		Scope:            scope,
		ScopeWasPartial:  file.Scope == ScopePartial,
		Enabled:          file.Enabled,
		Capture:          capture,
		Compare:          compare,
		Diff:             file.Diff,
		KeyPatterns:      file.KeyPatterns,
		ExpectModel:      file.ExpectModel,
		JudgeModel:       file.JudgeModel,
		JudgeInteractive: file.JudgeInteractive,
	}, nil
}

// refuseKey builds the refusal of a drift config key, naming the key and its way forward.
func refuseKey(key, want string) error {
	return fmt.Errorf("drift config key %q: %s; way forward: fix it with lym config drift", key, want)
}

// checkScope refuses a scope other than empty, off, partial or full.
// A stored partial passes, since LoadConfig reads it as full.
// The refusal names the key and its ways forward, the task's run_settings among them, since a task sets its scope there.
func checkScope(scope string) error {
	switch scope {
	case "", ScopeOff, ScopePartial, ScopeFull:
		return nil
	}
	return fmt.Errorf("drift config key \"scope\": %q; want %s, %s, or empty for %s; way forward: fix it in the board entry's run_settings, or with lym config drift", scope, ScopeOff, ScopeFull, ScopeFull)
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
	return CommandStep{Command: file.Command, Dir: file.Dir, Env: file.Env, TimeoutMin: *file.TimeoutMin}, nil
}

// CheckDirs loads the drift config under dirs and refuses a declared step whose dir is no existing directory in the worktree at worktreeRoot, whose anchored directory is anchorDir.
// A hub-wide drift config write runs it against the prime, so a dir that would fail every capture or compare is refused when it is written.
func CheckDirs(dirs configengine.Dirs, worktreeRoot, anchorDir string) error {
	cfg, err := LoadConfig(dirs)
	if err != nil {
		return err
	}
	steps := []struct {
		name string
		step CommandStep
	}{{stepCapture, cfg.Capture}, {stepCompare, cfg.Compare}}
	for _, declared := range steps {
		if declared.step.Command == "" {
			continue
		}
		if _, err := gateslot.StatCommandDir(worktreeRoot, anchorDir, declared.step.Dir); err != nil {
			return refuseKey(declared.name+".dir", err.Error())
		}
	}
	return nil
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

// StepWithHubEnv returns the declared step as Step does, with the env config module's variables loaded under dirs laid beneath the step's own env,
// so the command runs with both and the step's value wins a name both hold.
// A refused step and an env config that fails to load are errors.
func (c Config) StepWithHubEnv(name string, dirs configengine.Dirs) (CommandStep, error) {
	step, err := c.Step(name)
	if err != nil {
		return CommandStep{}, err
	}
	hubEnv, err := envconfig.LoadConfig(dirs)
	if err != nil {
		return CommandStep{}, err
	}
	step.Env = hubEnv.Beneath(step.Env)
	return step, nil
}

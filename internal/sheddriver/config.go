// config.go declares the shed config module, shed.yaml: the driver and tackler defaults every shed-driven recipe inherits, the override a module's own keys set over them, and the pure resolvers.

package sheddriver

import (
	_ "embed"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns shed.yaml's default YAML template.
func ConfigTemplate() string {
	return configTemplate
}

// configModule is the config module name shed.yaml is loaded under.
const configModule = "shed"

// tacklerDefaultModelSpec is the model-spec an empty tackler key takes.
const tacklerDefaultModelSpec = "haiku"

// tacklerDefaultTimeout is the timeout a zero tackler_timeout_min takes.
const tacklerDefaultTimeout = 10 * time.Minute

// Config is the resolved shed.yaml: the defaults a module's Override sits over.
type Config struct {
	// DriverKind is the driver a run is seeded with when nothing typed one: llm or go.
	DriverKind string `yaml:"driver_kind"`
	// Tackler is the model-spec of the tackler role; empty takes haiku.
	Tackler string `yaml:"tackler"`
	// TacklerTimeoutMin is how many minutes one tackler spawn may run; 0 takes 10.
	TacklerTimeoutMin int `yaml:"tackler_timeout_min"`
	// TacklerMaxSpawns is how many tackler spawns a run may use; 0 turns the tackler off.
	TacklerMaxSpawns int `yaml:"tackler_max_spawns"`
}

// Override is a module's override of the four shed.yaml keys.
// An empty DriverKind or Tackler, a zero TacklerTimeoutMin and a nil TacklerMaxSpawns inherit shed.yaml;
// a set TacklerMaxSpawns, 0 included, wins, and 0 turns the tackler off.
type Override struct {
	DriverKind        string
	Tackler           string
	TacklerTimeoutMin int
	TacklerMaxSpawns  *int
}

// ConfigSpec returns shed's layered-load spec: a per-worktree module whose every key is open to the local layer and so to a task's run settings.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     configModule,
		Template: []byte(ConfigTemplate()),
		Categories: map[string]configengine.Category{
			"driver_kind":         configengine.CategoryNone,
			"tackler":             configengine.CategoryNone,
			"tackler_timeout_min": configengine.CategoryNone,
			"tackler_max_spawns":  configengine.CategoryNone,
		},
	}
}

// LoadConfig resolves shed.yaml from the layer stack under dirs.
// An absent file in every layer resolves the template; a layer file that exists but is invalid, or a value CheckOverride refuses, is an error.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal shed config: %w", err))
	}
	override := Override{
		DriverKind:        cfg.DriverKind,
		Tackler:           cfg.Tackler,
		TacklerTimeoutMin: cfg.TacklerTimeoutMin,
		TacklerMaxSpawns:  &cfg.TacklerMaxSpawns,
	}
	if err := CheckOverride(configModule, override); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// ValidateLocalValues checks the keys a local-layer shed document sets, and leaves every other key to LoadConfig.
// A refusal is marked configengine.ErrInvalid and names the key.
func ValidateLocalValues(document []byte) error {
	return CheckLocalOverride(configModule, document)
}

// CheckOverride refuses a driver_kind other than empty, llm or go, a non-empty tackler that is not a model-spec, and a negative tackler_timeout_min or tackler_max_spawns.
// Each refusal names the module, the key, the bound and how to set the key.
func CheckOverride(module string, o Override) error {
	if o.DriverKind != "" {
		if err := shedrun.ValidateDriver(o.DriverKind); err != nil {
			return fmt.Errorf("%s config key %q: %w; set driver_kind to llm or go in %s.yaml or the task's run_settings%s", module, "driver_kind", err, module, inheritClause(module))
		}
	}
	if o.Tackler != "" {
		if _, err := modelspec.Parse(o.Tackler); err != nil {
			return fmt.Errorf("%s config key %q: %w; set tackler to a model-spec in %s.yaml or the task's run_settings%s", module, "tackler", err, module, inheritClause(module))
		}
	}
	if o.TacklerTimeoutMin < 0 {
		return fmt.Errorf("%s config key %q: must not be negative, got %d; set it to 0 for the 10 minute default, or to a positive integer, in %s.yaml or the task's run_settings%s", module, "tackler_timeout_min", o.TacklerTimeoutMin, module, inheritClause(module))
	}
	if o.TacklerMaxSpawns != nil && *o.TacklerMaxSpawns < 0 {
		return fmt.Errorf("%s config key %q: must not be negative, got %d; set it to 0 to turn the tackler off, or to a positive integer, in %s.yaml or the task's run_settings%s", module, "tackler_max_spawns", *o.TacklerMaxSpawns, module, inheritClause(module))
	}
	return nil
}

// inheritClause is the way-forward tail naming how a module's key takes shed.yaml's value; shed.yaml itself has nothing to inherit.
func inheritClause(module string) string {
	if module == configModule {
		return ""
	}
	return ", or leave it empty or null to inherit shed.yaml"
}

// CheckLocalOverride decodes a local-layer document of the named module and applies CheckOverride to the keys it sets.
// A refusal is marked configengine.ErrInvalid.
func CheckLocalOverride(module string, document []byte) error {
	var set struct {
		DriverKind        *string `yaml:"driver_kind"`
		Tackler           *string `yaml:"tackler"`
		TacklerTimeoutMin *int    `yaml:"tackler_timeout_min"`
		TacklerMaxSpawns  *int    `yaml:"tackler_max_spawns"`
	}
	if err := yaml.Unmarshal(document, &set); err != nil {
		return configengine.MarkInvalid(fmt.Errorf("unmarshal %s local values: %w", module, err))
	}
	override := Override{TacklerMaxSpawns: set.TacklerMaxSpawns}
	if set.DriverKind != nil {
		override.DriverKind = *set.DriverKind
	}
	if set.Tackler != nil {
		override.Tackler = *set.Tackler
	}
	if set.TacklerTimeoutMin != nil {
		override.TacklerTimeoutMin = *set.TacklerTimeoutMin
	}
	if err := CheckOverride(module, override); err != nil {
		return configengine.MarkInvalid(err)
	}
	return nil
}

// ResolveDriverKind returns the driver an unseeded run is seeded with when nothing typed one: the override's driver kind, else shed.yaml's, else shedrun.DriverLLM.
func ResolveDriverKind(cfg Config, o Override) string {
	if o.DriverKind != "" {
		return o.DriverKind
	}
	if cfg.DriverKind != "" {
		return cfg.DriverKind
	}
	return shedrun.DriverLLM
}

// ResolveTackler parses and resolves the tackler role's model-spec through reg, and takes each key from the override, else shed.yaml, else its default.
// The model-spec defaults to haiku and the timeout to 10 minutes; the spawn budget comes from a set override, 0 included, else shed.yaml's.
// A spec that does not parse or resolve is an error naming the tackler key.
func ResolveTackler(cfg Config, o Override, reg modelspec.Registry) (TacklerSettings, error) {
	specText := o.Tackler
	if specText == "" {
		specText = cfg.Tackler
	}
	if specText == "" {
		specText = tacklerDefaultModelSpec
	}
	spec, err := modelspec.Parse(specText)
	if err != nil {
		return TacklerSettings{}, fmt.Errorf("sheddriver: ResolveTackler: tackler role model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return TacklerSettings{}, fmt.Errorf("sheddriver: ResolveTackler: tackler role model-spec: %w", err)
	}

	timeout := tacklerDefaultTimeout
	if o.TacklerTimeoutMin > 0 {
		timeout = time.Duration(o.TacklerTimeoutMin) * time.Minute
	} else if cfg.TacklerTimeoutMin > 0 {
		timeout = time.Duration(cfg.TacklerTimeoutMin) * time.Minute
	}
	maxSpawns := cfg.TacklerMaxSpawns
	if o.TacklerMaxSpawns != nil {
		maxSpawns = *o.TacklerMaxSpawns
	}
	return TacklerSettings{
		ModelChoice: shuttleengine.NewModelChoice(resolved),
		Timeout:     timeout,
		MaxSpawns:   maxSpawns,
	}, nil
}

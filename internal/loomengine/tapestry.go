// tapestry.go holds the tapestry config module: the tapestry.yaml template, spec, loader and local-value check.
// tapestry.yaml holds the plan perch's keys, which only the tapestry recipe reads.
// loom.yaml keeps the keys every loom recipe shares.

package loomengine

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reviewbudget"
	"gopkg.in/yaml.v3"
)

//go:embed tapestry.yaml
var tapestryTemplate string

// tapestryModule is the config module whose YAML file holds the plan perch's keys.
const tapestryModule = "tapestry"

// TapestryConfig is the resolved plan-perch configuration from tapestry.yaml.
// It holds the model-specs, timeout, lens fans and review settings.
// The implementer key is read where the recipe builds the Webster row, never here.
type TapestryConfig struct {
	Plan  PlanConfig  `yaml:"plan"`
	Parts PartsConfig `yaml:"parts"`
}

// PlanConfig is the plan area of tapestry.yaml: the Plan-Write agent and the Plan-Review perch.
type PlanConfig struct {
	// Model is the model-spec of the plan agent.
	Model      string `yaml:"model"`
	TimeoutMin int    `yaml:"timeout_min"`

	// Lenses names the lenses.yaml fan the Plan-Write lead forks itself for once its plan is written and self-checked.
	// An empty value turns the forks off.
	Lenses string `yaml:"lenses"`

	// GateFixer is the model-spec of the cold fixer of Plan-Write's plan gate; empty turns the fixer off.
	GateFixer string `yaml:"gate_fixer"`

	// ReviewerModel and FixerModel are the Plan-Review segment's own model lists; an unset list takes loom.yaml's shared one.
	ReviewerModel burlerengine.ModelSpecList `yaml:"reviewer_model"`
	FixerModel    burlerengine.ModelSpecList `yaml:"fixer_model"`

	ReviewerFan lenses.FanList `yaml:"reviewer_fan"`
	FixerFan    lenses.FanList `yaml:"fixer_fan"`

	// AssistReviewerModel and AssistReviewerFan are the assistant's per-round lists; an empty model entry skips it.
	AssistReviewerModel burlerengine.ModelSpecList `yaml:"assist_reviewer_model"`
	AssistReviewerFan   lenses.FanList             `yaml:"assist_reviewer_fan"`

	// Judge and Seed override loom.yaml's shared judge and seed for the plan segment when non-empty.
	Judge string `yaml:"judge"`
	Seed  string `yaml:"seed"`

	// MaxBounces, HardMaxBounces and AutoAccept are the plan perch's bounce budget, hard cap and auto-accept flag.
	// A nil budget takes loom.yaml's review.max_bounces and a hard cap of 0 takes its review.hard_max_bounces.
	MaxBounces     *int `yaml:"max_bounces"`
	HardMaxBounces int  `yaml:"hard_max_bounces"`
	AutoAccept     bool `yaml:"auto_accept"`

	// Split turns the Parts-Write and Parts-Plan-Write rows on and Plan-Write off.
	Split bool `yaml:"split"`
}

// PartsConfig is the parts area of tapestry.yaml: the agents that write a plan in parts.
type PartsConfig struct {
	// Model, Writer and Chair are the model-specs of the Parts-Write agent, of every part writer and of the chair.
	Model  string `yaml:"model"`
	Writer string `yaml:"writer"`
	Chair  string `yaml:"chair"`

	// Depth is the Parts-Write agent's reading depth, light or medium.
	Depth string `yaml:"depth"`

	// Max is the most parts a parts file may hold.
	Max int `yaml:"max"`

	// ChairTimeoutMin is the minutes the chair's run may take; nil takes twice the effective plan timeout.
	ChairTimeoutMin *int `yaml:"chair_timeout_min"`
}

// WithTapestry returns c with its plan and parts areas set from t.
func (c Config) WithTapestry(t TapestryConfig) Config {
	c.Plan = t.Plan
	c.Parts = t.Parts
	return c
}

// PlanTimeout returns the run's effective plan timeout: plan.timeout_min minutes, or runTimeout when it is 0.
// runTimeout is shuttle's run_timeout_min as a duration, which tapestry.yaml's load cannot read.
func (c Config) PlanTimeout(runTimeout time.Duration) time.Duration {
	if c.Plan.TimeoutMin == 0 {
		return runTimeout
	}
	return time.Duration(c.Plan.TimeoutMin) * time.Minute
}

// PartsChairTimeout returns the Parts-Plan-Write chair's timeout.
// It is parts.chair_timeout_min minutes when set, or twice PlanTimeout(runTimeout) when null.
// A set value not above PlanTimeout(runTimeout) is an error naming the key.
// The chair spans every part writer, which is why the set value must sit above the plan timeout.
func (c Config) PartsChairTimeout(runTimeout time.Duration) (time.Duration, error) {
	planTimeout := c.PlanTimeout(runTimeout)
	if c.Parts.ChairTimeoutMin == nil {
		return 2 * planTimeout, nil
	}
	chairTimeout := time.Duration(*c.Parts.ChairTimeoutMin) * time.Minute
	if chairTimeout <= planTimeout {
		return 0, fmt.Errorf("config key %q: %d minutes is not above the effective plan timeout of %s; set it above %s (or %s when that key is 0) or to null for twice it", FullKeyName(partsChairTimeoutMinKey), *c.Parts.ChairTimeoutMin, planTimeout, FullKeyName(planTimeoutMinKey), shuttleRunTimeoutName())
	}
	return chairTimeout, nil
}

// TapestryConfigTemplate returns the default YAML template for the tapestry config module.
func TapestryConfigTemplate() string {
	return tapestryTemplate
}

// TapestryConfigOpenMaps returns the tapestry.yaml paths whose value is a per-round list.
// configengine carries these values whole through reconcile and --set.
func TapestryConfigOpenMaps() []string {
	return []string{"plan.reviewer_model", "plan.fixer_model", "plan.reviewer_fan", "plan.fixer_fan", "plan.assist_reviewer_model", "plan.assist_reviewer_fan"}
}

// TapestryConfigSpec returns tapestry's layered-load spec.
// The module is per-worktree.
// plan.lenses, the two plan fan keys and plan.auto_accept can hollow a review or approval gate.
// They are barred from the local layer; every other key may be overridden there.
// The plan perch's budget keys come from reviewbudget, without the three run-wide keys, which loom.yaml owns.
func TapestryConfigSpec() configengine.Spec {
	none := configengine.CategoryNone
	guard := configengine.CategoryGuard
	spec := configengine.Spec{
		Name:      tapestryModule,
		Template:  []byte(TapestryConfigTemplate()),
		OpenMaps:  TapestryConfigOpenMaps(),
		MovedKeys: tapestryMovedKeys(),
		Categories: map[string]configengine.Category{
			"plan.model":          none,
			"plan.timeout_min":    none,
			"implementer":         none,
			"plan.lenses":         guard,
			"plan.gate_fixer":     none,
			"plan.reviewer_model": none,
			"plan.fixer_model":    none,
			"plan.reviewer_fan":   guard,
			"plan.fixer_fan":      guard,
			"plan.judge":          none,
			"plan.seed":           none,

			"plan.assist_reviewer_model": none,
			"plan.assist_reviewer_fan":   none,
		},
	}
	for _, key := range []string{planSplitKey, partsDepthKey, partsMaxKey, partsModelKey, partsWriterKey, partsChairKey, partsChairTimeoutMinKey} {
		spec.Categories[key] = none
	}
	for key, category := range reviewbudget.Categories([]string{string(PerchPlan)}) {
		switch key {
		case reviewbudget.RunSoftKey, reviewbudget.RunHardKey, reviewbudget.RunAmendKey:
			continue
		}
		spec.Categories[key] = category
	}
	return spec
}

// ValidateTapestryLocalValues checks the plan perch's budget and hard-cap keys a local-layer document sets.
// It goes through reviewbudget, and every other key is left to LoadTapestryConfig.
// A refusal is marked configengine.ErrInvalid and names the key.
func ValidateTapestryLocalValues(document []byte) error {
	var budgets map[string]any
	if err := yaml.Unmarshal(document, &budgets); err != nil {
		return configengine.MarkInvalid(fmt.Errorf("unmarshal tapestry local values: %w", err))
	}
	return reviewbudget.CheckLocal(tapestryModule, budgets, []string{string(PerchPlan)})
}

// LoadTapestryConfig loads the tapestry module's configuration strictly through the layer stack under dirs.
// It validates model-spec grammar at load time and reads lenses.yaml through the same dirs.
// It refuses a negative plan.timeout_min and a plan perch budget or hard cap below its bound.
// It also refuses an unparseable parts model, a parts.depth other than light or medium, and a parts.max below 1.
// It refuses a parts.chair_timeout_min not above a non-zero plan.timeout_min.
// The run-wide review.max_bounces, review.hard_max_bounces and review.amend_max_bounces come from loom.yaml.
// They load through the same dirs, so a plan.max_bounces of null and a plan.hard_max_bounces of 0 resolve against them.
// Each refusal names the module that owns the key it reports.
func LoadTapestryConfig(dirs configengine.Dirs, module string) (TapestryConfig, error) {
	spec := TapestryConfigSpec()
	spec.Name = module
	resolved, err := loadResolved(dirs, spec)
	if err != nil {
		return TapestryConfig{}, err
	}

	var cfg TapestryConfig
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		var shape *burlerengine.ModelSpecShapeError
		if errors.As(err, &shape) {
			return TapestryConfig{}, fmt.Errorf("config key %q: %w", configengine.KeyName(tapestryModule, configengine.KeyAtLine(resolved, shape.Line)), err)
		}
		var fanShape *lenses.FanShapeError
		if errors.As(err, &fanShape) {
			return TapestryConfig{}, fmt.Errorf("config key %q: %w", configengine.KeyName(tapestryModule, configengine.KeyAtLine(resolved, fanShape.Line)), err)
		}
		return TapestryConfig{}, fmt.Errorf("unmarshal tapestry config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Plan.Model); err != nil {
		return TapestryConfig{}, fmt.Errorf("config key %q: %w", FullKeyName(planModelKey), err)
	}

	// An empty plan.gate_fixer turns the fixer off and must load cleanly.
	if cfg.Plan.GateFixer != "" {
		if _, err := modelspec.Parse(cfg.Plan.GateFixer); err != nil {
			return TapestryConfig{}, fmt.Errorf("config key %q: %w; set a model-spec with \"lym config tapestry\", or \"\" to turn the fixer off", FullKeyName(planGateFixerKey), err)
		}
	}

	for _, list := range []struct {
		key   string
		specs burlerengine.ModelSpecList
	}{
		{segmentKey(areaPlan, leafReviewerModel), cfg.Plan.ReviewerModel},
		{segmentKey(areaPlan, leafFixerModel), cfg.Plan.FixerModel},
	} {
		if list.specs.Unset() {
			continue
		}
		if err := burlerengine.ValidateModelSpecList(tapestryModule, list.key, list.specs); err != nil {
			return TapestryConfig{}, err
		}
	}

	if err := burlerengine.ValidateAssistModelList(tapestryModule, segmentKey(areaPlan, leafAssistReviewerModel), cfg.Plan.AssistReviewerModel); err != nil {
		return TapestryConfig{}, err
	}

	if err := lenses.ValidateFanKeys(dirs, tapestryModule, []lenses.FanKey{
		{Key: segmentKey(areaPlan, leafReviewerFan), Fans: cfg.Plan.ReviewerFan},
		{Key: segmentKey(areaPlan, leafFixerFan), Fans: cfg.Plan.FixerFan},
		{Key: segmentKey(areaPlan, leafAssistReviewerFan), Fans: cfg.Plan.AssistReviewerFan},
	}); err != nil {
		return TapestryConfig{}, err
	}

	// The judge and seed overrides are validated only when non-empty:
	// an empty value takes loom.yaml's key at run start and must load cleanly.
	for _, override := range []struct {
		key   string
		value string
	}{
		{segmentKey(areaPlan, leafJudge), cfg.Plan.Judge},
		{segmentKey(areaPlan, leafSeed), cfg.Plan.Seed},
	} {
		if override.value == "" {
			continue
		}
		if _, err := modelspec.Parse(override.value); err != nil {
			return TapestryConfig{}, fmt.Errorf("config key %q: %w", FullKeyName(override.key), err)
		}
	}

	// Zero is accepted: a zero timeout defers to shuttle's run_timeout_min.
	if cfg.Plan.TimeoutMin < 0 {
		return TapestryConfig{}, fmt.Errorf("config key %q: must not be negative, got %d; use 0 to defer to %s", FullKeyName(planTimeoutMinKey), cfg.Plan.TimeoutMin, shuttleRunTimeoutName())
	}

	for _, model := range []struct {
		key   string
		value string
	}{
		{partsModelKey, cfg.Parts.Model},
		{partsWriterKey, cfg.Parts.Writer},
		{partsChairKey, cfg.Parts.Chair},
	} {
		if _, err := modelspec.Parse(model.value); err != nil {
			return TapestryConfig{}, fmt.Errorf("config key %q: %w", FullKeyName(model.key), err)
		}
	}
	if cfg.Parts.Depth != "light" && cfg.Parts.Depth != "medium" {
		return TapestryConfig{}, fmt.Errorf("config key %q: %q is not a reading depth; set it to light or medium", FullKeyName(partsDepthKey), cfg.Parts.Depth)
	}
	if cfg.Parts.Max < 1 {
		return TapestryConfig{}, fmt.Errorf("config key %q: must be at least 1, got %d; set it to 1 or more", FullKeyName(partsMaxKey), cfg.Parts.Max)
	}
	// With plan.timeout_min 0 the effective plan timeout is shuttle's run_timeout_min, which the table builder checks.
	if cfg.Parts.ChairTimeoutMin != nil && cfg.Plan.TimeoutMin > 0 && *cfg.Parts.ChairTimeoutMin <= cfg.Plan.TimeoutMin {
		return TapestryConfig{}, fmt.Errorf("config key %q: %d is not above %s %d; set it above that key or to null for twice it", FullKeyName(partsChairTimeoutMinKey), *cfg.Parts.ChairTimeoutMin, FullKeyName(planTimeoutMinKey), cfg.Plan.TimeoutMin)
	}

	loomSpec := ConfigSpec()
	loomResolved, err := loadResolved(dirs, loomSpec)
	if err != nil {
		return TapestryConfig{}, err
	}
	var runWide struct {
		Review struct {
			MaxBounces      int `yaml:"max_bounces"`
			HardMaxBounces  int `yaml:"hard_max_bounces"`
			AmendMaxBounces int `yaml:"amend_max_bounces"`
		} `yaml:"review"`
	}
	if err := yaml.Unmarshal(loomResolved, &runWide); err != nil {
		return TapestryConfig{}, fmt.Errorf("unmarshal loom config: %w", err)
	}
	plan := string(PerchPlan)
	if err := reviewbudget.CheckResolved(dirs, spec, reviewbudget.Values{
		RunSoft:     runWide.Review.MaxBounces,
		RunHard:     runWide.Review.HardMaxBounces,
		RunAmend:    runWide.Review.AmendMaxBounces,
		Perches:     []string{plan},
		PerchSoft:   map[string]*int{plan: cfg.Plan.MaxBounces},
		PerchHard:   map[string]int{plan: cfg.Plan.HardMaxBounces},
		RunWideSpec: loomSpec,
	}); err != nil {
		return TapestryConfig{}, err
	}

	return cfg, nil
}

// loadResolved loads spec's layer stack strictly under dirs and returns the resolved document.
// A hub whose config directory is absent is refused with the way forward to reconcile it.
func loadResolved(dirs configengine.Dirs, spec configengine.Spec) ([]byte, error) {
	resolved, err := configengine.Load(dirs, spec)
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return nil, fmt.Errorf("not initialized here; run \"lym fabric reconcile\"; a session lym refuses the verb from reports status: FAILED and the orch runs it")
		}
		return nil, err
	}
	return resolved, nil
}

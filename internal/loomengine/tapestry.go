// tapestry.go — the tapestry config module: tapestry.yaml's template, spec, loader and local-value check.
//
// tapestry.yaml holds the plan perch's keys, which only the tapestry recipe reads: the Plan-Write agent and its lens fan, the implementer engine, and the Plan-Review segment's models, fans, judge, seed and budget.
// loom.yaml keeps what every loom recipe shares, and LoadTapestryConfig reads its run-wide review keys to resolve the plan perch's budget.
// Config.WithTapestry copies the loaded values into a Config, for the callers that serve the Plan-Review segment.

package loomengine

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"

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

// TapestryConfig represents the resolved tapestry.yaml configuration: the plan perch's model-specs, timeout, lens fan and review settings.
// The implementer key is read where the recipe builds the Webster row, never here.
type TapestryConfig struct {
	Plan           string `yaml:"plan"`
	PlanTimeoutMin int    `yaml:"plan_timeout_min"`

	// PlanLenses names the lenses.yaml fan whose lenses the Plan-Write lead forks itself for once its plan is written and self-checked; empty turns the forks off.
	// It names the writer's lens fan, not the Plan-Review perch's PlanReviewerFan.
	PlanLenses string `yaml:"plan_lenses"`

	// PlanReviewerModel and PlanFixerModel are the Plan-Review segment's own model lists; an unset list takes loom.yaml's shared one.
	PlanReviewerModel burlerengine.ModelSpecList `yaml:"plan_reviewer_model"`
	PlanFixerModel    burlerengine.ModelSpecList `yaml:"plan_fixer_model"`

	PlanReviewerFan lenses.FanList `yaml:"plan_reviewer_fan"`
	PlanFixerFan    lenses.FanList `yaml:"plan_fixer_fan"`

	// PlanJudge and PlanSeed override loom.yaml's judge and seed for the plan segment when non-empty.
	PlanJudge string `yaml:"plan_judge"`
	PlanSeed  string `yaml:"plan_seed"`

	// PlanMaxBounces, PlanHardMaxBounces and PlanAutoAccept are the plan perch's bounce budget, hard cap and auto-accept flag.
	// A nil budget takes loom.yaml's review_max_bounces and a hard cap of 0 takes its review_hard_max_bounces.
	PlanMaxBounces     *int `yaml:"plan_max_bounces"`
	PlanHardMaxBounces int  `yaml:"plan_hard_max_bounces"`
	PlanAutoAccept     bool `yaml:"plan_auto_accept"`
}

// WithTapestry returns c with its plan fields set from t.
func (c Config) WithTapestry(t TapestryConfig) Config {
	c.Plan = t.Plan
	c.PlanTimeoutMin = t.PlanTimeoutMin
	c.PlanLenses = t.PlanLenses
	c.PlanReviewerModel = t.PlanReviewerModel
	c.PlanFixerModel = t.PlanFixerModel
	c.PlanReviewerFan = t.PlanReviewerFan
	c.PlanFixerFan = t.PlanFixerFan
	c.PlanJudge = t.PlanJudge
	c.PlanSeed = t.PlanSeed
	c.PlanMaxBounces = t.PlanMaxBounces
	c.PlanHardMaxBounces = t.PlanHardMaxBounces
	c.PlanAutoAccept = t.PlanAutoAccept
	return c
}

// TapestryConfigTemplate returns the default YAML template for the tapestry config module.
func TapestryConfigTemplate() string {
	return tapestryTemplate
}

// TapestryConfigOpenMaps returns the tapestry.yaml keys whose value is a per-round list, which configengine carries whole through reconcile and --set.
func TapestryConfigOpenMaps() []string {
	return []string{"plan_reviewer_model", "plan_fixer_model", "plan_reviewer_fan", "plan_fixer_fan"}
}

// TapestryConfigSpec returns tapestry's layered-load spec.
// The module is per-worktree.
// plan_lenses, the two plan fan keys and plan_auto_accept can hollow a review or approval gate and are barred from the local layer; every other key may be overridden there.
// The plan perch's budget keys come from reviewbudget, without the three run-wide keys, which loom.yaml owns.
func TapestryConfigSpec() configengine.Spec {
	none := configengine.CategoryNone
	guard := configengine.CategoryGuard
	spec := configengine.Spec{
		Name:     tapestryModule,
		Template: []byte(TapestryConfigTemplate()),
		OpenMaps: TapestryConfigOpenMaps(),
		Categories: map[string]configengine.Category{
			"plan":                none,
			"plan_timeout_min":    none,
			"implementer":         none,
			"plan_lenses":         guard,
			"plan_reviewer_model": none,
			"plan_fixer_model":    none,
			"plan_reviewer_fan":   guard,
			"plan_fixer_fan":      guard,
			"plan_judge":          none,
			"plan_seed":           none,
		},
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

// ValidateTapestryLocalValues checks the values of the plan perch's budget and hard-cap keys a local-layer document sets, through reviewbudget, and leaves every other key to LoadTapestryConfig.
// A refusal is marked configengine.ErrInvalid and names the key.
func ValidateTapestryLocalValues(document []byte) error {
	var budgets map[string]any
	if err := yaml.Unmarshal(document, &budgets); err != nil {
		return configengine.MarkInvalid(fmt.Errorf("unmarshal tapestry local values: %w", err))
	}
	return reviewbudget.CheckLocal(budgets, []string{string(PerchPlan)})
}

// LoadTapestryConfig loads and unmarshals configuration for the tapestry module strictly through the layer stack under dirs.
// It validates model-spec grammar at load time, reads lenses.yaml through the same dirs, refuses a negative plan_timeout_min, and refuses a plan perch budget or hard cap below its bound.
// The run-wide review_max_bounces, review_hard_max_bounces and review_amend_max_bounces come from loom.yaml, loaded strictly through the same dirs, so a plan_max_bounces of null and a plan_hard_max_bounces of 0 resolve against them.
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
			return TapestryConfig{}, fmt.Errorf("tapestry config key %q: %w", configengine.KeyAtLine(resolved, shape.Line), err)
		}
		var fanShape *lenses.FanShapeError
		if errors.As(err, &fanShape) {
			return TapestryConfig{}, fmt.Errorf("tapestry config key %q: %w", configengine.KeyAtLine(resolved, fanShape.Line), err)
		}
		return TapestryConfig{}, fmt.Errorf("unmarshal tapestry config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Plan); err != nil {
		return TapestryConfig{}, fmt.Errorf("tapestry config key %q: %w", "plan", err)
	}

	for _, list := range []struct {
		key   string
		specs burlerengine.ModelSpecList
	}{
		{"plan_reviewer_model", cfg.PlanReviewerModel},
		{"plan_fixer_model", cfg.PlanFixerModel},
	} {
		if list.specs.Unset() {
			continue
		}
		if err := burlerengine.ValidateModelSpecList(tapestryModule, list.key, list.specs); err != nil {
			return TapestryConfig{}, err
		}
	}

	if err := lenses.ValidateFanKeys(dirs, tapestryModule, []lenses.FanKey{
		{Key: "plan_reviewer_fan", Fans: cfg.PlanReviewerFan},
		{Key: "plan_fixer_fan", Fans: cfg.PlanFixerFan},
	}); err != nil {
		return TapestryConfig{}, err
	}

	// The judge and seed overrides are validated only when non-empty: an empty value takes loom.yaml's key at run start and must load cleanly.
	for _, override := range []struct {
		key   string
		value string
	}{
		{"plan_judge", cfg.PlanJudge},
		{"plan_seed", cfg.PlanSeed},
	} {
		if override.value == "" {
			continue
		}
		if _, err := modelspec.Parse(override.value); err != nil {
			return TapestryConfig{}, fmt.Errorf("tapestry config key %q: %w", override.key, err)
		}
	}

	// Zero is accepted: a zero timeout defers to shuttle's run_timeout_min.
	if cfg.PlanTimeoutMin < 0 {
		return TapestryConfig{}, fmt.Errorf("tapestry config key %q: must not be negative, got %d; use 0 to defer to shuttle's run_timeout_min", "plan_timeout_min", cfg.PlanTimeoutMin)
	}

	loomSpec := ConfigSpec()
	loomResolved, err := loadResolved(dirs, loomSpec)
	if err != nil {
		return TapestryConfig{}, err
	}
	var runWide struct {
		ReviewMaxBounces      int `yaml:"review_max_bounces"`
		ReviewHardMaxBounces  int `yaml:"review_hard_max_bounces"`
		ReviewAmendMaxBounces int `yaml:"review_amend_max_bounces"`
	}
	if err := yaml.Unmarshal(loomResolved, &runWide); err != nil {
		return TapestryConfig{}, fmt.Errorf("unmarshal loom config: %w", err)
	}
	plan := string(PerchPlan)
	if err := reviewbudget.CheckResolved(dirs, spec, reviewbudget.Values{
		RunSoft:     runWide.ReviewMaxBounces,
		RunHard:     runWide.ReviewHardMaxBounces,
		RunAmend:    runWide.ReviewAmendMaxBounces,
		Perches:     []string{plan},
		PerchSoft:   map[string]*int{plan: cfg.PlanMaxBounces},
		PerchHard:   map[string]int{plan: cfg.PlanHardMaxBounces},
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

// config.go defines the hardener.yaml configuration.
// It holds the Config type that mirrors the keys, the spec, the value check and the strict load.

package hardenshed

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reviewbudget"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for the hardener config module.
func ConfigTemplate() string {
	return configTemplate
}

// ConfigModule is the config module name hardener.yaml is loaded under.
const ConfigModule = "hardener"

// Config is the resolved hardener.yaml configuration.
type Config struct {
	// Packages are the worktree-relative directories of the packages the campaign drives.
	Packages []string `yaml:"packages"`
	// LLMDriving says whether the live tests drive a billed LLM session, which turns the live gate's copies off.
	LLMDriving bool `yaml:"llm_driving"`
	// MaxBounces is the soft cap of review rounds; 0 means no review round.
	MaxBounces int `yaml:"max_bounces"`
	// HardMaxBounces is the hard cap of review rounds.
	HardMaxBounces int `yaml:"hard_max_bounces"`
	// Reviewer lists the model-specs of the review rounds, taken in turn.
	Reviewer []string `yaml:"reviewer"`
	// Fixer lists the model-specs of the fix rounds, taken in turn.
	Fixer []string `yaml:"fixer"`
	// Judge is the model-spec of the Bouncer's judge; empty takes loom.yaml's judge.
	Judge string `yaml:"judge"`
	// Seed is the model-spec of the campaign's seed round; empty takes loom.yaml's seed.
	Seed string `yaml:"seed"`
	// ReviewerFan names the reviewer fan the review rounds run; empty runs a solo round.
	ReviewerFan string `yaml:"reviewer_fan"`
	// FixerFan names the fixer fan the fix rounds run; empty runs a solo round.
	FixerFan string `yaml:"fixer_fan"`
	// AutoAccept makes the Bouncer accept a round's findings with no judge, bar a red live gate.
	AutoAccept bool `yaml:"auto_accept"`
	// Concurrency is how many copies of the live gate run side by side when the live tests drive no LLM session.
	Concurrency int `yaml:"concurrency"`
	// LiveAttempts is how many times the live gate runs before a red result ends the fix row.
	LiveAttempts int `yaml:"live_attempts"`
	// VerifyAttempts caps the re-prompts of the fix row's verify gate before the run halts for the parent.
	VerifyAttempts int `yaml:"verify_attempts"`
	// TreeScope maps each worktree-relative tree path, without a trailing slash, to its test target.
	TreeScope map[string]string `yaml:"tree_scope"`
}

// ConfigOpenMaps returns the hardener.yaml keys whose value is the operator's own tree map.
func ConfigOpenMaps() []string {
	return []string{"tree_scope"}
}

// guardKeys are the hardener.yaml keys that switch off or hollow a guard, so only the hub file sets them.
var guardKeys = []string{"auto_accept", "reviewer_fan", "fixer_fan", "tree_scope"}

// ConfigSpec returns hardener's layered-load spec.
// A guard key is closed to the local layer, and a task's run_settings setting one is refused naming the hub file write.
func ConfigSpec() configengine.Spec {
	categories := map[string]configengine.Category{
		"packages":         configengine.CategoryNone,
		"llm_driving":      configengine.CategoryNone,
		"max_bounces":      configengine.CategoryNone,
		"hard_max_bounces": configengine.CategoryNone,
		"reviewer":         configengine.CategoryNone,
		"fixer":            configengine.CategoryNone,
		"judge":            configengine.CategoryNone,
		"seed":             configengine.CategoryNone,
		"concurrency":      configengine.CategoryNone,
		"live_attempts":    configengine.CategoryNone,
		"verify_attempts":  configengine.CategoryNone,
	}
	wayForward := map[string]string{}
	for _, key := range guardKeys {
		categories[key] = configengine.CategoryGuard
		wayForward[key] = fmt.Sprintf(`write it in the hub file with "lym config %s --set %s=<value>" from the prime`, ConfigModule, key)
	}
	return configengine.Spec{
		Name:              ConfigModule,
		Template:          []byte(ConfigTemplate()),
		HubWide:           true,
		OpenMaps:          ConfigOpenMaps(),
		Categories:        categories,
		HubOnlyWayForward: wayForward,
	}
}

// valueDocument holds the keys CheckValues bounds, each optional so a sparse local document is checked on what it sets.
type valueDocument struct {
	MaxBounces     *int     `yaml:"max_bounces"`
	HardMaxBounces *int     `yaml:"hard_max_bounces"`
	Concurrency    *int     `yaml:"concurrency"`
	LiveAttempts   *int     `yaml:"live_attempts"`
	VerifyAttempts *int     `yaml:"verify_attempts"`
	Reviewer       []string `yaml:"reviewer"`
	Fixer          []string `yaml:"fixer"`
	Judge          *string  `yaml:"judge"`
	Seed           *string  `yaml:"seed"`
}

// checkValues returns the first key of document whose value is out of bounds, and the refusal naming it.
// It returns an empty key for a document that does not parse.
func checkValues(document []byte) (string, error) {
	var values valueDocument
	if err := yaml.Unmarshal(document, &values); err != nil {
		return "", fmt.Errorf("unmarshal hardener config: %w", err)
	}
	for _, bound := range []struct {
		key   string
		value *int
		floor int
	}{
		{"concurrency", values.Concurrency, 1},
		{"live_attempts", values.LiveAttempts, 1},
		{"verify_attempts", values.VerifyAttempts, 1},
		{"hard_max_bounces", values.HardMaxBounces, 1},
		{"max_bounces", values.MaxBounces, 0},
	} {
		if bound.value != nil && *bound.value < bound.floor {
			return bound.key, fmt.Errorf("hardener config key %q: %d; want at least %d", bound.key, *bound.value, bound.floor)
		}
	}
	specs := []struct {
		key   string
		specs []string
	}{{"reviewer", values.Reviewer}, {"fixer", values.Fixer}}
	for _, key := range []string{"judge", "seed"} {
		value := values.Judge
		if key == "seed" {
			value = values.Seed
		}
		if value != nil && *value != "" {
			specs = append(specs, struct {
				key   string
				specs []string
			}{key, []string{*value}})
		}
	}
	for _, list := range specs {
		for _, spec := range list.specs {
			if _, err := modelspec.Parse(spec); err != nil {
				return list.key, fmt.Errorf("hardener config key %q: %w", list.key, err)
			}
		}
	}
	return "", nil
}

// CheckValues checks the values of one hardener.yaml document, as a task's run_settings or a local layer holds it.
// It refuses a concurrency, live_attempts, verify_attempts or hard_max_bounces below 1.
// It refuses a negative max_bounces.
// It refuses a reviewer, fixer, judge or seed entry whose model-spec does not parse.
// Each refusal names the key.
// The refusal is marked configengine.ErrInvalid and names the task's run_settings as the place to correct.
func CheckValues(document []byte) error {
	if _, err := checkValues(document); err != nil {
		return configengine.MarkInvalid(fmt.Errorf("%w; way forward: correct the value in the task's run_settings with \"lym board upsert\"", err))
	}
	return nil
}

// LoadConfig loads the module's configuration strictly through the hub-wide layer stack under dirs.
// It runs CheckValues over the resolved document, and keeps tree_scope keyed by trimmed tree path.
// A refusal names the write for the layer the value comes from, then `lym loom resume`.
// A malformed or repeated tree key wraps darnengine.ErrTreeScopeKey.
func LoadConfig(dirs configengine.Dirs, module string) (Config, error) {
	spec := ConfigSpec()
	spec.Name = module
	resolved, err := configengine.Load(dirs, spec)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal hardener config: %w", err)
	}
	if key, err := checkValues(resolved); err != nil {
		route, routeErr := reviewbudget.Route(dirs, spec, key)
		if routeErr != nil {
			return Config{}, fmt.Errorf("%w; the layer holding the value could not be read: %v", err, routeErr)
		}
		return Config{}, fmt.Errorf("%w; way forward: %s, then run \"lym loom resume\"", err, route)
	}
	treeScope, err := darnengine.NormalizeTrees(cfg.TreeScope)
	if err != nil {
		return Config{}, fmt.Errorf("hardener config key %q: %w; way forward: fix the named key in lym config hardener's editor from the prime, then run \"lym loom resume\"", "tree_scope", err)
	}
	cfg.TreeScope = treeScope
	return cfg, nil
}

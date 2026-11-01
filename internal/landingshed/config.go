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
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved landing.yaml configuration.
type Config struct {
	// DeployBranches is the list of base branch names a landing into which batten's Land row deploys.
	// An empty list deploys nothing.
	DeployBranches []string `yaml:"deploy_branches"`
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
	// PRReviewer is the model-spec string selecting the model the PR-Review row's interactive session runs under.
	PRReviewer string `yaml:"pr_reviewer"`
	// Credit selects whether the landing commit and the pull request's body end with the credit block.
	Credit bool `yaml:"credit"`
	// CreditHeading is the credit block's first line; empty leaves the items without a heading.
	CreditHeading string `yaml:"credit_heading"`
	// CreditLoomYard is the credit block's LoomYard item, listed whatever engines ran; empty adds none.
	CreditLoomYard string `yaml:"credit_loomyard"`
	// CreditEngines maps an engine name to the credit block's item for that engine, listed only when the engine ran a session in the task.
	// An engine left out, or mapped to empty text, adds nothing.
	CreditEngines map[string]string `yaml:"credit_engines"`
	// Deploy is the shell command batten's Land row runs in the prime after a landing into the prime's branch.
	// Empty runs nothing.
	Deploy string `yaml:"deploy"`
	// ParentLockIdleReleaseMin is the minutes a waiting run keeps its parent lock before the lock is released.
	// A run at Finalize and running, or with a live verify, keeps the lock past it.
	// A value below 1 reads as the default.
	ParentLockIdleReleaseMin int `yaml:"parent_lock_idle_release_min"`
	// Fixer is the model-spec string selecting the model the merge fixer session and the fix review session run under.
	Fixer string `yaml:"fixer"`
	// FixerTimeoutMin is the minutes one fixer turn, or the fix review session, may run.
	FixerTimeoutMin int `yaml:"fixer_timeout_min"`
	// ParentAlignFixAttempts is the fixer turns one Parent-Align row entry may spend.
	ParentAlignFixAttempts int `yaml:"parent_align_fix_attempts"`
	// FinalizeFixAttempts is the fixer turns one Finalize row entry may spend.
	FinalizeFixAttempts int `yaml:"finalize_fix_attempts"`
	// FinalizeFixMaxLines is the added plus deleted lines a fixer change may hold before it is over the cap.
	FinalizeFixMaxLines int `yaml:"finalize_fix_max_lines"`
	// FinalizeFixReviewRounds is the fix reviews one Finalize row entry may run over an over-cap fixer change before it halts for the operator.
	FinalizeFixReviewRounds int `yaml:"finalize_fix_review_rounds"`
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

// CreditBlock returns the plain-text credit block that ends the landing commit and the pull request's body.
// The block is the trimmed CreditHeading, then one ` - <item>` line for the trimmed CreditLoomYard and for the trimmed CreditEngines text of each name in enginesRun, in sorted name order, each name once.
// An empty or whitespace-only text adds no line.
// The block is empty when Credit is off or no item line remains, and an empty block means nothing follows the body.
func (c Config) CreditBlock(enginesRun []string) string {
	if !c.Credit {
		return ""
	}
	var items []string
	if text := strings.TrimSpace(c.CreditLoomYard); text != "" {
		items = append(items, text)
	}
	for _, name := range slices.Compact(slices.Sorted(slices.Values(enginesRun))) {
		if text := strings.TrimSpace(c.CreditEngines[name]); text != "" {
			items = append(items, text)
		}
	}
	if len(items) == 0 {
		return ""
	}
	var lines []string
	if heading := strings.TrimSpace(c.CreditHeading); heading != "" {
		lines = append(lines, heading)
	}
	for _, item := range items {
		lines = append(lines, " - "+item)
	}
	return strings.Join(lines, "\n")
}

// creditEnginesKey is the landing.yaml key whose entries map engine names to credit block items.
const creditEnginesKey = "credit_engines"

// ConfigOpenMaps returns the landing.yaml keys whose entries belong to the operator, which configengine carries whole through reconcile and the layer stack.
func ConfigOpenMaps() []string {
	return []string{creditEnginesKey}
}

// ConfigSpec returns landing's layered-load spec.
// Keys that gate or shape what lands on the parent branch, and the shell commands Publish and Land run, are barred from the local layer.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "landing",
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		OpenMaps: ConfigOpenMaps(),
		Categories: map[string]configengine.Category{
			"deploy_branches":      configengine.CategoryCrossPair,
			"squash":               configengine.CategoryCrossPair,
			"conflict":             configengine.CategoryNone,
			"conflict_timeout_min": configengine.CategoryNone,
			"describe":             configengine.CategoryNone,
			"describe_timeout_min": configengine.CategoryNone,
			"pr_reviewer":          configengine.CategoryNone,
			"credit":               configengine.CategoryCrossPair,
			"credit_heading":       configengine.CategoryCrossPair,
			"credit_loomyard":      configengine.CategoryCrossPair,
			creditEnginesKey:       configengine.CategoryCrossPair,
			"deploy":               configengine.CategoryExecutable,
			// The idle bound decides when one pair's parent lock frees for the next pair waiting on it.
			"parent_lock_idle_release_min": configengine.CategoryCrossPair,
			"fixer":                        configengine.CategoryNone,
			"fixer_timeout_min":            configengine.CategoryNone,
			// The attempt caps and the line cap bound what the fixer lands on the parent unreviewed.
			"parent_align_fix_attempts":  configengine.CategoryCrossPair,
			"finalize_fix_attempts":      configengine.CategoryCrossPair,
			"finalize_fix_max_lines":     configengine.CategoryCrossPair,
			"finalize_fix_review_rounds": configengine.CategoryCrossPair,
		},
		// A file still holding a retired key loads and reconcile prunes it.
		// publish_verify moved to the hub's publish gate.
		// The co-author and trailer keys gave way to the credit keys; their values are co-author lines, so none is carried over.
		RetiredKeys: map[string]configengine.Category{
			"publish_verify":  configengine.CategoryExecutable,
			"co_authored_by":  configengine.CategoryCrossPair,
			"trailer_text":    configengine.CategoryCrossPair,
			"engine_trailers": configengine.CategoryCrossPair,
		},
	}
}

// LoadConfig loads and unmarshals configuration for the landing module strictly through the hub-wide layer stack under dirs.
// It validates the conflict, describe, pr_reviewer and fixer keys' model-spec grammar at load time, refuses a fixer timeout, fixer attempt cap, fixer line cap or fix review round cap below 1, and refuses a credit_engines entry whose key names no engine.
func LoadConfig(dirs configengine.Dirs, module string) (Config, error) {
	spec := ConfigSpec()
	spec.Name = module
	resolved, err := configengine.Load(dirs, spec)
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lym fabric reconcile\"; a session lym refuses the verb from reports status: FAILED and the orch runs it")
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

	if _, err := modelspec.Parse(cfg.PRReviewer); err != nil {
		return Config{}, fmt.Errorf("landing config key %q: %w", "pr_reviewer", err)
	}

	if _, err := modelspec.Parse(cfg.Fixer); err != nil {
		return Config{}, fmt.Errorf("landing config key %q: %w", "fixer", err)
	}
	for _, positive := range []struct {
		key   string
		value int
	}{
		{"fixer_timeout_min", cfg.FixerTimeoutMin},
		{"parent_align_fix_attempts", cfg.ParentAlignFixAttempts},
		{"finalize_fix_attempts", cfg.FinalizeFixAttempts},
		{"finalize_fix_max_lines", cfg.FinalizeFixMaxLines},
		{"finalize_fix_review_rounds", cfg.FinalizeFixReviewRounds},
	} {
		if positive.value < 1 {
			return Config{}, fmt.Errorf("landing config key %q: %d is below 1; way forward: lym config landing --set %s=<n> from the prime", positive.key, positive.value, positive.key)
		}
	}
	engines := modelspec.EngineNames()
	for name := range cfg.CreditEngines {
		if !slices.Contains(engines, name) {
			return Config{}, fmt.Errorf("landing config key %q: %q is no engine; use one of %v, or remove the entry", creditEnginesKey+"."+name, name, engines)
		}
	}

	return cfg, nil
}

// retiredKeyRenames maps each retired landing.yaml key onto the key that replaced it.
var retiredKeyRenames = []yamlengine.KeyRename{
	{From: "require_pr_to_base", To: "deploy_branches"},
	{From: "merge_fix_attempts", To: "parent_align_fix_attempts"},
}

// MigrateConfig rewrites a landing.yaml document for lym config reconcile and returns the rewritten bytes with one line per rewrite.
// It renames the retired require_pr_to_base key to deploy_branches, keeping its value, an empty list included;
// a document already holding deploy_branches keeps it and drops the old key.
// It likewise renames the retired merge_fix_attempts key to parent_align_fix_attempts, keeping its value;
// a document already holding parent_align_fix_attempts keeps it and drops the old key.
// A document without a retired key is returned unchanged, so a migrated file is unchanged on a second pass.
// A document that does not parse, or whose root is not a mapping, returns an error marked configengine.ErrInvalid.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	migrated, lines, err := yamlengine.RenameKeys([]byte(ConfigTemplate()), existing, retiredKeyRenames)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate landing config: %w", err))
	}
	return migrated, lines, nil
}

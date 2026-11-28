// config.go defines the Config type that mirrors landing.yaml, its layered-load spec, and LoadConfig, which loads it.

package landingshed

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// Config represents the resolved landing.yaml configuration.
type Config struct {
	// DeployBranches is the list of base branch names a landing into which batten's Land row deploys.
	// An empty list deploys nothing.
	DeployBranches []string `yaml:"deploy_branches"`
	// Squash selects whether the merge into the parent branch squashes.
	Squash bool `yaml:"squash"`
	// Conflict is the model-spec string selecting the model both producers' conflict-resolution session runs under.
	Conflict string `yaml:"conflict"`
	// ConflictTimeoutMin is the conflict-resolution session's wall-clock budget in minutes.
	ConflictTimeoutMin int `yaml:"conflict_timeout_min"`
	// Describe is the model-spec string selecting the model the Describe row's session runs under.
	Describe string `yaml:"describe"`
	// DescribeTimeoutMin is the wall-clock budget in minutes of the Describe session's writer.
	// While a reviewer runs, the writer's and the reviewer's runs are each bounded by this plus DescribeReviewTimeoutMin.
	DescribeTimeoutMin int `yaml:"describe_timeout_min"`
	// DescribeForbiddenTerms are the regular expressions a change description's line may not match:
	// the names of the tooling that wrote the change, which a repository's reader never sees.
	// An empty list checks no term, for a repository whose own names they are.
	DescribeForbiddenTerms []string `yaml:"describe_forbidden_terms"`
	// forbiddenTerms are DescribeForbiddenTerms compiled, set by LoadConfig.
	forbiddenTerms []*regexp.Regexp
	// DescribeReview is the model-spec string of the Describe row's reviewer, which reviews the change description.
	// Empty runs no reviewer.
	DescribeReview string `yaml:"describe_review"`
	// DescribeReviewTimeoutMin is the longest unbroken wait of the writer's turn end on the reviewer, in minutes.
	// The wait begins once the description has passed its check.
	// While a reviewer runs, it is also added to DescribeTimeoutMin as the limit of both seats' runs.
	DescribeReviewTimeoutMin int `yaml:"describe_review_timeout_min"`
	// PRReviewer is the model-spec string selecting the model the PR-Review row's interactive session runs under.
	PRReviewer string `yaml:"pr_reviewer"`
	// Credit selects whether the landing commit and the pull request's body end with the credit block.
	Credit bool `yaml:"credit"`
	// CreditHeading is the credit block's first line; empty leaves the items without a heading.
	CreditHeading string `yaml:"credit_heading"`
	// CreditLoomYard is the credit block's LoomYard item, listed whatever engines ran; empty adds none.
	CreditLoomYard string `yaml:"credit_loomyard"`
	// CreditEngines maps an engine name to the credit block's item for that engine.
	// The item is listed only when the engine ran a session in the task.
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
	// FinalizeFixReviewRounds is the fix reviews one Finalize row entry may run over an over-cap fixer change.
	// Once they are spent, it halts for the operator.
	FinalizeFixReviewRounds int `yaml:"finalize_fix_review_rounds"`
	// FinalizeLostRaces is the times one Finalize call may find the parent moved since its lock-free merge and gate.
	// Once they are spent, Finalize merges and gates under the parent lock, so a run on a busy parent is never starved.
	// 0 merges and gates under the lock from the first pass.
	FinalizeLostRaces int `yaml:"finalize_lost_races"`
}

// defaultIdleReleaseMin is the idle bound in minutes.
// A missing or non-positive landing.parent_lock_idle_release_min reads as it.
const defaultIdleReleaseMin = 10

// IdleRelease returns the parent lock's idle bound.
// A landing.parent_lock_idle_release_min below 1, an absent key included, returns the default.
// It logs one Warn naming the key.
func (c Config) IdleRelease() time.Duration {
	if c.ParentLockIdleReleaseMin < 1 {
		name := keyName("parent_lock_idle_release_min")
		logger.Warn("landingshed: "+name+" is below 1 or absent, using the default", "key", name, "value", c.ParentLockIdleReleaseMin, "defaultMin", defaultIdleReleaseMin)
		return defaultIdleReleaseMin * time.Minute
	}
	return time.Duration(c.ParentLockIdleReleaseMin) * time.Minute
}

// CreditBlock returns the plain-text credit block that ends the landing commit and the pull request's body.
// The block is the trimmed CreditHeading, then one ` - <item>` line per item.
// The items are the trimmed CreditLoomYard text, then the trimmed CreditEngines text of each name in enginesRun.
// The engine items follow in sorted name order, each name once.
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

// ForbiddenTerms returns the compiled landing.describe_forbidden_terms.
// A change description's check refuses a line for them.
// It is empty for a Config LoadConfig did not return.
func (c Config) ForbiddenTerms() []*regexp.Regexp {
	return c.forbiddenTerms
}

// creditEnginesKey is the landing.credit_engines key whose entries map engine names to credit block items.
const creditEnginesKey = "credit_engines"

// ConfigOpenMaps returns the landing.yaml keys whose entries belong to the operator.
// configengine carries those keys whole through reconcile and the layer stack.
func ConfigOpenMaps() []string {
	return []string{creditEnginesKey}
}

// ConfigSpec returns landing's layered-load spec.
// Keys that gate or shape what lands on the parent branch are barred from the local layer.
// So is the shell command that Land runs.
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
			// The forbidden terms shape the text that lands on the parent branch and in the pull request.
			forbiddenTermsKey:           configengine.CategoryCrossPair,
			describeReviewKey:           configengine.CategoryNone,
			describeReviewTimeoutMinKey: configengine.CategoryNone,
			prReviewerKey:               configengine.CategoryNone,
			"credit":                    configengine.CategoryCrossPair,
			"credit_heading":            configengine.CategoryCrossPair,
			"credit_loomyard":           configengine.CategoryCrossPair,
			creditEnginesKey:            configengine.CategoryCrossPair,
			"deploy":                    configengine.CategoryExecutable,
			// The idle bound decides when one pair's parent lock frees for the next pair waiting on it.
			"parent_lock_idle_release_min": configengine.CategoryCrossPair,
			"fixer":                        configengine.CategoryNone,
			"fixer_timeout_min":            configengine.CategoryNone,
			// The attempt caps and the line cap bound what the fixer lands on the parent unreviewed.
			"parent_align_fix_attempts":  configengine.CategoryCrossPair,
			"finalize_fix_attempts":      configengine.CategoryCrossPair,
			"finalize_fix_max_lines":     configengine.CategoryCrossPair,
			"finalize_fix_review_rounds": configengine.CategoryCrossPair,
			// The lost-race bound decides how long one pair's landing gate holds the parent lock others wait on.
			finalizeLostRacesKey: configengine.CategoryCrossPair,
		},
		// The local and machine layers judge a retired key by the category it held, so both refuse every one here.
		// publish_verify belongs to the hub's publish gate.
		// The co-author and trailer keys are not carried into the credit keys, since their values are co-author lines.
		RetiredKeys: map[string]configengine.Category{
			"publish_verify":  configengine.CategoryExecutable,
			"co_authored_by":  configengine.CategoryCrossPair,
			"trailer_text":    configengine.CategoryCrossPair,
			"engine_trailers": configengine.CategoryCrossPair,
		},
	}
}

// LoadConfig loads and unmarshals the landing module's configuration through the hub-wide layer stack under dirs.
// The load is strict: an absent config file is an error.
// Its producers are reachable only inside a hub, where a missing config means the hub is broken.
// At load time it validates the model-spec grammar of these keys:
// `landing.conflict`, `landing.describe`, `landing.pr_reviewer` and `landing.fixer`.
// It validates that of `landing.describe_review` too when the key is set.
// A typo in one of them fails at load, not when its session spawns.
// It refuses a fixer timeout, fixer attempt cap, fixer line cap or fix review round cap below 1.
// It refuses a describe review timeout below 1 and a lost-race bound below 0.
// It also refuses a landing.credit_engines entry whose key names no engine,
// and a landing.describe_forbidden_terms entry that does not compile as a regular expression or matches the empty string.
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
		return Config{}, fmt.Errorf("config key %q: %w", keyName(conflictKey), err)
	}

	if _, err := modelspec.Parse(cfg.Describe); err != nil {
		return Config{}, fmt.Errorf("config key %q: %w", keyName(describeKey), err)
	}

	if _, err := modelspec.Parse(cfg.PRReviewer); err != nil {
		return Config{}, fmt.Errorf("config key %q: %w", keyName(prReviewerKey), err)
	}

	if _, err := modelspec.Parse(cfg.Fixer); err != nil {
		return Config{}, fmt.Errorf("config key %q: %w", keyName(fixerKey), err)
	}
	if cfg.DescribeReview != "" {
		if _, err := modelspec.Parse(cfg.DescribeReview); err != nil {
			return Config{}, fmt.Errorf("config key %q: %w", keyName(describeReviewKey), err)
		}
	}
	for _, positive := range []struct {
		key   string
		value int
	}{
		{fixerTimeoutMinKey, cfg.FixerTimeoutMin},
		{"parent_align_fix_attempts", cfg.ParentAlignFixAttempts},
		{"finalize_fix_attempts", cfg.FinalizeFixAttempts},
		{"finalize_fix_max_lines", cfg.FinalizeFixMaxLines},
		{"finalize_fix_review_rounds", cfg.FinalizeFixReviewRounds},
		{describeReviewTimeoutMinKey, cfg.DescribeReviewTimeoutMin},
	} {
		if positive.value < 1 {
			return Config{}, fmt.Errorf("config key %q: %d is below 1; way forward: lym config landing --set %s=<n> from the prime", keyName(positive.key), positive.value, positive.key)
		}
	}
	if cfg.FinalizeLostRaces < 0 {
		return Config{}, fmt.Errorf("config key %q: %d is below 0; way forward: lym config landing --set %s=<n> from the prime", keyName(finalizeLostRacesKey), cfg.FinalizeLostRaces, finalizeLostRacesKey)
	}
	engines := modelspec.EngineNames()
	for name := range cfg.CreditEngines {
		if !slices.Contains(engines, name) {
			return Config{}, fmt.Errorf("config key %q: %q is no engine; use one of %v, or remove the entry", keyName(creditEnginesKey+"."+name), name, engines)
		}
	}
	for i, term := range cfg.DescribeForbiddenTerms {
		pattern, err := regexp.Compile(term)
		if err != nil {
			return Config{}, fmt.Errorf("config key %q: entry %d %q is no regular expression: %w; way forward: fix the entry with lym config landing --set from the prime", keyName(forbiddenTermsKey), i, term, err)
		}
		// A line's match of such a pattern may be empty, which the check reads as no match.
		// The probe's word boundaries catch a pattern that matches empty only at one, such as `\b|term`.
		probe := pattern.FindStringIndex("a b")
		if pattern.MatchString("") || probe != nil && probe[0] == probe[1] {
			return Config{}, fmt.Errorf("config key %q: entry %d %q matches the empty string, so it would let lines holding its term through; way forward: fix the entry with lym config landing --set from the prime", keyName(forbiddenTermsKey), i, term)
		}
		cfg.forbiddenTerms = append(cfg.forbiddenTerms, pattern)
	}

	return cfg, nil
}

// config.go — configuration for the darn recipe.
//
// It defines the Config type mirroring darn.yaml's keys and the ConfigTemplate accessor.
// LoadConfig loads the file strictly.
// TreeScopedCommand classifies a diff against the configured trees and returns the tree-scoped command.
// The module is hub-wide, so every path here is told by the caller.

package darnengine

import (
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reviewbudget"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/yamlengine"
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

// The darn.yaml keys holding the Darn-Review perch's reviewer and fixer model lists and fan lists.
const (
	ReviewerModelKey = "darn_reviewer_model"
	FixerModelKey    = "darn_fixer_model"
	ReviewerFanKey   = "darn_reviewer_fan"
	FixerFanKey      = "darn_fixer_fan"
)

// treeScopeTargetsMarker is the placeholder in tree_scope_verify that the touched trees' targets replace.
const treeScopeTargetsMarker = "{targets}"

// The commands the tree-scope refusals name.
const (
	treeScopeAddTreeCommand = "lyx config darn --set tree_scope.<tree>=<target>"
	treeScopeOffCommand     = "lyx config darn --set tree_scope_verify="
	treeScopeVerifyCommand  = "lyx config darn --set tree_scope_verify=<one lyx gate test --wait call holding {targets}>"
)

// treeScopeControlOperators are the shell control operators a tree_scope_verify call may not hold; a newline and a carriage return end a command too.
var treeScopeControlOperators = []string{";", "&", "|", "\n", "\r", "`", "$(", "<", ">"}

// ErrTreeScopeWithoutTrees marks a tree_scope_verify that is set while tree_scope holds no tree.
var ErrTreeScopeWithoutTrees = errors.New("darn config key \"tree_scope_verify\" is set while \"tree_scope\" holds no tree")

// ErrTreeScopeKey marks a tree_scope key that is malformed or repeats another key after its trailing slash is trimmed.
var ErrTreeScopeKey = errors.New("darn config key \"tree_scope\" holds a malformed tree key")

// Config represents the resolved darn.yaml configuration.
type Config struct {
	// Writer is the model-spec string selecting the model the darn writer's session runs under.
	Writer string `yaml:"writer"`
	// WriterTimeoutMin is the writer session's wall-clock budget in minutes.
	WriterTimeoutMin int `yaml:"writer_timeout_min"`
	// VerifyAttempts is how many times a verify gate, the writer's or a Darn-Review fix round's, re-prompts its session before it halts the run.
	VerifyAttempts int `yaml:"verify_attempts"`
	// TreeScope maps each worktree-relative tree path, without a trailing slash, to the test target the tree contributes to TreeScopeVerify's {targets}.
	// An empty target means the tree has no tests of its own.
	TreeScope map[string]string `yaml:"tree_scope"`
	// TreeScopeVerify is the command a tree-scoped diff runs at every darn verify site in place of the landing call, holding {targets}.
	// It is one `lyx gate test` call holding `--wait` and no shell control operator, so the whole tree-scoped command is slotted and waits out a busy hub.
	// Tree scope is on only when it and TreeScope are both non-empty.
	TreeScopeVerify string `yaml:"tree_scope_verify"`
	// ReviewMaxBounces is the bounce budget of the Darn-Review perch that reviews the writer's diff before Publish; 0 runs neither the review nor Describe.
	// It is settable per run through the task's run_settings.
	ReviewMaxBounces int `yaml:"review_max_bounces"`
	// ReviewHardMaxBounces is the round number at or past which the Darn-Review perch never continues a round; it is at least 1 and not below ReviewMaxBounces, and settable per run.
	ReviewHardMaxBounces int `yaml:"review_hard_max_bounces"`
	// ReviewAmendMaxBounces is the review rounds an amendment generation gets after a settled Darn-Review subject changed, clamped to ReviewMaxBounces; 0 settles every amendment approved with no review round.
	// It is settable per run through the task's run_settings.
	ReviewAmendMaxBounces int `yaml:"review_amend_max_bounces"`
	// ReviewerModel and FixerModel are the Darn-Review perch's per-round reviewer and fixer model lists;
	// an empty key takes loom.yaml's shared reviewer_model or fixer_model.
	ReviewerModel burlerengine.ModelSpecList `yaml:"darn_reviewer_model"`
	FixerModel    burlerengine.ModelSpecList `yaml:"darn_fixer_model"`
	// ReviewerFan and FixerFan are the Darn-Review perch's per-round reviewer and fixer fan lists, each entry a lenses.yaml fan name;
	// an empty entry runs that round solo or with the single fixer.
	ReviewerFan lenses.FanList `yaml:"darn_reviewer_fan"`
	FixerFan    lenses.FanList `yaml:"darn_fixer_fan"`
	// DriverKind, Tackler, TacklerTimeoutMin and TacklerMaxSpawns override shed.yaml's keys of the same names for a darn run;
	// an empty string, a zero and a null inherit shed.yaml, and a set TacklerMaxSpawns of 0 turns the tackler off.
	DriverKind        string `yaml:"driver_kind"`
	Tackler           string `yaml:"tackler"`
	TacklerTimeoutMin int    `yaml:"tackler_timeout_min"`
	TacklerMaxSpawns  *int   `yaml:"tackler_max_spawns"`
	// PRReview turns on the PR-Review stub's log line; the stub opens no pull request and acts on nothing either way.
	PRReview bool `yaml:"pr_review"`
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

// ConfigOpenMaps returns the darn.yaml keys whose value is a scalar, a per-round list or the operator's own tree map, which configengine carries whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{ReviewerModelKey, FixerModelKey, ReviewerFanKey, FixerFanKey, "tree_scope"}
}

// retiredKeyRenames maps each retired darn.yaml fan key onto the key that replaced it.
var retiredKeyRenames = []yamlengine.KeyRename{
	{From: "review_fan", To: ReviewerFanKey},
	{From: "review_fix_fan", To: FixerFanKey},
}

// MigrateConfig rewrites a darn.yaml document for lyx config reconcile and returns the rewritten bytes with one line per rewrite.
// It renames the retired review_fan and review_fix_fan keys to darn_reviewer_fan and darn_fixer_fan with their values;
// a document already holding the new name keeps its value and drops the old key.
// A document without a retired key is returned unchanged, so a migrated file is unchanged on a second pass.
// A document that does not parse returns an error marked configengine.ErrInvalid.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	migrated, lines, err := yamlengine.RenameKeys([]byte(ConfigTemplate()), existing, retiredKeyRenames)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate darn config: %w", err))
	}
	return migrated, lines, nil
}

// ConfigSpec returns darn's layered-load spec.
// tree_scope_verify runs a shell command, so it may not be set in the local layer.
// tree_scope picks which diffs skip the landing gate, so it is a guard key and closed to the local layer too; no task run_settings sets it or tree_scope_verify.
// verify is retired, so a hub file that still sets it loads and reconcile prunes it.
// The two fan keys are closed to the local layer too, so no local override or task run_settings changes the perch's fans.
// The two budget keys come from reviewbudget and are ordinary keys, so a task's local layer, and its run_settings, set them.
func ConfigSpec() configengine.Spec {
	categories := map[string]configengine.Category{
		"writer":             configengine.CategoryNone,
		"writer_timeout_min": configengine.CategoryNone,
		"verify_attempts":    configengine.CategoryNone,
		"tree_scope":         configengine.CategoryGuard,
		"tree_scope_verify":  configengine.CategoryExecutable,
		ReviewerModelKey:     configengine.CategoryNone,
		FixerModelKey:        configengine.CategoryNone,
		ReviewerFanKey:       configengine.CategoryGuard,
		FixerFanKey:          configengine.CategoryGuard,

		"driver_kind":         configengine.CategoryNone,
		"tackler":             configengine.CategoryNone,
		"tackler_timeout_min": configengine.CategoryNone,
		"tackler_max_spawns":  configengine.CategoryNone,
		"pr_review":           configengine.CategoryNone,
	}
	for key, category := range reviewbudget.Categories(nil) {
		categories[key] = category
	}
	return configengine.Spec{
		Name:       configModule,
		Template:   []byte(ConfigTemplate()),
		HubWide:    true,
		OpenMaps:   ConfigOpenMaps(),
		Categories: categories,
		RetiredKeys: map[string]configengine.Category{
			"verify": configengine.CategoryExecutable,
		},
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
// It validates the writer key's model-spec grammar at load time and leaves verify_attempts and review_max_bounces to their readers.
// darn_reviewer_model and darn_fixer_model, when set, are refused as an empty list, a value that is neither a model-spec nor a list of them, or an entry that is no model-spec.
// darn_reviewer_fan and darn_fixer_fan are refused as an empty list, a value that is neither a fan name nor a list of them, or an entry naming a fan lenses.yaml does not resolve.
// review_max_bounces and review_amend_max_bounces are refused below 0, and review_hard_max_bounces, default 2, below 1 and below review_max_bounces;
// each refusal names the command that fixes it for the layer the value comes from.
// It returns tree_scope keyed by trimmed tree path and refuses a malformed or repeated tree key wrapping ErrTreeScopeKey, a tree_scope_verify with no tree wrapping ErrTreeScopeWithoutTrees, a tree_scope_verify without {targets},
// and a tree_scope_verify that is not exactly one `lyx gate test` call holding `--wait` and no shell control operator.
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
		var fanShape *lenses.FanShapeError
		if errors.As(err, &fanShape) {
			return Config{}, fmt.Errorf("darn config key %q: %w", configengine.KeyAtLine(resolved, fanShape.Line), err)
		}
		var modelShape *burlerengine.ModelSpecShapeError
		if errors.As(err, &modelShape) {
			return Config{}, fmt.Errorf("darn config key %q: %w", configengine.KeyAtLine(resolved, modelShape.Line), err)
		}
		return Config{}, fmt.Errorf("unmarshal darn config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Writer); err != nil {
		return Config{}, fmt.Errorf("darn config key %q: %w", "writer", err)
	}

	if err := sheddriver.CheckOverride(configModule, cfg.ShedOverride()); err != nil {
		return Config{}, err
	}

	// An empty model key takes loom.yaml's shared list at run start, so only a set one is validated.
	for _, model := range []struct {
		key   string
		specs burlerengine.ModelSpecList
	}{
		{ReviewerModelKey, cfg.ReviewerModel},
		{FixerModelKey, cfg.FixerModel},
	} {
		if model.specs.Unset() {
			continue
		}
		if err := burlerengine.ValidateModelSpecList(configModule, model.key, model.specs); err != nil {
			return Config{}, err
		}
	}

	fanKeys := []lenses.FanKey{{Key: ReviewerFanKey, Fans: cfg.ReviewerFan}, {Key: FixerFanKey, Fans: cfg.FixerFan}}
	if err := lenses.ValidateFanKeys(dirs, configModule, fanKeys); err != nil {
		return Config{}, err
	}

	if err := reviewbudget.CheckResolved(dirs, spec, reviewbudget.Values{RunSoft: cfg.ReviewMaxBounces, RunHard: cfg.ReviewHardMaxBounces, RunAmend: cfg.ReviewAmendMaxBounces}); err != nil {
		return Config{}, err
	}

	treeScope, err := normalizeTreeScope(cfg)
	if err != nil {
		return Config{}, err
	}
	cfg.TreeScope = treeScope

	return cfg, nil
}

// normalizeTreeScope returns cfg's tree_scope keyed by trimmed tree path, or the refusal of a malformed key, a repeated tree, a tree_scope_verify without a tree, one without {targets}, or one that is not a single waiting gate call.
func normalizeTreeScope(cfg Config) (map[string]string, error) {
	keys := make([]string, 0, len(cfg.TreeScope))
	for key := range cfg.TreeScope {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	trees := make(map[string]string, len(keys))
	written := make(map[string]string, len(keys))
	for _, key := range keys {
		tree := strings.TrimSuffix(key, "/")
		if strings.Contains(tree, `\`) || slices.ContainsFunc(strings.Split(tree, "/"), func(segment string) bool {
			return segment == "" || segment == "." || segment == ".."
		}) {
			return nil, fmt.Errorf("%w %q", ErrTreeScopeKey, key)
		}
		if first, repeated := written[tree]; repeated {
			return nil, fmt.Errorf("%w: %q and %q name the same tree", ErrTreeScopeKey, first, key)
		}
		written[tree] = key
		trees[tree] = cfg.TreeScope[key]
	}

	if strings.TrimSpace(cfg.TreeScopeVerify) == "" {
		return trees, nil
	}
	if len(trees) == 0 {
		return nil, fmt.Errorf("%w; run %q in the same call as the command, or %q to turn tree scope off; a tree path holding \".\", such as \".github\", is written in lyx config darn's editor", ErrTreeScopeWithoutTrees, treeScopeAddTreeCommand, treeScopeOffCommand)
	}
	if !strings.Contains(cfg.TreeScopeVerify, treeScopeTargetsMarker) {
		return nil, fmt.Errorf("darn config key %q holds no %s; run %q", "tree_scope_verify", treeScopeTargetsMarker, treeScopeVerifyCommand)
	}
	if !isSingleWaitingGateCall(cfg.TreeScopeVerify) {
		return nil, fmt.Errorf("darn config key %q is not exactly one lyx gate test call holding --wait and no shell control operator, so the call would not be slotted and would not wait out a busy hub; run %q", "tree_scope_verify", treeScopeVerifyCommand)
	}
	return trees, nil
}

// isSingleWaitingGateCall reports whether command is one `lyx gate test` call that holds `--wait` and no shell control operator.
// It reads static text only, so a script the call runs is outside it.
func isSingleWaitingGateCall(command string) bool {
	for _, operator := range treeScopeControlOperators {
		if strings.Contains(command, operator) {
			return false
		}
	}
	words := strings.Fields(command)
	return len(words) >= 3 && words[0] == "lyx" && words[1] == "gate" && words[2] == "test" && slices.Contains(words, "--wait")
}

// WithEditorWayForward appends the editor way forward to an error wrapping ErrTreeScopeKey, since --set cannot remove a key the hub file holds, and returns every other error, nil included, unchanged.
// A reader of the hub darn.yaml that refuses on a load error returns its error through it.
func WithEditorWayForward(err error) error {
	if !errors.Is(err, ErrTreeScopeKey) {
		return err
	}
	return fmt.Errorf("%w; fix the named tree_scope key in lyx config darn's editor from the prime, then re-run the lyx command that refused", err)
}

// TreeScopedCommand classifies a diff against cfg's trees and returns the tree-scoped command and whether the diff is tree-scoped.
// cfg is one LoadConfig returned, so its tree keys carry no trailing slash and a non-empty TreeScopeVerify comes with at least one tree.
// Tree scope is off, and the answer ("", false), when TreeScopeVerify is empty.
// A diff is tree-scoped when every path lies in some tree, an empty path list included.
// A path touches every tree it lies in, nested trees included.
// The command is TreeScopeVerify with {targets} replaced by the touched trees' non-empty targets, deduplicated, in tree-path order, so no path text reaches it.
// When no touched tree contributes a target, as for a docs-only or empty diff, the answer is scoped with an empty command, which every darn site runs as nothing.
func TreeScopedCommand(cfg Config, paths []string) (string, bool) {
	if strings.TrimSpace(cfg.TreeScopeVerify) == "" {
		return "", false
	}
	trees := make([]string, 0, len(cfg.TreeScope))
	for tree := range cfg.TreeScope {
		trees = append(trees, tree)
	}
	slices.Sort(trees)

	touched := make(map[string]bool, len(trees))
	for _, path := range paths {
		inAnyTree := false
		for _, tree := range trees {
			if path == tree || strings.HasPrefix(path, tree+"/") {
				touched[tree] = true
				inAnyTree = true
			}
		}
		if !inAnyTree {
			return "", false
		}
	}

	var targets []string
	for _, tree := range trees {
		target := cfg.TreeScope[tree]
		if touched[tree] && target != "" && !slices.Contains(targets, target) {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		return "", true
	}
	return strings.ReplaceAll(cfg.TreeScopeVerify, treeScopeTargetsMarker, strings.Join(targets, " ")), true
}

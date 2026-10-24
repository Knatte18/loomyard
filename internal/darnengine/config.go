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

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/sheddriver"
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
	// VerifyAttempts is how many times a verify gate, the writer's or a Code-Review fix round's, re-prompts its session before it halts the run.
	VerifyAttempts int `yaml:"verify_attempts"`
	// TreeScope maps each worktree-relative tree path, without a trailing slash, to the test target the tree contributes to TreeScopeVerify's {targets}.
	// An empty target means the tree has no tests of its own.
	TreeScope map[string]string `yaml:"tree_scope"`
	// TreeScopeVerify is the command a tree-scoped diff runs at every darn verify site in place of the landing call, holding {targets}.
	// It is one `lyx gate test` call holding `--wait` and no shell control operator, so the whole tree-scoped command is slotted and waits out a busy hub.
	// Tree scope is on only when it and TreeScope are both non-empty.
	TreeScopeVerify string `yaml:"tree_scope_verify"`
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

// ConfigOpenMaps returns the darn.yaml keys whose value is the operator's own tree map, which configengine carries whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{"tree_scope"}
}

// ConfigSpec returns darn's layered-load spec.
// tree_scope_verify runs a shell command, so it may not be set in the local layer.
// tree_scope picks which diffs skip the landing gate, so it is a guard key and closed to the local layer too; no task run_settings sets it or tree_scope_verify.
// verify is retired, so a hub file that still sets it loads and reconcile prunes it.
func ConfigSpec() configengine.Spec {
	categories := map[string]configengine.Category{
		"writer":             configengine.CategoryNone,
		"writer_timeout_min": configengine.CategoryNone,
		"verify_attempts":    configengine.CategoryNone,
		"tree_scope":         configengine.CategoryGuard,
		"tree_scope_verify":  configengine.CategoryExecutable,

		"driver_kind":         configengine.CategoryNone,
		"tackler":             configengine.CategoryNone,
		"tackler_timeout_min": configengine.CategoryNone,
		"tackler_max_spawns":  configengine.CategoryNone,
		"pr_review":           configengine.CategoryNone,
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

// ValidateLocalValues checks the shed override keys a local-layer document sets against their bounds, and leaves every other key to LoadConfig.
// A refusal is marked configengine.ErrInvalid and names the key.
func ValidateLocalValues(document []byte) error {
	return sheddriver.CheckLocalOverride(configModule, document)
}

// LoadConfig loads and unmarshals the darn module's configuration strictly through the hub-wide layer stack under dirs.
// It validates the writer key's model-spec grammar at load time and leaves verify_attempts to its reader.
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
		return Config{}, fmt.Errorf("unmarshal darn config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Writer); err != nil {
		return Config{}, fmt.Errorf("darn config key %q: %w", "writer", err)
	}

	if err := sheddriver.CheckOverride(configModule, cfg.ShedOverride()); err != nil {
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

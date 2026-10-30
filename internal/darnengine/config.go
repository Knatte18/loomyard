// config.go — configuration for the darn recipe.
//
// It defines the Config type mirroring darn.yaml's keys and the ConfigTemplate accessor.
// LoadConfig loads the file strictly.
// TreeScopedTargets classifies a diff against the configured trees and returns the touched trees' targets.
// MigrateConfig carries a hub file off the retired tree_scope_verify key.
// The module is hub-wide, so every path here is told by the caller.

package darnengine

import (
	"bytes"
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
	// TreeScope maps each worktree-relative tree path, without a trailing slash, to the test target the tree contributes to a tree-scoped site's call.
	// An empty target means the tree has no tests of its own.
	// Tree scope is on exactly when it holds a tree.
	TreeScope map[string]string `yaml:"tree_scope"`
	// DriverKind, Tackler, TacklerTimeoutMin, TacklerMaxSpawns, Arbiter, ArbiterTimeoutMin and ArbiterMaxSpawns override shed.yaml's keys of the same names for a darn run;
	// an empty string, a zero and a null inherit shed.yaml, and a set TacklerMaxSpawns or ArbiterMaxSpawns of 0 turns that role off.
	DriverKind        string `yaml:"driver_kind"`
	Tackler           string `yaml:"tackler"`
	TacklerTimeoutMin int    `yaml:"tackler_timeout_min"`
	TacklerMaxSpawns  *int   `yaml:"tackler_max_spawns"`
	Arbiter           string `yaml:"arbiter"`
	ArbiterTimeoutMin int    `yaml:"arbiter_timeout_min"`
	ArbiterMaxSpawns  *int   `yaml:"arbiter_max_spawns"`
}

// ShedOverride returns the driver, tackler and arbiter keys of darn.yaml as the override of shed.yaml's.
func (c Config) ShedOverride() sheddriver.Override {
	return sheddriver.Override{
		DriverKind:        c.DriverKind,
		Tackler:           c.Tackler,
		TacklerTimeoutMin: c.TacklerTimeoutMin,
		TacklerMaxSpawns:  c.TacklerMaxSpawns,
		Arbiter:           c.Arbiter,
		ArbiterTimeoutMin: c.ArbiterTimeoutMin,
		ArbiterMaxSpawns:  c.ArbiterMaxSpawns,
	}
}

// ConfigOpenMaps returns the darn.yaml keys whose value is the operator's own tree map, which configengine carries whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{"tree_scope"}
}

// ConfigSpec returns darn's layered-load spec.
// tree_scope picks which diffs run the narrower targets, so it is a guard key and closed to the local layer; no task run_settings sets it.
// verify and tree_scope_verify are retired, as is pr_review, so a hub file that still sets any of them loads and reconcile prunes it.
func ConfigSpec() configengine.Spec {
	categories := map[string]configengine.Category{
		"writer":             configengine.CategoryNone,
		"writer_timeout_min": configengine.CategoryNone,
		"verify_attempts":    configengine.CategoryNone,
		"tree_scope":         configengine.CategoryGuard,

		"driver_kind":         configengine.CategoryNone,
		"tackler":             configengine.CategoryNone,
		"tackler_timeout_min": configengine.CategoryNone,
		"tackler_max_spawns":  configengine.CategoryNone,
		"arbiter":             configengine.CategoryNone,
		"arbiter_timeout_min": configengine.CategoryNone,
		"arbiter_max_spawns":  configengine.CategoryNone,
	}
	return configengine.Spec{
		Name:       configModule,
		Template:   []byte(ConfigTemplate()),
		HubWide:    true,
		OpenMaps:   ConfigOpenMaps(),
		Categories: categories,
		RetiredKeys: map[string]configengine.Category{
			"verify":            configengine.CategoryExecutable,
			"tree_scope_verify": configengine.CategoryExecutable,
			"pr_review":         configengine.CategoryNone,
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
// It returns tree_scope keyed by trimmed tree path and refuses a malformed or repeated tree key wrapping ErrTreeScopeKey.
func LoadConfig(dirs configengine.Dirs, module string) (Config, error) {
	spec := ConfigSpec()
	spec.Name = module
	resolved, err := configengine.Load(dirs, spec)
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lym fabric reconcile\"")
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

// normalizeTreeScope returns cfg's tree_scope keyed by trimmed tree path, or the refusal of a malformed key or a repeated tree.
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
	return trees, nil
}

// MigrateConfig carries a darn.yaml document off the retired tree_scope_verify key, for lym config reconcile.
// It fires only while the document still holds the key.
// A key held empty empties tree_scope and is pruned, so an off hub stays off; a key held set is pruned and tree_scope kept.
// A document without the key is returned unchanged, so trees the operator adds later are never touched, and so a second pass changes nothing.
// Each rewrite returns one line, and every comment and other key is carried through yaml node editing.
// A document that does not parse, or whose root is not a mapping, returns an error marked configengine.ErrInvalid.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate darn config: unmarshal yaml: %w", err))
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return existing, nil, nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate darn config: yaml root is not a mapping"))
	}

	retiredAt := mappingKeyIndex(root, "tree_scope_verify")
	if retiredAt < 0 {
		return existing, nil, nil
	}
	held := strings.TrimSpace(root.Content[retiredAt+1].Value)
	root.Content = slices.Delete(root.Content, retiredAt, retiredAt+2)

	line := "tree_scope_verify: retired, removed; tree_scope kept"
	if held == "" {
		line = "tree_scope_verify: retired, removed; tree scope was off, tree_scope emptied"
		if at := mappingKeyIndex(root, "tree_scope"); at >= 0 {
			old := root.Content[at+1]
			root.Content[at+1] = &yaml.Node{
				Kind:        yaml.MappingNode,
				Tag:         "!!map",
				Style:       yaml.FlowStyle,
				HeadComment: old.HeadComment,
				LineComment: old.LineComment,
				FootComment: old.FootComment,
			}
		}
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate darn config: encode yaml: %w", err))
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate darn config: encode yaml: %w", err))
	}
	return out.Bytes(), []string{line}, nil
}

// mappingKeyIndex returns the index of key among mapping's content, which is the key node's, or -1 when mapping holds no such key.
func mappingKeyIndex(mapping *yaml.Node, key string) int {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// WithEditorWayForward appends the editor way forward to an error wrapping ErrTreeScopeKey, since --set cannot remove a key the hub file holds, and returns every other error, nil included, unchanged.
// A reader of the hub darn.yaml that refuses on a load error returns its error through it.
func WithEditorWayForward(err error) error {
	if !errors.Is(err, ErrTreeScopeKey) {
		return err
	}
	return fmt.Errorf("%w; fix the named tree_scope key in lym config darn's editor from the prime, then re-run the lym command that refused", err)
}

// TreeScopedTargets classifies a diff against cfg's trees and returns the touched trees' targets and whether the diff is tree-scoped.
// cfg is one LoadConfig returned, so its tree keys carry no trailing slash.
// Tree scope is off, and the answer (nil, false), when cfg holds no tree.
// A diff is tree-scoped when every path lies in some tree, an empty path list included.
// A path touches every tree it lies in, nested trees included.
// The targets are the touched trees' non-empty targets, deduplicated, in tree-path order, so no path text reaches them.
// When no touched tree contributes a target, as for a docs-only or empty diff, the answer is scoped with no targets, which every darn site runs as nothing.
func TreeScopedTargets(cfg Config, paths []string) ([]string, bool) {
	if len(cfg.TreeScope) == 0 {
		return nil, false
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
			return nil, false
		}
	}

	var targets []string
	for _, tree := range trees {
		target := cfg.TreeScope[tree]
		if touched[tree] && target != "" && !slices.Contains(targets, target) {
			targets = append(targets, target)
		}
	}
	return targets, true
}

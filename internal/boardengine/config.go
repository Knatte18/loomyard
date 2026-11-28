// config.go holds the board's configuration: the Config and Outputs types, the label vocabulary, and LoadConfig.
//
// LoadConfig resolves the board config file with internal/configengine.Load, strictly checked against ConfigTemplate().
// The boardengine module never reads config files or knows their layout itself.

package boardengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"gopkg.in/yaml.v3"
)

// Config is the configuration of a board module, read from board.yaml.
// Its run-time fields, such as Path and Clock, are supplied by the caller rather than read from the file.
type Config struct {
	// Path is the absolute path to the board data directory.
	// The caller sets it, from the hub's board directory or the --board-path flag, never the config file.
	// yaml:"-" keeps the yaml.v3 unmarshaller from mapping a leftover path: key onto this field.
	Path         string `yaml:"-"`
	Readme       string `yaml:"readme"`
	DesignPrefix string `yaml:"design_prefix"`
	// DefaultRecipe is the recipe a task with no recipe field runs.
	DefaultRecipe string `yaml:"default_recipe"`
	// Types are the type labels;
	// Labels are every other configured label, both in board.yaml order.
	// LoadConfig decodes them from the yaml node tree,
	// so the tags are inert.
	Types  []Label `yaml:"-"`
	Labels []Label `yaml:"-"`
	// SkipGit and SkipPush are populated from BOARD_SKIP_* env at the CLI entry;
	// ApplySkipEnv is the fold every CLI entry calls.
	SkipGit  bool
	SkipPush bool
	// Clock is the time a write stamps entry dates with and renders the README's ages as of;
	// nil is the wall clock.
	Clock func() time.Time `yaml:"-"`
	// ParentOf names the slug of an entry's parent pair, "" for the prime.
	// Reads and writes both resolve an effective owner from it;
	// nil reads every parent as the prime.
	ParentOf func(slug string) (string, error) `yaml:"-"`
	// PairExists reports whether an entry's pair exists on this machine,
	// so a write that moves or renames an entry with a pair is refused;
	// nil reads every pair as absent.
	PairExists func(slug string) bool `yaml:"-"`
	// ValidateSlug applies the pair slug rule to a task or project slug a write makes, returning the rule's refusal;
	// nil checks nothing, which is what a Board that makes no slug builds.
	ValidateSlug func(slug string) error `yaml:"-"`
	// StrandName is the full agent name of the session making the writes, "" for the operator at a terminal;
	// a run agent's name makes every write that creates or promotes a task refused.
	StrandName string `yaml:"-"`
}

// Label is one configured label with its description, which is empty when board.yaml gives none.
type Label struct {
	Name        string `json:"label"`
	Description string `json:"description"`
}

// ConfigOpenMaps returns the board.yaml keys whose entries are the repository's own.
// configengine carries these keys whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{"types", "labels"}
}

// labelNames returns the names of labels in order.
func labelNames(labels []Label) []string {
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return names
}

// Outputs holds the configuration values that a board's rendering reads, derived from a Config.
type Outputs struct {
	Readme       string
	DesignPrefix string
	// DefaultRecipe is the recipe a task with no recipe field runs.
	DefaultRecipe string
	// Types is the configured type-label order, which the renderer sorts Notes sections by.
	Types []string
}

// Outputs returns the values a board's rendering reads from c.
func (c Config) Outputs() Outputs {
	return Outputs{
		Readme:        c.Readme,
		DesignPrefix:  c.DesignPrefix,
		DefaultRecipe: c.DefaultRecipe,
		Types:         labelNames(c.Types),
	}
}

// recipeOf returns the recipe task runs: its own recipe field, or DefaultRecipe when the field is empty.
// It is the one place an empty recipe field is resolved, so the README and the run's seeding agree.
func (o Outputs) recipeOf(task Task) string {
	if task.Recipe != "" {
		return task.Recipe
	}
	return o.DefaultRecipe
}

// Vocabulary answers whether a label is a configured type label or a configured plain label.
// The zero value knows no label, which is the vocabulary of a Config that sets only Path.
type Vocabulary struct {
	Types  []string
	Labels []string
}

// Vocabulary returns the label vocabulary configured in c.
func (c Config) Vocabulary() Vocabulary {
	return Vocabulary{Types: labelNames(c.Types), Labels: labelNames(c.Labels)}
}

// IsType reports whether label is a configured type label.
func (v Vocabulary) IsType(label string) bool {
	return slices.Contains(v.Types, label)
}

// Known reports whether label is configured, as a type or as a plain label.
func (v Vocabulary) Known(label string) bool {
	return v.IsType(label) || slices.Contains(v.Labels, label)
}

// ConfigSpec returns the board's layered-load spec.
// Every key names the board's vocabulary or files, so none may be set in the local layer.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "board",
		Template: []byte(ConfigTemplate()),
		OpenMaps: ConfigOpenMaps(),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			"readme":         configengine.CategoryCrossPair,
			"design_prefix":  configengine.CategoryCrossPair,
			"default_recipe": configengine.CategoryCrossPair,
			"types":          configengine.CategoryCrossPair,
			"labels":         configengine.CategoryCrossPair,
		},
	}
}

// LoadConfig loads and unmarshals the board module configuration strictly through the hub-wide layer stack under dirs.
// An absent `_lym/` under the hub's board dir refuses the load, as does an absent board.yaml under a present one.
// The refusal names the file and the `lym fabric reconcile` way forward.
// The built-in sketch type label leads the types unless board.yaml lists it.
// Type labels that share a handle letter with each other, with the sketch type label or with tasks are refused.
// The refusal names the label to rename.
func LoadConfig(dirs configengine.Dirs, module string) (Config, error) {
	spec := ConfigSpec()
	spec.Name = module
	configFile, _ := configengine.LayerFile(dirs, spec, configengine.BaseLayer(dirs, spec))
	notInitialized := fmt.Errorf("board config %s not initialized; run \"lym fabric reconcile\"; a session lym refuses the verb from reports status: FAILED and the orch runs it", configFile)
	if _, err := os.Stat(configFile); errors.Is(err, fs.ErrNotExist) {
		return Config{}, notInitialized
	}

	// Load and resolve the config file using the template.
	resolved, err := configengine.Load(dirs, spec)
	if err != nil {
		if errors.Is(err, configengine.ErrNotInitialized) {
			return Config{}, notInitialized
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal board config: %w", err)
	}

	var raw struct {
		Types  yaml.Node `yaml:"types"`
		Labels yaml.Node `yaml:"labels"`
	}
	if err := yaml.Unmarshal(resolved, &raw); err != nil {
		return Config{}, fmt.Errorf("unmarshal board config: %w", err)
	}
	if cfg.Types, err = decodeLabels("types", &raw.Types); err != nil {
		return Config{}, err
	}
	if cfg.Labels, err = decodeLabels("labels", &raw.Labels); err != nil {
		return Config{}, err
	}
	cfg.Types = withSketchType(cfg.Types)
	if err := validateTypeLetters(cfg.Types); err != nil {
		return Config{}, err
	}
	if err := checkDesignPrefix(cfg.DesignPrefix); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// checkDesignPrefix refuses a design_prefix that would render a design file onto a decision record's name.
// A decision record is named decision-<slug>.md, so a prefix of "decision-" is refused, the empty one included.
// A prefix that starts with "decision-" is refused too.
func checkDesignPrefix(prefix string) error {
	if strings.HasPrefix(decisionFilePrefix, prefix) || strings.HasPrefix(prefix, decisionFilePrefix) {
		return fmt.Errorf("board.yaml: design_prefix %q would name a design file %s<slug>%s, the name of an entry's decision record: set design_prefix in board.yaml to another value, such as \"design-\", with \"lym config board --set design_prefix=design-\"", prefix, decisionFilePrefix, decisionFileSuffix)
	}
	return nil
}

// decodeLabels reads one board.yaml key into labels in file order.
// A mapping gives each name its scalar description, where an empty or null one is the empty string;
// a sequence of names is also accepted, read with empty descriptions.
func decodeLabels(key string, node *yaml.Node) ([]Label, error) {
	switch {
	case node.Kind == 0, node.Kind == yaml.ScalarNode && node.Tag == "!!null":
		return nil, nil
	case node.Kind == yaml.MappingNode:
		labels := make([]Label, 0, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			name, desc := node.Content[i], node.Content[i+1]
			if desc.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("board.yaml: %s: the description of %q must be a scalar", key, name.Value)
			}
			text := desc.Value
			if desc.Tag == "!!null" {
				text = ""
			}
			labels = append(labels, Label{Name: name.Value, Description: text})
		}
		return labels, nil
	case node.Kind == yaml.SequenceNode:
		labels := make([]Label, 0, len(node.Content))
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("board.yaml: %s: each entry of the list must be a label name", key)
			}
			labels = append(labels, Label{Name: item.Value})
		}
		return labels, nil
	}
	return nil, fmt.Errorf("board.yaml: %s must be a map from label to description, or a list of labels", key)
}

// OpenHub opens the board of the hub at hubPath.
// It reads the config from the hub's own Bolt checkout and points Path at its board directory.
// No worktree link is followed, and `ApplySkipEnv` is applied so render and sync behave as they do for `lym board`.
// It is told only a hub path, so it loads with no anchor and therefore no local layer.
func OpenHub(hubPath string) (*Board, error) {
	boltDir := fabricengine.BoltDir(hubPath)
	boardDir := fabricengine.BoardDir(hubPath)
	cfg, err := LoadConfig(configengine.Dirs{Board: boltDir}, "board")
	if err != nil {
		return nil, err
	}
	cfg.Path = boardDir
	return New(ApplySkipEnv(cfg)), nil
}

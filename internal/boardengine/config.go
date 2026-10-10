// config.go — configuration for the boardengine module.
//
// Defines the Config and Outputs types and LoadConfig.
// LoadConfig uses internal/configengine.Load with the ConfigTemplate() to strictly validate and
// resolve the board config file;
// the boardengine module never reads config files or knows their layout itself.

package boardengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"gopkg.in/yaml.v3"
)

// Config represents the configuration for a board module.
type Config struct {
	// Path is the absolute path to the board data directory. It is set by the
	// caller (boardcli.Command's PersistentPreRunE via fabricengine.BoardDir or the
	// --board-path flag), never by the config file. yaml:"-" prevents the
	// yaml.v3 unmarshaller from mapping any leftover path: key onto this field.
	Path         string `yaml:"-"`
	Readme       string `yaml:"readme"`
	DesignPrefix string `yaml:"design_prefix"`
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
}

// Label is one configured label with its description, which is empty when board.yaml gives none.
type Label struct {
	Name        string `json:"label"`
	Description string `json:"description"`
}

// ConfigOpenMaps returns the board.yaml keys whose entries are the repository's own, which configengine carries whole through reconcile and --set.
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

// Outputs represents the output configuration values derived from Config.
type Outputs struct {
	Readme       string
	DesignPrefix string
	// Types is the configured type-label order, which the renderer sorts Notes sections by.
	Types []string
}

// Outputs returns the Outputs derived from a Config.
func (c Config) Outputs() Outputs {
	return Outputs{
		Readme:       c.Readme,
		DesignPrefix: c.DesignPrefix,
		Types:        labelNames(c.Types),
	}
}

// Vocabulary is the one answer to "is this a type label" and "is this label configured".
// The zero value, a path-only Config's vocabulary, knows no label.
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

// LoadConfig loads and unmarshals the board module configuration from baseDir, the hub's board dir.
// An absent `_lyx/` under baseDir and an absent board.yaml under a present one both refuse naming the file and the `lyx fabric reconcile` way forward.
func LoadConfig(baseDir, module string) (Config, error) {
	configFile := configengine.ConfigFile(baseDir, module)
	notInitialized := fmt.Errorf("board config %s not initialized; run \"lyx fabric reconcile\"; a session lyx refuses the verb from reports status: FAILED and the orch runs it", configFile)
	if _, err := os.Stat(configFile); errors.Is(err, fs.ErrNotExist) {
		return Config{}, notInitialized
	}

	// Load and resolve the config file using the template.
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()), ConfigOpenMaps()...)
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

	return cfg, nil
}

// decodeLabels reads one board.yaml key into labels in file order.
// A mapping gives each name its scalar description, where an empty or null one is the empty string;
// a sequence of names is the older shape, read with empty descriptions.
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
// It reads the config from the hub's own board directory and points Path at it, so no worktree link is followed,
// and applies ApplySkipEnv so render and sync behave as they do for `lyx board`.
func OpenHub(hubPath string) (*Board, error) {
	boardDir := fabricengine.BoardDir(hubPath)
	cfg, err := LoadConfig(boardDir, "board")
	if err != nil {
		return nil, err
	}
	cfg.Path = boardDir
	return New(ApplySkipEnv(cfg)), nil
}

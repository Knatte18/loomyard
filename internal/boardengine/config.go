// config.go — configuration for the boardengine module.
//
// Defines the Config and Outputs types and LoadConfig.
// LoadConfig uses internal/configengine.Load with the ConfigTemplate() to strictly validate and
// resolve the board config file;
// the boardengine module never reads config files or knows their layout itself.

package boardengine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
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
	// Types are the type labels; Labels are every other configured label.
	Types  []string `yaml:"types"`
	Labels []string `yaml:"labels"`
	// SkipGit and SkipPush are populated from BOARD_SKIP_* env at the CLI entry;
	// ApplySkipEnv is the fold every CLI entry calls.
	SkipGit  bool
	SkipPush bool
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
		Types:        c.Types,
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
	return Vocabulary{Types: c.Types, Labels: c.Labels}
}

// IsType reports whether label is a configured type label.
func (v Vocabulary) IsType(label string) bool {
	return slices.Contains(v.Types, label)
}

// Known reports whether label is configured, as a type or as a plain label.
func (v Vocabulary) Known(label string) bool {
	return v.IsType(label) || slices.Contains(v.Labels, label)
}

// LoadConfig loads and unmarshals the board module configuration.
func LoadConfig(baseDir, module string) (Config, error) {
	// Load and resolve the config file using the template.
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx fabric reconcile\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal board config: %w", err)
	}

	return cfg, nil
}

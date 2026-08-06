// config.go — configuration for the boardengine module.
//
// Defines the Config and Outputs types and LoadConfig.
// LoadConfig uses internal/configengine.Load with the ConfigTemplate() to strictly validate and
// resolve the board config file;
// the boardengine module never reads config files or knows their layout itself.

package boardengine

import (
	"fmt"
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
	// SkipGit and SkipPush are populated from BOARD_SKIP_* env at the CLI entry.
	SkipGit  bool
	SkipPush bool
}

// Outputs represents the output configuration values derived from Config.
type Outputs struct {
	Readme       string
	DesignPrefix string
}

// Outputs returns the Outputs derived from a Config.
func (c Config) Outputs() Outputs {
	return Outputs{
		Readme:       c.Readme,
		DesignPrefix: c.DesignPrefix,
	}
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

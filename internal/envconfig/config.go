// config.go defines env.yaml: the Config type, its template and spec accessors, and LoadConfig.
// It also holds the forms the variables take when a process starts.

package envconfig

import (
	_ "embed"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for the env config module.
func ConfigTemplate() string {
	return configTemplate
}

// configModule is the config module name env.yaml is loaded under.
const configModule = "env"

// varsKey is the per-name map of variable name to value.
const varsKey = "vars"

// reservedPrefix starts the names of lym's own variables, which the module may not set.
const reservedPrefix = "LYM_"

// reservedPath is the search path variable, which the module may not set.
// A strand pane's prelude prepends the spawning lym directory to it, and a module value would replace that entry.
const reservedPath = "PATH"

// namePattern is the grammar of an environment variable name a shell can export.
var namePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Config is the resolved env.yaml: the variables lym exports to the processes it starts for the hub.
type Config struct {
	// Vars maps a variable name to its value; it is never nil.
	Vars map[string]string `yaml:"vars"`
}

// ConfigPerNameMaps returns the env.yaml keys whose entries the operator names.
// Those entries merge name by name, so a machine value overrides one hub value and keeps the rest.
func ConfigPerNameMaps() []string {
	return []string{varsKey}
}

// ConfigSpec returns the env module's layered-load spec.
// The module is hub-wide and `vars` is a machine key,
// so the machine layer may set a variable and the local layer may not.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:        configModule,
		Template:    []byte(ConfigTemplate()),
		PerNameMaps: ConfigPerNameMaps(),
		HubWide:     true,
		Categories: map[string]configengine.Category{
			varsKey: configengine.CategoryMachine,
		},
	}
}

// LoadConfig loads and unmarshals the env module's configuration through the hub-wide layer stack under dirs.
// A hub whose base file is absent resolves the template, which sets no variable.
// A name that is no environment variable name, or that is PATH or starts with LYM_ in any letter case, is an error.
// The error is marked configengine.ErrInvalid and names the key.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal env config: %w", err))
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Vars)) {
		upper := strings.ToUpper(name)
		if !namePattern.MatchString(name) || upper == reservedPath || strings.HasPrefix(upper, reservedPrefix) {
			return Config{}, configengine.MarkInvalid(fmt.Errorf("env config: key %q is no variable lym may export: a name is letters, digits and underscores, starts with no digit, is not %s and never starts with %s, in any letter case; way forward: remove it with lym config env, from the prime for the hub file or with --layer machine for this machine's file", varsKey+"."+name, reservedPath, reservedPrefix))
		}
	}
	if cfg.Vars == nil {
		cfg.Vars = map[string]string{}
	}
	return cfg, nil
}

// Entries returns the variables as NAME=value entries in name order, the form a process environment takes.
// Appended after the inherited environment, they replace an inherited value of the same name.
func (c Config) Entries() []string {
	entries := make([]string, 0, len(c.Vars))
	for _, name := range slices.Sorted(maps.Keys(c.Vars)) {
		entries = append(entries, name+"="+c.Vars[name])
	}
	return entries
}

// Beneath returns the variables with own laid over them, so a name own holds keeps own's value.
// Neither map is changed.
func (c Config) Beneath(own map[string]string) map[string]string {
	merged := maps.Clone(c.Vars)
	if merged == nil {
		merged = map[string]string{}
	}
	maps.Copy(merged, own)
	return merged
}

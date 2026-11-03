// config.go declares the codex engine's config module, codex.yaml: its template, its layered-load spec, LoadConfig and the resolution of its home paths.

package codexengine

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns codex.yaml's default YAML template.
func ConfigTemplate() string {
	return configTemplate
}

// Config is the resolved codex.yaml: the Codex binary, the shared lym Codex home and the operator's own, the efforts a launch may take and the skill roots.
type Config struct {
	// Binary is the path to the codex executable; empty falls back to "codex" on PATH.
	Binary string `yaml:"binary"`
	// Home is the shared lym Codex home every launch runs under; a leading "~/" names the user's home directory.
	Home string `yaml:"home"`
	// OperatorHome is the operator's own Codex home, which lym never writes; a leading "~/" names the user's home directory.
	OperatorHome string `yaml:"operator_home"`
	// Efforts are the reasoning efforts a launch may take.
	Efforts []string `yaml:"efforts"`
	// SkillRoots are the directories a skill is looked up under when a session loads skills.
	SkillRoots []string `yaml:"skill_roots"`
}

// ConfigSpec returns codex.yaml's layered-load spec: a per-worktree module whose binary a local override may not set.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "codex",
		Template: []byte(ConfigTemplate()),
		Categories: map[string]configengine.Category{
			"binary":        configengine.CategoryExecutable,
			"home":          configengine.CategoryNone,
			"operator_home": configengine.CategoryNone,
			"efforts":       configengine.CategoryNone,
			"skill_roots":   configengine.CategoryNone,
		},
	}
}

// LoadConfig resolves codex.yaml from the layer stack under dirs.
// An absent file in every layer resolves the template; a layer file that exists but is invalid is an error.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal codex config: %w", err))
	}
	return cfg, nil
}

// loadConfigOrRefuse loads codex.yaml from dirs.
// A load error keeps configengine's text naming the file and key and its invalid mark, and gains the way forward.
func loadConfigOrRefuse(dirs configengine.Dirs) (Config, error) {
	codex, err := LoadConfig(dirs)
	if err != nil {
		return Config{}, fmt.Errorf("codexengine: %w; way forward: correct or remove the key in the file the error names, with lym config codex --layer <default|worktree|local> (or --set for a value)", err)
	}
	return codex, nil
}

// codexBinary returns codex.Binary if set, otherwise "codex".
func codexBinary(codex Config) string {
	if codex.Binary != "" {
		return codex.Binary
	}
	return "codex"
}

// homeDirs is the pair of Codex homes a launch uses, both absolute.
type homeDirs struct {
	// lym is the shared lym Codex home the launch runs under.
	lym string
	// operator is the operator's own Codex home the lym home links into.
	operator string
}

// resolveHomes resolves codex.yaml's home and operator_home keys into absolute paths, refusing an empty or relative value by its key.
func resolveHomes(codex Config) (homeDirs, error) {
	lym, err := expandHomePath("home", codex.Home)
	if err != nil {
		return homeDirs{}, err
	}
	operator, err := expandHomePath("operator_home", codex.OperatorHome)
	if err != nil {
		return homeDirs{}, err
	}
	return homeDirs{lym: lym, operator: operator}, nil
}

// expandHomePath returns value with a leading "~/" replaced by the user's home directory, refusing a value that is then empty or relative.
// key names the codex.yaml entry the value came from, so the refusal points at what to fix.
func expandHomePath(key, value string) (string, error) {
	if rest, ok := strings.CutPrefix(value, "~/"); ok {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("codexengine: resolve codex.yaml's %s %q: %w", key, value, err)
		}
		value = filepath.Join(userHome, rest)
	}
	if value == "" || !filepath.IsAbs(value) {
		return "", fmt.Errorf("codexengine: codex.yaml's %s %q is not an absolute path or one under ~/; way forward: set it with lym config codex", key, value)
	}
	return filepath.Clean(value), nil
}

// validateEffort refuses a non-empty effort that codex.yaml's efforts does not list, naming the key.
func validateEffort(effort string, codex Config) error {
	if effort == "" || slices.Contains(codex.Efforts, effort) {
		return nil
	}
	return fmt.Errorf("codexengine: effort %q is not in codex.yaml's efforts %v; way forward: name one of them in the model-spec, or add it to efforts with lym config codex", effort, codex.Efforts)
}

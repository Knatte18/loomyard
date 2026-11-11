// config.go holds the codex engine's codex.yaml: its default template, its layered-load spec and LoadConfig.
// It also resolves the home and writable-root paths the config names.

package codexengine

import (
	_ "embed"
	"errors"
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

// ConfigTemplate returns codex.yaml's default YAML template, the base layer that every layer file overrides.
func ConfigTemplate() string {
	return configTemplate
}

// Config is the resolved codex.yaml, the settings that decide how a Codex session launches.
// Each field maps to one codex.yaml key.
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
	// SandboxMode is how a launch picks its Codex sandbox: sandboxAuto, sandboxWorkspaceWrite or sandboxFullAccess.
	SandboxMode string `yaml:"sandbox_mode"`
	// WritableRoots are folders outside the workspace that a launch's sandbox lets the session write, such as a build cache;
	// a leading "~/" names the user's home directory.
	WritableRoots []string `yaml:"writable_roots"`
}

// The modes codex.yaml's sandbox_mode may name.
// A launch runs one of the two Codex sandbox modes, workspace-write or danger-full-access;
// auto is lym's own and resolves to one of them at each launch.
const (
	// sandboxAuto runs workspace-write when Codex's sandbox starts on the machine.
	// It runs danger-full-access when the sandbox refuses to start.
	// A probe that cannot run to a verdict refuses the launch.
	sandboxAuto = "auto"
	// sandboxWorkspaceWrite bounds the session's writes to the workspace, the writable roots and the temporary folders.
	sandboxWorkspaceWrite = "workspace-write"
	// sandboxFullAccess runs the session with no sandbox.
	// It serves a machine where Codex's Linux sandbox cannot start.
	// Bubblewrap there may not create the user namespace it needs.
	sandboxFullAccess = "danger-full-access"
)

// ConfigSpec returns the layered-load spec of codex.yaml, a per-worktree module.
// The local layer may not set the binary or the writable roots, since a writable root widens the sandbox.
// The sandbox mode follows a fact of the machine, whether its bubblewrap can start Codex's sandbox:
// the machine layer may set it, so one machine pins its mode while the tracked files keep auto for the rest,
// and the local layer may not.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "codex",
		Template: []byte(ConfigTemplate()),
		Categories: map[string]configengine.Category{
			"binary":         configengine.CategoryExecutable,
			"home":           configengine.CategoryNone,
			"operator_home":  configengine.CategoryNone,
			"efforts":        configengine.CategoryNone,
			"skill_roots":    configengine.CategoryNone,
			"sandbox_mode":   configengine.CategoryMachine,
			"writable_roots": configengine.CategoryGuard,
		},
	}
}

// LoadConfig resolves codex.yaml from the layer stack under dirs.
// An absent file in every layer resolves the template; a layer file that exists but is invalid is an error.
// A sandbox_mode other than auto, workspace-write or danger-full-access is invalid.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal codex config: %w", err))
	}
	if cfg.SandboxMode != sandboxAuto && cfg.SandboxMode != sandboxWorkspaceWrite && cfg.SandboxMode != sandboxFullAccess {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("codex.yaml's sandbox_mode %q is none of %s, %s and %s", cfg.SandboxMode, sandboxAuto, sandboxWorkspaceWrite, sandboxFullAccess))
	}
	return cfg, nil
}

// loadConfigOrRefuse returns a load error with the way forward added.
// The error still names the file and key and still carries its invalid mark.
func loadConfigOrRefuse(dirs configengine.Dirs) (Config, error) {
	codex, err := LoadConfig(dirs)
	if err != nil {
		return Config{}, fmt.Errorf("codexengine: %w; way forward: correct or remove the key in the file the error names, with lym config codex --layer <default|worktree|machine|local> (or --set for a value)", err)
	}
	return codex, nil
}

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

// resolveHomes resolves codex.yaml's home and operator_home keys into absolute paths.
// It refuses an empty or relative value by its key.
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

// expandHomePath returns value with a leading "~/" replaced by the user's home directory.
// It refuses a value that is then empty or relative.
// The key names the codex.yaml entry the value came from, so the refusal points at what to fix.
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

// configuredWritableRoots returns codex.yaml's writable_roots as absolute, symlink-resolved folders.
// It refuses an empty or relative value by its key.
// A folder that does not exist is left out,
// so a root the machine has not made, such as a cache of a toolchain it lacks, adds nothing to the launch.
func configuredWritableRoots(codex Config) ([]string, error) {
	var roots []string
	for _, root := range codex.WritableRoots {
		expanded, err := expandHomePath("writable_roots", root)
		if err != nil {
			return nil, err
		}
		resolved, err := filepath.EvalSymlinks(expanded)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("codexengine: resolve codex.yaml's writable_roots %q: %w", root, err)
		}
		roots = append(roots, resolved)
	}
	return roots, nil
}

// validateEffort refuses a non-empty effort that codex.yaml's efforts does not list, naming the key.
func validateEffort(effort string, codex Config) error {
	if effort == "" || slices.Contains(codex.Efforts, effort) {
		return nil
	}
	return fmt.Errorf("codexengine: effort %q is not in codex.yaml's efforts %v; way forward: name one of them in the model-spec, or add it to efforts with lym config codex", effort, codex.Efforts)
}

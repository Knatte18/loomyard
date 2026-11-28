// scope.go decides a Claude session's scope: the plugins, skills and memory the session gets.
// It reads claude.yaml's session_scope block, the skills the session preloads and the plugins Claude Code has installed.
// It also declares the settings fields that carry the scope.

package claudeengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

// sessionScopeKey is claude.yaml's key of the session_scope block.
const sessionScopeKey = "session_scope"

// ScopeConfig is claude.yaml's session_scope block:
// whether lym scopes what each session loads, and the skills it turns off in every session.
type ScopeConfig struct {
	// Enabled writes the scope into every session's settings;
	// when off, a session's plugins, skills and memory follow its other settings layers.
	Enabled bool `yaml:"enabled"`
	// SkillsOff names the skills turned off in every session that does not preload them, as Claude Code lists them.
	// A plugin's skill named here stays listed, because Claude Code ignores skillOverrides for an installed plugin's skills.
	SkillsOff []string `yaml:"skills_off"`
}

// skillOff is the skillOverrides value that turns a skill off.
const skillOff = "off"

// claudeConfigDirEnv names the variable that moves Claude Code's config directory away from the default.
// The default is claudeConfigDirName under the home directory.
const claudeConfigDirEnv = "CLAUDE_CONFIG_DIR"

// claudeConfigDirName is Claude Code's config directory under the home directory.
const claudeConfigDirName = ".claude"

// installedPluginsPath is the file, relative to Claude Code's config directory, that names each installed plugin.
var installedPluginsPath = filepath.Join("plugins", "installed_plugins.json")

// sessionScope is the part of a settings document that scopes what a session loads.
// Every field is omitted when zero,
// so a document built without a scope carries none of it and leaves each to the session's other settings layers.
type sessionScope struct {
	// EnabledPlugins switches each installed plugin on or off by its id.
	EnabledPlugins map[string]bool `json:"enabledPlugins,omitempty"`
	// SkillOverrides maps a skill's listed name to skillOff.
	SkillOverrides map[string]string `json:"skillOverrides,omitempty"`
	// AutoMemoryEnabled, when set, switches the session's auto-memory; nil leaves it to the other layers.
	AutoMemoryEnabled *bool `json:"autoMemoryEnabled,omitempty"`
	// Env holds the environment variables the session sets, such as the effort its subagents fall back to.
	Env map[string]string `json:"env,omitempty"`
}

// resolveSessionScope returns the scope of a session from its preloaded skills and the installed plugin ids.
// A disabled cfg returns the zero scope.
// Otherwise an installed plugin is on exactly when the session preloads a skill listed under the plugin's name;
// it is off.
// Every skill of cfg.SkillsOff the session does not preload is off.
// A plugin that is on lists all of its skills: no setting hides one skill of it.
// The orch session alone keeps its auto-memory; every other session has it off.
// The scope changes no session's tools, so every session keeps the same tool set.
func resolveSessionScope(cfg ScopeConfig, installed []string, preloaded []string, orch bool) sessionScope {
	if !cfg.Enabled {
		return sessionScope{}
	}
	loaded := map[string]bool{}
	namespaces := map[string]bool{}
	for _, skill := range preloaded {
		loaded[skill] = true
		if namespace, _, qualified := strings.Cut(skill, ":"); qualified {
			namespaces[namespace] = true
		}
	}

	plugins := map[string]bool{}
	for _, id := range installed {
		name, _, _ := strings.Cut(id, "@")
		plugins[id] = namespaces[name]
	}
	overrides := map[string]string{}
	for _, skill := range cfg.SkillsOff {
		if !loaded[skill] {
			overrides[skill] = skillOff
		}
	}

	var scope sessionScope
	if len(plugins) > 0 {
		scope.EnabledPlugins = plugins
	}
	if len(overrides) > 0 {
		scope.SkillOverrides = overrides
	}
	if !orch {
		memory := false
		scope.AutoMemoryEnabled = &memory
	}
	return scope
}

// sessionScopeFor returns the scope of a session that preloads skills and has the given strand role.
// It reads the installed plugins from the engine's Claude Code config directory.
// A directory or plugins file it cannot read is logged and names no plugin,
// so the scope still turns off cfg's skills and the memory of a session that is not the orch.
func (c *Claude) sessionScopeFor(cfg ScopeConfig, skills []string, role string) sessionScope {
	if !cfg.Enabled {
		return sessionScope{}
	}
	installed, err := c.installedPlugins()
	if err != nil {
		logger.Warn("claudeengine: the session's scope names no installed plugin", "err", err)
	}
	return resolveSessionScope(cfg, installed, skills, agentname.MatchesRole(role, agentname.RoleOrch))
}

// installedPlugins reads the ids of the plugins installed under the engine's Claude Code config directory.
// When the engine holds no directory, it resolves one from the environment.
func (c *Claude) installedPlugins() ([]string, error) {
	dir := c.claudeDir
	if dir == "" {
		resolved, err := claudeConfigDir()
		if err != nil {
			return nil, err
		}
		dir = resolved
	}
	return readInstalledPlugins(dir)
}

// claudeConfigDir returns Claude Code's config directory:
// the directory claudeConfigDirEnv names, else claudeConfigDirName under the home directory.
func claudeConfigDir() (string, error) {
	if dir := os.Getenv(claudeConfigDirEnv); dir != "" {
		return dir, nil
	}
	home, err := claudeHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve the home directory: %w", err)
	}
	return filepath.Join(home, claudeConfigDirName), nil
}

// readInstalledPlugins returns the sorted `<name>@<marketplace>` ids of the plugins configDir names as installed.
// These are the keys Claude Code's enabledPlugins setting takes.
// An absent plugins file names none.
func readInstalledPlugins(configDir string) ([]string, error) {
	path := filepath.Join(configDir, installedPluginsPath)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read installed plugins: %w", err)
	}
	var file struct {
		Plugins map[string]json.RawMessage `json:"plugins"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse installed plugins %s: %w", path, err)
	}

	ids := make([]string, 0, len(file.Plugins))
	for id := range file.Plugins {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}

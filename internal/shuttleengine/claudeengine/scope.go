// scope.go decides a Claude session's scope: the plugins, skills, connectors, memory and tools the session gets.
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

// lyPluginName is the name of the ly plugin, whose skills are the slash commands the operator types by hand.
const lyPluginName = "ly"

// sessionScopeKey is claude.yaml's key of the session_scope block.
const sessionScopeKey = "session_scope"

// ScopeConfig is claude.yaml's session_scope block: whether lym scopes what each session loads, and the skills it turns off in every session.
type ScopeConfig struct {
	// Enabled writes the scope into every session's settings; off, a session's plugins, skills, connectors and memory follow its other settings layers.
	Enabled bool `yaml:"enabled"`
	// SkillsOff names the skills turned off in every session that does not preload them, as Claude Code lists them.
	SkillsOff []string `yaml:"skills_off"`
}

// skillOff is the skillOverrides value that turns a skill off.
const skillOff = "off"

// listAgentsTool is the Claude Code tool every session but the orch is denied: it lists the sessions a session can message, and their messages go through creel.
const listAgentsTool = "ListAgents"

// claudeConfigDirEnv names the variable that moves Claude Code's config directory away from claudeConfigDirName under the home directory.
const claudeConfigDirEnv = "CLAUDE_CONFIG_DIR"

// claudeConfigDirName is Claude Code's config directory under the home directory.
const claudeConfigDirName = ".claude"

// installedPluginsPath is the file, relative to Claude Code's config directory, that names each installed plugin and its install paths.
var installedPluginsPath = filepath.Join("plugins", "installed_plugins.json")

// pluginSkillsDirName is the directory of a plugin's install path that holds one directory per skill, named as the skill.
const pluginSkillsDirName = "skills"

// installedPlugin is one plugin Claude Code has installed.
type installedPlugin struct {
	// id is the plugin's `<name>@<marketplace>` key, the key Claude Code's enabledPlugins setting takes.
	id string
	// skills are the plugin's skills as Claude Code lists them, `<name>:<skill>`, sorted.
	skills []string
}

// sessionScope is the part of a settings document that scopes what a session loads.
// Every field is omitted when zero, so a document built without a scope carries none of it and leaves each to the session's other settings layers.
type sessionScope struct {
	// EnabledPlugins switches each installed plugin on or off by its id.
	EnabledPlugins map[string]bool `json:"enabledPlugins,omitempty"`
	// SkillOverrides maps a skill's listed name to skillOff.
	SkillOverrides map[string]string `json:"skillOverrides,omitempty"`
	// DisableClaudeAiConnectors keeps the account's claude.ai connectors out of the session.
	DisableClaudeAiConnectors bool `json:"disableClaudeAiConnectors,omitempty"`
	// DisableBundledSkills keeps the skills shipped inside Claude Code out of the session.
	DisableBundledSkills bool `json:"disableBundledSkills,omitempty"`
	// AutoMemoryEnabled, when set, switches the session's auto-memory; nil leaves it to the other layers.
	AutoMemoryEnabled *bool `json:"autoMemoryEnabled,omitempty"`
	// Permissions, when set, carries the tools the session is denied.
	Permissions *scopePermissions `json:"permissions,omitempty"`
}

// scopePermissions is the permissions object of a scoped settings document: the tools it denies, which Claude Code adds to the deny rules of the other layers.
type scopePermissions struct {
	Deny []string `json:"deny"`
}

// resolveSessionScope returns the scope of a session that preloads the skills preloaded, given the plugins Claude Code has installed.
// A disabled cfg returns the zero scope.
// Otherwise an installed plugin is on exactly when the session preloads a skill listed under the plugin's name, and off otherwise;
// claude.ai connectors and bundled skills are off, and every skill of cfg.SkillsOff the session does not preload is off.
// A plugin that is on has every skill but the preloaded ones off, except the ly plugin, whose skills the operator types as slash commands and which keeps all of them.
// The orch session alone keeps its auto-memory and the ListAgents tool; every other session has auto-memory off and the tool denied.
func resolveSessionScope(cfg ScopeConfig, installed []installedPlugin, preloaded []string, orch bool) sessionScope {
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
	overrides := map[string]string{}
	for _, plugin := range installed {
		name, _, _ := strings.Cut(plugin.id, "@")
		on := namespaces[name]
		plugins[plugin.id] = on
		if !on || name == lyPluginName {
			continue
		}
		for _, skill := range plugin.skills {
			if !loaded[skill] {
				overrides[skill] = skillOff
			}
		}
	}
	for _, skill := range cfg.SkillsOff {
		if !loaded[skill] {
			overrides[skill] = skillOff
		}
	}

	scope := sessionScope{DisableClaudeAiConnectors: true, DisableBundledSkills: true}
	if len(plugins) > 0 {
		scope.EnabledPlugins = plugins
	}
	if len(overrides) > 0 {
		scope.SkillOverrides = overrides
	}
	if !orch {
		memory := false
		scope.AutoMemoryEnabled = &memory
		scope.Permissions = &scopePermissions{Deny: []string{listAgentsTool}}
	}
	return scope
}

// sessionScopeFor returns the scope of a session of strand role role that preloads skills, reading the installed plugins from the engine's Claude Code config directory.
// A directory or plugins file it cannot read is logged and names no plugin, so the scope still turns off the connectors, the bundled skills and cfg's skills.
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

// installedPlugins reads the plugins installed under the engine's Claude Code config directory, resolved from the environment when the engine holds none.
func (c *Claude) installedPlugins() ([]installedPlugin, error) {
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

// claudeConfigDir returns Claude Code's config directory: the directory claudeConfigDirEnv names, else claudeConfigDirName under the home directory.
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

// readInstalledPlugins returns the plugins Claude Code's config directory configDir names as installed, sorted by id.
// Each plugin carries the skills found under the skills directories of its install paths.
// An absent plugins file names none, and a plugin whose install paths hold no readable skills directory has no skills.
func readInstalledPlugins(configDir string) ([]installedPlugin, error) {
	path := filepath.Join(configDir, installedPluginsPath)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read installed plugins: %w", err)
	}
	var file struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse installed plugins %s: %w", path, err)
	}

	ids := make([]string, 0, len(file.Plugins))
	for id := range file.Plugins {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	plugins := make([]installedPlugin, 0, len(ids))
	for _, id := range ids {
		name, _, _ := strings.Cut(id, "@")
		skills := map[string]bool{}
		for _, install := range file.Plugins[id] {
			if install.InstallPath == "" {
				continue
			}
			entries, err := os.ReadDir(filepath.Join(install.InstallPath, pluginSkillsDirName))
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() {
					skills[name+":"+entry.Name()] = true
				}
			}
		}
		plugin := installedPlugin{id: id}
		for skill := range skills {
			plugin.skills = append(plugin.skills, skill)
		}
		slices.Sort(plugin.skills)
		plugins = append(plugins, plugin)
	}
	return plugins, nil
}

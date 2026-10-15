// config.go declares the claude engine's config module, claude.yaml: its template, its layered-load spec and LoadConfig.

package claudeengine

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns claude.yaml's default YAML template.
func ConfigTemplate() string {
	return configTemplate
}

// Config is the resolved claude.yaml: the Claude binary, the tool denies and the prompt-cache lifetimes.
type Config struct {
	// Binary is the path to the claude executable; empty falls back to "claude" on PATH.
	Binary string `yaml:"binary"`
	// DenyAgentTool switches the PreToolUse deny for the in-process Agent tool.
	DenyAgentTool bool `yaml:"deny_agent_tool"`
	// DenyAskUserQuestion switches the PreToolUse deny for the AskUserQuestion tool.
	DenyAskUserQuestion bool `yaml:"deny_ask_user_question"`
	// PromptCacheTTL is Claude Code's prompt-cache TTL for a role without a map entry; it is validated where a run starts, not by LoadConfig.
	PromptCacheTTL string `yaml:"prompt_cache_ttl"`
	// PromptCacheTTLRoles maps a strand's role segment to Claude Code's prompt-cache TTL; it is validated where a run starts, not by LoadConfig.
	PromptCacheTTLRoles map[string]string `yaml:"prompt_cache_ttl_roles"`
}

// ConfigOpenMaps returns the claude.yaml keys whose entries are the operator's own, which configengine carries whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{"prompt_cache_ttl_roles"}
}

// ConfigSpec returns claude.yaml's layered-load spec: a per-worktree module whose binary and tool denies a local override may not set.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "claude",
		Template: []byte(ConfigTemplate()),
		OpenMaps: ConfigOpenMaps(),
		Categories: map[string]configengine.Category{
			"binary":                 configengine.CategoryExecutable,
			"deny_agent_tool":        configengine.CategoryGuard,
			"deny_ask_user_question": configengine.CategoryGuard,
			"prompt_cache_ttl":       configengine.CategoryNone,
			"prompt_cache_ttl_roles": configengine.CategoryNone,
		},
	}
}

// LoadConfig resolves claude.yaml from the layer stack under dirs.
// An absent file in every layer resolves the template; a layer file that exists but is invalid is an error.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal claude config: %w", err))
	}
	return cfg, nil
}

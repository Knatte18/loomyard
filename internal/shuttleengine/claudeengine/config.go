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
	// DenyAskUserQuestion switches the PreToolUse deny for the AskUserQuestion tool.
	DenyAskUserQuestion bool `yaml:"deny_ask_user_question"`
	// PromptCacheTTL is Claude Code's prompt-cache TTL for a role without a map entry; it is validated where a run starts, not by LoadConfig.
	PromptCacheTTL string `yaml:"prompt_cache_ttl"`
	// PromptCacheTTLRoles maps a strand's role segment to Claude Code's prompt-cache TTL; it is validated where a run starts, not by LoadConfig.
	PromptCacheTTLRoles map[string]string `yaml:"prompt_cache_ttl_roles"`
	// KeepWarmCap is the longest one keep-warm wait for a role without a map entry, a Go duration; ValidateKeepWarm checks it where a run starts and on a config write, not LoadConfig.
	KeepWarmCap string `yaml:"keep_warm_cap"`
	// KeepWarmCapRoles maps a strand's role segment to its keep-warm cap, replacing KeepWarmCap for that role; ValidateKeepWarm checks it, not LoadConfig.
	KeepWarmCapRoles map[string]string `yaml:"keep_warm_cap_roles"`
}

// ConfigOpenMaps returns the claude.yaml keys whose entries are the operator's own, which configengine carries whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{"prompt_cache_ttl_roles", "keep_warm_cap_roles"}
}

// retiredAgentDenyKey is the key that once switched the Agent tool deny; no run denies the Agent tool, so a layer holding it is read without it.
const retiredAgentDenyKey = "deny_agent_tool"

// retiredAgentDenyWayForward is the way forward a local override setting retiredAgentDenyKey is refused with.
const retiredAgentDenyWayForward = "delete the key: no run denies the Agent tool, so it switches nothing"

// ConfigSpec returns claude.yaml's layered-load spec: a per-worktree module whose binary and tool denies a local override may not set.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "claude",
		Template: []byte(ConfigTemplate()),
		OpenMaps: ConfigOpenMaps(),
		Categories: map[string]configengine.Category{
			"binary":                 configengine.CategoryExecutable,
			"deny_ask_user_question": configengine.CategoryGuard,
			"prompt_cache_ttl":       configengine.CategoryNone,
			"prompt_cache_ttl_roles": configengine.CategoryNone,
			"keep_warm_cap":          configengine.CategoryNone,
			"keep_warm_cap_roles":    configengine.CategoryNone,
		},
		RetiredKeys:       map[string]configengine.Category{retiredAgentDenyKey: configengine.CategoryGuard},
		RetiredWayForward: map[string]string{retiredAgentDenyKey: retiredAgentDenyWayForward},
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

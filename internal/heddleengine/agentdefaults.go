// agentdefaults.go derives the claude-settings entry that carries the hub's agents.yaml effort defaults,
// so a Claude session started in a worktree with no --effort runs at the effort lym's own launches default to.

package heddleengine

import (
	"encoding/json"
	"fmt"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
)

const (
	// AgentDefaultsSlug is the slug of the claude-settings entry derived from agents.yaml.
	AgentDefaultsSlug = "lym-agent-defaults"
	// agentDefaultsOrder places the derived entry after every shipped claude-settings entry.
	agentDefaultsOrder = 200
	// effortLevelKey is Claude Code's setting for the reasoning effort of a session started with no --effort.
	effortLevelKey = "effortLevel"
	// settingsEnvKey is Claude Code's setting that holds the environment variables a session sets.
	settingsEnvKey = "env"
)

// agentDefaultsEntry returns the claude-settings entry built from the agents.yaml that dirs resolve.
// It sets the session effort and the effort a subagent falls back to, both to the efforts table's default.
// An agents.yaml that does not load gives no entry and a warning naming its error.
func agentDefaultsEntry(dirs configengine.Dirs) (Entry, string) {
	cfg, err := agentconfig.Load(dirs)
	if err != nil {
		return Entry{}, fmt.Sprintf("the effort defaults are left out of %s: %v", ClaudeSettingsFile, err)
	}
	effort := cfg.Effort("", "")
	text, err := json.Marshal(map[string]any{
		effortLevelKey: effort,
		settingsEnvKey: map[string]string{agentconfig.SubagentEffortEnv: effort},
	})
	if err != nil {
		panic(fmt.Sprintf("heddleengine: a map of strings did not encode: %v", err))
	}
	return Entry{Slug: AgentDefaultsSlug, Text: string(text), Order: agentDefaultsOrder, Targets: []string{claudeSettingsTarget}}, ""
}

// addDerived sets the layers' derived entries from the config dirs resolve.
// It sets DerivedWarning when one could not be derived.
func (l *Layers) addDerived(dirs configengine.Dirs) {
	entry, warning := agentDefaultsEntry(dirs)
	l.Derived, l.DerivedWarning = nil, warning
	if warning == "" {
		l.Derived = []Entry{entry}
	}
}

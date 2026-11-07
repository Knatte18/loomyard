// target.go holds the target table: the places an entry's text is delivered to.

package heddleengine

import (
	"slices"

	"github.com/Knatte18/loomyard/internal/agentname"
)

// The deliveries a target has.
const (
	// DeliveryFile renders the target into a file at a worktree's anchor.
	DeliveryFile = "file"
	// DeliveryPrompt appends the target's passages to a session's system prompt.
	DeliveryPrompt = "prompt"
)

// OrchPromptTarget is the prompt target whose passages reach an orch session alone, the home of a repository's orch rules.
const OrchPromptTarget = "orch-prompt"

// ClaudeSettingsFile is the anchor-relative file the claude-settings target renders into:
// the local Claude Code settings every Claude session started in the worktree loads.
const ClaudeSettingsFile = ".claude/settings.local.json"

// SettingsExcludeReinterpretation names the reconcile change that adds ClaudeSettingsFile to a hub's exclude list.
// A hub whose fabric.yaml sets that list would otherwise leave the file unwritten.
const SettingsExcludeReinterpretation = "heddle-claude-settings-excluded"

const (
	// claudeFileTarget is the file target rendered into CLAUDE.local.md, the instructions a Claude session reads.
	claudeFileTarget = "claude-file"
	// codexFileTarget is the file target rendered into AGENTS.override.md, the instructions a Codex session reads.
	codexFileTarget = "codex-file"
	// claudeSettingsTarget is the JSON-object file target merged into ClaudeSettingsFile.
	claudeSettingsTarget = "claude-settings"
)

// Target is one place an entry's text is delivered to.
// File is the anchor-relative path of a file target and empty for a prompt target.
// ReRender lists the anchor-relative paths whose change re-renders a file target.
// JSONObject marks a file target whose entries each hold one JSON object, merged into the file in place of joined text.
// Engine and Role narrow the sessions a prompt target reaches.
// Engine must equal the session's engine, and Role must match the session's role or a numbered sibling of it;
// an empty field narrows nothing.
type Target struct {
	Name       string
	Delivery   string
	File       string
	ReRender   []string
	JSONObject bool
	Engine     string
	Role       string
}

// targetTable holds one row per target; a further target is one new row.
var targetTable = []Target{
	{Name: claudeFileTarget, Delivery: DeliveryFile, File: "CLAUDE.local.md", ReRender: []string{"CLAUDE.local.md", "AGENTS.md", "CLAUDE.md", ".claude/AGENTS.md", ".claude/CLAUDE.md"}},
	{Name: codexFileTarget, Delivery: DeliveryFile, File: "AGENTS.override.md", ReRender: []string{"AGENTS.md", "CLAUDE.md", "AGENTS.override.md"}},
	{Name: claudeSettingsTarget, Delivery: DeliveryFile, File: ClaudeSettingsFile, JSONObject: true},
	{Name: "claude-prompt", Delivery: DeliveryPrompt, Engine: "claude"},
	{Name: "codex-prompt", Delivery: DeliveryPrompt, Engine: "codex"},
	{Name: OrchPromptTarget, Delivery: DeliveryPrompt, Role: agentname.RoleOrch},
}

// LookupTarget returns the target named name, and false when the table holds none.
func LookupTarget(name string) (Target, bool) {
	for _, target := range targetTable {
		if target.Name == name {
			target.ReRender = slices.Clone(target.ReRender)
			return target, true
		}
	}
	return Target{}, false
}

// TargetNames returns the names of the table's targets in table order.
func TargetNames() []string {
	names := make([]string, 0, len(targetTable))
	for _, target := range targetTable {
		names = append(names, target.Name)
	}
	return names
}

// objectTargetNames returns the names of the JSON-object targets in table order.
func objectTargetNames() []string {
	var names []string
	for _, target := range targetTable {
		if target.JSONObject {
			names = append(names, target.Name)
		}
	}
	return names
}

// objectTargetCount returns how many of names are JSON-object targets.
func objectTargetCount(names []string) int {
	count := 0
	for _, name := range names {
		if target, ok := LookupTarget(name); ok && target.JSONObject {
			count++
		}
	}
	return count
}

// promptTargetsFor returns the names of the prompt targets that reach a session of engine whose strand role is role.
func promptTargetsFor(engine, role string) []string {
	var names []string
	for _, target := range targetTable {
		if target.Delivery != DeliveryPrompt {
			continue
		}
		if target.Engine != "" && target.Engine != engine {
			continue
		}
		if target.Role != "" && !agentname.MatchesRole(role, target.Role) {
			continue
		}
		names = append(names, target.Name)
	}
	return names
}

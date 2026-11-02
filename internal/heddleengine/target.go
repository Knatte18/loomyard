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

// Target is one place an entry's text is delivered to.
// File is the anchor-relative path of a file target and empty for a prompt target.
// ReRender lists the anchor-relative paths whose change re-renders a file target.
// Engine and Role narrow the sessions a prompt target reaches: a session of that engine, and a session of that role or a numbered sibling of it; an empty field narrows nothing.
type Target struct {
	Name     string
	Delivery string
	File     string
	ReRender []string
	Engine   string
	Role     string
}

// targetTable holds one row per target; a further target is one new row.
var targetTable = []Target{
	{Name: "claude-file", Delivery: DeliveryFile, File: "CLAUDE.local.md", ReRender: []string{"CLAUDE.local.md", "AGENTS.md", "CLAUDE.md", ".claude/AGENTS.md", ".claude/CLAUDE.md"}},
	{Name: "codex-file", Delivery: DeliveryFile, File: "AGENTS.override.md", ReRender: []string{"AGENTS.md", "AGENTS.override.md"}},
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

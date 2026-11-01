// target.go holds the target table: the places an entry's text is delivered to.

package heddleengine

import "slices"

// The deliveries a target has.
const (
	// DeliveryFile renders the target into a file at a worktree's root.
	DeliveryFile = "file"
	// DeliveryPrompt appends the target's passages to a session's system prompt.
	DeliveryPrompt = "prompt"
)

// Target is one place an entry's text is delivered to.
// File is the root-relative path of a file target and empty for a prompt target.
// ReRender lists the root-relative paths whose change re-renders a file target.
type Target struct {
	Name     string
	Delivery string
	File     string
	ReRender []string
}

// targetTable holds one row per target; a further target is one new row.
var targetTable = []Target{
	{Name: "claude-file", Delivery: DeliveryFile, File: "CLAUDE.local.md", ReRender: []string{"CLAUDE.local.md", "AGENTS.md", "CLAUDE.md", ".claude/AGENTS.md", ".claude/CLAUDE.md"}},
	{Name: "codex-file", Delivery: DeliveryFile, File: "AGENTS.override.md", ReRender: []string{"AGENTS.md", "AGENTS.override.md"}},
	{Name: "claude-prompt", Delivery: DeliveryPrompt},
	{Name: "codex-prompt", Delivery: DeliveryPrompt},
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

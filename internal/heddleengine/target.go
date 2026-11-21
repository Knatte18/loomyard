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

// The engine names the target table keys its rows to.
const (
	claudeEngine = "claude"
	codexEngine  = "codex"
)

// ClaudeLocalInstructionsFile is the anchor-relative file the claude-file target renders into:
// the local instructions every Claude session started in the worktree loads.
const ClaudeLocalInstructionsFile = "CLAUDE.local.md"

// CodexOverrideInstructionsFile is the anchor-relative file the codex-file target renders into:
// the override instructions a Codex session started in the worktree reads before its other instruction files.
const CodexOverrideInstructionsFile = "AGENTS.override.md"

// ClaudeSettingsFile is the anchor-relative file the claude-settings target renders into:
// the local Claude Code settings every Claude session started in the worktree loads.
const ClaudeSettingsFile = ".claude/settings.local.json"

const (
	// claudeFileTarget is the file target rendered into CLAUDE.local.md, the instructions a Claude session reads.
	claudeFileTarget = "claude-file"
	// codexFileTarget is the file target rendered into AGENTS.override.md, the instructions a Codex session reads.
	codexFileTarget = "codex-file"
	// agentsFileTarget is the shared file target for an instruction every agent family reads.
	// It renders into the files of claudeFileTarget and codexFileTarget.
	agentsFileTarget = "agents-file"
	// agentsPromptTarget is the shared prompt target for a passage every Claude and Codex session's prompt carries.
	// It renders into the passages of the claude-prompt and codex-prompt targets.
	agentsPromptTarget = "agents-prompt"
	// claudeSettingsTarget is the JSON-object file target merged into ClaudeSettingsFile.
	claudeSettingsTarget = "claude-settings"
)

// Target is one place an entry's text is delivered to.
// File is the anchor-relative path of a file target and empty for a prompt target.
// ReRender lists the anchor-relative paths whose change re-renders a file target.
// JSONObject marks a file target whose entries each hold one JSON object, merged into the file in place of joined text.
// RendersInto names the targets, of either delivery, that a shared target's entries reach.
// A shared target has no File or ReRender of its own.
// Engine and Role narrow the sessions a prompt target reaches.
// Engine must equal the session's engine, and Role must match the session's role or a numbered sibling of it;
// an empty field narrows nothing.
// A single-engine file target names its engine too, which keys its entries to that engine; it narrows no prompt.
type Target struct {
	Name        string
	Delivery    string
	File        string
	ReRender    []string
	JSONObject  bool
	RendersInto []string
	Engine      string
	Role        string
}

// targetTable holds one row per target; a further target is one new row.
var targetTable = []Target{
	{Name: claudeFileTarget, Delivery: DeliveryFile, File: ClaudeLocalInstructionsFile, ReRender: []string{ClaudeLocalInstructionsFile, "AGENTS.md", "CLAUDE.md", ".claude/AGENTS.md", ".claude/CLAUDE.md"}, Engine: claudeEngine},
	{Name: codexFileTarget, Delivery: DeliveryFile, File: CodexOverrideInstructionsFile, ReRender: []string{"AGENTS.md", "CLAUDE.md", CodexOverrideInstructionsFile}, Engine: codexEngine},
	{Name: agentsFileTarget, Delivery: DeliveryFile, RendersInto: []string{claudeFileTarget, codexFileTarget}},
	{Name: claudeSettingsTarget, Delivery: DeliveryFile, File: ClaudeSettingsFile, JSONObject: true},
	{Name: "claude-prompt", Delivery: DeliveryPrompt, Engine: claudeEngine},
	{Name: "codex-prompt", Delivery: DeliveryPrompt, Engine: codexEngine},
	{Name: OrchPromptTarget, Delivery: DeliveryPrompt, Role: agentname.RoleOrch},
	{Name: agentsPromptTarget, Delivery: DeliveryPrompt, RendersInto: []string{"claude-prompt", "codex-prompt"}},
}

// LookupTarget returns the target named name, and false when the table holds none.
func LookupTarget(name string) (Target, bool) {
	for _, target := range targetTable {
		if target.Name == name {
			target.ReRender = slices.Clone(target.ReRender)
			target.RendersInto = slices.Clone(target.RendersInto)
			return target, true
		}
	}
	return Target{}, false
}

// TargetFiles returns the anchor-relative files the file target named name renders into.
// A shared target's files are those of the targets it renders into.
// It returns nil for a prompt target and for a name the table does not hold.
func TargetFiles(name string) []string {
	target, ok := LookupTarget(name)
	if !ok || target.Delivery != DeliveryFile {
		return nil
	}
	if target.File != "" {
		return []string{target.File}
	}
	var files []string
	for _, into := range target.RendersInto {
		files = append(files, TargetFiles(into)...)
	}
	return files
}

// reachesFileTarget reports whether an entry aimed at targets renders into the file or prompt target named name:
// the entry names it, or names a shared target that renders into it.
func reachesFileTarget(name string, targets []string) bool {
	for _, aimed := range targets {
		if aimed == name {
			return true
		}
		if target, ok := LookupTarget(aimed); ok && slices.Contains(target.RendersInto, name) {
			return true
		}
	}
	return false
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
// A shared prompt target is left out: its entries reach a session through a target it renders into.
func promptTargetsFor(engine, role string) []string {
	var names []string
	for _, target := range targetTable {
		if target.Delivery != DeliveryPrompt || len(target.RendersInto) > 0 {
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

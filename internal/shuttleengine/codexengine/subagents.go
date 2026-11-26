// subagents.go resolves what a Codex session gives the subagents it spawns:
// the reasoning effort from agents.yaml, and the developer-instructions passage that names it and the session's skills.

package codexengine

import (
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentconfig"
)

// subagentEffort returns the effort a subagent of role takes on a model agents does not list.
// A Codex model is none of the Claude models agents.yaml keys efforts by, so the fallback effort applies to every one.
// It returns "" when agents names no effort, and when codex.yaml's efforts does not list the effort,
// so the session's subagents take Codex's default rather than a value Codex rejects.
// An unlisted effort logs a warning naming it.
func (c *Codex) subagentEffort(agents agentconfig.Config, role agentconfig.Role, codex Config) string {
	_, effort := agents.ModelEfforts(role)
	if effort == "" || slices.Contains(codex.Efforts, effort) {
		return effort
	}
	c.warn("codexengine: "+otherKeyName("agents", "efforts")+" gives subagents an effort that "+keyName(effortsKey)+" does not list, so they take Codex's default; way forward: add it to "+keyName(effortsKey)+" with lym config codex", "effort", effort, "efforts", codex.Efforts)
	return ""
}

// subagentPassage returns the developer-instructions passage that tells a session what each subagent it spawns gets:
// the explicit effort of every fresh spawn, and those of skills that fit the subagent's work.
// It returns "" for an empty effort.
func subagentPassage(effort string, skills []string) string {
	if effort == "" {
		return ""
	}
	text := "Every fresh `spawn_agent` call you make sets `reasoning_effort` to `" + effort + "` and leaves `model` unset, so the child runs on your model;" +
		" a `fork_turns: \"all\"` fork shares your effort and sets none."
	if len(skills) == 0 {
		return text
	}
	return text + " Its brief names those of your skills that fit the child's work, for it to load by reading each skill's SKILL.md before anything else." +
		" Your skills are " + strings.Join(skills, ", ") + "." +
		" A child that writes or reviews code, tests or comments loads each of them that governs code, tests, comments or prose;" +
		" one that only looks things up loads none."
}

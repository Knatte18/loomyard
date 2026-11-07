// subagents.go resolves a launch's effort from agents.yaml and writes what a session gives the subagents it spawns:
// the appended passage naming their effort and skills, and the fallback effort in the session's settings.

package claudeengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentconfig"
)

// subagentEffortEnvKey is the variable Claude Code reads as the effort of a subagent no call or definition names one for.
const subagentEffortEnvKey = "CLAUDE_CODE_SUBAGENT_EFFORT"

// modelIDPrefix opens every full Claude model id, such as claude-haiku-5-5.
const modelIDPrefix = "claude-"

// effortFor returns effort when it is set, else the effort agents gives role on model.
func effortFor(agents agentconfig.Config, role agentconfig.Role, effort, model string) string {
	if effort != "" {
		return effort
	}
	return agents.Effort(role, modelAlias(model))
}

// modelAlias returns the alias of a full Claude model id, haiku for claude-haiku-5-5, and any other model as it is.
func modelAlias(model string) string {
	rest, isID := strings.CutPrefix(model, modelIDPrefix)
	if !isID {
		return model
	}
	alias, _, _ := strings.Cut(rest, "-")
	return alias
}

// subagentEffortEnv returns the settings environment that sets the effort a subagent of role falls back to.
// It returns nil when agents names no effort.
func subagentEffortEnv(agents agentconfig.Config, role agentconfig.Role) map[string]string {
	_, effort := agents.ModelEfforts(role)
	if effort == "" {
		return nil
	}
	return map[string]string{subagentEffortEnvKey: effort}
}

// subagentPassage returns the appended-prompt passage that tells a session of role what each subagent it spawns gets:
// an explicit effort by model, and those of skills that fit the subagent's work.
// It returns "" when agents names no effort.
func subagentPassage(agents agentconfig.Config, role agentconfig.Role, skills []string) string {
	byModel, other := agents.ModelEfforts(role)
	if other == "" {
		return ""
	}
	efforts := make([]string, 0, len(byModel)+1)
	for _, entry := range byModel {
		efforts = append(efforts, fmt.Sprintf("`%s` on %s", entry.Effort, entry.Model))
	}
	if len(byModel) == 0 {
		efforts = append(efforts, fmt.Sprintf("`%s` on any model", other))
	} else {
		efforts = append(efforts, fmt.Sprintf("`%s` on any other model", other))
	}
	text := "Every subagent you spawn through the Agent tool gets an explicit `effort` in its Agent call: " +
		strings.Join(efforts, ", ") + "; a fork subagent shares your effort and takes none."
	if len(skills) == 0 {
		return text
	}
	return text + " Its brief names those of your skills that fit its work, for it to load with the Skill tool before anything else." +
		" Your skills are " + strings.Join(skills, ", ") + "." +
		" A subagent that writes or reviews code, tests or comments loads each of them that governs code, tests, comments or prose;" +
		" one that only looks things up loads none."
}
